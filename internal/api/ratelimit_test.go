package api

import (
	"testing"
	"time"
)

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
