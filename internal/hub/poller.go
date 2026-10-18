package hub

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// trinityEnginePrefix starts the `engine` infostring field (com_engine)
// a Trinity server reports. The poller treats servers without it as
// offline, so they stay out of the live UI.
const trinityEnginePrefix = "trinity-engine/"

// StatusQuerier answers a game server's getstatus.
type StatusQuerier interface {
	QueryStatus(address string) (*domain.ServerStatus, error)
}

// IdentityResolver maps a GUID to its players row and verified/admin flags.
type IdentityResolver interface {
	ResolveIdentity(ctx context.Context, guid string) (playerID int64, verified, admin, ok bool)
}

// SourceConns reports the IPs of the NATS connections authenticated as
// userPubKey, most recently active first; empty means no collector is
// connected. The poller queries remote servers at these IPs, so a
// collector that reconnects from a new IP is reachable at once rather
// than after a DNS TTL. Index 0 matters while a stale connection
// overlaps the new one.
type SourceConns interface {
	ConnectedClientIPs(userPubKey string) []netip.Addr
}

// RemotePoller polls every pollable server and caches the latest ServerStatus,
// enriching each PlayerStatus via the presence tracker and identity resolver.
type RemotePoller struct {
	store    *storage.Store
	querier  StatusQuerier
	interval time.Duration
	presence *Presence
	identity IdentityResolver
	conns    SourceConns

	mu        sync.RWMutex
	statuses  map[int64]*domain.ServerStatus
	sink      LiveEventSink
	liveState *LiveState
	// warnedNonTrinity holds the engine string already logged per
	// server, so each mismatch logs once.
	warnedNonTrinity map[int64]string

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewRemotePoller constructs a poller. interval <= 0 means 10s.
// presence, identity and conns are optional; with no conns, every row
// polls r.Address as given.
func NewRemotePoller(store *storage.Store, q StatusQuerier, interval time.Duration, presence *Presence, identity IdentityResolver, conns SourceConns) *RemotePoller {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &RemotePoller{
		store:            store,
		querier:          q,
		interval:         interval,
		presence:         presence,
		identity:         identity,
		conns:            conns,
		statuses:         make(map[int64]*domain.ServerStatus),
		warnedNonTrinity: make(map[int64]string),
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
	}
}

// pollTarget returns the address to query for r. Local rows use
// r.Address. Remote rows use the IP of the source's freshest NATS
// connection with r.Address's port. It returns "" when a remote row has
// no credentials or no connected collector, which polls as offline.
func (p *RemotePoller) pollTarget(r storage.RemoteServer) string {
	if !r.IsRemote || p.conns == nil {
		return r.Address
	}
	if r.UserPubKey == "" {
		return ""
	}
	ips := p.conns.ConnectedClientIPs(r.UserPubKey)
	if len(ips) == 0 {
		return ""
	}
	_, portStr, err := net.SplitHostPort(r.Address)
	if err != nil {
		return ""
	}
	return net.JoinHostPort(ips[0].String(), portStr)
}

// SetSink attaches a sink that receives a server_update event after
// each poll. Safe to call before or after Start.
func (p *RemotePoller) SetSink(sink LiveEventSink) {
	p.mu.Lock()
	p.sink = sink
	p.mu.Unlock()
}

// SetLiveState attaches the live-stream state so polled statuses carry
// is_live. It's optional and safe to call before or after Start.
func (p *RemotePoller) SetLiveState(ls *LiveState) {
	p.mu.Lock()
	p.liveState = ls
	p.mu.Unlock()
}

func (p *RemotePoller) Start(ctx context.Context) {
	go p.run(ctx)
}

// Stop halts the poll loop and waits for it to exit.
func (p *RemotePoller) Stop() {
	p.stopOnce.Do(func() { close(p.stopCh) })
	<-p.doneCh
}

// GetServerStatus returns the most recent status for serverID, or nil
// if no poll has succeeded yet.
func (p *RemotePoller) GetServerStatus(serverID int64) *domain.ServerStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.statuses[serverID]
	if !ok {
		return nil
	}
	copy := *s
	return &copy
}

// GetAllStatuses returns every cached status, sorted by server id.
func (p *RemotePoller) GetAllStatuses() []domain.ServerStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]domain.ServerStatus, 0, len(p.statuses))
	for _, s := range p.statuses {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServerID < out[j].ServerID })
	return out
}

func (p *RemotePoller) run(ctx context.Context) {
	defer close(p.doneCh)
	p.pollAll(ctx)

	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			p.pollAll(ctx)
		}
	}
}

func (p *RemotePoller) pollAll(ctx context.Context) {
	servers, err := p.store.ListPollableServers(ctx)
	if err != nil {
		log.Printf("hub.RemotePoller: list servers: %v", err)
		return
	}
	for _, r := range servers {
		var status *domain.ServerStatus
		var err error
		if target := p.pollTarget(r); target != "" {
			status, err = p.querier.QueryStatus(target)
		} else {
			err = fmt.Errorf("no live collector connection for source %q", r.Source)
		}
		now := time.Now().UTC()
		if err != nil || status == nil {
			p.mu.Lock()
			existing, ok := p.statuses[r.ID]
			if !ok {
				existing = &domain.ServerStatus{ServerID: r.ID, Key: r.Key, Source: r.Source, Address: r.Address}
				p.statuses[r.ID] = existing
			}
			existing.Source = r.Source
			existing.Online = false
			existing.IsLive = false
			existing.LiveDelaySeconds = 0
			existing.LiveViewers = 0
			existing.LastUpdated = now
			// LastSeenAt keeps the last successful query, for the
			// offline duration.
			snapshot := *existing
			sink := p.sink
			p.mu.Unlock()
			p.broadcast(sink, snapshot)
			continue
		}
		status.ServerID = r.ID
		status.Key = r.Key
		status.Source = r.Source
		status.Address = r.Address
		status.LastUpdated = now
		engine := status.ServerVars["engine"]
		if !strings.HasPrefix(engine, trinityEnginePrefix) {
			p.noteNonTrinity(r.ID, r.Source, r.Key, engine)
			p.mu.Lock()
			existing, ok := p.statuses[r.ID]
			if !ok {
				existing = &domain.ServerStatus{ServerID: r.ID, Key: r.Key, Source: r.Source, Address: r.Address}
				p.statuses[r.ID] = existing
			}
			existing.Source = r.Source
			existing.Online = false
			existing.IsLive = false
			existing.LiveDelaySeconds = 0
			existing.LiveViewers = 0
			existing.LastUpdated = now
			snapshot := *existing
			sink := p.sink
			p.mu.Unlock()
			p.broadcast(sink, snapshot)
			continue
		}
		// Cleared so a later mismatch logs again.
		p.mu.Lock()
		delete(p.warnedNonTrinity, r.ID)
		p.mu.Unlock()

		status.Online = true
		seen := now
		status.LastSeenAt = &seen
		status.HumanCount = 0
		status.BotCount = 0
		p.enrichPlayers(ctx, r.ID, &status.HumanCount, &status.BotCount, status.Players)
		p.mu.Lock()
		status.IsLive = p.liveState.Get(r.Source, r.Key)
		status.LiveDelaySeconds = p.liveState.GetDelay(r.Source, r.Key)
		status.LiveViewers = p.liveState.GetViewers(r.Source, r.Key)
		p.statuses[r.ID] = status
		snapshot := *status
		sink := p.sink
		p.mu.Unlock()
		p.broadcast(sink, snapshot)
	}
}

// noteNonTrinity logs a server's engine mismatch once per distinct
// engine string.
func (p *RemotePoller) noteNonTrinity(serverID int64, source, key, engine string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if prior, ok := p.warnedNonTrinity[serverID]; ok && prior == engine {
		return
	}
	p.warnedNonTrinity[serverID] = engine
	if engine == "" {
		log.Printf("hub.RemotePoller: %s/%s (id=%d) reports no engine field; hiding it from live status",
			source, key, serverID)
	} else {
		log.Printf("hub.RemotePoller: %s/%s (id=%d) engine=%q does not match %q; hiding it from live status",
			source, key, serverID, engine, trinityEnginePrefix)
	}
}

func (p *RemotePoller) broadcast(sink LiveEventSink, status domain.ServerStatus) {
	if sink == nil {
		return
	}
	sink.Broadcast(domain.Event{
		Type:      domain.EventServerUpdate,
		ServerID:  status.ServerID,
		Timestamp: status.LastUpdated,
		Data:      status,
	})
}

// enrichPlayers fills in identity and metadata on each PlayerStatus.
// A slot with no presence entry counts as a bot until its GUID is seen.
func (p *RemotePoller) enrichPlayers(ctx context.Context, serverID int64, humanCount, botCount *int, players []domain.PlayerStatus) {
	for i := range players {
		ps := &players[i]
		if ps.ClientNum < 0 {
			continue
		}
		var (
			entry    PresenceEntry
			haveSlot bool
		)
		if p.presence != nil {
			entry, haveSlot = p.presence.Lookup(serverID, ps.ClientNum)
		}
		if !haveSlot {
			ps.IsBot = true
			if botCount != nil {
				*botCount++
			}
			continue
		}
		ps.GUID = entry.GUID
		ps.IsBot = entry.IsBot
		ps.Model = entry.Model
		ps.IsVR = entry.IsVR
		if entry.IsBot {
			ps.Skill = entry.Skill
			if botCount != nil {
				*botCount++
			}
		} else if humanCount != nil {
			*humanCount++
		}
		ps.Impressives = entry.Impressives
		ps.Excellents = entry.Excellents
		ps.Humiliations = entry.Humiliations
		ps.Defends = entry.Defends
		ps.Captures = entry.Captures
		ps.Assists = entry.Assists
		ps.FlagReturns = entry.FlagReturns
		ps.SkullsDelivered = entry.SkullsDelivered
		if p.identity == nil {
			continue
		}
		playerID, verified, admin, ok := p.identity.ResolveIdentity(ctx, entry.GUID)
		if !ok {
			continue
		}
		id := playerID
		ps.PlayerID = &id
		ps.IsVerified = verified
		ps.IsAdmin = admin
	}
}
