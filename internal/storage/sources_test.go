package storage

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func TestUpsertPendingSourceRefreshes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	reg := domain.Registration{
		SourceUUID: "uuid-1",
		Source:     "ffa",
		Version:    "1.12.0",
		Servers:    []domain.RegdServer{{LocalID: 1, Name: "ffa", Address: "x:27960"}},
	}
	if err := s.UpsertPendingSource(ctx, reg); err != nil {
		t.Fatalf("insert: %v", err)
	}

	list, err := s.ListPendingSources(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 pending, got %d", len(list))
	}
	if list[0].Source != "ffa" || list[0].Version != "1.12.0" {
		t.Errorf("row = %+v", list[0])
	}

	// Upsert with a later version, confirm it refreshes.
	reg.Version = "1.13.0"
	if err := s.UpsertPendingSource(ctx, reg); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	list, _ = s.ListPendingSources(ctx)
	if len(list) != 1 {
		t.Fatalf("want 1 pending after upsert, got %d", len(list))
	}
	if list[0].Version != "1.13.0" {
		t.Errorf("version = %q, want 1.13.0", list[0].Version)
	}
}

func TestIsSourceApprovedFollowsServersRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if ok, _ := s.IsSourceApproved(ctx, "nope"); ok {
		t.Error("unknown source should not be approved")
	}

	srv := &domain.Server{Name: "ffa", Address: "x:27960"}
	if err := s.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if err := s.TagLocalServerSource(ctx, srv.ID, "uuid-local", srv.ID); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if ok, _ := s.IsSourceApproved(ctx, "uuid-local"); !ok {
		t.Error("tagged source should be approved")
	}
}

func TestTouchSourceHeartbeatUpdatesRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	srv := &domain.Server{Name: "ffa", Address: "x:27960"}
	if err := s.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.TagLocalServerSource(ctx, srv.ID, "uuid-1", srv.ID); err != nil {
		t.Fatalf("tag: %v", err)
	}

	beat := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := s.TouchSourceHeartbeat(ctx, "uuid-1", beat); err != nil {
		t.Fatalf("touch: %v", err)
	}

	var stamp time.Time
	err := s.db.QueryRowContext(ctx, "SELECT last_heartbeat_at FROM servers WHERE id = ?", srv.ID).Scan(&stamp)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !stamp.Equal(beat) {
		t.Errorf("last_heartbeat_at = %v, want %v", stamp, beat)
	}
}

func TestResolveServerIDForSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	srv := &domain.Server{Name: "ffa", Address: "x:27960"}
	if err := s.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.TagLocalServerSource(ctx, srv.ID, "uuid-1", 42); err != nil {
		t.Fatalf("tag: %v", err)
	}

	got, err := s.ResolveServerIDForSource(ctx, "uuid-1", 42)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != srv.ID {
		t.Errorf("resolved = %d, want %d", got, srv.ID)
	}

	missing, err := s.ResolveServerIDForSource(ctx, "uuid-1", 999)
	if err != nil {
		t.Fatalf("resolve missing: %v", err)
	}
	if missing != 0 {
		t.Errorf("missing = %d, want 0", missing)
	}
}

func TestApproveRemoteServersFlow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	reg := domain.Registration{
		SourceUUID: "remote-1",
		Source:     "remote",
		Servers: []domain.RegdServer{
			{LocalID: 1, Name: "r1", Address: "r1.example.com:27960"},
			{LocalID: 2, Name: "r2", Address: "r2.example.com:27961"},
		},
	}
	if err := s.UpsertPendingSource(ctx, reg); err != nil {
		t.Fatalf("pending: %v", err)
	}
	if err := s.ApproveRemoteServers(ctx, reg, "https://remote.example.com"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// Pending row cleared.
	list, _ := s.ListPendingSources(ctx)
	if len(list) != 0 {
		t.Errorf("pending still has %d rows after approve", len(list))
	}
	// Both servers exist and resolve back.
	id1, _ := s.ResolveServerIDForSource(ctx, "remote-1", 1)
	id2, _ := s.ResolveServerIDForSource(ctx, "remote-1", 2)
	if id1 == 0 || id2 == 0 || id1 == id2 {
		t.Errorf("resolved ids: %d %d", id1, id2)
	}
	// Approved.
	if ok, _ := s.IsSourceApproved(ctx, "remote-1"); !ok {
		t.Error("source not approved post-approve")
	}
}
