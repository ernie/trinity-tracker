// Package crypto provides the shared crypto primitives used between
// the hub, collector, and the Trinity engine (in C). All functions in
// this package are byte-identical to their counterparts in
// trinity-engine/code/client/cl_trinity_rconset.c.
//
// Two layers:
//
//   - SipHash128Raw / SipHash128Hex: SipHash-2-4 with 128-bit output.
//     Hex form is what the Trinity account-auth handshake uses today
//     (BG_HashKeyed). Raw form is what the rcon-stuff protocol uses
//     because its message inputs include a binary nonce that may
//     contain NUL bytes.
//
//   - DeriveRconsetKey / EncryptRconset: the framed protocol that
//     pushes an encrypted rcon password down to the engine over a
//     trinity_rconset server command. See rconset.go.
package crypto

import "encoding/hex"

// SipHash128Raw computes SipHash-2-4 with a 128-bit output over msg,
// keyed by key. Both arguments are arbitrary-length byte slices; NUL
// bytes are permitted in either. The output byte layout is chosen so
// that hex.EncodeToString of the returned array is byte-identical to
// SipHash128Hex(string(key), string(message)) — preserves the existing
// auth-handshake wire format.
func SipHash128Raw(key, msg []byte) [16]byte {
	k0, k1 := deriveKey(key)

	v0 := k0 ^ 0x736f6d6570736575
	v1 := k1 ^ 0x646f72616e646f6d
	v2 := k0 ^ 0x6c7967656e657261
	v3 := k1 ^ 0x7465646279746573

	v1 ^= 0xee // 128-bit output tag

	blocks := len(msg) / 8
	for i := 0; i < blocks; i++ {
		m := uint64(msg[i*8]) |
			uint64(msg[i*8+1])<<8 |
			uint64(msg[i*8+2])<<16 |
			uint64(msg[i*8+3])<<24 |
			uint64(msg[i*8+4])<<32 |
			uint64(msg[i*8+5])<<40 |
			uint64(msg[i*8+6])<<48 |
			uint64(msg[i*8+7])<<56
		v3 ^= m
		v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
		v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
		v0 ^= m
	}

	var m uint64
	left := len(msg) & 7
	for j := left - 1; j >= 0; j-- {
		m <<= 8
		m |= uint64(msg[blocks*8+j])
	}
	m |= uint64(len(msg)&0xff) << 56
	v3 ^= m
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0 ^= m

	v2 ^= 0xee
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	hash0 := v0 ^ v1 ^ v2 ^ v3

	v1 ^= 0xdd
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	hash1 := v0 ^ v1 ^ v2 ^ v3

	// Byte layout chosen to match the legacy sipHashHex string format:
	//   bytes 0-3:  hash0 low 32 bits, big-endian
	//   bytes 4-7:  hash0 high 32 bits, big-endian
	//   bytes 8-11: hash1 low 32 bits, big-endian
	//   bytes 12-15: hash1 high 32 bits, big-endian
	var out [16]byte
	out[0] = byte(hash0 >> 24)
	out[1] = byte(hash0 >> 16)
	out[2] = byte(hash0 >> 8)
	out[3] = byte(hash0)
	out[4] = byte(hash0 >> 56)
	out[5] = byte(hash0 >> 48)
	out[6] = byte(hash0 >> 40)
	out[7] = byte(hash0 >> 32)
	out[8] = byte(hash1 >> 24)
	out[9] = byte(hash1 >> 16)
	out[10] = byte(hash1 >> 8)
	out[11] = byte(hash1)
	out[12] = byte(hash1 >> 56)
	out[13] = byte(hash1 >> 48)
	out[14] = byte(hash1 >> 40)
	out[15] = byte(hash1 >> 32)
	return out
}

// SipHash128Hex is the string-flavored form of SipHash128Raw. Inputs
// must be NUL-free for byte-identical behavior with the C BG_HashKeyed
// helper (which treats arguments as C strings).
func SipHash128Hex(key, message string) string {
	raw := SipHash128Raw([]byte(key), []byte(message))
	return hex.EncodeToString(raw[:])
}

func sipRound(v0, v1, v2, v3 uint64) (uint64, uint64, uint64, uint64) {
	v0 += v1
	v2 += v3
	v1 = v1<<13 | v1>>(64-13)
	v3 = v3<<16 | v3>>(64-16)
	v1 ^= v0
	v3 ^= v2
	v0 = v0<<32 | v0>>(64-32)
	v2 += v1
	v0 += v3
	v1 = v1<<17 | v1>>(64-17)
	v3 = v3<<21 | v3>>(64-21)
	v1 ^= v2
	v3 ^= v0
	v2 = v2<<32 | v2>>(64-32)
	return v0, v1, v2, v3
}

// deriveKey folds an arbitrary-length byte slice into two uint64
// SipHash key halves. Byte-identical to DeriveKey in bg_hash.c, which
// also runs over the key byte-by-byte (in C it terminates at NUL; the
// rcon-stuff path only ever passes NUL-free token strings as the key,
// so the two behaviors agree in practice).
func deriveKey(key []byte) (uint64, uint64) {
	h := [4]uint32{0x736f6d65, 0x646f7261, 0x6c796765, 0x74656462}
	for i := 0; i < len(key); i++ {
		h[i&3] ^= uint32(key[i])
		h[i&3] *= 0x01000193
	}
	k0 := uint64(h[0])<<32 | uint64(h[1])
	k1 := uint64(h[2])<<32 | uint64(h[3])
	return k0, k1
}

// DeriveKeyForTest exposes deriveKey for cross-package test harnesses
// (e.g. the legacy hub package SipHash tests). Not for production use.
func DeriveKeyForTest(key []byte) (uint64, uint64) {
	return deriveKey(key)
}
