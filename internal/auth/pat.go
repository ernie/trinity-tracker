package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// PATPrefix distinguishes personal access tokens from JWTs in the
// Authorization header so validation can branch without guessing.
const PATPrefix = "trin_"

// NewPAT mints a personal access token: the plaintext (shown to the
// user exactly once) and the SHA-256 hex digest stored at rest. The
// 32 random bytes make the token high-entropy enough that a fast
// unsalted hash is the right at-rest form — lookup stays a single
// indexed equality.
func NewPAT() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("minting PAT: %w", err)
	}
	token = PATPrefix + hex.EncodeToString(raw)
	return token, HashPAT(token), nil
}

// HashPAT returns the at-rest form of a PAT.
func HashPAT(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IsPAT reports whether a bearer credential is a personal access
// token rather than a JWT.
func IsPAT(token string) bool {
	return strings.HasPrefix(token, PATPrefix)
}
