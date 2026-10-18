package directory

import (
	"net/netip"
	"sync"
	"time"
)

// rateLimiter is a per-IP token bucket on getservers and getserversExt,
// the requests that amplify. Its limits are loose because clients
// refresh their server browser at human pace, and aren't configurable.
type rateLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	capacity float64 // max tokens
	refill   float64 // tokens added per second
	buckets  map[netip.Addr]*bucket
	lastGC   time.Time
	gcEvery  time.Duration
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(capacity float64, refillPerSecond float64, now func() time.Time) *rateLimiter {
	if now == nil {
		now = time.Now
	}
	return &rateLimiter{
		now:      now,
		capacity: capacity,
		refill:   refillPerSecond,
		buckets:  make(map[netip.Addr]*bucket),
		lastGC:   now(),
		gcEvery:  5 * time.Minute,
	}
}

// Allow consumes one token from ip's bucket and reports false when it's
// empty. The caller drops the request without replying, since a UDP
// error reply would itself amplify.
func (r *rateLimiter) Allow(ip netip.Addr) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	if now.Sub(r.lastGC) > r.gcEvery {
		r.gcLocked(now)
	}

	b, ok := r.buckets[ip]
	if !ok {
		b = &bucket{tokens: r.capacity, last: now}
		r.buckets[ip] = b
	} else {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens = min64(r.capacity, b.tokens+elapsed*r.refill)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// gcLocked drops buckets that have refilled to capacity, since they
// hold nothing worth keeping.
func (r *rateLimiter) gcLocked(now time.Time) {
	for ip, b := range r.buckets {
		elapsed := now.Sub(b.last).Seconds()
		if b.tokens+elapsed*r.refill >= r.capacity {
			delete(r.buckets, ip)
		}
	}
	r.lastGC = now
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
