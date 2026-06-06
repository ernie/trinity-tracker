package collector

import (
	"sync"
	"testing"
	"time"
)

// watchHarness drives a WatchNotifier with a controllable count and records
// sends. Delays are shrunk so the timer-driven transitions run in
// milliseconds.
type watchHarness struct {
	mu    sync.Mutex
	count int
	sends chan int
	w     *WatchNotifier
}

func newWatchHarness() *watchHarness {
	h := &watchHarness{sends: make(chan int, 16)}
	h.w = NewWatchNotifier(
		func(string) int {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.count
		},
		func(_ string, count int) { h.sends <- count },
	)
	h.w.confirmDelay = 20 * time.Millisecond
	h.w.graceDelay = 40 * time.Millisecond
	h.w.coalesceDelay = 20 * time.Millisecond
	h.w.repushDelay = 10 * time.Millisecond
	return h
}

// set updates the harness count and fires the hook, like retain/release do.
func (h *watchHarness) set(n int) {
	h.mu.Lock()
	h.count = n
	h.mu.Unlock()
	h.w.ViewersChanged("ffa", n)
}

func (h *watchHarness) expectSend(t *testing.T, want int) {
	t.Helper()
	select {
	case got := <-h.sends:
		if got != want {
			t.Fatalf("send = %d, want %d", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no send within deadline, want %d", want)
	}
}

func (h *watchHarness) expectQuiet(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case got := <-h.sends:
		t.Fatalf("unexpected send %d", got)
	case <-time.After(d):
	}
}

func TestWatchConfirmThenSend(t *testing.T) {
	h := newWatchHarness()
	h.set(1)
	h.expectSend(t, 1) // after confirm delay
}

func TestWatchDriveByIsFiltered(t *testing.T) {
	h := newWatchHarness()
	h.set(1)
	h.set(0) // gone before confirm expires
	h.expectQuiet(t, 60*time.Millisecond)
}

func TestWatchGraceAbsorbsReload(t *testing.T) {
	h := newWatchHarness()
	h.set(1)
	h.expectSend(t, 1)
	h.set(0) // page reload...
	h.set(1) // ...and back within grace
	h.expectQuiet(t, 80*time.Millisecond)
}

func TestWatchDepartAfterGrace(t *testing.T) {
	h := newWatchHarness()
	h.set(1)
	h.expectSend(t, 1)
	h.set(0)
	h.expectSend(t, 0) // after grace delay
}

func TestWatchCountUpdateCoalesced(t *testing.T) {
	h := newWatchHarness()
	h.set(1)
	h.expectSend(t, 1)
	h.set(2)
	h.set(3) // second change while coalesce pending: one send, latest count
	h.expectSend(t, 3)
	h.expectQuiet(t, 60*time.Millisecond)
}

func TestWatchSessionRepush(t *testing.T) {
	h := newWatchHarness()
	h.set(2)
	h.expectSend(t, 2)
	h.w.SessionStart("ffa") // map change: fresh level needs the count again
	h.expectSend(t, 2)
}

func TestWatchSessionRepushOnlyWhileWatched(t *testing.T) {
	h := newWatchHarness()
	h.w.SessionStart("ffa")
	h.expectQuiet(t, 40*time.Millisecond)
}
