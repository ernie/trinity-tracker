package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LocalWatermarkFilename is the on-disk location of the standalone
// writer's progress marker, under the database's directory.
const LocalWatermarkFilename = "publish_watermark.json"

// Flush batching — mirrors natsbus.WatermarkFlush* so the standalone
// mode feels similar on restart.
const (
	localWatermarkFlushInterval = 250 * time.Millisecond
	localWatermarkFlushEvery    = 50
)

// LocalWatermark is the minimal persisted shape: just the last
// dispatched event timestamp. Standalone mode has no seq counter
// (nothing is published to NATS) so ts is all we need to decide where
// log replay starts on next boot.
type LocalWatermark struct {
	LastTS time.Time `json:"last_ts"`
}

// LocalWatermarkTracker hooks Writer's dispatch loop. On each dispatched
// event the writer calls Observe with the event's timestamp; the tracker
// batches writes to disk on the same cadence as the NATS-side tracker.
type LocalWatermarkTracker struct {
	dir string

	mu           sync.Mutex
	current      LocalWatermark
	updatesSince int
	lastFlush    time.Time
}

// LoadLocalWatermark reads the watermark from dir. Missing file yields
// a zero-value watermark — on a fresh run the caller should treat log
// history as already-replayed (publish from "now" onward).
func LoadLocalWatermark(dir string) (LocalWatermark, error) {
	path := filepath.Join(dir, LocalWatermarkFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LocalWatermark{}, nil
		}
		return LocalWatermark{}, fmt.Errorf("hub: reading %s: %w", path, err)
	}
	var wm LocalWatermark
	if err := json.Unmarshal(data, &wm); err != nil {
		return LocalWatermark{}, fmt.Errorf("hub: parsing %s: %w", path, err)
	}
	return wm, nil
}

// SaveLocalWatermark atomically writes the watermark via tmp + rename.
func SaveLocalWatermark(dir string, wm LocalWatermark) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("hub: MkdirAll %s: %w", dir, err)
	}
	path := filepath.Join(dir, LocalWatermarkFilename)
	tmp := path + ".tmp"
	body, err := json.Marshal(wm)
	if err != nil {
		return fmt.Errorf("hub: marshal watermark: %w", err)
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("hub: open %s: %w", tmp, err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return fmt.Errorf("hub: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("hub: fsync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("hub: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("hub: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// NewLocalWatermarkTracker seeds from disk so callers can query the
// resumed position immediately.
func NewLocalWatermarkTracker(dir string) (*LocalWatermarkTracker, error) {
	wm, err := LoadLocalWatermark(dir)
	if err != nil {
		return nil, err
	}
	return &LocalWatermarkTracker{
		dir:       dir,
		current:   wm,
		lastFlush: time.Now(),
	}, nil
}

// Current returns the latest in-memory watermark.
func (t *LocalWatermarkTracker) Current() LocalWatermark {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.current
}

// Observe records a dispatched event's timestamp. Monotonic in ts —
// out-of-order events (shouldn't happen per-source, but possible if
// multiple servers' logs interleave) do not regress the watermark.
func (t *LocalWatermarkTracker) Observe(ts time.Time) error {
	ts = ts.UTC()
	t.mu.Lock()
	if !ts.After(t.current.LastTS) {
		t.mu.Unlock()
		return nil
	}
	t.current = LocalWatermark{LastTS: ts}
	t.updatesSince++
	now := time.Now()
	shouldFlush := t.updatesSince >= localWatermarkFlushEvery || now.Sub(t.lastFlush) >= localWatermarkFlushInterval
	var toSave LocalWatermark
	if shouldFlush {
		toSave = t.current
		t.updatesSince = 0
		t.lastFlush = now
	}
	t.mu.Unlock()

	if shouldFlush {
		return SaveLocalWatermark(t.dir, toSave)
	}
	return nil
}

// Flush forces an immediate disk write. Call on graceful shutdown.
func (t *LocalWatermarkTracker) Flush() error {
	t.mu.Lock()
	wm := t.current
	t.mu.Unlock()
	if wm.LastTS.IsZero() {
		return nil
	}
	if err := SaveLocalWatermark(t.dir, wm); err != nil {
		return err
	}
	t.mu.Lock()
	t.updatesSince = 0
	t.lastFlush = time.Now()
	t.mu.Unlock()
	return nil
}
