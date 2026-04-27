package hub

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func TestHandleRegistrationUnknownSourceGoesToPending(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	reg := domain.Registration{
		SourceUUID: "uuid-remote",
		Source:     "remote",
		Version:    "1.12.0",
		Servers:    []domain.RegdServer{{LocalID: 1, Name: "r", Address: "r.example:27960"}},
	}
	if err := w.HandleRegistration(ctx, reg); err != nil {
		t.Fatalf("HandleRegistration: %v", err)
	}

	list, err := store.ListPendingSources(ctx)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(list) != 1 || list[0].SourceUUID != "uuid-remote" {
		t.Errorf("pending list = %+v", list)
	}
}

func TestHandleRegistrationKnownSourceUpdatesHeartbeat(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	srv := &domain.Server{Name: "ffa", Address: "x:27960"}
	if err := store.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.TagLocalServerSource(ctx, srv.ID, "uuid-local", srv.ID); err != nil {
		t.Fatalf("tag: %v", err)
	}

	reg := domain.Registration{SourceUUID: "uuid-local", Source: "ffa"}
	before := time.Now().UTC()
	if err := w.HandleRegistration(ctx, reg); err != nil {
		t.Fatalf("HandleRegistration: %v", err)
	}

	// No pending row was created.
	list, _ := store.ListPendingSources(ctx)
	if len(list) != 0 {
		t.Errorf("unexpected pending rows: %+v", list)
	}
	stamp, err := store.GetServerLastHeartbeat(ctx, srv.ID)
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	if stamp.IsZero() {
		t.Fatal("last_heartbeat_at not set")
	}
	if stamp.Before(before.Add(-time.Second)) {
		t.Errorf("last_heartbeat_at = %v, want ≥ %v", stamp, before)
	}
}
