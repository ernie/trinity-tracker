package hub

import (
	"testing"
)

// The expected values come from BG_HashKeyed in the QVM; a change to
// either implementation must change both.
func TestSipHashHexCrossValidation(t *testing.T) {
	tests := []struct {
		key, message, expected string
	}{
		{"", "", "64b79b3f6116cc68b08fbf1a3dd6df0e"},
		{"testtoken1234567890abcdef12345678", "abcdef0123456789", "bcabf0bb3054990d21ef5c39d20bebf1"},
		{"abc", "xyz", "d87996ffc4dbfbec2b38ad54521a2cad"},
	}

	for i, tt := range tests {
		got := sipHashHex(tt.key, tt.message)
		if got != tt.expected {
			t.Errorf("Vector %d: sipHashHex(%q, %q) = %q, want %q", i+1, tt.key, tt.message, got, tt.expected)
		}
	}
}

func TestSipHashHexDifferentInputs(t *testing.T) {
	h1 := sipHashHex("key1", "nonce")
	h2 := sipHashHex("key2", "nonce")
	if h1 == h2 {
		t.Error("Different keys produced same hash")
	}

	h3 := sipHashHex("key", "nonce1")
	h4 := sipHashHex("key", "nonce2")
	if h3 == h4 {
		t.Error("Different messages produced same hash")
	}
}

func TestDeriveKey(t *testing.T) {
	k0a, k1a := deriveKey("testtoken")
	k0b, k1b := deriveKey("testtoken")
	if k0a != k0b || k1a != k1b {
		t.Error("deriveKey is not deterministic")
	}

	k0c, k1c := deriveKey("othertoken")
	if k0a == k0c && k1a == k1c {
		t.Error("Different tokens produced same key")
	}
}
