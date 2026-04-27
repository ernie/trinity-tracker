package natsbus

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

func fact(t *testing.T, uuid string, ts time.Time) domain.FactEvent {
	t.Helper()
	return domain.FactEvent{
		Type:      domain.FactMatchStart,
		ServerID:  1,
		Timestamp: ts,
		Data: domain.MatchStartData{
			MatchUUID:         uuid,
			MapName:           "q3dm17",
			GameType:          "FFA",
			StartedAt:         ts,
			HandshakeRequired: true,
		},
	}
}

func TestSpillRoundTrip(t *testing.T) {
	dir := t.TempDir()
	q, err := NewSpillQueue(dir)
	if err != nil {
		t.Fatalf("NewSpillQueue: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	if !q.IsEmpty() {
		t.Fatal("fresh queue not empty")
	}
	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := q.Append(fact(t, "uuid-a", ts)); err != nil {
		t.Fatalf("Append a: %v", err)
	}
	if err := q.Append(fact(t, "uuid-b", ts.Add(time.Second))); err != nil {
		t.Fatalf("Append b: %v", err)
	}

	got, ok, err := q.Peek()
	if err != nil || !ok {
		t.Fatalf("Peek 1: err=%v ok=%v", err, ok)
	}
	if got.Type != domain.FactMatchStart {
		t.Errorf("Peek Type = %q", got.Type)
	}
	// Data survives as json.RawMessage — confirm it round-trips via json.
	raw, ok := got.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("Data type = %T, want json.RawMessage", got.Data)
	}
	if !bytes.Contains(raw, []byte(`"uuid-a"`)) {
		t.Errorf("first payload missing uuid-a: %s", raw)
	}
	if err := q.Advance(); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	got, ok, err = q.Peek()
	if err != nil || !ok {
		t.Fatalf("Peek 2: err=%v ok=%v", err, ok)
	}
	raw, _ = got.Data.(json.RawMessage)
	if !bytes.Contains(raw, []byte(`"uuid-b"`)) {
		t.Errorf("second payload missing uuid-b: %s", raw)
	}
	if err := q.Advance(); err != nil {
		t.Fatalf("Advance 2: %v", err)
	}
	if !q.IsEmpty() {
		t.Error("queue not empty after draining both entries")
	}
}

func TestSpillPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	q, err := NewSpillQueue(dir)
	if err != nil {
		t.Fatalf("NewSpillQueue: %v", err)
	}
	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := q.Append(fact(t, "persist-a", ts)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := q.Append(fact(t, "persist-b", ts.Add(time.Second))); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Drain one entry then close to exercise the head-persist path.
	if _, _, err := q.Peek(); err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if err := q.Advance(); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	q2, err := NewSpillQueue(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = q2.Close() })

	got, ok, err := q2.Peek()
	if err != nil || !ok {
		t.Fatalf("Peek after reopen: err=%v ok=%v", err, ok)
	}
	raw, _ := got.Data.(json.RawMessage)
	if !bytes.Contains(raw, []byte(`"persist-b"`)) {
		t.Errorf("expected persist-b at head after reopen, got %s", raw)
	}
}

func TestSpillRotatesOnOverflow(t *testing.T) {
	dir := t.TempDir()
	// Temporarily shrink the cap so the test can exercise rotation.
	old := overrideSpillCap(t, 1024)
	defer old()

	q, err := NewSpillQueue(dir)
	if err != nil {
		t.Fatalf("NewSpillQueue: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	// Pack the queue well past the cap so rotation drops oldest
	// entries. Each entry is ~200–300 bytes.
	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 30; i++ {
		if err := q.Append(fact(t, "u-"+strings.Repeat("x", i%5), ts.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	if q.Dropped() == 0 {
		t.Errorf("expected Dropped() > 0 after overflow, got 0 (size=%d)", q.Size())
	}
	if q.Size() > 1024 {
		t.Errorf("Size %d > cap 1024", q.Size())
	}
}

func TestSpillHeadFileWritten(t *testing.T) {
	dir := t.TempDir()
	q, err := NewSpillQueue(dir)
	if err != nil {
		t.Fatalf("NewSpillQueue: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	ts := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	if err := q.Append(fact(t, "head-a", ts)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, _, err := q.Peek(); err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if err := q.Advance(); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	headPath := filepath.Join(dir, SpillHeadFilename)
	h, err := loadSpillHead(headPath)
	if err != nil {
		t.Fatalf("loadSpillHead: %v", err)
	}
	// Queue drained → reset to 0.
	if h.Offset != 0 {
		t.Errorf("head offset = %d, want 0 after drain-then-reset", h.Offset)
	}
}

// overrideSpillCap temporarily swaps the package-level SpillCapBytes so
// tests can exercise rotation without writing 100MB.
func overrideSpillCap(t *testing.T, n int64) func() {
	t.Helper()
	orig := spillCapOverride
	spillCapOverride = n
	return func() { spillCapOverride = orig }
}
