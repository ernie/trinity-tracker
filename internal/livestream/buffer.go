// internal/livestream/buffer.go
package livestream

import (
	"context"
	"io"
	"sync"
	"time"
)

// clock abstracts time so the relay's delay pacing is deterministic in tests.
type clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

type bufSegment struct {
	raw     []byte    // full marshaled segment bytes, forwarded verbatim
	arrival time.Time // wall-clock arrival; drives the delay
}

// defaultStreamTick guards against a busy-spin on a zero/negative tick.
const defaultStreamTick = 100 * time.Millisecond

// defaultEvictMargin is slack beyond the delay window before front-eviction,
// covering a briefly-stalled writer. Generous: over-retention is cheap, but
// evicting a still-needed segment costs a viewer a skip.
const defaultEvictMargin = 30 * time.Second

// Buffer ingests a live stream's segments and serves each viewer a delayed,
// paced, fanned-out copy. Safe for one writer (the source) and many readers.
//
// Segments are addressed by a monotonic absolute ID that never resets for the
// life of the buffer. The backing slice is front-evicted once a segment ages
// out (see evictLocked), so segs[0] is segment `base`, and absolute ID `id`
// lives at segs[id-base]. Indexing through base keeps viewer cursors stable
// across evictions.
type Buffer struct {
	clk        clock
	delay      time.Duration
	evictAfter time.Duration // age at which a front segment is reclaimed (delay+margin)
	header     []byte

	mu    sync.Mutex
	segs  []bufSegment
	base  int // absolute ID of segs[0]; advanced as front segments are evicted
	ended bool
}

// NewBuffer creates a Buffer for a stream with an already-parsed header.
func NewBuffer(header []byte, delay time.Duration) *Buffer {
	return &Buffer{clk: realClock{}, delay: delay, evictAfter: delay + defaultEvictMargin, header: header}
}

// Append records a segment and reclaims any that have aged out. Implements segmentSink.
func (b *Buffer) Append(s Segment) {
	raw := s.Marshal()
	b.mu.Lock()
	now := b.clk.Now()
	b.segs = append(b.segs, bufSegment{raw: raw, arrival: now})
	b.evictLocked(now)
	b.mu.Unlock()
}

// evictLocked drops aged-out front segments. Survivors are recopied so evicted
// payloads become GC-able. Caller holds b.mu.
func (b *Buffer) evictLocked(now time.Time) {
	n := 0
	for n < len(b.segs) && now.Sub(b.segs[n].arrival) >= b.evictAfter {
		n++
	}
	if n == 0 {
		return
	}
	b.segs = append([]bufSegment(nil), b.segs[n:]...)
	b.base += n
}

// End marks the stream finished. Implements segmentSink.
func (b *Buffer) End() {
	b.mu.Lock()
	b.ended = true
	b.mu.Unlock()
}

// latestReleasable returns the newest segment aged >= delay, or -1 if none qualify.
func (b *Buffer) latestReleasable(now time.Time) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.segs) - 1; i >= 0; i-- {
		if now.Sub(b.segs[i].arrival) >= b.delay {
			return b.base + i
		}
	}
	return -1
}

// segAt returns segment id's bytes plus existence / evicted (id < base) / ended flags.
func (b *Buffer) segAt(id int) (raw []byte, arrival time.Time, exists, evicted, ended bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	i := id - b.base
	if i < 0 {
		return nil, time.Time{}, false, true, b.ended
	}
	if i < len(b.segs) {
		s := b.segs[i]
		return s.raw, s.arrival, true, false, b.ended
	}
	return nil, time.Time{}, false, false, b.ended
}

// firstID returns the oldest buffered absolute ID, or (0,false) if empty.
func (b *Buffer) firstID() (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.segs) == 0 {
		return 0, false
	}
	return b.base, true
}

// lastIndex returns the highest absolute ID (or -1) and whether the stream ended.
func (b *Buffer) lastIndex() (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.segs) == 0 {
		return -1, b.ended
	}
	return b.base + len(b.segs) - 1, b.ended
}

// Stream writes the header, the entry segment (newest one already `delay` old),
// then each subsequent segment as it ages past `delay`. Returns at end-of-stream
// or ctx cancel. tick is the poll interval.
func (b *Buffer) Stream(ctx context.Context, w io.Writer, flush func(), tick time.Duration) error {
	if tick <= 0 {
		tick = defaultStreamTick
	}

	// 1. Wait for an entry point (newest releasable segment).
	var entry int
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry = b.latestReleasable(b.clk.Now())
		if entry >= 0 {
			break
		}
		// If the stream ended before anything aged out (match shorter than the
		// delay), fall back to the newest segment so the viewer sees something.
		if last, ended := b.lastIndex(); ended {
			if last >= 0 {
				entry = last
				break
			}
			return nil // ended with zero segments
		}
		b.clk.Sleep(tick)
	}

	if _, err := w.Write(b.header); err != nil {
		return err
	}
	raw, _, _, _, _ := b.segAt(entry)
	if _, err := w.Write(raw); err != nil {
		return err
	}
	flush()

	// 2. Release subsequent segments as they age past the delay.
	for idx := entry + 1; ; idx++ {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			raw, arrival, exists, evicted, ended := b.segAt(idx)
			if evicted {
				// This viewer fell far enough behind that segment idx was
				// reclaimed before it could be sent. Skip forward to the oldest
				// still-buffered segment so the connection survives; each
				// segment is independently keyframed, so the gap self-heals on
				// the next decode.
				if first, ok := b.firstID(); ok {
					idx = first
					continue
				}
				// Nothing buffered yet — fall through and wait/drain below.
			}
			if exists {
				if b.clk.Now().Sub(arrival) >= b.delay {
					if _, err := w.Write(raw); err != nil {
						return err
					}
					flush()
					break // advance to idx+1
				}
				b.clk.Sleep(tick)
				continue
			}
			if ended {
				return nil // drained
			}
			b.clk.Sleep(tick) // wait for more segments
		}
	}
}
