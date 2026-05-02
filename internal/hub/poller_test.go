package hub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

// seedOwnerID inserts an "owner" user into store and returns its id
// as a *int64. Used to satisfy CreateSource's remote-source
// requirement that owner_user_id reference a real users row.
func seedOwnerID(t *testing.T, store *storage.Store) *int64 {
	t.Helper()
	if err := store.CreateUser(context.Background(), "owner", "x", false, nil); err != nil {
		t.Fatalf("seed owner user: %v", err)
	}
	u, err := store.GetUserByUsername(context.Background(), "owner")
	if err != nil {
		t.Fatalf("lookup seeded owner user: %v", err)
	}
	return &u.ID
}

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

	// Seed a provisioned remote source + its server roster.
	reg := domain.Registration{
		Source:  "remote",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "r.example:27960"}},
	}
	if err := store.CreateSource(ctx, reg.Source, true, seedOwnerID(t, store)); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := store.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("upsert roster: %v", err)
	}
	// ListPollableServers requires handshake_required=1; mark the test
	// server enforcing so the poller treats it as live.
	for _, s := range reg.Servers {
		id, _ := store.ResolveServerIDForSource(ctx, reg.Source, s.LocalID)
		if err := store.SetServerHandshakeRequired(ctx, id, true); err != nil {
			t.Fatalf("SetServerHandshakeRequired: %v", err)
		}
	}

	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{
		"r.example:27960": {
			Map:        "q3dm17",
			GameType:   "FFA",
			ServerVars: map[string]string{"engine": "trinity-engine/0.4.2"},
		},
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
		Source:  "remote",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "dead:27960"}},
	}
	if err := store.CreateSource(ctx, reg.Source, true, seedOwnerID(t, store)); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := store.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("upsert roster: %v", err)
	}
	for _, s := range reg.Servers {
		id, _ := store.ResolveServerIDForSource(ctx, reg.Source, s.LocalID)
		if err := store.SetServerHandshakeRequired(ctx, id, true); err != nil {
			t.Fatalf("SetServerHandshakeRequired: %v", err)
		}
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

// TestRemotePollerHidesNonTrinityEngine verifies that a server which
// answers getstatus but does not advertise the trinity-engine fork is
// kept out of live status (Online=false). Stock ioquake3 has no
// `engine` infostring field, so its absence is enough to fail.
func TestRemotePollerHidesNonTrinityEngine(t *testing.T) {
	cases := []struct {
		name   string
		engine string
	}{
		{"missing engine field", ""},
		{"stock ioq3", "ioq3-1.36"},
		{"close-but-no-cigar prefix", "trinity-engine-fake/1.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, store := newTestWriter(t)
			ctx := context.Background()
			reg := domain.Registration{
				Source:  "remote",
				Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "imposter:27960"}},
			}
			if err := store.CreateSource(ctx, reg.Source, true, seedOwnerID(t, store)); err != nil {
				t.Fatalf("create source: %v", err)
			}
			if err := store.UpsertRemoteServers(ctx, reg); err != nil {
				t.Fatalf("upsert roster: %v", err)
			}
			for _, s := range reg.Servers {
				id, _ := store.ResolveServerIDForSource(ctx, reg.Source, s.LocalID)
				if err := store.SetServerHandshakeRequired(ctx, id, true); err != nil {
					t.Fatalf("SetServerHandshakeRequired: %v", err)
				}
			}

			vars := map[string]string{}
			if tc.engine != "" {
				vars["engine"] = tc.engine
			}
			q := &fakeQuerier{responses: map[string]*domain.ServerStatus{
				"imposter:27960": {Map: "q3dm17", ServerVars: vars},
			}}
			poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil)
			pctx, cancel := context.WithCancel(ctx)
			defer cancel()
			poller.Start(pctx)
			defer poller.Stop()

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
			if statuses[0].Online {
				t.Errorf("expected Online=false for engine=%q", tc.engine)
			}
		})
	}
}
