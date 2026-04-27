package hub

import (
	"context"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func TestSourceRegistryStateApprovedFromDB(t *testing.T) {
	_, store := newTestWriter(t)
	ctx := context.Background()

	srv := &domain.Server{Name: "ffa", Address: "x:27960"}
	if err := store.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.TagLocalServerSource(ctx, srv.ID, "uuid-a", srv.ID); err != nil {
		t.Fatalf("tag: %v", err)
	}

	reg := NewSourceRegistry(store)
	state, err := reg.State(ctx, "uuid-a")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state != SourceApproved {
		t.Errorf("state = %v, want SourceApproved", state)
	}
}

func TestSourceRegistryStatePendingForUnknown(t *testing.T) {
	_, store := newTestWriter(t)
	reg := NewSourceRegistry(store)
	state, err := reg.State(context.Background(), "uuid-unknown")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state != SourcePending {
		t.Errorf("state = %v, want SourcePending", state)
	}
}

func TestSourceRegistryMarkApprovedOverridesBlock(t *testing.T) {
	_, store := newTestWriter(t)
	reg := NewSourceRegistry(store)
	reg.Reject("u")
	if state, _ := reg.State(context.Background(), "u"); state != SourceBlocked {
		t.Fatalf("pre-check state = %v", state)
	}
	reg.MarkApproved("u")
	if state, _ := reg.State(context.Background(), "u"); state != SourceApproved {
		t.Errorf("post-approve state = %v", state)
	}
}

func TestSourceRegistryEnqueueDLQOverflowDropsOldest(t *testing.T) {
	_, store := newTestWriter(t)
	reg := NewSourceRegistry(store)
	// Simulate cap by enqueueing MaxPendingDLQ + 5 distinct envelopes
	// and verifying the first 5 are gone.
	for i := 0; i < MaxPendingDLQ+5; i++ {
		reg.EnqueueDLQ("u", domain.Envelope{Seq: uint64(i + 1)})
	}
	q := reg.TakeDLQ("u")
	if len(q) != MaxPendingDLQ {
		t.Fatalf("dlq len = %d, want %d", len(q), MaxPendingDLQ)
	}
	// Oldest retained envelope should have seq=6 (first 5 dropped).
	if q[0].Seq != 6 {
		t.Errorf("oldest retained seq = %d, want 6", q[0].Seq)
	}
}

func TestApproveSourceDrainsDLQ(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	// Seed a remote collector via registration (pending).
	reg := domain.Registration{
		SourceUUID: "remote-uuid",
		Source:     "remote",
		Servers:    []domain.RegdServer{{LocalID: 1, Name: "r1", Address: "r.example:27960"}},
	}
	if err := store.UpsertPendingSource(ctx, reg); err != nil {
		t.Fatalf("pending: %v", err)
	}

	// Pending ServerStartup envelope arrives — should DLQ, not dispatch.
	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	env := envelopeFor(t, reg.SourceUUID, 1, domain.FactServerStartup, domain.ServerStartupData{StartedAt: ts})
	env.RemoteServerID = 1
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}
	if seq, _ := store.GetConsumedSeq(ctx, reg.SourceUUID); seq != 0 {
		t.Errorf("pending event advanced consumed_seq to %d", seq)
	}

	// Approve; DLQ drains.
	if err := w.ApproveSource(ctx, reg, ""); err != nil {
		t.Fatalf("ApproveSource: %v", err)
	}
	if seq, _ := store.GetConsumedSeq(ctx, reg.SourceUUID); seq != 1 {
		t.Errorf("post-approve consumed_seq = %d, want 1", seq)
	}
}

func TestRejectSourceBlocksLater(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()

	// Pending source with a DLQ'd envelope.
	reg := domain.Registration{
		SourceUUID: "bad-uuid",
		Source:     "bad",
		Servers:    []domain.RegdServer{{LocalID: 1, Name: "b", Address: "b.example:27960"}},
	}
	if err := store.UpsertPendingSource(ctx, reg); err != nil {
		t.Fatalf("pending: %v", err)
	}
	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	env := envelopeFor(t, reg.SourceUUID, 1, domain.FactServerStartup, domain.ServerStartupData{StartedAt: ts})
	env.RemoteServerID = 1
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}

	if err := w.RejectSource(reg.SourceUUID); err != nil {
		t.Fatalf("RejectSource: %v", err)
	}

	// New event now dropped silently.
	env2 := envelopeFor(t, reg.SourceUUID, 2, domain.FactServerStartup, domain.ServerStartupData{StartedAt: ts})
	env2.RemoteServerID = 1
	if err := w.HandleEnvelope(ctx, env2); err != nil {
		t.Fatalf("HandleEnvelope second: %v", err)
	}
	if seq, _ := store.GetConsumedSeq(ctx, reg.SourceUUID); seq != 0 {
		t.Errorf("blocked event advanced consumed_seq to %d", seq)
	}
}
