package natsbus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernie/trinity-tracker/internal/domain"
)

// SpillFilename is the on-disk queue of fact events that couldn't be
// absorbed by the in-memory ring.
const SpillFilename = "buffer.jsonl"

// SpillHeadFilename tracks the byte offset of the next unread entry in
// SpillFilename, so we don't have to rewrite the queue on every drain.
const SpillHeadFilename = "buffer.head.json"

// SpillCapBytes caps the live size of SpillFilename (tail - head). When
// a write would push past the cap, the queue compacts (rewrites from
// head onward) and, if still over cap, drops oldest entries until
// under cap — each dropped entry increments Dropped. Declared as a
// var (not const) so tests can shrink it via spillCapOverride.
var SpillCapBytes int64 = 100 * 1024 * 1024

// spillCapOverride is test-only: when non-zero it supersedes
// SpillCapBytes for the current process. Production code uses the
// exported cap directly via the effectiveCap helper.
var spillCapOverride int64

func effectiveCap() int64 {
	if spillCapOverride > 0 {
		return spillCapOverride
	}
	return SpillCapBytes
}

// SpillQueue is an append-only JSONL queue of fact events that
// survives NATS outages longer than the in-memory ring can buffer.
// Read-position is tracked in a sibling head file; we rewrite the main
// file only on overflow compaction.
type SpillQueue struct {
	dir string

	mu     sync.Mutex
	file   *os.File // open for append + random read
	tail   int64    // byte offset of end-of-file
	head   int64    // byte offset of next-unread line
	closed bool

	dropped atomic.Uint64
}

type spillHead struct {
	Offset int64 `json:"offset"`
}

// NewSpillQueue opens (creating if needed) the on-disk queue under
// dir. If either the data file or the head file already exists, the
// queue resumes from the persisted offsets.
func NewSpillQueue(dir string) (*SpillQueue, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("natsbus.SpillQueue: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, SpillFilename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("natsbus.SpillQueue: open %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("natsbus.SpillQueue: stat %s: %w", path, err)
	}
	q := &SpillQueue{
		dir:  dir,
		file: f,
		tail: info.Size(),
	}
	// Load persisted head offset; silently default to 0 on first-run
	// or parse failure.
	if head, err := loadSpillHead(filepath.Join(dir, SpillHeadFilename)); err == nil {
		if head.Offset > q.tail {
			// File was truncated externally; reset to start.
			q.head = 0
		} else {
			q.head = head.Offset
		}
	}
	return q, nil
}

// IsEmpty reports whether there is no unread data remaining.
func (q *SpillQueue) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.head >= q.tail
}

// Size returns the live on-disk byte usage (tail - head).
func (q *SpillQueue) Size() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.tail - q.head
}

// Dropped returns the count of events lost to rotation (cap exceeded).
// Ring-to-disk spill is not counted — that's preservation, not loss.
func (q *SpillQueue) Dropped() uint64 { return q.dropped.Load() }

// Append writes one event to the end of the queue. If the resulting
// live size would exceed SpillCapBytes, the queue compacts first; if
// still over cap after compaction, oldest entries are discarded and
// Dropped is bumped per discarded entry.
func (q *SpillQueue) Append(e domain.FactEvent) error {
	payload, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("natsbus.SpillQueue: marshal %s payload: %w", e.Type, err)
	}
	line, err := json.Marshal(spillRecord{
		Type:      e.Type,
		ServerID:  e.ServerID,
		Timestamp: e.Timestamp,
		Data:      payload,
	})
	if err != nil {
		return fmt.Errorf("natsbus.SpillQueue: marshal %s: %w", e.Type, err)
	}
	line = append(line, '\n')

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return fmt.Errorf("natsbus.SpillQueue: closed")
	}
	if err := q.ensureRoomLocked(int64(len(line))); err != nil {
		return err
	}
	if _, err := q.file.WriteAt(line, q.tail); err != nil {
		return fmt.Errorf("natsbus.SpillQueue: write: %w", err)
	}
	q.tail += int64(len(line))
	return nil
}

// Peek returns the next-unread event without advancing. ok=false when
// the queue is empty.
func (q *SpillQueue) Peek() (domain.FactEvent, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.head >= q.tail {
		return domain.FactEvent{}, false, nil
	}
	return q.readAtLocked(q.head)
}

// Advance moves the read head past the previously-peeked entry and
// persists the new head offset.
func (q *SpillQueue) Advance() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.head >= q.tail {
		return nil
	}
	// Recompute line length: scan forward from head to the next '\n'.
	offset, err := q.scanNewlineLocked(q.head)
	if err != nil {
		return err
	}
	q.head = offset
	// If we've drained everything, reset head + truncate.
	if q.head >= q.tail {
		if err := q.resetLocked(); err != nil {
			return err
		}
	}
	return q.persistHeadLocked()
}

// Close flushes the head marker and releases the file handle.
func (q *SpillQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	q.closed = true
	if err := q.persistHeadLocked(); err != nil {
		_ = q.file.Close()
		return err
	}
	return q.file.Close()
}

// readAtLocked decodes one line starting at offset. Caller holds q.mu.
func (q *SpillQueue) readAtLocked(offset int64) (domain.FactEvent, bool, error) {
	line, err := q.readLineLocked(offset)
	if err != nil {
		return domain.FactEvent{}, false, err
	}
	var rec spillRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return domain.FactEvent{}, false, fmt.Errorf("natsbus.SpillQueue: decode line at %d: %w", offset, err)
	}
	return domain.FactEvent{
		Type:      rec.Type,
		ServerID:  rec.ServerID,
		Timestamp: rec.Timestamp,
		Data:      rec.Data,
	}, true, nil
}

// readLineLocked returns the JSON bytes (no trailing newline) of the
// line starting at offset.
func (q *SpillQueue) readLineLocked(offset int64) ([]byte, error) {
	if _, err := q.file.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("natsbus.SpillQueue: seek: %w", err)
	}
	reader := bufio.NewReader(q.file)
	line, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("natsbus.SpillQueue: read: %w", err)
	}
	if n := len(line); n > 0 && line[n-1] == '\n' {
		line = line[:n-1]
	}
	return line, nil
}

// scanNewlineLocked returns the byte offset immediately after the
// newline terminating the line that starts at offset.
func (q *SpillQueue) scanNewlineLocked(offset int64) (int64, error) {
	if _, err := q.file.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("natsbus.SpillQueue: seek: %w", err)
	}
	reader := bufio.NewReader(q.file)
	line, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return 0, fmt.Errorf("natsbus.SpillQueue: read: %w", err)
	}
	return offset + int64(len(line)), nil
}

// ensureRoomLocked compacts + truncates so appending addBytes stays
// under SpillCapBytes. Caller holds q.mu.
func (q *SpillQueue) ensureRoomLocked(addBytes int64) error {
	cap := effectiveCap()
	if q.tail-q.head+addBytes <= cap {
		return nil
	}
	if err := q.compactLocked(); err != nil {
		return err
	}
	for q.tail+addBytes > cap && q.head < q.tail {
		next, err := q.scanNewlineLocked(q.head)
		if err != nil {
			return err
		}
		q.head = next
		q.dropped.Add(1)
		if err := q.compactLocked(); err != nil {
			return err
		}
	}
	return nil
}

// compactLocked rewrites the file from head onward, resetting head to
// 0 and tail to the new size. No-op when head == 0.
func (q *SpillQueue) compactLocked() error {
	if q.head == 0 {
		return nil
	}
	if q.head >= q.tail {
		return q.resetLocked()
	}
	// Read live slice into memory, rewrite the file, reset offsets.
	live := q.tail - q.head
	buf := make([]byte, live)
	if _, err := q.file.ReadAt(buf, q.head); err != nil {
		return fmt.Errorf("natsbus.SpillQueue: compact read: %w", err)
	}
	if err := q.file.Truncate(0); err != nil {
		return fmt.Errorf("natsbus.SpillQueue: compact truncate: %w", err)
	}
	if _, err := q.file.WriteAt(buf, 0); err != nil {
		return fmt.Errorf("natsbus.SpillQueue: compact write: %w", err)
	}
	q.head = 0
	q.tail = live
	return q.persistHeadLocked()
}

func (q *SpillQueue) resetLocked() error {
	if err := q.file.Truncate(0); err != nil {
		return fmt.Errorf("natsbus.SpillQueue: reset truncate: %w", err)
	}
	q.head = 0
	q.tail = 0
	return nil
}

func (q *SpillQueue) persistHeadLocked() error {
	return saveSpillHead(filepath.Join(q.dir, SpillHeadFilename), spillHead{Offset: q.head})
}

// spillRecord is the on-disk shape for a FactEvent. Data is held as
// the already-marshaled JSON bytes so SpillQueue stays agnostic to
// the concrete FactEvent payload types — the drain path re-decodes
// into the right type via decodeEventPayload when the publisher
// needs a typed value.
type spillRecord struct {
	Type      string          `json:"type"`
	ServerID  int64           `json:"server_id"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

func loadSpillHead(path string) (spillHead, error) {
	var h spillHead
	data, err := os.ReadFile(path)
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return h, err
	}
	return h, nil
}

func saveSpillHead(path string, h spillHead) error {
	body, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("natsbus.SpillQueue: marshal head: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("natsbus.SpillQueue: write head: %w", err)
	}
	return os.Rename(tmp, path)
}
