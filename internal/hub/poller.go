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

// StatusQuerier is any UDP client capable of answering a Q3 server's
// getstatus. *collector.Q3Client satisfies it; tests can substitute a
// fake.
type StatusQuerier interface {
	QueryStatus(address string) (*domain.ServerStatus, error)
}

// IdentityResolver answers "what players row is this GUID linked to,
// and what are its verified/admin flags?". Satisfied by *Writer.
type IdentityResolver interface {
	ResolveIdentity(ctx context.Context, guid string) (playerID int64, verified, admin, ok bool)
}

// RemotePoller periodically polls every servers row that has a usable
// address and caches the latest ServerStatus. It enriches each
// PlayerStatus by resolving (serverID, ClientNum) → GUID via the
// presence tracker and then GUID → player_id / verified / admin via
// the identity resolver. Originally added for remote-only deployments
// (M2 phase 8) it is now the unified poller for hub, hub+collector,
// and standalone modes.
type RemotePoller struct {
	store    *storage.Store
	querier  StatusQuerier
	interval time.Duration
	presence *Presence
	identity IdentityResolver

	mu       sync.RWMutex
	statuses map[int64]*domain.ServerStatus

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewRemotePoller constructs a poller. interval <= 0 falls back to a
// conservative 10s so a misconfigured instance doesn't hammer remote
// hosts. presence and identity are both optional — a nil presence
// skips enrichment, a nil identity leaves verified/admin false.
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

// Start launches the poll loop. Returns immediately.
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
	// Initial poll.
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
		status, err := p.querier.QueryStatus(r.RemoteAddress)
		if err != nil || status == nil {
			p.mu.Lock()
			existing, ok := p.statuses[r.ID]
			if !ok {
				existing = &domain.ServerStatus{ServerID: r.ID, Name: r.Name, Address: r.RemoteAddress}
				p.statuses[r.ID] = existing
			}
			existing.Online = false
			existing.LastUpdated = time.Now().UTC()
			p.mu.Unlock()
			continue
		}
		status.ServerID = r.ID
		status.Name = r.Name
		status.Address = r.RemoteAddress
		status.Online = true
		status.LastUpdated = time.Now().UTC()
		p.enrichPlayers(ctx, r.ID, status.Players)
		p.mu.Lock()
		p.statuses[r.ID] = status
		p.mu.Unlock()
	}
}

// enrichPlayers maps each PlayerStatus.ClientNum → GUID via the
// presence tracker, then GUID → player_id / verified / admin via the
// identity resolver. Silent no-op when either collaborator is nil.
func (p *RemotePoller) enrichPlayers(ctx context.Context, serverID int64, players []domain.PlayerStatus) {
	if p.presence == nil {
		return
	}
	for i := range players {
		ps := &players[i]
		if ps.ClientNum < 0 {
			continue
		}
		guid := p.presence.Lookup(serverID, ps.ClientNum)
		if guid == "" {
			continue
		}
		ps.GUID = guid
		if p.identity == nil {
			continue
		}
		playerID, verified, admin, ok := p.identity.ResolveIdentity(ctx, guid)
		if !ok {
			continue
		}
		id := playerID
		ps.PlayerID = &id
		ps.IsVerified = verified
		ps.IsAdmin = admin
	}
}
