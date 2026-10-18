package livestream

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// feed is a server key's succession of Buffers, one per match; on a map
// change the tap registers the next match's Buffer under the same key.
// cur is nil between matches, and closed means the tap is gone for good.
// change is closed and replaced whenever cur or closed changes, so
// waiters can block on it and then re-check.
type feed struct {
	cur     *Buffer
	closed  bool
	viewers int // relay responses following this feed, including ones waiting between matches
	change  chan struct{}
}

// broadcast wakes everyone waiting on the current change channel. The
// caller holds the registry mutex.
func (f *feed) broadcast() {
	close(f.change)
	f.change = make(chan struct{})
}

// Registry maps a live server key, such as "ffa", to its feed. The
// collector's tap registers a Buffer per match (Set), clears it between
// matches (RemoveIf), and closes the feed when the tap drops for good.
// One viewer response follows a feed across several Buffers (see
// RelayServer.ServeHTTP and WaitNext).
//
// The key is the bare server key: a collector serves one source, and
// keys are unique within a source. The hub's /tv/<source>/<key> route
// and LiveState supply the source. Using this registry for several
// sources would need (source, key) keys, or two sources' "ffa" would
// collide.
type Registry struct {
	mu    sync.Mutex
	feeds map[string]*feed

	// Watch hooks, set once before the relay serves (SetWatchHooks).
	// They run outside r.mu, and callers may hold their own locks, so a
	// hook may call back into the Registry only through Viewers.
	viewersHook func(key string, n int)
	sessionHook func(key string)
}

func NewRegistry() *Registry {
	return &Registry{feeds: make(map[string]*feed)}
}

// SetWatchHooks wires the collector's watch notifier: viewers runs on
// every retain and release with the new count, and session runs when a
// new match's Buffer registers under a key, which is when the collector
// pushes the map change.
func (r *Registry) SetWatchHooks(viewers func(key string, n int), session func(key string)) {
	r.mu.Lock()
	r.viewersHook = viewers
	r.sessionHook = session
	r.mu.Unlock()
}

// feedFor returns the key's feed, creating it on first use. The caller
// holds r.mu.
func (r *Registry) feedFor(key string) *feed {
	f := r.feeds[key]
	if f == nil {
		f = &feed{change: make(chan struct{})}
		r.feeds[key] = f
	}
	return f
}

// Set registers the current match's buffer for the key, replacing any
// earlier one and reopening the feed if it was closed.
func (r *Registry) Set(key string, b *Buffer) {
	r.mu.Lock()
	f := r.feedFor(key)
	f.cur = b
	f.closed = false
	f.broadcast()
	hook := r.sessionHook
	r.mu.Unlock()
	if hook != nil {
		hook(key)
	}
}

// Get returns the key's current buffer, or (nil, false) when nothing is
// live. New viewers then get a 503; viewers already connected cross the
// gap through WaitNext.
func (r *Registry) Get(key string) (*Buffer, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.feeds[key]
	if f == nil || f.cur == nil {
		return nil, false
	}
	return f.cur, true
}

// Remove clears the key's current buffer and leaves the feed open.
func (r *Registry) Remove(key string) {
	r.mu.Lock()
	if f := r.feeds[key]; f != nil && f.cur != nil {
		f.cur = nil
		f.broadcast()
	}
	r.mu.Unlock()
}

// RemoveIf clears the key's current buffer only if it is still b, so a
// stale tap (a discarded dial, or one whose session ended) can't remove
// a newer buffer another tap registered under the same key. The feed
// stays open so a connected viewer waits for the next match.
func (r *Registry) RemoveIf(key string, b *Buffer) {
	r.mu.Lock()
	if f := r.feeds[key]; f != nil && f.cur == b {
		f.cur = nil
		f.broadcast()
	}
	r.mu.Unlock()
}

// Close marks the key's feed finished because the tap dropped for good,
// releasing viewers waiting in WaitNext so their responses can end.
func (r *Registry) Close(key string) {
	r.mu.Lock()
	if f := r.feeds[key]; f != nil && !f.closed {
		f.closed = true
		f.broadcast()
	}
	r.mu.Unlock()
}

// retain and release bracket one viewer's relay response under key. The
// count is per feed, so a viewer waiting in WaitNext between matches
// still counts: it holds a connection and will resume.
func (r *Registry) retain(key string) {
	r.mu.Lock()
	f := r.feedFor(key)
	f.viewers++
	n := f.viewers
	hook := r.viewersHook
	r.mu.Unlock()
	if hook != nil {
		hook(key, n)
	}
}

func (r *Registry) release(key string) {
	r.mu.Lock()
	f := r.feeds[key]
	if f == nil {
		r.mu.Unlock()
		return
	}
	f.viewers--
	n := f.viewers
	hook := r.viewersHook
	r.mu.Unlock()
	if hook != nil {
		hook(key, n)
	}
}

// Viewers returns how many relay responses are following key's feed.
// The collector reports it in the heartbeat as live_viewers.
func (r *Registry) Viewers(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f := r.feeds[key]; f != nil {
		return f.viewers
	}
	return 0
}

// WaitNext blocks until a buffer other than prev becomes current under
// key, returning (buf, true), or until the feed closes or ctx is
// canceled, returning (nil, false). The relay uses it to carry one HTTP
// response across the gap between matches.
func (r *Registry) WaitNext(ctx context.Context, key string, prev *Buffer) (*Buffer, bool) {
	for {
		r.mu.Lock()
		f := r.feeds[key]
		if f == nil {
			r.mu.Unlock()
			return nil, false
		}
		if f.cur != nil && f.cur != prev {
			b := f.cur
			r.mu.Unlock()
			return b, true
		}
		if f.closed {
			r.mu.Unlock()
			return nil, false
		}
		ch := f.change
		r.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, false
		}
	}
}

// RelayServer serves GET /tv/{key} by streaming the match's Buffer as a
// TVL1 stream.
type RelayServer struct {
	reg  *Registry
	tick time.Duration
}

// NewRelayServer creates a relay over reg. Each Buffer sets its own
// viewer delay.
func NewRelayServer(reg *Registry) *RelayServer {
	return &RelayServer{reg: reg, tick: 100 * time.Millisecond}
}

// ServeHTTP serves /tv/{key}, where key names the in-memory Buffer the
// loopback tap fills.
func (s *RelayServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	const prefix = "/tv/"
	if req.Method != http.MethodGet || !strings.HasPrefix(req.URL.Path, prefix) {
		http.NotFound(w, req)
		return
	}
	key := strings.TrimPrefix(req.URL.Path, prefix)
	if key == "" || strings.Contains(key, "/") {
		http.NotFound(w, req)
		return
	}
	b, ok := s.reg.Get(key)
	if !ok {
		// Between matches: the next match registers a new buffer under
		// this key, so tell the client to retry.
		w.Header().Set("Retry-After", "3")
		http.Error(w, "not live yet", http.StatusServiceUnavailable)
		return
	}

	// One viewer for the life of this response, waits between matches
	// included.
	s.reg.retain(key)
	defer s.reg.release(key)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering
	w.WriteHeader(http.StatusOK)

	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	// One response follows the feed across map changes: stream a match's
	// buffer (its own TVL1 header and segments), write TVLe when it ends,
	// then continue with the next match's buffer. The browser engine reads
	// TVLe followed by TVL1 as an in-place map change. The response ends
	// when the tap closes for good or the viewer disconnects.
	ctx := req.Context()
	cur := b
	for {
		if err := cur.Stream(ctx, w, flush, s.tick); err != nil {
			return
		}
		if _, err := w.Write([]byte(endMagic)); err != nil {
			return
		}
		flush()
		next, ok := s.reg.WaitNext(ctx, key, cur)
		if !ok {
			return
		}
		cur = next
	}
}
