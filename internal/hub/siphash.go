package hub

import "github.com/ernie/trinity-tracker/internal/crypto"

// sipHashHex is the legacy SipHash-128 hex helper used by the Trinity
// auth handshake. Real implementation lives in internal/crypto.
func sipHashHex(key, message string) string {
	return crypto.SipHash128Hex(key, message)
}

// deriveKey is retained for tests that exercise the key-derivation
// invariants directly. Production callers should not need it.
func deriveKey(key string) (uint64, uint64) {
	return crypto.DeriveKeyForTest([]byte(key))
}
