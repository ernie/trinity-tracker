package hub

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

type fakeSourceConns map[string][]netip.Addr

func (f fakeSourceConns) ConnectedClientIPs(userPubKey string) []netip.Addr {
	return f[userPubKey]
}

// seedOwnerID inserts an owner user and returns its ID, since a remote
// source needs a real owner_user_id.
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
	mu        sync.Mutex
	calls     []string
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
	// ListPollableServers lists only servers with handshake_required set.
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
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, nil)
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

func TestPollerStampsIsLive(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()

	reg := domain.Registration{
		Source:  "remote",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "live.example:27960"}},
	}
	if err := store.CreateSource(ctx, reg.Source, true, seedOwnerID(t, store)); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := store.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("upsert roster: %v", err)
	}
	var serverID int64
	for _, s := range reg.Servers {
		id, _ := store.ResolveServerIDForSource(ctx, reg.Source, s.LocalID)
		serverID = id
		if err := store.SetServerHandshakeRequired(ctx, id, true); err != nil {
			t.Fatalf("SetServerHandshakeRequired: %v", err)
		}
	}

	ls := NewLiveState()
	ls.Set(reg.Source, "r1", true)

	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{
		"live.example:27960": {
			Map:        "q3dm17",
			ServerVars: map[string]string{"engine": "trinity-engine/0.4.2"},
		},
	}}
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, nil)
	poller.SetLiveState(ls)
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poller.Start(pctx)
	defer poller.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := poller.GetServerStatus(serverID)
		if st != nil && st.Online {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := poller.GetServerStatus(serverID)
	if st == nil {
		t.Fatal("no status cached")
	}
	if !st.IsLive {
		t.Fatal("IsLive not stamped from LiveState")
	}
}

func TestPollerClearsLiveDelayWhenOffline(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()
	reg := domain.Registration{
		Source:  "remote",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "flap.example:27960"}},
	}
	if err := store.CreateSource(ctx, reg.Source, true, seedOwnerID(t, store)); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := store.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("upsert roster: %v", err)
	}
	var serverID int64
	for _, s := range reg.Servers {
		id, _ := store.ResolveServerIDForSource(ctx, reg.Source, s.LocalID)
		serverID = id
		if err := store.SetServerHandshakeRequired(ctx, id, true); err != nil {
			t.Fatalf("SetServerHandshakeRequired: %v", err)
		}
	}

	ls := NewLiveState()
	ls.Set(reg.Source, "r1", true)
	ls.SetDelay(reg.Source, "r1", 5)

	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{
		"flap.example:27960": {Map: "q3dm17", ServerVars: map[string]string{"engine": "trinity-engine/0.4.2"}},
	}}
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, nil)
	poller.SetLiveState(ls)
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poller.Start(pctx)
	defer poller.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st := poller.GetServerStatus(serverID); st != nil && st.Online && st.LiveDelaySeconds == 5 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := poller.GetServerStatus(serverID); st == nil || st.LiveDelaySeconds != 5 {
		t.Fatalf("precondition not met: live delay not stamped while online: %+v", st)
	}

	q.mu.Lock()
	q.responses = map[string]*domain.ServerStatus{}
	q.mu.Unlock()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st := poller.GetServerStatus(serverID); st != nil && !st.Online {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := poller.GetServerStatus(serverID)
	if st == nil {
		t.Fatal("no status cached")
	}
	if st.IsLive {
		t.Error("IsLive not cleared when offline")
	}
	if st.LiveDelaySeconds != 0 {
		t.Errorf("LiveDelaySeconds = %d when offline, want 0 (stale live delay survived)", st.LiveDelaySeconds)
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
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, nil)
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

func TestRemotePollerSubstitutesConnIP(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()
	reg := domain.Registration{
		Source:  "remote",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "pi.example:27960"}},
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

	// The fake answers only on the live IP. MintUserCreds isn't wired
	// here, so user_pubkey is seeded directly.
	const remotePub = "UABCDEFREMOTE1234567890PUBKEY"
	if err := store.SetSourceUserPubKey(ctx, reg.Source, remotePub); err != nil {
		t.Fatalf("SetSourceUserPubKey: %v", err)
	}
	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{
		"203.0.113.42:27960": {
			Map:        "q3dm17",
			ServerVars: map[string]string{"engine": "trinity-engine/0.4.2"},
		},
	}}
	conns := fakeSourceConns{remotePub: {netip.MustParseAddr("203.0.113.42")}}
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, conns)
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poller.Start(pctx)
	defer poller.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statuses := poller.GetAllStatuses()
		if len(statuses) > 0 && statuses[0].Online {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	statuses := poller.GetAllStatuses()
	if len(statuses) != 1 || !statuses[0].Online {
		t.Fatalf("expected one online status, got %+v", statuses)
	}
	// The displayed address keeps the hostname.
	if statuses[0].Address != "pi.example:27960" {
		t.Errorf("Address = %q, want %q (display should keep hostname)", statuses[0].Address, "pi.example:27960")
	}
	q.mu.Lock()
	calls := q.calls
	q.mu.Unlock()
	for _, c := range calls {
		if c == "pi.example:27960" {
			t.Errorf("poller hit hostname %q; expected only IP-form calls", c)
		}
	}
}

func TestRemotePollerOfflineWhenSourceUnconnected(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()
	reg := domain.Registration{
		Source:  "remote",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "pi.example:27960"}},
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

	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{}}
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, fakeSourceConns{})
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
		t.Errorf("expected offline when collector is not connected")
	}
	q.mu.Lock()
	calls := q.calls
	q.mu.Unlock()
	if len(calls) != 0 {
		t.Errorf("expected no QueryStatus calls when no live conn, got %v", calls)
	}
}

func TestRemotePollerPicksFreshestConn(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()
	reg := domain.Registration{
		Source:  "remote",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "pi.example:27960"}},
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
	const remotePub = "UABCDEFREMOTE1234567890PUBKEY"
	if err := store.SetSourceUserPubKey(ctx, reg.Source, remotePub); err != nil {
		t.Fatalf("SetSourceUserPubKey: %v", err)
	}

	// The fake answers only on the freshest IP.
	freshIP := "198.51.100.50"
	staleIP := "203.0.113.99"
	conns := fakeSourceConns{remotePub: {
		netip.MustParseAddr(freshIP),
		netip.MustParseAddr(staleIP),
	}}
	q := &fakeQuerier{responses: map[string]*domain.ServerStatus{
		freshIP + ":27960": {
			Map:        "q3dm17",
			ServerVars: map[string]string{"engine": "trinity-engine/0.4.2"},
		},
	}}
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, conns)
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poller.Start(pctx)
	defer poller.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statuses := poller.GetAllStatuses()
		if len(statuses) > 0 && statuses[0].Online {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	statuses := poller.GetAllStatuses()
	if len(statuses) != 1 || !statuses[0].Online {
		t.Fatalf("expected one online status, got %+v", statuses)
	}
	q.mu.Lock()
	calls := q.calls
	q.mu.Unlock()
	for _, c := range calls {
		if c == staleIP+":27960" {
			t.Errorf("poller hit stale IP %q; it must target the freshest (ips[0])", c)
		}
	}
}

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
			poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, nil)
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
