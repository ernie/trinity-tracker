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

// Watermark is the publisher's persisted progress marker.
//
// LastSeq is the global monotonic sequence number for this source's
// publisher — used to resume seq numbering across restart.
//
// PerServer records (LastSeq, LastTS) per RemoteServerID. The
// collector's manager uses these as per-tailer replay cutoffs so an
// event on server A at second T can't censor an unprocessed
// same-second event on server B during next-boot recovery.
type Watermark struct {
	LastSeq   uint64                    `json:"last_seq"`
	PerServer map[int64]ServerWatermark `json:"per_server,omitempty"`
}

// ServerWatermark holds the (seq, ts) of the last event published
// for a single RemoteServerID.
type ServerWatermark struct {
	LastSeq uint64    `json:"last_seq"`
	LastTS  time.Time `json:"last_ts"`
}

// IsZero reports whether the watermark holds no state. Used in place
// of `wm == (Watermark{})` since the struct contains a map and is
// no longer directly comparable.
func (w Watermark) IsZero() bool {
	return w.LastSeq == 0 && len(w.PerServer) == 0
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

// Update records a monotonically-increasing Seq/TS pair for one
// RemoteServerID; older seqs are ignored (the global seq guard
// suffices since publisher seqs are monotonic across all servers).
// serverID==0 is treated as "no per-server bucket" — the global
// LastSeq/LastTS still advance, so non-server-scoped events still
// participate in seq continuity.
func (t *WatermarkTracker) Update(serverID int64, seq uint64, ts time.Time) error {
	t.mu.Lock()
	if seq <= t.current.LastSeq {
		t.mu.Unlock()
		return nil
	}
	if serverID != 0 {
		if t.current.PerServer == nil {
			t.current.PerServer = make(map[int64]ServerWatermark)
		}
		t.current.PerServer[serverID] = ServerWatermark{LastSeq: seq, LastTS: ts.UTC()}
	}
	t.current.LastSeq = seq
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
	if wm.IsZero() {
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
