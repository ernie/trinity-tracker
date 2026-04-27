package hub

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/storage"
)

func newTestWriter(t *testing.T) (*Writer, *storage.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(path)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	w := NewWriter(store)
	// No Start() — we call HandleEnvelope synchronously in tests.
	return w, store
}

func envelopeFor(t *testing.T, source string, seq uint64, event string, payload interface{}) domain.Envelope {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return domain.Envelope{
		SchemaVersion: domain.EnvelopeSchemaVersion,
		Source:        source,
		Seq:           seq,
		Timestamp:     time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		Event:         event,
		Data:          body,
	}
}

func TestHandleEnvelopeDedupsByConsumedSeq(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()
	const source = "src-a"
	w.MarkSourceApproved(source)

	// Seed consumed_seq to 10.
	if err := store.AdvanceConsumedSeq(ctx, source, 10); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// seq=10 should be dedup-dropped (not processed, not advanced).
	env := envelopeFor(t, source, 10, domain.FactServerStartup, domain.ServerStartupData{
		StartedAt: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
	})
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope seq=10: %v", err)
	}
	if seq, _ := store.GetConsumedSeq(ctx, source); seq != 10 {
		t.Errorf("consumed_seq after dup = %d, want 10", seq)
	}
}

func TestHandleEnvelopeAdvancesConsumedSeq(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()
	const source = "src-b"
	w.MarkSourceApproved(source)

	env := envelopeFor(t, source, 7, domain.FactServerStartup, domain.ServerStartupData{
		StartedAt: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
	})
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}
	if seq, _ := store.GetConsumedSeq(ctx, source); seq != 7 {
		t.Errorf("consumed_seq = %d, want 7", seq)
	}
}

func TestHandleEnvelopeRejectsUnknownEvent(t *testing.T) {
	w, _ := newTestWriter(t)
	const source = "src-c"
	w.MarkSourceApproved(source)
	env := domain.Envelope{
		SchemaVersion: 1,
		Source:        source,
		Seq:           1,
		Timestamp:     time.Now().UTC(),
		Event:         "not_a_real_event",
		Data:          json.RawMessage(`{}`),
	}
	if err := w.HandleEnvelope(context.Background(), env); err == nil {
		t.Error("expected error for unknown event type")
	}
}

func TestHandleEnvelopeRequiresSource(t *testing.T) {
	w, _ := newTestWriter(t)
	env := envelopeFor(t, "", 1, domain.FactServerStartup, domain.ServerStartupData{})
	if err := w.HandleEnvelope(context.Background(), env); err == nil {
		t.Error("expected error for missing source")
	}
}
