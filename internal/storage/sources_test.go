package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// fixtureOwner inserts a single "owner" user into s and returns its
// id as a *int64 — convenient for tests that just need a non-nil
// owner pointer to satisfy CreateSource's remote-source contract.
// Each test gets its own DB via newTestStore, so the fixed username
// never collides across tests.
func fixtureOwner(t *testing.T, s *Store) *int64 {
	t.Helper()
	id := mustCreateUser(t, s, "owner")
	return &id
}

func TestCreateAndListSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.CreateSource(ctx, "remote-1", true, fixtureOwner(t, s)); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	if err := s.UpsertLocalSource(ctx, "local"); err != nil {
		t.Fatalf("upsert local: %v", err)
	}
	// Upsert is idempotent.
	if err := s.UpsertLocalSource(ctx, "local"); err != nil {
		t.Fatalf("upsert local (repeat): %v", err)
	}

	// Remote collector reports its demo_base_url via a heartbeat; admin
	// never touches this field.
	if err := s.TouchSourceHeartbeat(ctx, "remote-1", time.Now().UTC(), "", "https://remote-1.example.com"); err != nil {
		t.Fatalf("touch remote heartbeat: %v", err)
	}

	list, err := s.ListApprovedSources(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 sources, got %d", len(list))
	}
	var remote, local *ApprovedSource
	for i := range list {
		switch list[i].Source {
		case "remote-1":
			remote = &list[i]
		case "local":
			local = &list[i]
		}
	}
	if remote == nil || !remote.IsRemote || remote.DemoBaseURL != "https://remote-1.example.com" {
		t.Errorf("remote source = %+v", remote)
	}
	if local == nil || local.IsRemote {
		t.Errorf("local source = %+v", local)
	}
}

func TestCreateSourceRejectsInvalidName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	owner := fixtureOwner(t, s)
	cases := []string{"", "with space", "with.dot", "has/slash", "star*", ">wild"}
	for _, name := range cases {
		if err := s.CreateSource(ctx, name, true, owner); err == nil {
			t.Errorf("CreateSource(%q) accepted, want rejection", name)
		}
	}
}

func TestIsSourceApprovedFollowsSourcesRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if ok, _ := s.IsSourceApproved(ctx, "nope"); ok {
		t.Error("unknown source should not be approved")
	}
	if err := s.CreateSource(ctx, "ffa", true, fixtureOwner(t, s)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if ok, _ := s.IsSourceApproved(ctx, "ffa"); !ok {
		t.Error("created source should be approved")
	}
}

func TestTouchSourceHeartbeatUpdatesBothTables(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.CreateSource(ctx, "ffa", true, fixtureOwner(t, s)); err != nil {
		t.Fatalf("create: %v", err)
	}
	srv := &domain.Server{Key: "ffa", Address: "x:27960"}
	if err := s.UpsertServer(ctx, "test", srv); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.TagLocalServerSource(ctx, srv.ID, "ffa", srv.ID); err != nil {
		t.Fatalf("tag: %v", err)
	}

	beat := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := s.TouchSourceHeartbeat(ctx, "ffa", beat, "1.13.0", "https://demos.example.com"); err != nil {
		t.Fatalf("touch: %v", err)
	}

	var serverStamp time.Time
	err := s.db.QueryRowContext(ctx, "SELECT last_heartbeat_at FROM servers WHERE id = ?", srv.ID).Scan(&serverStamp)
	if err != nil {
		t.Fatalf("read servers: %v", err)
	}
	if !serverStamp.Equal(beat) {
		t.Errorf("servers.last_heartbeat_at = %v, want %v", serverStamp, beat)
	}
	var sourceStamp time.Time
	var version string
	err = s.db.QueryRowContext(ctx, "SELECT last_heartbeat_at, version FROM sources WHERE source = ?", "ffa").Scan(&sourceStamp, &version)
	if err != nil {
		t.Fatalf("read sources: %v", err)
	}
	if !sourceStamp.Equal(beat) {
		t.Errorf("sources.last_heartbeat_at = %v, want %v", sourceStamp, beat)
	}
	if version != "1.13.0" {
		t.Errorf("sources.version = %q, want 1.13.0", version)
	}
}

func TestResolveServerIDForSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	srv := &domain.Server{Key: "ffa", Address: "x:27960"}
	if err := s.UpsertServer(ctx, "test", srv); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.TagLocalServerSource(ctx, srv.ID, "ffa", 42); err != nil {
		t.Fatalf("tag: %v", err)
	}

	got, err := s.ResolveServerIDForSource(ctx, "ffa", 42)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != srv.ID {
		t.Errorf("resolved = %d, want %d", got, srv.ID)
	}

	missing, err := s.ResolveServerIDForSource(ctx, "ffa", 999)
	if err != nil {
		t.Fatalf("resolve missing: %v", err)
	}
	if missing != 0 {
		t.Errorf("missing = %d, want 0", missing)
	}
}

func TestUpsertRemoteServersRoster(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.CreateSource(ctx, "remote-1", true, fixtureOwner(t, s)); err != nil {
		t.Fatalf("create source: %v", err)
	}
	reg := domain.Registration{
		Source: "remote-1",
		Servers: []domain.RegdServer{
			{LocalID: 1, Key: "r1", Address: "r1.example.com:27960"},
			{LocalID: 2, Key: "r2", Address: "r2.example.com:27961"},
		},
	}
	if err := s.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	id1, _ := s.ResolveServerIDForSource(ctx, "remote-1", 1)
	id2, _ := s.ResolveServerIDForSource(ctx, "remote-1", 2)
	if id1 == 0 || id2 == 0 || id1 == id2 {
		t.Errorf("resolved ids: %d %d", id1, id2)
	}

	// Re-running the roster (renamed server) should UPDATE, not duplicate.
	reg.Servers[0].Key = "renamed-r1"
	if err := s.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	var key string
	if err := s.db.QueryRowContext(ctx, "SELECT key FROM servers WHERE id = ?", id1).Scan(&key); err != nil {
		t.Fatalf("read key: %v", err)
	}
	if key != "renamed-r1" {
		t.Errorf("key = %q, want renamed-r1", key)
	}
}

func TestDeactivateSourceCascadesAndIsReversible(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.CreateSource(ctx, "remote-1", true, fixtureOwner(t, s)); err != nil {
		t.Fatalf("create: %v", err)
	}
	reg := domain.Registration{
		Source:  "remote-1",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r1", Address: "r1:27960"}},
	}
	if err := s.UpsertRemoteServers(ctx, reg); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := s.DeactivateSource(ctx, "remote-1"); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if ok, _ := s.IsSourceApproved(ctx, "remote-1"); ok {
		t.Error("source still approved after deactivate")
	}
	// Server row preserved (matches still need it) and its active=0.
	srvID, _ := s.ResolveServerIDForSource(ctx, "remote-1", 1)
	if srvID == 0 {
		t.Fatal("servers row removed; expected to be preserved after deactivate")
	}
	var active int
	if err := s.db.QueryRowContext(ctx, "SELECT active FROM servers WHERE id = ?", srvID).Scan(&active); err != nil {
		t.Fatalf("read active: %v", err)
	}
	if active != 0 {
		t.Errorf("server active = %d after deactivate, want 0", active)
	}

	// Reactivate: cascade flips both flags back on.
	if err := s.ReactivateSource(ctx, "remote-1"); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if ok, _ := s.IsSourceApproved(ctx, "remote-1"); !ok {
		t.Error("source not approved after reactivate")
	}
	if err := s.db.QueryRowContext(ctx, "SELECT active FROM servers WHERE id = ?", srvID).Scan(&active); err != nil {
		t.Fatalf("read active: %v", err)
	}
	if active != 1 {
		t.Errorf("server active = %d after reactivate, want 1", active)
	}
}
