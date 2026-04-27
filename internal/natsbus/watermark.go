package natsbus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const WatermarkFilename = "publish_watermark.json"

const (
	WatermarkFlushInterval = 250 * time.Millisecond
	WatermarkFlushEvery    = 50
)

// Watermark is the publisher's persisted progress marker. On restart
// the collector replays logs up to LastTS silently and resumes at
// LastSeq+1.
type Watermark struct {
	LastSeq uint64    `json:"last_seq"`
	LastTS  time.Time `json:"last_ts"`
}

// LoadWatermark returns the stored watermark or a zero value on
// missing file (first run).
func LoadWatermark(dataDir string) (Watermark, error) {
	path := filepath.Join(dataDir, WatermarkFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Watermark{}, nil
		}
		return Watermark{}, fmt.Errorf("natsbus: reading %s: %w", path, err)
	}
	var wm Watermark
	if err := json.Unmarshal(data, &wm); err != nil {
		return Watermark{}, fmt.Errorf("natsbus: parsing %s: %w", path, err)
	}
	return wm, nil
}

// SaveWatermark atomically writes via a .tmp sibling and rename.
func SaveWatermark(dataDir string, wm Watermark) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("natsbus: MkdirAll %s: %w", dataDir, err)
	}
	path := filepath.Join(dataDir, WatermarkFilename)
	tmp := path + ".tmp"
	body, err := json.Marshal(wm)
	if err != nil {
		return fmt.Errorf("natsbus: marshal watermark: %w", err)
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("natsbus: open %s: %w", tmp, err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return fmt.Errorf("natsbus: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("natsbus: fsync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("natsbus: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("natsbus: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// WatermarkTracker batches watermark writes to disk on a flush cadence.
type WatermarkTracker struct {
	dataDir string

	mu          sync.Mutex
	current     Watermark
	lastSaved   Watermark
	updatesSince int
	lastFlush    time.Time
}

func NewWatermarkTracker(dataDir string) (*WatermarkTracker, error) {
	wm, err := LoadWatermark(dataDir)
	if err != nil {
		return nil, err
	}
	return &WatermarkTracker{
		dataDir:   dataDir,
		current:   wm,
		lastSaved: wm,
		lastFlush: time.Now(),
	}, nil
}

// Current returns the latest in-memory watermark (may lead the on-disk copy).
func (t *WatermarkTracker) Current() Watermark {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.current
}

// Update records a monotonically-increasing Seq/TS pair; older seqs are ignored.
func (t *WatermarkTracker) Update(seq uint64, ts time.Time) error {
	t.mu.Lock()
	if seq <= t.current.LastSeq {
		t.mu.Unlock()
		return nil
	}
	t.current = Watermark{LastSeq: seq, LastTS: ts.UTC()}
	t.updatesSince++
	now := time.Now()
	shouldFlush := t.updatesSince >= WatermarkFlushEvery || now.Sub(t.lastFlush) >= WatermarkFlushInterval
	var toSave Watermark
	if shouldFlush {
		toSave = t.current
		t.updatesSince = 0
		t.lastFlush = now
	}
	t.mu.Unlock()

	if shouldFlush {
		if err := SaveWatermark(t.dataDir, toSave); err != nil {
			return err
		}
		t.mu.Lock()
		t.lastSaved = toSave
		t.mu.Unlock()
	}
	return nil
}

// Flush forces an immediate disk write. Call on graceful shutdown.
func (t *WatermarkTracker) Flush() error {
	t.mu.Lock()
	wm := t.current
	t.mu.Unlock()
	if wm == (Watermark{}) {
		return nil
	}
	if err := SaveWatermark(t.dataDir, wm); err != nil {
		return err
	}
	t.mu.Lock()
	t.lastSaved = wm
	t.updatesSince = 0
	t.lastFlush = time.Now()
	t.mu.Unlock()
	return nil
}
