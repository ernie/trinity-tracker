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
	poller := NewRemotePoller(store, q, 50*time.Millisecond, nil, nil, nil)
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

// TestRemotePollerSubstitutesConnIP verifies that for is_remote=1 rows
// the poller targets the live NATS-connection IP, not the (possibly
// stale) DNS resolution of r.Address. This is what keeps cards live
// across a dynamic-DNS IP rotation: the kernel knows the current IP
// the moment the collector reconnects, so we don't sit on the old IP
// for the DNS TTL.
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

	// The fake answers only on the live IP; if the poller hit the
	// hostname we'd get nil and Online would stay false.
	// MintUserCreds isn't wired in this test, so seed user_pubkey
	// directly. Real flow: AuthStore.MintUserCreds populates this on
	// admin approval; ListPollableServers JOINs it onto the row.
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
	// Display address must keep the friendly hostname, not the IP we
	// poll. The UI shows hostnames; the IP swap is a transport detail.
	if statuses[0].Address != "pi.example:27960" {
		t.Errorf("Address = %q, want %q (display should keep hostname)", statuses[0].Address, "pi.example:27960")
	}
	q.mu.Lock()
	calls := q.calls
	q.mu.Unlock()
	for _, c := range calls {
		if c == "pi.example:27960" {
			t.Errorf("poller hit hostname %q — expected only IP-form calls", c)
		}
	}
}

// TestRemotePollerOfflineWhenSourceUnconnected verifies that a remote
// row whose collector is currently disconnected is marked offline
// without ever calling QueryStatus — there's no IP we can trust to
// poll, and DNS would just give us the stale answer we're trying to
// avoid.
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

// TestRemotePollerPicksFreshestConn pins the SourceConns ordering
// contract: ips[0] is the most-recently-active connection, and the
// poller must target it. During a reconnect overlap the older session
// is still authenticated but on its way out — polling it would time
// out (the peer may have rebooted or rotated its WAN IP) until the
// keepalive collapses the dead conn.
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

	// The fake answers only on the freshest IP. Order is freshest-first
	// per the SourceConns contract; the stale IP is here to prove the
	// poller doesn't fall back to it.
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
			t.Errorf("poller hit stale IP %q — must target freshest (ips[0])", c)
		}
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
