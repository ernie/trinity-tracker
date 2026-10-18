package hub

import "sync"

// LiveState tracks which (source, key) servers have an active live-stream
// tap, as collectors report on each heartbeat. It lives in memory because
// liveness is transient match state. A collector that dies without a final
// "not live" heartbeat leaves a stale true, which is harmless: the frontend
// gates the live badge on the source's heartbeat.
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

// SetDelay records the collector's viewer delay. It's config, so it's kept
// whether or not the server is live.
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
// It has its own map because viewers can wait through the gap between
// matches while the server isn't live.
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
