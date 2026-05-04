package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimit_BucketsByForwardedClientIP(t *testing.T) {
	r := &Router{}
	rl := newRateLimiter(time.Minute, 2)
	handler := r.rateLimit(rl, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	call := func(realIP string) int {
		req := httptest.NewRequest("POST", "/x", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Header.Set("X-Real-IP", realIP)
		w := httptest.NewRecorder()
		handler(w, req)
		return w.Code
	}

	// Alice burns her bucket.
	if got := call("203.0.113.1"); got != http.StatusOK {
		t.Fatalf("alice call 1: got %d, want 200", got)
	}
	if got := call("203.0.113.1"); got != http.StatusOK {
		t.Fatalf("alice call 2: got %d, want 200", got)
	}
	if got := call("203.0.113.1"); got != http.StatusTooManyRequests {
		t.Fatalf("alice call 3: got %d, want 429", got)
	}

	// Bob (different real IP, same proxy upstream) must still be allowed.
	if got := call("203.0.113.2"); got != http.StatusOK {
		t.Fatalf("bob should not share alice's bucket: got %d, want 200", got)
	}
}

// X-Forwarded-For is client-controllable in our nginx setup, so
// getClientIP must ignore it — otherwise an attacker can rotate the
// header to dodge the rate limit.
func TestRateLimit_IgnoresSpoofableForwardedFor(t *testing.T) {
	r := &Router{}
	rl := newRateLimiter(time.Minute, 1)
	handler := r.rateLimit(rl, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	call := func(xff string) int {
		req := httptest.NewRequest("POST", "/x", nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Header.Set("X-Real-IP", "203.0.113.9")
		req.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		handler(w, req)
		return w.Code
	}

	if got := call("1.1.1.1"); got != http.StatusOK {
		t.Fatalf("first call: got %d, want 200", got)
	}
	if got := call("2.2.2.2"); got != http.StatusTooManyRequests {
		t.Fatalf("rotated XFF must not change bucket: got %d, want 429", got)
	}
}

func TestRotationLimiter_AllowsBurstUpToCap(t *testing.T) {
	rl := newRotationLimiter(5, 24*time.Hour)
	for i := 0; i < 5; i++ {
		if !rl.allow("alice-q3") {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}
	if rl.allow("alice-q3") {
		t.Fatal("6th call should be denied")
	}
}

func TestRotationLimiter_KeysAreIndependent(t *testing.T) {
	rl := newRotationLimiter(2, 24*time.Hour)
	rl.allow("a")
	rl.allow("a")
	if rl.allow("a") {
		t.Fatal("3rd call to 'a' should be denied")
	}
	if !rl.allow("b") {
		t.Fatal("'b' shouldn't share a bucket with 'a'")
	}
}

func TestRotationLimiter_RefillsAfterWindow(t *testing.T) {
	rl := newRotationLimiter(2, 50*time.Millisecond)
	rl.allow("a")
	rl.allow("a")
	if rl.allow("a") {
		t.Fatal("3rd call should deny")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.allow("a") {
		t.Fatal("after window expired should allow again")
	}
}

func TestRotationLimiter_DeniedCallsDoNotConsume(t *testing.T) {
	// Once at cap, repeated denied calls shouldn't extend the cooldown.
	rl := newRotationLimiter(1, 50*time.Millisecond)
	if !rl.allow("a") {
		t.Fatal("first call should pass")
	}
	for i := 0; i < 5; i++ {
		if rl.allow("a") {
			t.Fatalf("denied iteration %d unexpectedly allowed", i)
		}
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.allow("a") {
		t.Fatal("after window, denied calls should not have shifted timing")
	}
}
