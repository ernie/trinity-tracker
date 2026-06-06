// internal/livestream/relay.go
package livestream

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// feed is a server key's succession of session Buffers. One match's stream is
// one Buffer; across a map change the tap registers the next match's Buffer
// under the same key. `cur` is the current session's buffer (nil during the
// inter-match gap), `closed` marks the tap gone for good. `change` is closed and
// replaced on every cur/closed mutation, so waiters can block then re-check.
type feed struct {
	cur     *Buffer
	closed  bool
	viewers int // relay responses currently following this feed (incl. gap-parked waiters)
	change  chan struct{}
}

// broadcast wakes everyone parked on the current change channel. Caller holds
// the registry mutex.
func (f *feed) broadcast() {
	close(f.change)
	f.change = make(chan struct{})
}

// Registry maps a live server key (e.g. "ffa") to its feed of successive
// session Buffers. The collector's tap registers a fresh Buffer per session
// (Set), nils it during the inter-match gap (RemoveIf), and Closes the feed when
// the tap drops for good. A viewer's relay response follows one feed across
// several buffer generations (see RelayServer.ServeHTTP + WaitNext).
//
// The key is the bare server key, NOT (source, key): this registry is
// collector-local and a collector serves exactly one source, within which
// server keys are unique (schema: UNIQUE(source, key)). Global identity is
// (source, key) — the hub route /tv/<source>/<key> and the hub-side
// LiveState supply the source. Do not reuse this registry in a multi-source
// context without re-keying, or two sources' "ffa" would collide.
type Registry struct {
	mu    sync.Mutex
	feeds map[string]*feed
}

func NewRegistry() *Registry {
	return &Registry{feeds: make(map[string]*feed)}
}

// feedFor returns the key's feed, creating it on first use. Caller holds r.mu.
func (r *Registry) feedFor(key string) *feed {
	f := r.feeds[key]
	if f == nil {
		f = &feed{change: make(chan struct{})}
		r.feeds[key] = f
	}
	return f
}

// Set registers the current session's buffer for the key, replacing any prior
// one and reopening the feed (a key coming back live after a Close).
func (r *Registry) Set(key string, b *Buffer) {
	r.mu.Lock()
	f := r.feedFor(key)
	f.cur = b
	f.closed = false
	f.broadcast()
	r.mu.Unlock()
}

// Get returns the key's current buffer, or (nil,false) when none is live (no
// feed, or the inter-match gap). New viewers get a 503 from this; existing
// viewers cross the gap via WaitNext instead.
func (r *Registry) Get(key string) (*Buffer, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.feeds[key]
	if f == nil || f.cur == nil {
		return nil, false
	}
	return f.cur, true
}

// Remove clears the key's current buffer (without closing the feed).
func (r *Registry) Remove(key string) {
	r.mu.Lock()
	if f := r.feeds[key]; f != nil && f.cur != nil {
		f.cur = nil
		f.broadcast()
	}
	r.mu.Unlock()
}

// RemoveIf clears the key's current buffer only if it still holds exactly b.
// Identity-safe teardown: a stale tap (a discarded dial, or one whose stream
// session ended) must not evict a newer buffer another tap has since registered
// under the same key. Compare-and-clear closes the registry-emptying race. The
// feed itself stays open so a mid-response viewer waits for the next session.
func (r *Registry) RemoveIf(key string, b *Buffer) {
	r.mu.Lock()
	if f := r.feeds[key]; f != nil && f.cur == b {
		f.cur = nil
		f.broadcast()
	}
	r.mu.Unlock()
}

// Close marks the key's feed finished (the tap dropped for good), unblocking any
// viewers waiting in WaitNext for the next session so their response can end.
func (r *Registry) Close(key string) {
	r.mu.Lock()
	if f := r.feeds[key]; f != nil && !f.closed {
		f.closed = true
		f.broadcast()
	}
	r.mu.Unlock()
}

// retain/release bracket one viewer's relay response under key. The count is
// per-feed, not per-buffer, so a viewer parked in WaitNext across the
// inter-match gap still counts — it holds a connection and will resume.
func (r *Registry) retain(key string) {
	r.mu.Lock()
	r.feedFor(key).viewers++
	r.mu.Unlock()
}

func (r *Registry) release(key string) {
	r.mu.Lock()
	if f := r.feeds[key]; f != nil {
		f.viewers--
	}
	r.mu.Unlock()
}

// Viewers returns how many relay responses are currently following key's feed.
// The collector reports it on the registration heartbeat (live_viewers).
func (r *Registry) Viewers(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f := r.feeds[key]; f != nil {
		return f.viewers
	}
	return 0
}

// WaitNext blocks until a buffer other than prev becomes current under key (the
// next session), the feed closes, or ctx is cancelled. Returns (buf,true) for a
// fresh session buffer; (nil,false) on close/cancel. The relay uses it to span
// the inter-match gap within one HTTP response.
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

// RelayServer serves GET /tv/{key} by streaming the match's Buffer. Live is a
// stream (TVL1), not a .tvd recording, so the path carries no demo extension.
type RelayServer struct {
	reg  *Registry
	tick time.Duration
}

// NewRelayServer creates a relay over reg. Viewer delay is per-Buffer, set at
// each Buffer's creation.
func NewRelayServer(reg *Registry) *RelayServer {
	return &RelayServer{reg: reg, tick: 100 * time.Millisecond}
}

// ServeHTTP routes /tv/{key}. The key is the bare path segment — there is no
// file and no extension; it indexes the in-memory Buffer the loopback tap fills.
func (s *RelayServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	const prefix = "/tv/"
	if req.Method != http.MethodGet || !strings.HasPrefix(req.URL.Path, prefix) {
		http.NotFound(w, req)
		return
	}
	key := strings.TrimPrefix(req.URL.Path, prefix)
	if key == "" || strings.Contains(key, "/") {
		// Single bare segment only (e.g. "ffa"); no nested path.
		http.NotFound(w, req)
		return
	}
	b, ok := s.reg.Get(key)
	if !ok {
		// No active stream for this server key — an inter-match gap. Tell the
		// client to retry rather than treating it as a hard 404; the next
		// match re-registers a fresh buffer under the same key.
		w.Header().Set("Retry-After", "3")
		http.Error(w, "not live yet", http.StatusServiceUnavailable)
		return
	}

	// One viewer for the lifetime of this response, gap-parking included.
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

	// Follow the key's feed across map changes within this one response: stream a
	// session's buffer (which emits its own TVL1 header + segments), write the
	// TVLe boundary when it ends, then pick up the next session's buffer and
	// continue. The browser engine reads TVLe->TVL1 as an in-place map change
	// (no page reload). End only when the tap closes for good (true end) or the
	// viewer disconnects.
	ctx := req.Context()
	cur := b
	for {
		if err := cur.Stream(ctx, w, flush, s.tick); err != nil {
			return // viewer disconnected or request cancelled
		}
		// Session ended cleanly. Emit the end marker, then hold the response open
		// across the inter-match gap for the next session's buffer.
		if _, err := w.Write([]byte(endMagic)); err != nil {
			return
		}
		flush()
		next, ok := s.reg.WaitNext(ctx, key, cur)
		if !ok {
			return // tap closed for good (true end) or ctx cancelled
		}
		cur = next
	}
}
