package hub

import (
	"context"
	"sync"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// MaxPendingDLQ caps the per-source dead-letter queue for pending
// sources. Overflow drops the oldest entry — recent history is more
// valuable than ancient gaps when the operator finally approves.
const MaxPendingDLQ = 10_000

// SourceState classifies a source_uuid for event-dispatch gating.
type SourceState int

const (
	// SourceApproved: dispatch events normally.
	SourceApproved SourceState = iota
	// SourcePending: unknown or awaiting admin approval; events
	// accumulate in the in-memory DLQ (capped at MaxPendingDLQ per
	// source; oldest discarded on overflow).
	SourcePending
	// SourceBlocked: rejected by admin; events are silently dropped.
	// In-memory only, not persisted: a hub restart re-presents the
	// source as pending for reclassification.
	SourceBlocked
)

// SourceRegistry gates event dispatch by source_uuid. An approved
// source flows straight through; pending sources accumulate in a
// per-source DLQ until admin approval drains it; blocked sources are
// dropped.
type SourceRegistry struct {
	store *storage.Store

	mu       sync.Mutex
	approved map[string]struct{}
	blocked  map[string]struct{}
	dlqs     map[string][]domain.Envelope
}

// NewSourceRegistry constructs a registry bound to the given store.
// The store is consulted on cache miss to discover already-approved
// sources (i.e. those with servers rows referencing their uuid).
func NewSourceRegistry(store *storage.Store) *SourceRegistry {
	return &SourceRegistry{
		store:    store,
		approved: make(map[string]struct{}),
		blocked:  make(map[string]struct{}),
		dlqs:     make(map[string][]domain.Envelope),
	}
}

// State reports the current gate decision for a source_uuid. The DB
// is consulted once per uuid (results are cached) to recognise
// already-approved sources across restarts.
func (r *SourceRegistry) State(ctx context.Context, sourceUUID string) (SourceState, error) {
	r.mu.Lock()
	if _, ok := r.approved[sourceUUID]; ok {
		r.mu.Unlock()
		return SourceApproved, nil
	}
	if _, ok := r.blocked[sourceUUID]; ok {
		r.mu.Unlock()
		return SourceBlocked, nil
	}
	r.mu.Unlock()

	ok, err := r.store.IsSourceApproved(ctx, sourceUUID)
	if err != nil {
		return SourcePending, err
	}
	if ok {
		r.MarkApproved(sourceUUID)
		return SourceApproved, nil
	}
	return SourcePending, nil
}

// MarkApproved flags a source as approved for immediate dispatch.
// Used both on admin approval and to pre-approve the local collector
// source in hub+collector deployments.
func (r *SourceRegistry) MarkApproved(sourceUUID string) {
	r.mu.Lock()
	r.approved[sourceUUID] = struct{}{}
	delete(r.blocked, sourceUUID)
	r.mu.Unlock()
}

// EnqueueDLQ appends an envelope to the per-source pending queue.
// Returns the size of the queue after insertion; if the queue was
// already at the cap, the oldest entry is discarded silently.
func (r *SourceRegistry) EnqueueDLQ(sourceUUID string, env domain.Envelope) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := r.dlqs[sourceUUID]
	if len(q) >= MaxPendingDLQ {
		q = q[1:]
	}
	q = append(q, env)
	r.dlqs[sourceUUID] = q
	return len(q)
}

// TakeDLQ returns and clears the pending queue for a source, to be
// used by a drain callback. Caller re-submits each envelope through
// the normal dispatch path.
func (r *SourceRegistry) TakeDLQ(sourceUUID string) []domain.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := r.dlqs[sourceUUID]
	delete(r.dlqs, sourceUUID)
	return q
}

// Reject blocks the source and discards any queued envelopes. The
// block lives only in memory: a hub restart forgets the rejection,
// which is acceptable because re-presenting the source as pending
// lets the operator reclassify with fresh context.
func (r *SourceRegistry) Reject(sourceUUID string) {
	r.mu.Lock()
	r.blocked[sourceUUID] = struct{}{}
	delete(r.approved, sourceUUID)
	delete(r.dlqs, sourceUUID)
	r.mu.Unlock()
}
