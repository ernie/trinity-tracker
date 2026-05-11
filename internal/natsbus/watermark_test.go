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
	if !wm.IsZero() {
		t.Errorf("want zero, got %+v", wm)
	}
}

func TestSaveLoadWatermarkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	wm := Watermark{LastSeq: 12345}
	if err := SaveWatermark(dir, wm); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadWatermark(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LastSeq != wm.LastSeq {
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
	if err := tr.Update(1, 10, ts); err != nil {
		t.Fatalf("update 10: %v", err)
	}
	if err := tr.Update(1, 5, ts); err != nil {
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
	if err := tr.Update(1, 1, time.Now().UTC()); err != nil {
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

func TestWatermarkTrackerTracksPerServer(t *testing.T) {
	tr, err := NewWatermarkTracker(t.TempDir())
	if err != nil {
		t.Fatalf("tracker: %v", err)
	}
	ts1 := time.Date(2026, 5, 11, 17, 11, 58, 0, time.UTC)
	ts2 := time.Date(2026, 5, 11, 17, 12, 8, 0, time.UTC)
	// 1v1 (server 2) publishes at 17:11:58, then ctf-ta (server 4) at 17:12:08.
	// Without per-server tracking, both would share a single LastTS, and a
	// 1v1 event at 17:12:08 would be censored by ctf-ta's advance.
	if err := tr.Update(2, 100, ts1); err != nil {
		t.Fatalf("update server 2: %v", err)
	}
	if err := tr.Update(4, 101, ts2); err != nil {
		t.Fatalf("update server 4: %v", err)
	}
	got := tr.Current()
	if got.PerServer[2].LastTS != ts1 {
		t.Errorf("PerServer[2].LastTS = %v, want %v", got.PerServer[2].LastTS, ts1)
	}
	if got.PerServer[4].LastTS != ts2 {
		t.Errorf("PerServer[4].LastTS = %v, want %v", got.PerServer[4].LastTS, ts2)
	}
	if got.LastSeq != 101 {
		t.Errorf("global LastSeq = %d, want 101", got.LastSeq)
	}
}

func TestWatermarkPerServerRoundTrips(t *testing.T) {
	dir := t.TempDir()
	wm := Watermark{
		LastSeq: 200,
		PerServer: map[int64]ServerWatermark{
			2: {LastSeq: 100, LastTS: time.Date(2026, 5, 11, 17, 11, 58, 0, time.UTC)},
			4: {LastSeq: 200, LastTS: time.Date(2026, 5, 11, 17, 12, 8, 0, time.UTC)},
		},
	}
	if err := SaveWatermark(dir, wm); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadWatermark(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.PerServer) != 2 {
		t.Fatalf("PerServer len = %d, want 2", len(got.PerServer))
	}
	if got.PerServer[2].LastSeq != 100 || !got.PerServer[2].LastTS.Equal(wm.PerServer[2].LastTS) {
		t.Errorf("server 2 round-trip mismatch: got %+v", got.PerServer[2])
	}
	if got.PerServer[4].LastSeq != 200 || !got.PerServer[4].LastTS.Equal(wm.PerServer[4].LastTS) {
		t.Errorf("server 4 round-trip mismatch: got %+v", got.PerServer[4])
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
		if err := tr.Update(1, uint64(i), time.Now().UTC()); err != nil {
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
