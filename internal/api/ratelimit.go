package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a simple sliding-window rate limiter per IP.
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

// Allow checks if the given IP is within the rate limit.
func (rl *rateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Prune old entries for this IP
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

// cleanup periodically evicts stale entries to prevent unbounded growth.
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

// rateLimit wraps a handler with IP-based rate limiting.
func (r *Router) rateLimit(rl *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ip, _, _ := net.SplitHostPort(req.RemoteAddr)
		if ip == "" {
			ip = req.RemoteAddr
		}
		if !rl.Allow(ip) {
			writeError(w, http.StatusTooManyRequests, "too many requests, try again later")
			return
		}
		next(w, req)
	}
}

// rotationLimiter is a per-source sliding-window cap on credential
// rotations from the owner-self-service path (5/24h per spec). The
// IP-based rateLimiter above is the wrong granularity here — a user
// behind CGNAT could be rotating creds for a totally different
// account from their neighbor. In-memory only; restart resets every
// bucket, which is acceptable: the cap is a guardrail, not a
// security boundary.
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

// allow records a hit against `key` and returns true if the call is
// within the limit. Denied calls do NOT consume a slot.
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
