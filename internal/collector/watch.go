package collector

import (
	"sync"
	"time"
)

// Watch debounce constants. First guesses per the web-viewer-presence spec;
// tune after watching real traffic.
const (
	// watchConfirmDelay filters drive-by page loads: a viewer must persist
	// this long before the arena is told it's watched.
	watchConfirmDelay = 10 * time.Second
	// watchGraceDelay covers the live player's reconnect-by-page-reload
	// behavior, where a watched viewer briefly reads as 0.
	watchGraceDelay = 60 * time.Second
	// watchCoalesceDelay spaces count-update sends while watched (the QVM
	// only announces on 0<->n flips; these just refresh the HUD count).
	watchCoalesceDelay = 10 * time.Second
	// watchRepushDelay trails a new session registration (map change) so the
	// fresh server is settled before the count is re-pushed.
	watchRepushDelay = 2 * time.Second
)

// watchKeyState is one server key's debounce state. Timer fields double as
// the "pending" flags (nil = not armed).
type watchKeyState struct {
	watched    bool
	lastSent   int       // count last rcon'd while watched
	lastSendAt time.Time
	confirm    *time.Timer
	grace      *time.Timer
	coalesce   *time.Timer
	repush     *time.Timer
}

// WatchNotifier turns raw relay viewer-count changes into debounced
// trinity_watch rcon sends. Hook methods (ViewersChanged, SessionStart) only
// mutate state and arm timers — every send happens on a timer goroutine,
// never inline, because the registry hooks can fire under callers' locks
// (see Registry.SetWatchHooks and ServerManager.SetRegistrarKick's contract).
type WatchNotifier struct {
	mu     sync.Mutex
	states map[string]*watchKeyState

	viewers func(key string) int // current live count (Registry.Viewers)
	send    func(key string, count int)

	// Delays are fields so tests can shrink them.
	confirmDelay, graceDelay, coalesceDelay, repushDelay time.Duration
}

// NewWatchNotifier creates a notifier over the given count source and send
// callback. send runs on timer goroutines and may block briefly (rcon I/O).
func NewWatchNotifier(viewers func(key string) int, send func(key string, count int)) *WatchNotifier {
	return &WatchNotifier{
		states:        make(map[string]*watchKeyState),
		viewers:       viewers,
		send:          send,
		confirmDelay:  watchConfirmDelay,
		graceDelay:    watchGraceDelay,
		coalesceDelay: watchCoalesceDelay,
		repushDelay:   watchRepushDelay,
	}
}

// stateFor returns the key's state, creating it on first use. Caller holds w.mu.
func (w *WatchNotifier) stateFor(key string) *watchKeyState {
	s := w.states[key]
	if s == nil {
		s = &watchKeyState{}
		w.states[key] = s
	}
	return s
}

// ViewersChanged is the Registry viewers hook.
func (w *WatchNotifier) ViewersChanged(key string, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.stateFor(key)

	if !s.watched {
		if n > 0 {
			if s.confirm == nil {
				s.confirm = time.AfterFunc(w.confirmDelay, func() { w.confirmFire(key) })
			}
		} else if s.confirm != nil {
			s.confirm.Stop()
			s.confirm = nil
		}
		return
	}

	if n == 0 {
		if s.grace == nil {
			s.grace = time.AfterFunc(w.graceDelay, func() { w.graceFire(key) })
		}
		return
	}
	if s.grace != nil {
		s.grace.Stop()
		s.grace = nil
	}
	if n != s.lastSent && s.coalesce == nil {
		wait := w.coalesceDelay - time.Since(s.lastSendAt)
		if wait < 0 {
			wait = 0
		}
		s.coalesce = time.AfterFunc(wait, func() { w.coalesceFire(key) })
	}
}

// SessionStart is the Registry session hook: a new session buffer registered
// under key (match start / map change). The QVM's level state reset, so a
// watched key re-pushes its count for the fresh arena population.
func (w *WatchNotifier) SessionStart(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.stateFor(key)
	if !s.watched || s.repush != nil {
		return
	}
	s.repush = time.AfterFunc(w.repushDelay, func() { w.repushFire(key) })
}

// Timer callbacks: decide under w.mu, send outside it.

func (w *WatchNotifier) confirmFire(key string) {
	w.mu.Lock()
	s := w.stateFor(key)
	s.confirm = nil
	n := w.viewers(key)
	count := -1
	if !s.watched && n > 0 {
		s.watched = true
		s.lastSent = n
		s.lastSendAt = time.Now()
		count = n
	}
	w.mu.Unlock()
	if count >= 0 {
		w.send(key, count)
	}
}

func (w *WatchNotifier) graceFire(key string) {
	w.mu.Lock()
	s := w.stateFor(key)
	s.grace = nil
	count := -1
	if s.watched && w.viewers(key) == 0 {
		s.watched = false
		s.lastSent = 0
		s.lastSendAt = time.Now()
		count = 0
	}
	w.mu.Unlock()
	if count >= 0 {
		w.send(key, count)
	}
}

func (w *WatchNotifier) coalesceFire(key string) {
	w.mu.Lock()
	s := w.stateFor(key)
	s.coalesce = nil
	n := w.viewers(key)
	count := -1
	if s.watched && n > 0 && n != s.lastSent {
		s.lastSent = n
		s.lastSendAt = time.Now()
		count = n
	}
	w.mu.Unlock()
	if count >= 0 {
		w.send(key, count)
	}
}

func (w *WatchNotifier) repushFire(key string) {
	w.mu.Lock()
	s := w.stateFor(key)
	s.repush = nil
	n := w.viewers(key)
	count := -1
	if s.watched && n > 0 {
		// Unconditional resend (even if n == lastSent): the new level's
		// configstring starts empty.
		s.lastSent = n
		s.lastSendAt = time.Now()
		count = n
	}
	w.mu.Unlock()
	if count >= 0 {
		w.send(key, count)
	}
}
