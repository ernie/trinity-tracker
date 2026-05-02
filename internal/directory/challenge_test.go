package directory

import (
	"net/netip"
	"testing"
	"time"
)

func mustAddrPort(t *testing.T, s string) netip.AddrPort {
	t.Helper()
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ap
}

func TestChallengeRoundTrip(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	tr := newChallengeTracker(2*time.Second, 100, clock)

	addr := mustAddrPort(t, "10.0.0.1:27960")
	c := tr.Issue(addr)
	if c == "" {
		t.Fatal("Issue returned empty challenge")
	}
	if !tr.Take(addr, c) {
		t.Fatal("Take failed for valid challenge")
	}
	// Already consumed.
	if tr.Take(addr, c) {
		t.Fatal("Take succeeded a second time (entry should be one-shot)")
	}
}

func TestChallengeExpiry(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	tr := newChallengeTracker(2*time.Second, 100, clock)

	addr := mustAddrPort(t, "10.0.0.1:27960")
	c := tr.Issue(addr)
	now = now.Add(3 * time.Second)
	if tr.Take(addr, c) {
		t.Error("Take succeeded for expired challenge")
	}
}

func TestChallengeMismatch(t *testing.T) {
	tr := newChallengeTracker(2*time.Second, 100, nil)
	addr := mustAddrPort(t, "10.0.0.1:27960")
	tr.Issue(addr)
	if tr.Take(addr, "wrong") {
		t.Error("Take succeeded with wrong challenge value")
	}
}

func TestChallengeReissueOverwrites(t *testing.T) {
	tr := newChallengeTracker(2*time.Second, 100, nil)
	addr := mustAddrPort(t, "10.0.0.1:27960")
	c1 := tr.Issue(addr)
	c2 := tr.Issue(addr)
	if c1 == c2 {
		t.Error("two random challenges collided")
	}
	// Old challenge no longer valid.
	if tr.Take(addr, c1) {
		t.Error("old challenge accepted after reissue")
	}
}

func TestChallengeCapacity(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	tr := newChallengeTracker(2*time.Second, 2, clock)

	if tr.Issue(mustAddrPort(t, "10.0.0.1:27960")) == "" {
		t.Fatal("first Issue empty")
	}
	if tr.Issue(mustAddrPort(t, "10.0.0.2:27960")) == "" {
		t.Fatal("second Issue empty")
	}
	if got := tr.Issue(mustAddrPort(t, "10.0.0.3:27960")); got != "" {
		t.Errorf("third Issue should fail at cap, got %q", got)
	}
	// After both expire, a new entry should fit.
	now = now.Add(3 * time.Second)
	if tr.Issue(mustAddrPort(t, "10.0.0.3:27960")) == "" {
		t.Error("Issue failed after capacity gc")
	}
}

func TestRandomChallengeAlphabet(t *testing.T) {
	for i := 0; i < 100; i++ {
		c := randomChallenge()
		if len(c) != challengeBytes {
			t.Fatalf("len=%d", len(c))
		}
		for _, ch := range c {
			ok := false
			for _, allowed := range challengeAlphabet {
				if byte(ch) == allowed {
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("char %q not in alphabet", ch)
			}
		}
	}
}
