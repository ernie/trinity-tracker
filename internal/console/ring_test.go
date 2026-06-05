package console

import (
	"fmt"
	"testing"
)

func TestRingAppendSnapshotEviction(t *testing.T) {
	r := NewRing()
	for i := 0; i < RingSize+10; i++ {
		r.Append(fmt.Sprintf("line %d", i))
	}
	snap := r.Snapshot()
	if len(snap) != RingSize {
		t.Fatalf("snapshot len = %d, want %d", len(snap), RingSize)
	}
	if snap[0].Text != "line 10" || snap[0].Seq != 11 {
		t.Errorf("oldest = %+v, want line 10 / seq 11", snap[0])
	}
	if last := snap[len(snap)-1]; last.Seq != int64(RingSize+10) {
		t.Errorf("newest seq = %d, want %d", last.Seq, RingSize+10)
	}
}

func TestRingSubscribeLiveAndUnsubscribe(t *testing.T) {
	r := NewRing()
	r.Append("old")
	snap, ch := r.Subscribe()
	if len(snap) != 1 || snap[0].Text != "old" {
		t.Fatalf("snapshot = %+v", snap)
	}
	r.Append("new")
	if got := <-ch; got.Text != "new" || got.Seq != 2 {
		t.Errorf("live line = %+v", got)
	}
	r.Unsubscribe(ch)
	if _, ok := <-ch; ok {
		t.Error("channel not closed after Unsubscribe")
	}
	r.Append("after") // must not panic with no subscribers
}

func TestRingDropsSlowSubscriber(t *testing.T) {
	r := NewRing()
	_, ch := r.Subscribe()
	for i := 0; i < subscriberBuf+5; i++ {
		r.Append("x")
	}
	// Channel was closed on overflow; drain to the close.
	n := 0
	for range ch {
		n++
	}
	if n != subscriberBuf {
		t.Errorf("drained %d, want %d", n, subscriberBuf)
	}
	r.Unsubscribe(ch) // double-release must be safe
}

func TestRegistry(t *testing.T) {
	g := NewRegistry()
	if g.Lookup("ffa") != nil {
		t.Error("Lookup before Ring should be nil")
	}
	r := g.Ring("ffa")
	if g.Ring("ffa") != r {
		t.Error("Ring not idempotent")
	}
	if g.Lookup("ffa") != r {
		t.Error("Lookup mismatch")
	}
}
