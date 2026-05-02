package directory

import (
	"crypto/rand"
	"net/netip"
	"sync"
	"time"
)

const (
	challengeBytes = 12 // 12 chars from a 64-char alphabet ≈ 72 bits — well below the engine's 128-byte cap
)

// challengeAlphabet is the printable ASCII subset that survives the
// engine's infostring sanitizer. The engine strips `\` (separator) and
// `;` (cvar terminator); we additionally avoid `"` and whitespace.
var challengeAlphabet = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_")

// challengeTracker holds outstanding challenges issued in response to
// heartbeats. Each entry expires after challengeTimeout. The cap on
// total entries (max) bounds memory under a heartbeat-spoof flood.
type challengeTracker struct {
	mu        sync.Mutex
	now       func() time.Time
	timeout   time.Duration
	max       int
	pending   map[netip.AddrPort]challengeEntry
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

// Issue generates a new challenge for srcAddr, evicting any previous
// outstanding challenge from the same source. Returns the challenge
// string. If the cap is reached, Issue returns "" — caller should drop
// the heartbeat.
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

// Take consumes the challenge for srcAddr if value matches and the
// entry has not expired. Returns true on success. The entry is removed
// either way (a single-use token).
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

// gcLocked drops expired entries. Caller must hold mu.
func (t *challengeTracker) gcLocked() {
	now := t.now()
	for k, v := range t.pending {
		if now.After(v.expiresAt) {
			delete(t.pending, k)
		}
	}
}

// randomChallenge returns a fresh challenge string. Uses crypto/rand so
// an attacker who can't observe the wire can't forge one.
func randomChallenge() string {
	buf := make([]byte, challengeBytes)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failures are unrecoverable in Go; panicking matches
		// the rest of the stdlib's behavior on this code path.
		panic("directory: crypto/rand failed: " + err.Error())
	}
	out := make([]byte, challengeBytes)
	for i, b := range buf {
		out[i] = challengeAlphabet[int(b)%len(challengeAlphabet)]
	}
	return string(out)
}
