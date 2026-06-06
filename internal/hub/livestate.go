package hub

import "sync"

// LiveState tracks which (source, key) servers currently have an active
// live-stream tap, as reported by collectors on the registration heartbeat.
// It is intentionally in-memory and ephemeral: live-watchability is transient
// match state, not roster config, so it never touches the DB. A collector that
// dies without sending a final "not live" heartbeat leaves a stale true here,
// but the frontend gates the live badge on the source's heartbeat liveness, so
// a dead collector's badge is suppressed regardless.
type LiveState struct {
	mu      sync.RWMutex
	m       map[string]bool
	delay   map[string]int // (source,key) -> collector's viewer delay, seconds
	viewers map[string]int // (source,key) -> relay's current web-viewer count
}

func NewLiveState() *LiveState {
	return &LiveState{
		m:       make(map[string]bool),
		delay:   make(map[string]int),
		viewers: make(map[string]int),
	}
}

// liveKey composes the (source, key) tuple. The NUL separator can't appear in
// either component, so distinct tuples never collide.
func liveKey(source, key string) string { return source + "\x00" + key }

func (s *LiveState) Set(source, key string, live bool) {
	k := liveKey(source, key)
	s.mu.Lock()
	if live {
		s.m[k] = true
	} else {
		delete(s.m, k)
	}
	s.mu.Unlock()
}

// Get is nil-receiver safe so callers without live streaming wired stay simple.
func (s *LiveState) Get(source, key string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	live := s.m[liveKey(source, key)]
	s.mu.RUnlock()
	return live
}

// SetDelay records the collector's viewer delay. Unlike liveness it's config, not
// transient state, so it's kept regardless of whether the server is currently live.
func (s *LiveState) SetDelay(source, key string, seconds int) {
	k := liveKey(source, key)
	s.mu.Lock()
	s.delay[k] = seconds
	s.mu.Unlock()
}

// GetDelay returns the viewer delay in seconds, or 0 if unknown. Nil-receiver safe.
func (s *LiveState) GetDelay(source, key string) int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	d := s.delay[liveKey(source, key)]
	s.mu.RUnlock()
	return d
}

// SetViewers records the relay's current web-viewer count for the server.
// Transient like liveness, but kept as its own map: a count can be non-zero
// while not live (viewers parked across the inter-match gap).
func (s *LiveState) SetViewers(source, key string, n int) {
	k := liveKey(source, key)
	s.mu.Lock()
	s.viewers[k] = n
	s.mu.Unlock()
}

// GetViewers returns the last-reported web-viewer count, or 0 if unknown.
// Nil-receiver safe.
func (s *LiveState) GetViewers(source, key string) int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	n := s.viewers[liveKey(source, key)]
	s.mu.RUnlock()
	return n
}
