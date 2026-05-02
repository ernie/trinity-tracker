package hub

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func TestHandleRegistrationUnknownSourceIsRefused(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	reg := domain.Registration{
		Source:  "remote",
		Version: "1.12.0",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r", Address: "r.example:27960"}},
	}
	if err := w.HandleRegistration(ctx, reg); err != nil {
		t.Fatalf("HandleRegistration: %v", err)
	}

	// No servers row should have been created for an unprovisioned source.
	if id, _ := store.ResolveServerIDForSource(ctx, reg.Source, 1); id != 0 {
		t.Errorf("unprovisioned registration created servers row id=%d", id)
	}
	if ok, _ := store.IsSourceApproved(ctx, reg.Source); ok {
		t.Error("unprovisioned source is approved")
	}
}

func TestHandleRegistrationKnownSourceUpdatesHeartbeatAndRoster(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	if err := store.CreateSource(ctx, "remote", true, seedOwnerID(t, store)); err != nil {
		t.Fatalf("create source: %v", err)
	}

	reg := domain.Registration{
		Source:  "remote",
		Version: "1.13.0",
		Servers: []domain.RegdServer{{LocalID: 1, Key: "r", Address: "r.example:27960"}},
	}
	before := time.Now().UTC()
	if err := w.HandleRegistration(ctx, reg); err != nil {
		t.Fatalf("HandleRegistration: %v", err)
	}

	id, err := store.ResolveServerIDForSource(ctx, reg.Source, 1)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id == 0 {
		t.Fatal("servers row not created from roster")
	}
	stamp, err := store.GetServerLastHeartbeat(ctx, id)
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	if stamp.Before(before.Add(-time.Second)) {
		t.Errorf("last_heartbeat_at = %v, want ≥ %v", stamp, before)
	}
}
