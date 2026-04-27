package storage

import (
	"context"
	"path/filepath"
	"testing"
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

func TestGetConsumedSeqMissingReturnsZero(t *testing.T) {
	s := newTestStore(t)
	seq, err := s.GetConsumedSeq(context.Background(), "unknown-source")
	if err != nil {
		t.Fatalf("GetConsumedSeq: %v", err)
	}
	if seq != 0 {
		t.Errorf("missing source seq = %d, want 0", seq)
	}
}

func TestAdvanceConsumedSeqInsertsThenUpdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uuid := "abc-123"

	if err := s.AdvanceConsumedSeq(ctx, uuid, 5); err != nil {
		t.Fatalf("advance to 5: %v", err)
	}
	if seq, _ := s.GetConsumedSeq(ctx, uuid); seq != 5 {
		t.Errorf("after insert seq = %d, want 5", seq)
	}

	if err := s.AdvanceConsumedSeq(ctx, uuid, 10); err != nil {
		t.Fatalf("advance to 10: %v", err)
	}
	if seq, _ := s.GetConsumedSeq(ctx, uuid); seq != 10 {
		t.Errorf("after advance seq = %d, want 10", seq)
	}
}

func TestAdvanceConsumedSeqMonotonic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uuid := "abc-123"

	if err := s.AdvanceConsumedSeq(ctx, uuid, 100); err != nil {
		t.Fatalf("advance to 100: %v", err)
	}
	if err := s.AdvanceConsumedSeq(ctx, uuid, 50); err != nil {
		t.Fatalf("advance to 50 (should be no-op, not error): %v", err)
	}
	if seq, _ := s.GetConsumedSeq(ctx, uuid); seq != 100 {
		t.Errorf("after regression attempt seq = %d, want 100 (held)", seq)
	}
}
