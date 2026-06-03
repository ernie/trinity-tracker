// internal/livestream/relay.go
package livestream

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Registry maps a live server key (e.g. "ffa") to its current Buffer.
// The collector populates it when a match goes live (opening the engine
// tap) and Removes it when the match ends; the next match on the same
// server re-registers a fresh Buffer under the same key.
//
// The key is the bare server key, NOT (source, key): this registry is
// collector-local and a collector serves exactly one source, within which
// server keys are unique (schema: UNIQUE(source, key)). Global identity is
// (source, key) — the hub route /tv/<source>/<key> and the hub-side
// LiveState supply the source. Do not reuse this registry in a multi-source
// context without re-keying, or two sources' "ffa" would collide.
type Registry struct {
	mu   sync.RWMutex
	bufs map[string]*Buffer
}

func NewRegistry() *Registry {
	return &Registry{bufs: make(map[string]*Buffer)}
}

// Set registers a buffer, replacing any prior one for the key.
func (r *Registry) Set(key string, b *Buffer) {
	r.mu.Lock()
	r.bufs[key] = b
	r.mu.Unlock()
}

func (r *Registry) Get(key string) (*Buffer, bool) {
	r.mu.RLock()
	b, ok := r.bufs[key]
	r.mu.RUnlock()
	return b, ok
}

func (r *Registry) Remove(key string) {
	r.mu.Lock()
	delete(r.bufs, key)
	r.mu.Unlock()
}

// RemoveIf drops the buffer for the server key only if it still holds exactly
// b. Identity-safe teardown: a stale tap (a discarded dial, or one whose stream
// session ended) must not evict a newer buffer another tap has since registered
// under the same key. Compare-and-delete closes the registry-emptying race.
func (r *Registry) RemoveIf(key string, b *Buffer) {
	r.mu.Lock()
	if r.bufs[key] == b {
		delete(r.bufs, key)
	}
	r.mu.Unlock()
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

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering
	w.WriteHeader(http.StatusOK)

	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	_ = b.Stream(req.Context(), w, flush, s.tick)
}
