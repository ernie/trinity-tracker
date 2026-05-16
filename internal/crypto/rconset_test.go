package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vector is the on-disk representation in testdata/rconset_vectors.json.
// The same vectors are baked into the engine's self-test in
// trinity-engine/code/qcommon/trinity_rconset_test_vectors.h, generated
// from this file by running the test with TRINITY_REGEN_VECTORS=1.
type vector struct {
	Name           string `json:"name"`
	Token          string `json:"token"`
	EpochNonceHex  string `json:"epoch_nonce_hex"`
	Plaintext      string `json:"plaintext"`
	ExpectedKeyHex string `json:"expected_key_hex,omitempty"`
	ExpectedBlob   string `json:"expected_blob_hex,omitempty"`
}

// vectorInputs lists the fixed (token, nonce, plaintext) tuples that
// produce the committed vectors. To regenerate the expected fields,
// set TRINITY_REGEN_VECTORS=1 and run this test; it writes
// testdata/rconset_vectors.json in place.
var vectorInputs = []vector{
	{
		Name:          "min_1byte_plaintext",
		Token:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EpochNonceHex: "00000000000000000000000000000001",
		Plaintext:     "x",
	},
	{
		Name:          "exactly_one_block",
		Token:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EpochNonceHex: "00112233445566778899aabbccddeeff",
		Plaintext:     "1234567890abcdef", // 16 bytes
	},
	{
		Name:          "spans_two_blocks",
		Token:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EpochNonceHex: "00112233445566778899aabbccddeeff",
		Plaintext:     "1234567890abcdefg", // 17 bytes
	},
	{
		Name:          "max_64_bytes",
		Token:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EpochNonceHex: "fedcba9876543210fedcba9876543210",
		Plaintext:     "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@",
	},
	{
		Name:          "typical_rcon_password",
		Token:         "deadbeefcafef00d1234567890abcdef0123456789abcdef1111222233334444",
		EpochNonceHex: "0102030405060708090a0b0c0d0e0f10",
		Plaintext:     "hunter2hunter2",
	},
	{
		Name:          "punctuation_password",
		Token:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EpochNonceHex: "ababababababababcdcdcdcdcdcdcdcd",
		Plaintext:     "!#$%&'()*+,-./", // tests every printable, no-space ASCII run
	},
	{
		Name:          "short_token",
		Token:         "abc",
		EpochNonceHex: "1111111111111111aaaaaaaaaaaaaaaa",
		Plaintext:     "pw",
	},
	{
		Name:          "long_token",
		Token:         strings.Repeat("9", 128),
		EpochNonceHex: "deadbeefdeadbeefdeadbeefdeadbeef",
		Plaintext:     "rconAdmin99",
	},
}

func TestRconsetRoundTrip(t *testing.T) {
	t.Parallel()
	for _, v := range vectorInputs {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			t.Parallel()
			nonce, err := decodeNonce(v.EpochNonceHex)
			if err != nil {
				t.Fatalf("nonce decode: %v", err)
			}
			K := DeriveRconsetKey(v.Token, nonce)
			blob, err := EncryptRconset(K, nonce, []byte(v.Plaintext))
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			got, err := decryptRconsetForTest(v.Token, blob)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if string(got) != v.Plaintext {
				t.Errorf("round-trip mismatch: got %q, want %q", got, v.Plaintext)
			}
		})
	}
}

// TestRconsetVectors locks the committed test vectors. The same blobs
// must be produced byte-for-byte by the engine's BG_SipHash128Raw and
// the trinity_rconset decoder. If you change the labels, framing, or
// SipHash byte layout, this test will fail loudly and the engine's
// self-test will need parallel updates.
func TestRconsetVectors(t *testing.T) {
	path := filepath.Join("testdata", "rconset_vectors.json")
	if os.Getenv("TRINITY_REGEN_VECTORS") == "1" {
		if err := regenerateVectors(path); err != nil {
			t.Fatalf("regenerate: %v", err)
		}
		t.Logf("regenerated %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vectors: %v (try TRINITY_REGEN_VECTORS=1)", err)
	}
	var committed []vector
	if err := json.Unmarshal(raw, &committed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(committed) != len(vectorInputs) {
		t.Fatalf("vector count drift: file has %d, inputs has %d", len(committed), len(vectorInputs))
	}
	for i, v := range committed {
		if v.Name != vectorInputs[i].Name {
			t.Errorf("vector %d: name drift %q vs %q", i, v.Name, vectorInputs[i].Name)
			continue
		}
		nonce, err := decodeNonce(v.EpochNonceHex)
		if err != nil {
			t.Errorf("%s: nonce: %v", v.Name, err)
			continue
		}
		K := DeriveRconsetKey(v.Token, nonce)
		keyHex := hex.EncodeToString(K[:])
		if keyHex != v.ExpectedKeyHex {
			t.Errorf("%s: K = %s, want %s", v.Name, keyHex, v.ExpectedKeyHex)
		}
		blob, err := EncryptRconset(K, nonce, []byte(v.Plaintext))
		if err != nil {
			t.Errorf("%s: encrypt: %v", v.Name, err)
			continue
		}
		if blob != v.ExpectedBlob {
			t.Errorf("%s: blob = %s, want %s", v.Name, blob, v.ExpectedBlob)
		}
	}
}

func TestRconsetPlaintextLengthGuards(t *testing.T) {
	var K [16]byte
	var nonce [RconsetNonceLen]byte
	if _, err := EncryptRconset(K, nonce, nil); err == nil {
		t.Error("empty plaintext should error")
	}
	oversize := make([]byte, RconsetMaxPlaintext+1)
	if _, err := EncryptRconset(K, nonce, oversize); err == nil {
		t.Error("oversize plaintext should error")
	}
	exactMax := make([]byte, RconsetMaxPlaintext)
	for i := range exactMax {
		exactMax[i] = 'A'
	}
	if _, err := EncryptRconset(K, nonce, exactMax); err != nil {
		t.Errorf("max-size plaintext rejected: %v", err)
	}
}

func TestRconsetMacRejection(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var nonce [RconsetNonceLen]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	K := DeriveRconsetKey(token, nonce)
	blob, err := EncryptRconset(K, nonce, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	// Flip one bit in the ciphertext region (offset RconsetHeaderLen*2 in hex)
	mutated := []byte(blob)
	idx := (RconsetHeaderLen) * 2
	mutated[idx] ^= 0x01
	if _, err := decryptRconsetForTest(token, string(mutated)); err == nil {
		t.Error("MAC verification should fail on mutated ciphertext")
	}
}

func TestRconsetPropertyRandom(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in -short mode")
	}
	const token = "deadbeefcafe1234567890abcdef0123deadbeefcafe1234567890abcdef0123"
	for i := 0; i < 200; i++ {
		var nonce [RconsetNonceLen]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			t.Fatal(err)
		}
		ptLen := 1 + (i % RconsetMaxPlaintext)
		pt := make([]byte, ptLen)
		for j := range pt {
			// printable, no space
			pt[j] = byte(0x21 + (i*7+j)%(0x7E-0x21+1))
		}
		K := DeriveRconsetKey(token, nonce)
		blob, err := EncryptRconset(K, nonce, pt)
		if err != nil {
			t.Fatalf("iter %d: encrypt: %v", i, err)
		}
		got, err := decryptRconsetForTest(token, blob)
		if err != nil {
			t.Fatalf("iter %d: decrypt: %v", i, err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatalf("iter %d: pt mismatch: got %q want %q", i, got, pt)
		}
	}
}

func decodeNonce(s string) ([RconsetNonceLen]byte, error) {
	var n [RconsetNonceLen]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return n, err
	}
	if len(b) != RconsetNonceLen {
		return n, &lengthError{got: len(b), want: RconsetNonceLen}
	}
	copy(n[:], b)
	return n, nil
}

type lengthError struct{ got, want int }

func (e *lengthError) Error() string {
	return "nonce length mismatch"
}

func regenerateVectors(path string) error {
	out := make([]vector, 0, len(vectorInputs))
	for _, v := range vectorInputs {
		nonce, err := decodeNonce(v.EpochNonceHex)
		if err != nil {
			return err
		}
		K := DeriveRconsetKey(v.Token, nonce)
		blob, err := EncryptRconset(K, nonce, []byte(v.Plaintext))
		if err != nil {
			return err
		}
		v.ExpectedKeyHex = hex.EncodeToString(K[:])
		v.ExpectedBlob = blob
		out = append(out, v)
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}
