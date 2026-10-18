package api

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a sliding-window limit per IP.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	window   time.Duration
	max      int
}

func newRateLimiter(window time.Duration, max int) *rateLimiter {
	rl := &rateLimiter{
		attempts: make(map[string][]time.Time),
		window:   window,
		max:      max,
	}
	go rl.cleanup()
	return rl
}

// Reset clears all recorded attempts for the given IP.
func (rl *rateLimiter) Reset(ip string) {
	rl.mu.Lock()
	delete(rl.attempts, ip)
	rl.mu.Unlock()
}

// Allow records an attempt from ip and reports whether it is within the
// limit. A refused attempt isn't recorded.
func (rl *rateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	attempts := rl.attempts[ip]
	valid := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.max {
		rl.attempts[ip] = valid
		return false
	}

	rl.attempts[ip] = append(valid, now)
	return true
}

// cleanup drops expired attempts so idle IPs don't accumulate.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.window)
		for ip, attempts := range rl.attempts {
			valid := attempts[:0]
			for _, t := range attempts {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.attempts, ip)
			} else {
				rl.attempts[ip] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func (r *Router) rateLimit(rl *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !rl.Allow(getClientIP(req)) {
			writeError(w, http.StatusTooManyRequests, "Too many requests. Try again later.")
			return
		}
		next(w, req)
	}
}

// rotationLimiter caps owner credential rotations per source over a
// sliding window. It is keyed by source because users behind one NAT
// share an IP. It lives in memory, so a restart resets it; it is a
// guardrail, not a security boundary.
type rotationLimiter struct {
	cap    int
	window time.Duration
	mu     sync.Mutex
	hits   map[string][]time.Time
}

func newRotationLimiter(cap int, window time.Duration) *rotationLimiter {
	return &rotationLimiter{
		cap:    cap,
		window: window,
		hits:   make(map[string][]time.Time),
	}
}

// allow records a hit against key and reports whether it is within the
// limit. A denied call isn't recorded.
func (r *rotationLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-r.window)
	pruned := r.hits[key][:0]
	for _, t := range r.hits[key] {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	if len(pruned) >= r.cap {
		r.hits[key] = pruned
		return false
	}
	r.hits[key] = append(pruned, now)
	return true
}
