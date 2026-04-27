package hub_test

import (
	"testing"
	"time"

	"github.com/ernie/trinity-tracker/internal/hub"
)

func TestLocalWatermarkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	tracker, err := hub.NewLocalWatermarkTracker(dir)
	if err != nil {
		t.Fatalf("NewLocalWatermarkTracker: %v", err)
	}
	if !tracker.Current().LastTS.IsZero() {
		t.Errorf("fresh tracker LastTS = %v, want zero", tracker.Current().LastTS)
	}

	ts := time.Date(2026, 4, 19, 15, 30, 0, 0, time.UTC)
	if err := tracker.Observe(ts); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := tracker.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Reload from disk.
	reloaded, err := hub.NewLocalWatermarkTracker(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := reloaded.Current().LastTS
	if !got.Equal(ts) {
		t.Errorf("reloaded LastTS = %v, want %v", got, ts)
	}
}

func TestLocalWatermarkMonotonic(t *testing.T) {
	tracker, err := hub.NewLocalWatermarkTracker(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalWatermarkTracker: %v", err)
	}
	newer := time.Now().UTC()
	older := newer.Add(-time.Hour)

	if err := tracker.Observe(newer); err != nil {
		t.Fatalf("Observe newer: %v", err)
	}
	if err := tracker.Observe(older); err != nil {
		t.Fatalf("Observe older: %v", err)
	}
	if !tracker.Current().LastTS.Equal(newer) {
		t.Errorf("LastTS regressed to %v, want %v", tracker.Current().LastTS, newer)
	}
}

func TestLocalWatermarkFlushNoopOnZero(t *testing.T) {
	tracker, err := hub.NewLocalWatermarkTracker(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalWatermarkTracker: %v", err)
	}
	if err := tracker.Flush(); err != nil {
		t.Errorf("Flush on zero watermark: %v", err)
	}
}
