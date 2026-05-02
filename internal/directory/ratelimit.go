package directory

import (
	"net/netip"
	"sync"
	"time"
)

// rateLimiter is a per-IP token bucket guarding the amplification
// vectors (getservers / getserversExt). Defaults are intentionally
// loose for v1 — Q3 clients refresh their server browser at human
// timescales — and not exposed in YAML; tighten if abuse is observed.
type rateLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	capacity float64       // max tokens
	refill   float64       // tokens added per second
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

// Allow consumes one token from ip's bucket. Returns false if the
// bucket is empty (caller should drop the request silently — no error
// reply, since UDP error replies themselves amplify).
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

// gcLocked drops buckets that have been quiet long enough to be at
// full capacity — those carry no state we need to remember.
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
