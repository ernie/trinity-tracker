package hub

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// StatusQuerier answers a Q3 server's getstatus.
type StatusQuerier interface {
	QueryStatus(address string) (*domain.ServerStatus, error)
}

// IdentityResolver maps a GUID to its players row and verified/admin flags.
type IdentityResolver interface {
	ResolveIdentity(ctx context.Context, guid string) (playerID int64, verified, admin, ok bool)
}

// RemotePoller polls every pollable server and caches the latest ServerStatus,
// enriching each PlayerStatus via the presence tracker and identity resolver.
type RemotePoller struct {
	store    *storage.Store
	querier  StatusQuerier
	interval time.Duration
	presence *Presence
	identity IdentityResolver

	mu       sync.RWMutex
	statuses map[int64]*domain.ServerStatus
	sink     LiveEventSink

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewRemotePoller constructs a poller. interval <= 0 falls back to 10s.
// presence and identity are both optional.
func NewRemotePoller(store *storage.Store, q StatusQuerier, interval time.Duration, presence *Presence, identity IdentityResolver) *RemotePoller {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &RemotePoller{
		store:    store,
		querier:  q,
		interval: interval,
		presence: presence,
		identity: identity,
		statuses: make(map[int64]*domain.ServerStatus),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// SetSink attaches a sink that receives a server_update event after
// each poll. Safe to call before or after Start.
func (p *RemotePoller) SetSink(sink LiveEventSink) {
	p.mu.Lock()
	p.sink = sink
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
		status, err := p.querier.QueryStatus(r.Address)
		if err != nil || status == nil {
			p.mu.Lock()
			existing, ok := p.statuses[r.ID]
			if !ok {
				existing = &domain.ServerStatus{ServerID: r.ID, Key: r.Key, Source: r.Source, Address: r.Address}
				p.statuses[r.ID] = existing
			}
			existing.Source = r.Source
			existing.Online = false
			existing.LastUpdated = time.Now().UTC()
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
		status.Online = true
		status.LastUpdated = time.Now().UTC()
		status.HumanCount = 0
		status.BotCount = 0
		p.enrichPlayers(ctx, r.ID, &status.HumanCount, &status.BotCount, status.Players)
		p.mu.Lock()
		p.statuses[r.ID] = status
		snapshot := *status
		sink := p.sink
		p.mu.Unlock()
		p.broadcast(sink, snapshot)
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

// enrichPlayers fills in identity + metadata on each PlayerStatus.
// Slots without a presence entry default to IsBot=true so unknown
// clients don't count as humans until their GUID is observed.
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
