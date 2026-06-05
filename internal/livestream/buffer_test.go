// internal/livestream/buffer_test.go
package livestream

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock: Now is virtual; Sleep advances it. Single-goroutine test use.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time        { return c.now }
func (c *fakeClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

func TestBufferLatestReleasable(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 10*time.Second)
	b.clk = clk

	// Three segments arriving at t=0,3,6. Keyframe times 3s apart, so the buffer
	// measures a 3s keyframe interval ⇒ holdback = target(10s) − 3s = 7s.
	b.Append(Segment{KeyframeServerTime: 0})
	clk.now = time.Unix(3, 0)
	b.Append(Segment{KeyframeServerTime: 3000})
	clk.now = time.Unix(6, 0)
	b.Append(Segment{KeyframeServerTime: 6000})

	// At t=6 nothing is 7s old yet.
	if got := b.latestReleasable(clk.Now()); got != -1 {
		t.Fatalf("at t=6 latestReleasable = %d, want -1", got)
	}
	// At t=14 (holdback 7s): seg0 (age 14), seg1 (age 11), seg2 (age 8) are all
	// >=7s; the newest qualifying is seg2.
	clk.now = time.Unix(14, 0)
	if got := b.latestReleasable(clk.Now()); got != 2 {
		t.Fatalf("at t=14 latestReleasable = %d, want 2", got)
	}
}

func TestBufferDerivesHoldbackFromKeyframeInterval(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 10*time.Second)
	b.clk = clk

	// Before any interval is observed, the buffer assumes the engine default (1s):
	// holdback = target(10s) − 1s = 9s.
	b.Append(Segment{KeyframeServerTime: 0})
	if got := b.holdback(); got != 9*time.Second {
		t.Fatalf("unmeasured holdback = %v, want 9s (target − default 1s)", got)
	}

	// Two more segments 2s apart ⇒ measured interval 2s ⇒ holdback stays 8s.
	clk.now = time.Unix(2, 0)
	b.Append(Segment{KeyframeServerTime: 2000})
	clk.now = time.Unix(4, 0)
	b.Append(Segment{KeyframeServerTime: 4000})
	if got := b.holdback(); got != 8*time.Second {
		t.Fatalf("measured-2s holdback = %v, want 8s", got)
	}

	// A forced early keyframe (500ms after the last) must NOT shrink the measured
	// interval — max-tracking ignores deltas below the running max — so holdback
	// is unchanged.
	clk.now = time.Unix(4, 500*1e6)
	b.Append(Segment{KeyframeServerTime: 4500})
	if got := b.holdback(); got != 8*time.Second {
		t.Fatalf("after forced keyframe holdback = %v, want 8s (max-tracking ignores the short delta)", got)
	}
}

func TestBufferHoldbackClampsWhenTargetBelowInterval(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 1500*time.Millisecond) // below a 2s keyframe interval
	b.clk = clk
	b.Append(Segment{KeyframeServerTime: 0})
	clk.now = time.Unix(2, 0)
	b.Append(Segment{KeyframeServerTime: 2000})
	// target (1.5s) <= interval (2s): the encode latency alone already overshoots,
	// so holdback floors at 0 (realized delay ≈ the 2s interval).
	if got := b.holdback(); got != 0 {
		t.Fatalf("holdback = %v, want 0 (target below the keyframe interval clamps to 0)", got)
	}
}

// collectWriter records everything written.
type collectWriter struct{ bytes.Buffer }

func (w *collectWriter) flush() {}

func TestBufferStreamEntryAndPacing(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 10*time.Second)
	b.clk = clk

	// Segments arrive at t=0,3,6,9 then stream ends.
	for i, at := range []int64{0, 3, 6, 9} {
		clk.now = time.Unix(at, 0)
		b.Append(Segment{KeyframeServerTime: int32(i * 3000), Payload: []byte{byte('A' + i)}})
	}
	b.End()

	// Keyframe times are 3s apart ⇒ measured interval 3s ⇒ holdback = 10s−3s = 7s.
	// A viewer connects at t=14. Releasable (age>=7): seg0(14),seg1(11),seg2(8);
	// the newest is seg2, so entry=seg2.
	clk.now = time.Unix(14, 0)
	var out collectWriter
	err := b.Stream(context.Background(), &out, out.flush, 1*time.Second)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Expect: header, then seg2, seg3 (each released as it ages to 7s), since the
	// stream already ended and the remaining segments age out as the fake clock
	// advances via Sleep.
	want := bytes.Buffer{}
	want.Write([]byte("HDR"))
	want.Write(Segment{KeyframeServerTime: 6000, Payload: []byte("C")}.Marshal())
	want.Write(Segment{KeyframeServerTime: 9000, Payload: []byte("D")}.Marshal())

	if !bytes.Equal(out.Bytes(), want.Bytes()) {
		t.Fatalf("stream bytes mismatch:\n got %v\nwant %v", out.Bytes(), want.Bytes())
	}
}

func TestBufferFanOutTwoViewers(t *testing.T) {
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 10*time.Second)
	b.clk = clk
	for i, at := range []int64{0, 3, 6, 9} {
		clk.now = time.Unix(at, 0)
		b.Append(Segment{KeyframeServerTime: int32(i * 3000), Payload: []byte{byte('A' + i)}})
	}
	b.End()

	// Two viewers connect at the same logical time (t=14). Run sequentially
	// against the ended buffer; reset the clock before each so the entry
	// selection is reproducible. Assert both produce identical, header-led output.
	var v1, v2 collectWriter
	clk.now = time.Unix(14, 0)
	if err := b.Stream(context.Background(), &v1, v1.flush, time.Second); err != nil {
		t.Fatalf("v1: %v", err)
	}
	clk.now = time.Unix(14, 0)
	if err := b.Stream(context.Background(), &v2, v2.flush, time.Second); err != nil {
		t.Fatalf("v2: %v", err)
	}
	if !bytes.Equal(v1.Bytes(), v2.Bytes()) {
		t.Fatalf("viewers diverged:\n v1 %v\n v2 %v", v1.Bytes(), v2.Bytes())
	}
	if !bytes.HasPrefix(v1.Bytes(), []byte("HDR")) {
		t.Fatalf("v1 missing header prefix")
	}
}

func TestBufferConcurrentAppendAndStream(t *testing.T) {
	// Real clock, zero delay so every segment is immediately releasable,
	// tiny tick so Stream polls quickly. One goroutine appends while another
	// streams — this is what exercises the Append/Stream lock under -race.
	b := NewBuffer([]byte("HDR"), 0)

	type safeBuf struct {
		mu  sync.Mutex
		buf bytes.Buffer
	}
	out := &safeBuf{}
	w := writerFunc(func(p []byte) (int, error) {
		out.mu.Lock()
		defer out.mu.Unlock()
		return out.buf.Write(p)
	})

	done := make(chan error, 1)
	go func() {
		done <- b.Stream(context.Background(), w, func() {}, time.Millisecond)
	}()

	for i := 0; i < 20; i++ {
		b.Append(Segment{KeyframeServerTime: int32(i), Payload: []byte{byte(i)}})
		time.Sleep(time.Millisecond)
	}
	b.End()

	if err := <-done; err != nil {
		t.Fatalf("Stream: %v", err)
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	if !bytes.HasPrefix(out.buf.Bytes(), []byte("HDR")) {
		t.Fatalf("output missing header prefix")
	}
	if out.buf.Len() <= len("HDR") {
		t.Fatalf("expected at least one segment after header, got %d bytes", out.buf.Len())
	}
}

func TestBufferEvictsAgedSegments(t *testing.T) {
	// A long-running match: 1000 segments at one per second, delay 5s. Nothing
	// older than the delay window (plus a small eviction margin) is reachable —
	// a new viewer always enters at latestReleasable and only moves forward — so
	// the retained set must stay bounded instead of growing for the whole match.
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 5*time.Second)
	b.clk = clk
	for i := 0; i < 1000; i++ {
		clk.now = time.Unix(int64(i), 0)
		b.Append(Segment{KeyframeServerTime: int32(i)})
	}
	b.mu.Lock()
	n := len(b.segs)
	b.mu.Unlock()
	// A 5s delay window needs at most a few dozen seconds of segments; retaining
	// all 1000 means Append never trims (the unbounded-growth bug).
	if n > 60 {
		t.Fatalf("retained %d segments over a 1000s stream; eviction never trims (unbounded growth)", n)
	}
}

func TestBufferIndexTranslationAcrossEviction(t *testing.T) {
	// After eviction, absolute IDs must still resolve to the right segments and
	// latestReleasable must report an absolute ID, not a rebased slice index.
	clk := &fakeClock{now: time.Unix(0, 0)}
	b := NewBuffer([]byte("HDR"), 5*time.Second)
	b.clk = clk
	b.evictAfter = 8 * time.Second // small window so eviction kicks in quickly
	for i := 0; i < 20; i++ {
		clk.now = time.Unix(int64(i), 0)
		b.Append(Segment{KeyframeServerTime: int32(i * 1000), Payload: []byte{byte('a' + i)}})
	}
	// now=19, evictAfter=8 → segments with arrival <= 11 (age >= 8) are gone;
	// base advances to 12.
	b.mu.Lock()
	base := b.base
	b.mu.Unlock()
	if base == 0 {
		t.Fatal("eviction never ran; base did not advance")
	}

	// An evicted ID reports evicted, not exists.
	if _, _, exists, evicted, _ := b.segAt(0); exists || !evicted {
		t.Fatalf("segAt(0): exists=%v evicted=%v, want exists=false evicted=true", exists, evicted)
	}

	// A live absolute ID resolves to the correct segment (payload byte 'a'+id).
	id := base + 1
	raw, _, exists, evicted, _ := b.segAt(id)
	if !exists || evicted {
		t.Fatalf("segAt(%d): exists=%v evicted=%v, want a live segment", id, exists, evicted)
	}
	want := Segment{KeyframeServerTime: int32(id * 1000), Payload: []byte{byte('a' + id)}}.Marshal()
	if !bytes.Equal(raw, want) {
		t.Fatalf("segAt(%d) returned the wrong segment after rebase:\n got %v\nwant %v", id, raw, want)
	}

	// Keyframe times are 1s apart ⇒ measured interval 1s ⇒ holdback = 5s−1s = 4s.
	// latestReleasable returns an absolute ID: newest with age >= 4s (arrival
	// <= 15) is segment 15.
	if lr := b.latestReleasable(clk.Now()); lr != 15 {
		t.Fatalf("latestReleasable = %d, want 15 (absolute ID, not slice index)", lr)
	}
}

func TestBufferStreamServesContiguouslyAcrossEviction(t *testing.T) {
	// A keeping-up viewer must receive a contiguous, in-order run of segments
	// even while old ones are being front-evicted concurrently. Real clock,
	// zero delay (every segment immediately releasable), tiny eviction window.
	b := NewBuffer([]byte("HDR"), 0)
	b.evictAfter = 20 * time.Millisecond

	var mu sync.Mutex
	var got []byte // collected segment payload markers, in receipt order
	header := true
	w := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		if header {
			header = false // first write is the header
		} else {
			// Each segment marshals with its single-byte payload last.
			got = append(got, p[len(p)-1])
		}
		return len(p), nil
	})

	done := make(chan error, 1)
	go func() { done <- b.Stream(context.Background(), w, func() {}, time.Millisecond) }()

	for i := 0; i < 60; i++ {
		b.Append(Segment{KeyframeServerTime: int32(i), Payload: []byte{byte(i)}})
		time.Sleep(2 * time.Millisecond)
	}
	b.End()
	if err := <-done; err != nil {
		t.Fatalf("Stream: %v", err)
	}

	b.mu.Lock()
	base := b.base
	b.mu.Unlock()
	if base == 0 {
		t.Fatal("eviction never ran during the stream; test exercised nothing")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) < 20 {
		t.Fatalf("viewer received only %d segments; expected a keeping-up viewer to get most of 60", len(got))
	}
	// Contiguity: a keeping-up viewer must not skip a segment, eviction or not.
	for i := 1; i < len(got); i++ {
		if got[i] != got[i-1]+1 {
			t.Fatalf("non-contiguous receipt at %d: got[%d]=%d after %d (eviction caused a skip)", i, i, got[i], got[i-1])
		}
	}
}

// writerFunc adapts a function to io.Writer.
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
