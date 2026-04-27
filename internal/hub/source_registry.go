package hub

import (
	"context"
	"sync"
	"time"

	"github.com/ernie/trinity-tracker/internal/storage"
)

// UnknownCheckTTL gates repeated IsSourceApproved lookups for the
// same unknown source. A misconfigured or compromised client could
// otherwise drive one SQLite query per message it publishes.
const UnknownCheckTTL = 30 * time.Second

type SourceState int

const (
	SourceApproved SourceState = iota
	SourceUnknown              // not in sources table — event/RPC refused
	SourceBlocked              // admin deleted/revoked since last check
)

// SourceRegistry caches sources-table membership for the ingest path.
// Pre-provisioning is the real trust boundary (NATS auth); this is
// defense-in-depth plus a place to record admin revocations without
// waiting for the next DB round-trip.
type SourceRegistry struct {
	store *storage.Store
	now   func() time.Time

	mu           sync.Mutex
	approved     map[string]struct{}
	blocked      map[string]struct{}
	unknownSince map[string]time.Time
}

func NewSourceRegistry(store *storage.Store) *SourceRegistry {
	return &SourceRegistry{
		store:        store,
		now:          time.Now,
		approved:     make(map[string]struct{}),
		blocked:      make(map[string]struct{}),
		unknownSince: make(map[string]time.Time),
	}
}

func (r *SourceRegistry) State(ctx context.Context, source string) (SourceState, error) {
	r.mu.Lock()
	if _, ok := r.approved[source]; ok {
		r.mu.Unlock()
		return SourceApproved, nil
	}
	if _, ok := r.blocked[source]; ok {
		r.mu.Unlock()
		return SourceBlocked, nil
	}
	now := r.now()
	if at, ok := r.unknownSince[source]; ok && now.Sub(at) < UnknownCheckTTL {
		r.mu.Unlock()
		return SourceUnknown, nil
	}
	r.mu.Unlock()

	ok, err := r.store.IsSourceApproved(ctx, source)
	if err != nil {
		return SourceUnknown, err
	}
	if ok {
		r.MarkApproved(source)
		return SourceApproved, nil
	}
	r.mu.Lock()
	r.unknownSince[source] = now
	r.mu.Unlock()
	return SourceUnknown, nil
}

func (r *SourceRegistry) MarkApproved(source string) {
	r.mu.Lock()
	r.approved[source] = struct{}{}
	delete(r.blocked, source)
	delete(r.unknownSince, source)
	r.mu.Unlock()
}

// MarkBlocked records a revocation so in-flight messages racing the
// admin delete don't slip through until the DB cache ages out.
func (r *SourceRegistry) MarkBlocked(source string) {
	r.mu.Lock()
	r.blocked[source] = struct{}{}
	delete(r.approved, source)
	delete(r.unknownSince, source)
	r.mu.Unlock()
}
