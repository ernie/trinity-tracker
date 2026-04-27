package natsbus

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadWatermarkMissingReturnsZero(t *testing.T) {
	wm, err := LoadWatermark(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWatermark: %v", err)
	}
	if wm != (Watermark{}) {
		t.Errorf("want zero, got %+v", wm)
	}
}

func TestSaveLoadWatermarkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	wm := Watermark{LastSeq: 12345, LastTS: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)}
	if err := SaveWatermark(dir, wm); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadWatermark(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LastSeq != wm.LastSeq || !got.LastTS.Equal(wm.LastTS) {
		t.Errorf("round trip mismatch: got=%+v want=%+v", got, wm)
	}
}

func TestSaveWatermarkIsAtomic(t *testing.T) {
	// Sentinel: after a successful Save, no .tmp file remains.
	dir := t.TempDir()
	if err := SaveWatermark(dir, Watermark{LastSeq: 1}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, WatermarkFilename+".tmp")); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not exist after save: err=%v", err)
	}
}

func TestWatermarkTrackerUpdateIsMonotonic(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewWatermarkTracker(dir)
	if err != nil {
		t.Fatalf("tracker: %v", err)
	}

	ts := time.Now().UTC()
	if err := tr.Update(10, ts); err != nil {
		t.Fatalf("update 10: %v", err)
	}
	if err := tr.Update(5, ts); err != nil {
		t.Fatalf("update 5 (should no-op): %v", err)
	}
	if got := tr.Current(); got.LastSeq != 10 {
		t.Errorf("Current.LastSeq = %d, want 10 (regression blocked)", got.LastSeq)
	}
}

func TestWatermarkTrackerFlushForcesDisk(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewWatermarkTracker(dir)
	if err != nil {
		t.Fatalf("tracker: %v", err)
	}
	// Single Update shouldn't cross the 50-batch threshold but may be
	// recent enough to skip the 250ms interval.
	if err := tr.Update(1, time.Now().UTC()); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := tr.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	loaded, err := LoadWatermark(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.LastSeq != 1 {
		t.Errorf("on-disk LastSeq = %d, want 1", loaded.LastSeq)
	}
}

func TestWatermarkTrackerBatchFlushTripsAtThreshold(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewWatermarkTracker(dir)
	if err != nil {
		t.Fatalf("tracker: %v", err)
	}
	// Push WatermarkFlushEvery updates — the last one must trigger
	// the flush.
	for i := 1; i <= WatermarkFlushEvery; i++ {
		if err := tr.Update(uint64(i), time.Now().UTC()); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}
	loaded, err := LoadWatermark(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.LastSeq != uint64(WatermarkFlushEvery) {
		t.Errorf("on-disk LastSeq = %d, want %d", loaded.LastSeq, WatermarkFlushEvery)
	}
}
