package directory

import (
	"crypto/rand"
	"net/netip"
	"sync"
	"time"
)

const (
	challengeBytes = 12 // about 72 bits, well under the engine's 128-byte cap
)

// challengeAlphabet survives the engine's infostring sanitizer, which
// strips \ and ;, and leaves out " and whitespace too.
var challengeAlphabet = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_")

// challengeTracker holds the challenges issued to heartbeats until they
// expire. max bounds memory under a flood of spoofed heartbeats.
type challengeTracker struct {
	mu      sync.Mutex
	now     func() time.Time
	timeout time.Duration
	max     int
	pending map[netip.AddrPort]challengeEntry
}

type challengeEntry struct {
	value     string
	expiresAt time.Time
}

func newChallengeTracker(timeout time.Duration, max int, now func() time.Time) *challengeTracker {
	if now == nil {
		now = time.Now
	}
	return &challengeTracker{
		now:     now,
		timeout: timeout,
		max:     max,
		pending: make(map[netip.AddrPort]challengeEntry),
	}
}

// Issue returns a new challenge for srcAddr, replacing any earlier one.
// It returns "" when the tracker is full, and the caller drops the
// heartbeat.
func (t *challengeTracker) Issue(srcAddr netip.AddrPort) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.pending[srcAddr]; !exists && len(t.pending) >= t.max {
		t.gcLocked()
		if len(t.pending) >= t.max {
			return ""
		}
	}

	c := randomChallenge()
	t.pending[srcAddr] = challengeEntry{
		value:     c,
		expiresAt: t.now().Add(t.timeout),
	}
	return c
}

// Take reports whether value matches srcAddr's unexpired challenge. The
// challenge is single use, so it's removed either way.
func (t *challengeTracker) Take(srcAddr netip.AddrPort, value string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.pending[srcAddr]
	if !ok {
		return false
	}
	delete(t.pending, srcAddr)
	if t.now().After(entry.expiresAt) {
		return false
	}
	return entry.value == value
}

// gcLocked drops expired entries. The caller holds mu.
func (t *challengeTracker) gcLocked() {
	now := t.now()
	for k, v := range t.pending {
		if now.After(v.expiresAt) {
			delete(t.pending, k)
		}
	}
}

// randomChallenge uses crypto/rand so an attacker who can't see the
// traffic can't forge a challenge.
func randomChallenge() string {
	buf := make([]byte, challengeBytes)
	if _, err := rand.Read(buf); err != nil {
		panic("directory: crypto/rand failed: " + err.Error())
	}
	out := make([]byte, challengeBytes)
	for i, b := range buf {
		out[i] = challengeAlphabet[int(b)%len(challengeAlphabet)]
	}
	return string(out)
}
