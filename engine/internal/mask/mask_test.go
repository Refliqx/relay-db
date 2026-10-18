package mask

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/refliqx/relaydb/engine/internal/protocol"
	"golang.org/x/crypto/bcrypt"
)

// resetHashCache clears the process-global `default_hash` cache. Used by tests
// to simulate a FRESH PROCESS: without it, the cache would hide the
// non-determinism that only appears across processes.
func resetHashCache() {
	globalCache.mu.Lock()
	globalCache.m = map[string]string{}
	globalCache.mu.Unlock()
}

func computeDefaultHash(t *testing.T, seed string, opts map[string]any, value string) string {
	t.Helper()
	resetHashCache()
	e := New(seed)
	v, err := e.Apply("users", "password", protocol.StrategyDefaultHash, opts, value)
	if err != nil {
		t.Fatalf("apply default_hash: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("default_hash must be a string, got %T", v)
	}
	return s
}

func TestDeterminismAndCrossTableConsistency(t *testing.T) {
	e1 := New("seed-abc")
	e2 := New("seed-abc")
	e3 := New("seed-xyz")

	// Same value -> same result, even across different tables/columns. This is
	// the property that keeps relationships between tables consistent.
	a, _ := e1.Apply("users", "email", protocol.StrategyFakeEmail, nil, "budi@mail.com")
	b, _ := e2.Apply("users", "email", protocol.StrategyFakeEmail, nil, "budi@mail.com")
	c, _ := e1.Apply("customers", "contact_email", protocol.StrategyFakeEmail, nil, "budi@mail.com")
	if a != b {
		t.Fatalf("the same seed must produce the same value: %v vs %v", a, b)
	}
	if a != c {
		t.Fatalf("the same value in different tables must be consistent (relationship integrity): %v vs %v", a, c)
	}

	d, _ := e3.Apply("users", "email", protocol.StrategyFakeEmail, nil, "budi@mail.com")
	if a == d {
		t.Fatalf("different seeds must produce different values")
	}
}

func TestUniquenessPreserved(t *testing.T) {
	e := New("seed")
	seen := map[any]bool{}
	for _, src := range []string{
		"a@mail.com", "b@mail.com", "c@mail.com", "d@mail.com", "e@mail.com",
		"f@mail.com", "g@mail.com", "h@mail.com", "i@mail.com", "j@mail.com",
	} {
		v, err := e.Apply("users", "email", protocol.StrategyFakeEmail, nil, src)
		if err != nil {
			t.Fatal(err)
		}
		if seen[v] {
			t.Fatalf("value collision on a UNIQUE column: %v", v)
		}
		seen[v] = true

		if !strings.Contains(v.(string), "@") {
			t.Fatalf("fake_email must still look like an email: %v", v)
		}
	}
}

func TestScramblePreservesShape(t *testing.T) {
	e := New("seed")
	got, _ := e.Apply("users", "phone", protocol.StrategyScramble, nil, "0812-3456-7890")
	s := got.(string)
	if len(s) != len("0812-3456-7890") {
		t.Fatalf("length must be preserved: %q", s)
	}
	for i, r := range s {
		src := rune("0812-3456-7890"[i])
		if src == '-' && r != '-' {
			t.Fatalf("separators must be preserved")
		}
		if src >= '0' && src <= '9' && (r < '0' || r > '9') {
			t.Fatalf("letters must not replace digits: %q", s)
		}
	}
	if s == "0812-3456-7890" {
		t.Fatalf("the value must change")
	}
}

func TestScrambleKeepPrefix(t *testing.T) {
	e := New("seed")
	got, _ := e.Apply("t", "c", protocol.StrategyScramble, map[string]any{"keep_prefix": 4}, "0812345678")
	if !strings.HasPrefix(got.(string), "0812") {
		t.Fatalf("keep_prefix was not honoured: %v", got)
	}
}

func TestNullifyAndKeep(t *testing.T) {
	e := New("seed")
	if v, _ := e.Apply("t", "c", protocol.StrategyNullify, nil, "x"); v != nil {
		t.Fatalf("nullify must produce nil")
	}
	if v, _ := e.Apply("t", "c", protocol.StrategyKeep, nil, "x"); v != "x" {
		t.Fatalf("keep must be a passthrough")
	}
	// An empty value is always NULL, whatever the strategy (column semantics
	// are preserved).
	if v, _ := e.Apply("t", "c", protocol.StrategyFakeEmail, nil, nil); v != nil {
		t.Fatalf("nil must stay nil")
	}
}

func TestJSONScrubOnTextField(t *testing.T) {
	e := New("seed")
	src := `{"token":"tok_abc123","phone":"081234567890","theme":"dark"}`
	got, err := e.Apply("users", "settings", protocol.StrategyJSONScrub, map[string]any{
		"keys": map[string]any{"token": "hash_sha256", "phone": "fake_phone"},
	}, src)
	if err != nil {
		t.Fatal(err)
	}
	out := got.(string)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("the result must be valid JSON: %v (%s)", err, out)
	}
	if parsed["theme"] != "dark" {
		t.Fatalf("a non-sensitive key must not change: %v", parsed["theme"])
	}
	if strings.Contains(out, "tok_abc123") {
		t.Fatalf("the original token leaked: %s", out)
	}
	if strings.Contains(out, "081234567890") {
		t.Fatalf("the original number leaked: %s", out)
	}
	// Determinism on the JSON text path.
	again, _ := e.Apply("users", "settings", protocol.StrategyJSONScrub, map[string]any{
		"keys": map[string]any{"token": "hash_sha256", "phone": "fake_phone"},
	}, src)
	if again != got {
		t.Fatalf("json_scrub must be deterministic")
	}
}

func TestDefaultHashIsConstantAndCached(t *testing.T) {
	e := New("seed")
	a, _ := e.Apply("users", "password", protocol.StrategyDefaultHash, nil, "one")
	b, _ := e.Apply("users", "password", protocol.StrategyDefaultHash, nil, "two")
	if a != b {
		t.Fatalf("default_hash must be constant for all rows (every dev account can log in)")
	}
	s := a.(string)
	if !strings.HasPrefix(s, "$2") || len(s) != 60 {
		t.Fatalf("must be a valid bcrypt hash: %q", s)
	}
}

// TestDefaultHashSeedReproducible is the main safety net for the
// "reproducible export" guarantee. The process-global cache is deliberately
// cleared before each computation so this test is equivalent to running the
// engine in a FRESH PROCESS; the only way to catch randomness coming from
// crypto/rand.
func TestDefaultHashSeedReproducible(t *testing.T) {
	opts := map[string]any{"value": "password123"}

	first := computeDefaultHash(t, "ci-seed", opts, "row-1")
	second := computeDefaultHash(t, "ci-seed", opts, "row-2")
	if first != second {
		t.Fatalf("two computations with the same seed must be identical (randomness leaking?):\n  %s\n  %s", first, second)
	}

	// The hash must actually match the requested password, otherwise the
	// "every dev account can log in" promise is cosmetic only.
	if err := bcrypt.CompareHashAndPassword([]byte(first), []byte("password123")); err != nil {
		t.Fatalf("the bcrypt verifier rejected our hash: %v (%s)", err, first)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(first), []byte("password124")); err == nil {
		t.Fatal("a wrong password should not pass verification")
	}

	// A different seed must produce a different salt (and hash).
	if computeDefaultHash(t, "other-seed", opts, "row-3") == first {
		t.Fatal("different seeds must produce different hashes")
	}

	// Golden: pins the value so an unintentional implementation or randomness
	// change is caught immediately in CI.
	const golden = "$2a$04$DP5bpFOja/TggQl/v.YV3.vUcKqChZWTFCkKcnvZ3N9qr039.gGHu"
	if first != golden {
		t.Fatalf("the default hash changed:\n  got: %s\n  want: %s", first, golden)
	}
}

func TestSequentialUnique(t *testing.T) {
	e := New("seed")
	a, _ := e.Apply("docs", "no", protocol.StrategySequential, nil, "x")
	b, _ := e.Apply("docs", "no", protocol.StrategySequential, nil, "y")
	c, _ := e.Apply("docs", "no", protocol.StrategySequential, nil, "x")
	if a == b {
		t.Fatalf("sequential must produce different values for different source values")
	}
	if a != c {
		t.Fatalf("sequential must be stable for the same source value")
	}
}

func TestUnknownStrategyFailsLoud(t *testing.T) {
	e := New("seed")
	if _, err := e.Apply("t", "c", "does_not_exist", nil, "x"); err == nil {
		t.Fatalf("an unknown strategy must error, not silently pass the original value through")
	}
}

func TestOptionsChangeOutput(t *testing.T) {
	e := New("seed")
	a, _ := e.Apply("t", "c", protocol.StrategyFakeEmail, map[string]any{"domain": "a.test"}, "x@y.z")
	b, _ := e.Apply("t", "c", protocol.StrategyFakeEmail, map[string]any{"domain": "b.test"}, "x@y.z")
	if !strings.HasSuffix(a.(string), "@a.test") || !strings.HasSuffix(b.(string), "@b.test") {
		t.Fatalf("the domain option must be honoured: %v %v", a, b)
	}
}

func TestRedact(t *testing.T) {
	e := New("seed")
	got, _ := e.Apply("t", "c", protocol.StrategyRedact, map[string]any{"keep_start": 3, "keep_end": 2}, "0812345678")
	s := got.(string)
	if !strings.HasPrefix(s, "081") || !strings.HasSuffix(s, "78") {
		t.Fatalf("redact must preserve the prefix/suffix: %q", s)
	}
	if strings.Contains(s, "2345") {
		t.Fatalf("the middle part must be masked: %q", s)
	}
}
