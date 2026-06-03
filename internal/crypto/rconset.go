package crypto

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// Wire-format constants. Mirror the engine's cl_trinity_rconset.c.
const (
	RconsetVersion      byte = 1
	RconsetMaxPlaintext      = 64
	RconsetNonceLen          = 16
	RconsetMACLen            = 8
	RconsetHeaderLen         = 2 + RconsetNonceLen // version + ct_len + nonce
)

// Domain-separation labels. Must match cl_trinity_rconset.c byte-for-byte.
const (
	labelKey = "trinity-rconset-key-v1"
	labelKS  = "trinity-rconset-ks-v1"
	labelMAC = "trinity-rconset-mac-v1"
)

// ErrPlaintextLen is returned by EncryptRconset when the plaintext is
// empty or exceeds RconsetMaxPlaintext.
var ErrPlaintextLen = errors.New("crypto: rconset plaintext length out of range")

// DeriveRconsetKey returns the per-handshake 16-byte key K used by the
// rcon-stuff protocol. Hub-only: only the hub knows the user's
// long-lived token. The collector receives K (not the token) and only
// for as long as it needs to encrypt one message.
func DeriveRconsetKey(token string, epochNonce [RconsetNonceLen]byte) [16]byte {
	msg := make([]byte, 0, len(labelKey)+RconsetNonceLen)
	msg = append(msg, labelKey...)
	msg = append(msg, epochNonce[:]...)
	return SipHash128Raw([]byte(token), msg)
}

// EncryptRconset packages plaintext for the engine's trinity_rconset
// server-command handler. Returns the hex string that goes directly
// into the rcon "sv_cmd trinity_rconset <clientID> <hexblob>"
// argument. The returned string is lowercase hex.
//
// Wire layout (decoded):
//
//	offset  size  field
//	0       1     version (= RconsetVersion)
//	1       1     ct_len  (= len(plaintext))
//	2       16    epoch_nonce
//	18      L     ciphertext = plaintext XOR keystream
//	18+L    8     mac        = SipHash128Raw(key, labelMAC || header || ciphertext)[:8]
func EncryptRconset(key [16]byte, epochNonce [RconsetNonceLen]byte, plaintext []byte) (string, error) {
	if len(plaintext) == 0 || len(plaintext) > RconsetMaxPlaintext {
		return "", fmt.Errorf("%w: got %d, want 1..%d", ErrPlaintextLen, len(plaintext), RconsetMaxPlaintext)
	}

	ctLen := len(plaintext)
	blob := make([]byte, RconsetHeaderLen+ctLen+RconsetMACLen)

	// Header
	blob[0] = RconsetVersion
	blob[1] = byte(ctLen)
	copy(blob[2:2+RconsetNonceLen], epochNonce[:])

	// Ciphertext = plaintext XOR keystream
	ks := keystream(key, epochNonce, ctLen)
	for i := 0; i < ctLen; i++ {
		blob[RconsetHeaderLen+i] = plaintext[i] ^ ks[i]
	}

	// MAC = SipHash128(key, labelMAC || header || ciphertext)[:8]
	macMsg := make([]byte, 0, len(labelMAC)+RconsetHeaderLen+ctLen)
	macMsg = append(macMsg, labelMAC...)
	macMsg = append(macMsg, blob[:RconsetHeaderLen+ctLen]...)
	macFull := SipHash128Raw(key[:], macMsg)
	copy(blob[RconsetHeaderLen+ctLen:], macFull[:RconsetMACLen])

	return hex.EncodeToString(blob), nil
}

// keystream returns ctLen bytes of keystream derived from
// SipHash128(key, labelKS || epochNonce || u32be(counter)) over counter
// = 0, 1, 2, ...
func keystream(key [16]byte, epochNonce [RconsetNonceLen]byte, ctLen int) []byte {
	out := make([]byte, 0, ctLen+16)
	var counter [4]byte
	msg := make([]byte, len(labelKS)+RconsetNonceLen+4)
	copy(msg, labelKS)
	copy(msg[len(labelKS):], epochNonce[:])
	for i := 0; len(out) < ctLen; i++ {
		binary.BigEndian.PutUint32(counter[:], uint32(i))
		copy(msg[len(labelKS)+RconsetNonceLen:], counter[:])
		block := SipHash128Raw(key[:], msg)
		out = append(out, block[:]...)
	}
	return out[:ctLen]
}

// decryptRconsetForTest is the inverse of EncryptRconset; test-only.
// Production decryption only ever happens on the engine side, in C.
// Exported only as a helper for the cross-language round-trip test.
func decryptRconsetForTest(token string, hexBlob string) ([]byte, error) {
	blob, err := hex.DecodeString(hexBlob)
	if err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}
	if len(blob) < RconsetHeaderLen+RconsetMACLen {
		return nil, fmt.Errorf("blob too short: %d", len(blob))
	}
	if blob[0] != RconsetVersion {
		return nil, fmt.Errorf("bad version: %d", blob[0])
	}
	ctLen := int(blob[1])
	if ctLen < 1 || ctLen > RconsetMaxPlaintext {
		return nil, fmt.Errorf("bad ct_len: %d", ctLen)
	}
	if len(blob) != RconsetHeaderLen+ctLen+RconsetMACLen {
		return nil, fmt.Errorf("blob length mismatch: got %d, want %d", len(blob), RconsetHeaderLen+ctLen+RconsetMACLen)
	}
	var epochNonce [RconsetNonceLen]byte
	copy(epochNonce[:], blob[2:2+RconsetNonceLen])
	ct := blob[RconsetHeaderLen : RconsetHeaderLen+ctLen]
	macRecv := blob[RconsetHeaderLen+ctLen:]

	key := DeriveRconsetKey(token, epochNonce)

	macMsg := make([]byte, 0, len(labelMAC)+RconsetHeaderLen+ctLen)
	macMsg = append(macMsg, labelMAC...)
	macMsg = append(macMsg, blob[:RconsetHeaderLen+ctLen]...)
	macFull := SipHash128Raw(key[:], macMsg)
	if !constantTimeEqual(macFull[:RconsetMACLen], macRecv) {
		return nil, errors.New("mac mismatch")
	}

	ks := keystream(key, epochNonce, ctLen)
	pt := make([]byte, ctLen)
	for i := 0; i < ctLen; i++ {
		pt[i] = ct[i] ^ ks[i]
	}
	return pt, nil
}

func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
