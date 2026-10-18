package livestream

import (
	"context"
	"io"
	"log"
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

// defaultStreamTick replaces a zero or negative tick, which would busy-spin.
const defaultStreamTick = 100 * time.Millisecond

// defaultEvictMargin is how long a segment outlives the delay window,
// covering a briefly stalled writer. Keeping segments longer is cheap;
// evicting one a viewer still needs makes that viewer skip.
const defaultEvictMargin = 30 * time.Second

// Bounds on the keyframe interval measured from KeyframeServerTime
// deltas. Shorter deltas are the engine's forced early keyframes
// (baseline resets) or degenerate streams; longer ones are stalls or
// serverTime jumps. The buffer keeps the largest plausible delta:
// forced keyframes only come early, so the maximum converges on the
// real sv_tvLiveKeyframeMsec cadence, and an overestimate would cut the
// holdback too far and cause hitching.
const (
	kfMinPlausibleMs = 250
	kfMaxPlausibleMs = 10000
	kfDefaultMs      = 1000 // sv_tvLiveKeyframeMsec default; used until measured
)

// Buffer takes in a live stream's segments and serves each viewer its
// own delayed, paced copy. It's safe for one writer (the source) and
// many readers.
//
// Segments have absolute IDs that never reset. Aged-out segments are
// evicted from the front, so segs[0] is segment base and ID id lives at
// segs[id-base], which keeps viewer cursors valid across evictions.
type Buffer struct {
	clk        clock
	target     time.Duration // target server-side viewer delay (encode + holdback)
	evictAfter time.Duration // age at which a front segment is reclaimed (target+margin)
	header     []byte

	mu           sync.Mutex
	segs         []bufSegment
	base         int // absolute ID of segs[0]; advanced as front segments are evicted
	ended        bool
	kfIntervalMs int   // measured keyframe interval (max plausible delta); 0 until observed
	lastKfTime   int32 // KeyframeServerTime of the previous appended segment
	haveLastKf   bool  // a previous segment's keyframe time has been recorded
	clampWarned  bool  // the one-shot "target below keyframe interval" warning has fired
}

// NewBuffer creates a Buffer for a stream whose header is already
// parsed. target is the viewer delay to aim for (encode latency plus
// holdback); holdback derives the per-segment wait from it.
func NewBuffer(header []byte, target time.Duration) *Buffer {
	return &Buffer{
		clk:        realClock{},
		target:     target,
		evictAfter: target + defaultEvictMargin,
		header:     header,
	}
}

// holdback returns how long each segment waits before release: the
// target minus the keyframe interval, since a segment covering game
// time [T, T+ki] is only finished at T+ki. With a target at or below
// the interval, the encode latency alone meets it, so segments go out
// as they arrive, the real delay is about one interval, and a warning
// is logged once.
func (b *Buffer) holdback() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.holdbackLocked()
}

// holdbackLocked is holdback for a caller that holds b.mu.
func (b *Buffer) holdbackLocked() time.Duration {
	kfMs := b.kfIntervalMs
	if kfMs == 0 {
		kfMs = kfDefaultMs // not yet measured; assume the engine default
	}
	ki := time.Duration(kfMs) * time.Millisecond
	if h := b.target - ki; h > 0 {
		return h
	}
	if !b.clampWarned {
		b.clampWarned = true
		log.Printf("livestream: target delay %v at or below keyframe interval %v; holdback clamped to 0, realized delay will be ~%v", b.target, ki, ki)
	}
	return 0
}

// observeKeyframeLocked updates the measured keyframe interval from a
// segment's KeyframeServerTime. The caller holds b.mu. The measurement
// starts at 0 so a real interval shorter than the default still
// registers.
func (b *Buffer) observeKeyframeLocked(kfTime int32) {
	if b.haveLastKf {
		delta := int(kfTime - b.lastKfTime)
		if delta >= kfMinPlausibleMs && delta <= kfMaxPlausibleMs && delta > b.kfIntervalMs {
			b.kfIntervalMs = delta
		}
	}
	b.lastKfTime = kfTime
	b.haveLastKf = true
}

// Append records a segment and evicts any that have aged out.
func (b *Buffer) Append(s Segment) {
	raw := s.Marshal()
	b.mu.Lock()
	now := b.clk.Now()
	b.observeKeyframeLocked(s.KeyframeServerTime)
	b.segs = append(b.segs, bufSegment{raw: raw, arrival: now})
	b.evictLocked(now)
	b.mu.Unlock()
}

// evictLocked drops aged-out front segments. The survivors are copied
// to a new slice so the evicted payloads can be collected. The caller
// holds b.mu.
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

// End marks the stream finished.
func (b *Buffer) End() {
	b.mu.Lock()
	b.ended = true
	b.mu.Unlock()
}

// latestReleasable returns the newest segment aged >= the holdback, or -1 if none qualify.
func (b *Buffer) latestReleasable(now time.Time) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	hold := b.holdbackLocked()
	for i := len(b.segs) - 1; i >= 0; i-- {
		if now.Sub(b.segs[i].arrival) >= hold {
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

// Stream writes the header, then the newest segment already past the
// holdback, then each later segment as it passes the holdback, until
// the stream ends or ctx is canceled. tick is the poll interval.
func (b *Buffer) Stream(ctx context.Context, w io.Writer, flush func(), tick time.Duration) error {
	if tick <= 0 {
		tick = defaultStreamTick
	}

	var entry int
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry = b.latestReleasable(b.clk.Now())
		if entry >= 0 {
			break
		}
		// A match shorter than the holdback ends before anything ages
		// out; start from its newest segment so the viewer sees something.
		if last, ended := b.lastIndex(); ended {
			if last >= 0 {
				entry = last
				break
			}
			return nil
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

	hold := b.holdback()
	for idx := entry + 1; ; idx++ {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			raw, arrival, exists, evicted, ended := b.segAt(idx)
			if evicted {
				// The viewer fell behind and idx was evicted. Jump to the
				// oldest buffered segment to keep the connection; every
				// segment starts with a keyframe, so playback recovers.
				if first, ok := b.firstID(); ok {
					idx = first
					continue
				}
			}
			if exists {
				if b.clk.Now().Sub(arrival) >= hold {
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
