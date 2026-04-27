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

const SpillFilename = "buffer.jsonl"
const SpillHeadFilename = "buffer.head.json"

// SpillCapBytes caps the live size (tail - head). Overflow compacts and
// then drops oldest entries (incrementing Dropped) until under cap. Var
// rather than const so tests can shrink via spillCapOverride.
var SpillCapBytes int64 = 100 * 1024 * 1024

var spillCapOverride int64 // test-only; takes precedence when non-zero

func effectiveCap() int64 {
	if spillCapOverride > 0 {
		return spillCapOverride
	}
	return SpillCapBytes
}

// SpillQueue is an append-only JSONL queue for fact events during NATS
// outages. Read position lives in a sibling head file; the main file
// is rewritten only on overflow compaction.
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

// NewSpillQueue opens or creates the on-disk queue under dir, resuming
// from any persisted offsets.
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
	if head, err := loadSpillHead(filepath.Join(dir, SpillHeadFilename)); err == nil {
		if head.Offset > q.tail {
			q.head = 0 // data file was truncated externally
		} else {
			q.head = head.Offset
		}
	}
	return q, nil
}

func (q *SpillQueue) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.head >= q.tail
}

func (q *SpillQueue) Size() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.tail - q.head
}

// Dropped counts events discarded due to cap overflow (not ring-to-disk spill).
func (q *SpillQueue) Dropped() uint64 { return q.dropped.Load() }

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

// Peek returns the next-unread event without advancing.
func (q *SpillQueue) Peek() (domain.FactEvent, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.head >= q.tail {
		return domain.FactEvent{}, false, nil
	}
	return q.readAtLocked(q.head)
}

// Advance moves the read head past the previously-peeked entry.
func (q *SpillQueue) Advance() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.head >= q.tail {
		return nil
	}
	offset, err := q.scanNewlineLocked(q.head)
	if err != nil {
		return err
	}
	q.head = offset
	if q.head >= q.tail {
		if err := q.resetLocked(); err != nil {
			return err
		}
	}
	return q.persistHeadLocked()
}

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

// compactLocked rewrites the file from head onward and resets offsets.
func (q *SpillQueue) compactLocked() error {
	if q.head == 0 {
		return nil
	}
	if q.head >= q.tail {
		return q.resetLocked()
	}
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

// spillRecord is the on-disk shape. Data stays as raw JSON so the
// queue doesn't need to know concrete payload types.
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
