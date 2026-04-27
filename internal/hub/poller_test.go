package hub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

type fakeQuerier struct {
	mu       sync.Mutex
	calls    []string
	responses map[string]*domain.ServerStatus
}

func (f *fakeQuerier) QueryStatus(addr string) (*domain.ServerStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, addr)
	s, ok := f.responses[addr]
	if !ok {
		return nil, nil
	}
	copy := *s
	return &copy, nil
}

func TestRemotePollerPollsRegisteredServers(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()

	// Seed a remote server via approval.
	reg := domain.Registration{
		SourceUUID: "r-uuid",
		Source:     "r",
		Servers:    []domain.RegdServer{{LocalID: 1, Name: "r1", Address: "r.example:27960"}},
	}
	if err := store.UpsertPendingSource(ctx, reg); err != nil {
		t.Fatalf("pending: %v", err)
	}
	if err := store.ApproveRemoteServers(ctx, reg, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{
		"r.example:27960": {Map: "q3dm17", GameType: "FFA"},
	}}
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil)
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poller.Start(pctx)
	defer poller.Stop()

	// Wait for at least one poll.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(poller.GetAllStatuses()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	statuses := poller.GetAllStatuses()
	if len(statuses) != 1 {
		t.Fatalf("statuses len = %d, want 1", len(statuses))
	}
	if statuses[0].Map != "q3dm17" {
		t.Errorf("Map = %q", statuses[0].Map)
	}
	if !statuses[0].Online {
		t.Errorf("expected Online=true")
	}
	q.mu.Lock()
	calls := q.calls
	q.mu.Unlock()
	if len(calls) == 0 {
		t.Error("expected at least one QueryStatus call")
	}
}

func TestRemotePollerUnreachableMarksOffline(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()
	reg := domain.Registration{
		SourceUUID: "r-uuid",
		Source:     "r",
		Servers:    []domain.RegdServer{{LocalID: 1, Name: "r1", Address: "dead:27960"}},
	}
	if err := store.UpsertPendingSource(ctx, reg); err != nil {
		t.Fatalf("pending: %v", err)
	}
	if err := store.ApproveRemoteServers(ctx, reg, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{}} // no response for dead
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil)
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poller.Start(pctx)
	defer poller.Stop()

	time.Sleep(200 * time.Millisecond)
	statuses := poller.GetAllStatuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Online {
		t.Errorf("expected offline for unreachable server")
	}
}
