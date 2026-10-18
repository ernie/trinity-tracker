package hub

import "github.com/ernie/trinity-tracker/internal/crypto"

// sipHashHex is the SipHash-128 hex digest the Trinity auth handshake uses.
func sipHashHex(key, message string) string {
	return crypto.SipHash128Hex(key, message)
}

// deriveKey exposes the key derivation to tests.
func deriveKey(key string) (uint64, uint64) {
	return crypto.DeriveKeyForTest([]byte(key))
}
