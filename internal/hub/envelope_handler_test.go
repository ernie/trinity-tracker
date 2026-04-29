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

func envelopeAt(t *testing.T, source string, seq uint64, ts time.Time, event string, payload interface{}) domain.Envelope {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return domain.Envelope{
		SchemaVersion: domain.EnvelopeSchemaVersion,
		Source:        source,
		Seq:           seq,
		Timestamp:     ts,
		Event:         event,
		Data:          body,
	}
}

// envelopeFor uses a fixed default timestamp; for tests that don't
// care about the TS axis of dedup.
func envelopeFor(t *testing.T, source string, seq uint64, event string, payload interface{}) domain.Envelope {
	return envelopeAt(t, source, seq, time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC), event, payload)
}

func TestHandleEnvelopeDedupsByConsumedSeq(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()
	const source = "src-a"
	w.MarkSourceApproved(source)

	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := store.AdvanceSourceProgress(ctx, source, 10, ts); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// seq=10 ≤ stored 10 → dropped (no advance).
	env := envelopeAt(t, source, 10, ts, domain.FactServerStartup, domain.ServerStartupData{StartedAt: ts})
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}
	prog, _ := store.GetSourceProgress(ctx, source)
	if prog.ConsumedSeq != 10 {
		t.Errorf("after dup consumed_seq = %d, want 10", prog.ConsumedSeq)
	}
}

func TestHandleEnvelopeAdvancesProgress(t *testing.T) {
	w, store := newTestWriter(t)
	ctx := context.Background()
	const source = "src-b"
	w.MarkSourceApproved(source)

	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	env := envelopeAt(t, source, 7, ts, domain.FactServerStartup, domain.ServerStartupData{StartedAt: ts})
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}
	prog, _ := store.GetSourceProgress(ctx, source)
	if prog.ConsumedSeq != 7 || !prog.LastConsumedTS.Equal(ts) {
		t.Errorf("prog = (%d, %v), want (7, %v)", prog.ConsumedSeq, prog.LastConsumedTS, ts)
	}
}

func TestHandleEnvelopeAcceptsOlderTSWithHigherSeq(t *testing.T) {
	// q3 log timestamps aren't strictly monotonic at second
	// resolution: events queued slightly out of order can have an
	// older TS than the previous envelope's. Seq must still be the
	// authoritative dedup signal — older TS does NOT mean drop.
	w, store := newTestWriter(t)
	ctx := context.Background()
	const source = "jitter"
	w.MarkSourceApproved(source)

	tNew := time.Date(2026, 4, 19, 12, 0, 36, 0, time.UTC)
	tOld := time.Date(2026, 4, 19, 12, 0, 34, 0, time.UTC) // 2s earlier
	if err := store.AdvanceSourceProgress(ctx, source, 100, tNew); err != nil {
		t.Fatalf("seed: %v", err)
	}

	env := envelopeAt(t, source, 101, tOld, domain.FactServerStartup, domain.ServerStartupData{StartedAt: tOld})
	if err := w.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("HandleEnvelope: %v", err)
	}
	prog, _ := store.GetSourceProgress(ctx, source)
	if prog.ConsumedSeq != 101 {
		t.Errorf("after older-TS-but-newer-seq advance, consumed_seq = %d, want 101", prog.ConsumedSeq)
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
	env := envelopeAt(t, "", 1, time.Now().UTC(), domain.FactServerStartup, domain.ServerStartupData{})
	if err := w.HandleEnvelope(context.Background(), env); err == nil {
		t.Error("expected error for missing source")
	}
}
