package hub

import "fmt"

// sipHashHex matches the BG_HashKeyed implementation in the QVM:
// SipHash-2-4 with 128-bit output, keyed by token, message is nonce.
// Used for Trinity account-auth verification; byte-identical to
// trinity-engine/code/game/bg_hash.c.
func sipHashHex(key, message string) string {
	k0, k1 := deriveKey(key)
	msg := []byte(message)

	v0 := k0 ^ 0x736f6d6570736575
	v1 := k1 ^ 0x646f72616e646f6d
	v2 := k0 ^ 0x6c7967656e657261
	v3 := k1 ^ 0x7465646279746573

	v1 ^= 0xee

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

	return fmt.Sprintf("%08x%08x%08x%08x",
		uint32(hash0), uint32(hash0>>32),
		uint32(hash1), uint32(hash1>>32))
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

// deriveKey folds a variable-length key into two uint64 SipHash key halves.
// Must match the DeriveKey function in bg_hash.c exactly.
func deriveKey(key string) (uint64, uint64) {
	h := [4]uint32{0x736f6d65, 0x646f7261, 0x6c796765, 0x74656462}
	for i := 0; i < len(key); i++ {
		h[i&3] ^= uint32(key[i])
		h[i&3] *= 0x01000193
	}
	k0 := uint64(h[0])<<32 | uint64(h[1])
	k1 := uint64(h[2])<<32 | uint64(h[3])
	return k0, k1
}
