package bcryptdet

import (
	"crypto/sha256"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestInteropUpstream is the most important test in this package: the hashes
// we produce MUST be accepted by the official bcrypt verifier. Because the
// upstream verifier parses the salt out of the hash and recomputes it, this
// test validates the core of our algorithm end-to-end (not just its string
// shape).
func TestInteropUpstream(t *testing.T) {
	salt := sha256.Sum256([]byte("deterministic-salt"))
	for _, tc := range []struct {
		name     string
		password string
	}{
		{"default dev password", "password123"},
		{"short password", "x"},
		{"72 bytes (bcrypt limit)", strings.Repeat("a", 72)},
		{"unicode", "secret-ñ-ü-🎉"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := GenerateFromPasswordSalt([]byte(tc.password), MinCost, salt[:SaltSize])
			if err != nil {
				t.Fatalf("GenerateFromPasswordSalt: %v", err)
			}
			if err := bcrypt.CompareHashAndPassword(hash, []byte(tc.password)); err != nil {
				t.Fatalf("the upstream verifier rejected our hash: %v (hash=%s)", err, hash)
			}
			// A wrong password must be rejected. Note: bcrypt only uses the
			// first 72 bytes, so we flip a byte near the start (rather than
			// appending a suffix) to make the difference actually visible.
			wrong := []byte(tc.password)
			if len(wrong) == 0 {
				wrong = []byte("x")
			} else {
				wrong[0] ^= 0x01
			}
			if err := bcrypt.CompareHashAndPassword(hash, wrong); err == nil {
				t.Fatal("a wrong password should FAIL verification")
			}
			cost, err := bcrypt.Cost(hash)
			if err != nil {
				t.Fatalf("Cost: %v", err)
			}
			if cost != MinCost {
				t.Fatalf("cost = %d, want %d", cost, MinCost)
			}
			if len(hash) != hashLen {
				t.Fatalf("hash length = %d, want %d (%s)", len(hash), hashLen, hash)
			}
			if !strings.HasPrefix(string(hash), "$2a$04$") {
				t.Fatalf("unexpected hash prefix: %s", hash)
			}
		})
	}
}

// TestDeterministik guards the core promise: the same (password, cost, salt)
// must produce the same bytes, forever, in any process.
func TestDeterministik(t *testing.T) {
	salt := sha256.Sum256([]byte("ci-seed|default_hash|bcrypt-salt|password123"))

	first, err := GenerateFromPasswordSalt([]byte("password123"), MinCost, salt[:SaltSize])
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := GenerateFromPasswordSalt([]byte("password123"), MinCost, salt[:SaltSize])
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("non-deterministic at iteration %d: %s != %s", i, again, first)
		}
	}

	// A different salt -> a different hash. This is what proves the salt
	// actually matters (otherwise it is mere decoration).
	other := sha256.Sum256([]byte("other-seed"))
	otherHash, err := GenerateFromPasswordSalt([]byte("password123"), MinCost, other[:SaltSize])
	if err != nil {
		t.Fatal(err)
	}
	if string(otherHash) == string(first) {
		t.Fatal("a different salt produced the same hash")
	}
}

func TestValidasiArgumen(t *testing.T) {
	salt := make([]byte, SaltSize)
	if _, err := GenerateFromPasswordSalt([]byte("ok"), MinCost, make([]byte, 8)); err == nil {
		t.Fatal("a salt with the wrong length should error")
	}
	if _, err := GenerateFromPasswordSalt([]byte(strings.Repeat("a", 73)), MinCost, salt); err == nil {
		t.Fatal("a password > 72 bytes should error")
	}
	if _, err := GenerateFromPasswordSalt([]byte("ok"), MinCost-1, salt); err == nil {
		t.Fatal("a cost below MinCost should error")
	}
	if _, err := GenerateFromPasswordSalt([]byte("ok"), MaxCost+1, salt); err == nil {
		t.Fatal("a cost above MaxCost should error")
	}
}

// TestGolden pins the output value. If someone later swaps the salt for an RNG
// (or changes the implementation), this test fails, and that is the point,
// because determinism is a product contract, not an internal detail.
func TestGolden(t *testing.T) {
	salt := sha256.Sum256([]byte("relaydb-golden"))
	hash, err := GenerateFromPasswordSalt([]byte("password123"), MinCost, salt[:SaltSize])
	if err != nil {
		t.Fatal(err)
	}
	const want = "$2a$04$L2gN29YWXo9MF8sf00UEJe8cShpVv3bHwl6CWCbcyVOgoFmhV3.Vm"
	if string(hash) != want {
		t.Fatalf("hash changed:\n  got: %s\n  want: %s", hash, want)
	}
}
