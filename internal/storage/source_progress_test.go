package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestGetSourceProgressMissingReturnsZero(t *testing.T) {
	s := newTestStore(t)
	prog, err := s.GetSourceProgress(context.Background(), "unknown-source")
	if err != nil {
		t.Fatalf("GetSourceProgress: %v", err)
	}
	if prog.ConsumedSeq != 0 {
		t.Errorf("missing source seq = %d, want 0", prog.ConsumedSeq)
	}
	if !prog.LastConsumedTS.IsZero() {
		t.Errorf("missing source ts = %v, want zero", prog.LastConsumedTS)
	}
}

func TestAdvanceSourceProgressInsertsThenUpdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	source := "abc-123"
	t1 := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 29, 14, 5, 0, 0, time.UTC)

	if err := s.AdvanceSourceProgress(ctx, source, 5, t1); err != nil {
		t.Fatalf("advance to (5, %v): %v", t1, err)
	}
	prog, _ := s.GetSourceProgress(ctx, source)
	if prog.ConsumedSeq != 5 || !prog.LastConsumedTS.Equal(t1) {
		t.Errorf("after insert prog = (%d, %v), want (5, %v)", prog.ConsumedSeq, prog.LastConsumedTS, t1)
	}

	if err := s.AdvanceSourceProgress(ctx, source, 10, t2); err != nil {
		t.Fatalf("advance to (10, %v): %v", t2, err)
	}
	prog, _ = s.GetSourceProgress(ctx, source)
	if prog.ConsumedSeq != 10 || !prog.LastConsumedTS.Equal(t2) {
		t.Errorf("after advance prog = (%d, %v), want (10, %v)", prog.ConsumedSeq, prog.LastConsumedTS, t2)
	}
}

func TestAdvanceSourceProgressMonotonicOnSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	source := "abc-123"
	tNow := time.Date(2026, 4, 29, 14, 30, 0, 0, time.UTC)
	tOlder := time.Date(2026, 4, 29, 14, 0, 0, 0, time.UTC)

	if err := s.AdvanceSourceProgress(ctx, source, 100, tNow); err != nil {
		t.Fatalf("advance to (100, %v): %v", tNow, err)
	}
	// Lower seq must not regress, regardless of TS.
	if err := s.AdvanceSourceProgress(ctx, source, 50, tNow); err != nil {
		t.Fatalf("advance to lower seq should be no-op: %v", err)
	}
	prog, _ := s.GetSourceProgress(ctx, source)
	if prog.ConsumedSeq != 100 || !prog.LastConsumedTS.Equal(tNow) {
		t.Errorf("after regression attempt prog = (%d, %v), want (100, %v)", prog.ConsumedSeq, prog.LastConsumedTS, tNow)
	}

	// Higher seq advances even with an older TS — q3 log timestamps
	// aren't strictly monotonic at second resolution.
	if err := s.AdvanceSourceProgress(ctx, source, 101, tOlder); err != nil {
		t.Fatalf("advance to (101, tOlder): %v", err)
	}
	prog, _ = s.GetSourceProgress(ctx, source)
	if prog.ConsumedSeq != 101 || !prog.LastConsumedTS.Equal(tOlder) {
		t.Errorf("after older-TS advance prog = (%d, %v), want (101, %v)", prog.ConsumedSeq, prog.LastConsumedTS, tOlder)
	}
}
