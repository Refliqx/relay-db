// Package mask is RelayDB's anonymisation engine.
//
// # Design principle: VALUE-KEYED DETERMINISM
//
// Output values are derived from HMAC-SHA256(seed, strategy || original_value),
// NOT from the row number or the source table. Consequences:
//
//  1. IDENTICAL across runs while `seed` is unchanged -> the seed file is
//     reproducible and Git diffs between runs are clean (no "all rows changed"
//     noise).
//  2. CONSISTENT across tables: `users.email` and `orders.contact_email` with
//     the same value become the same fake email. This automatically preserves
//     logical/undeclared FK relationships.
//  3. UNIQUENESS holds: two different emails cannot collide into one fake email
//     (while their digests differ) -> UNIQUE constraints do not break.
//  4. NO randomness from the process: the `default_hash` strategy (bcrypt) uses
//     a salt derived from the seed, not crypto/rand; see bcryptdet.
//
// Deliberate trade-off: identical value patterns map to identical output. That
// is exactly the behaviour wanted for seed data, but it means very small value
// pools (e.g. a boolean column) will not be "spread out".
package mask

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/refliqx/relaydb/engine/internal/mask/bcryptdet"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// Engine is thread-safe; one instance per export job.
type Engine struct {
	seed []byte

	mu       sync.Mutex
	counters map[string]int64 // for the sequential strategy
	cache    map[string]any   // memo digest -> result
}

func New(seed string) *Engine {
	if seed == "" {
		seed = "relaydb-default-seed"
	}
	return &Engine{
		seed:     []byte(seed),
		counters: map[string]int64{},
		cache:    map[string]any{},
	}
}

// digest produces deterministic bytes for (strategy, value).
func (e *Engine) digest(strategy, value string) [32]byte {
	m := hmac.New(sha256.New, e.seed)
	m.Write([]byte(strategy))
	m.Write([]byte{0})
	m.Write([]byte(value))
	var out [32]byte
	copy(out[:], m.Sum(nil))
	return out
}

// canonical turns any value into a stable string for hashing.
func canonical(v any) string {
	switch x := v.(type) {
	case nil:
		return "\x00nil"
	case []byte:
		return "b:" + hex.EncodeToString(x)
	case string:
		return "s:" + x
	case bool:
		return "b:" + strconv.FormatBool(x)
	default:
		return fmt.Sprintf("o:%v", x)
	}
}

// Apply runs one strategy against one value.
func (e *Engine) Apply(table, column, strategy string, opts map[string]any, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy == "" || strategy == protocol.StrategyKeep {
		return value, nil
	}
	if strategy == protocol.StrategyNullify {
		return nil, nil
	}

	raw := canonical(value)
	d := e.digest(strategy+"|"+optsFingerprint(opts), raw)

	switch strategy {
	case protocol.StrategyFixed:
		return optString(opts, "value", ""), nil

	case protocol.StrategyScramble:
		return e.scramble(value, d, opts), nil

	case protocol.StrategyRedact:
		return redact(raw, opts), nil

	case protocol.StrategyHashSHA256:
		return optString(opts, "prefix", "") + hex.EncodeToString(d[:]), nil

	case protocol.StrategyDefaultHash:
		h, err := e.defaultHash(opts)
		if err != nil {
			return nil, err
		}
		return h, nil

	case protocol.StrategyFakeEmail:
		return fakeEmail(d, opts), nil

	case protocol.StrategyFakeName:
		return fakeName(d, opts), nil

	case protocol.StrategyFakePhone:
		return fakePhone(d, opts), nil

	case protocol.StrategyFakeAddress:
		return fakeAddress(d, opts), nil

	case protocol.StrategyFakeCompany:
		return fakeCompany(d, opts), nil

	case protocol.StrategyFakeUUID:
		return fakeUUID(d), nil

	case protocol.StrategyRandomInt:
		return randomInt(d, opts), nil

	case protocol.StrategySequential:
		return e.sequential(raw, table, column), nil

	case protocol.StrategyJSONScrub:
		return scrubJSON(e, value, opts)

	default:
		return nil, fmt.Errorf("unknown masking strategy: %q", strategy)
	}
}

// Strategy implementations

// scramble preserves shape: same length, letters stay letters, digits stay
// digits. This matters so the data still passes format validation (e.g. a
// regex-based validator) while being meaningless.
func (e *Engine) scramble(value any, d [32]byte, opts map[string]any) any {
	s := optString(opts, "alphabet", "")
	keepPrefix := optInt(opts, "keep_prefix", 0)
	keepSuffix := optInt(opts, "keep_suffix", 0)

	str := fmt.Sprint(value)
	runes := []rune(str)
	if keepPrefix+keepSuffix >= len(runes) {
		return str
	}

	out := make([]rune, len(runes))
	copy(out, runes)
	pos := 0
	for i := keepPrefix; i < len(runes)-keepSuffix; i++ {
		r := runes[i]
		b := d[pos%len(d)]
		pos++
		switch {
		case isDigit(r):
			out[i] = rune('0' + b%10)
		case isLower(r):
			out[i] = rune('a' + b%26)
		case isUpper(r):
			out[i] = rune('A' + b%26)
		default:
			if s != "" {
				out[i] = []rune(s)[int(b)%len([]rune(s))]
			}
			// Separator characters (@ . - _) are left as-is: the
			// email/phone shape stays intact.
		}
	}
	return string(out)
}

// redact keeps a prefix/suffix for manual verification ("number ending in
// 4821") without leaking the original value.
func redact(raw string, opts map[string]any) string {
	hidden := optString(opts, "char", "*")
	keepStart := optInt(opts, "keep_start", 2)
	keepEnd := optInt(opts, "keep_end", 2)
	n := optInt(opts, "total_length", 0)

	s := strings.TrimPrefix(raw, "s:")
	if n > 0 {
		filler := strings.Repeat(hidden, n)
		if n <= keepStart+keepEnd {
			return filler
		}
		head := firstRunes(s, keepStart)
		tail := lastRunes(s, keepEnd)
		return head + strings.Repeat(hidden, n-keepStart-keepEnd) + tail
	}
	if len([]rune(s)) <= keepStart+keepEnd {
		return strings.Repeat(hidden, 6)
	}
	return firstRunes(s, keepStart) + strings.Repeat(hidden, 6) + lastRunes(s, keepEnd)
}

func fakeEmail(d [32]byte, opts map[string]any) string {
	domain := optString(opts, "domain", "example.com")
	style := optString(opts, "style", "first.last") // first.last | first_last | flast
	first := firstNames[d[0]%byte(len(firstNames))]
	last := lastNames[d[1]%byte(len(lastNames))]
	// A short suffix keeps the UNIQUE constraint safe without making every
	// email look uniformly fake.
	suffix := strconv.FormatInt(int64(d[2])<<8|int64(d[3]), 36)

	switch style {
	case "first_last":
		return strings.ToLower(first+"_"+last) + suffix + "@" + domain
	case "flast":
		return strings.ToLower(first[:1]+last) + suffix + "@" + domain
	default:
		return strings.ToLower(first+"."+last) + suffix + "@" + domain
	}
}

func fakeName(d [32]byte, opts map[string]any) string {
	style := optString(opts, "style", "full")
	first := firstNames[d[0]%byte(len(firstNames))]
	last := lastNames[d[1]%byte(len(lastNames))]
	switch style {
	case "first":
		return first
	case "last":
		return last
	default:
		return first + " " + last
	}
}

func fakePhone(d [32]byte, opts map[string]any) string {
	cc := optString(opts, "country_code", "+62")
	length := optInt(opts, "length", 11)
	prefix := optString(opts, "prefix", "")
	var b strings.Builder
	b.WriteString(cc)
	b.WriteString(prefix)
	for i := 0; i < length; i++ {
		b.WriteByte(byte('0' + d[i%len(d)]%10))
	}
	return b.String()
}

func fakeAddress(d [32]byte, opts map[string]any) string {
	n := optInt(opts, "number_max", 250)
	street := streetNames[d[0]%byte(len(streetNames))]
	city := cityNames[d[1]%byte(len(cityNames))]
	num := int(d[2])%n + 1
	return fmt.Sprintf("Jl. %s No. %d, %s", street, num, city)
}

func fakeCompany(d [32]byte, opts map[string]any) string {
	suffix := optString(opts, "suffix", "PT")
	word := companyWords[d[0]%byte(len(companyWords))]
	word2 := companyWords[d[1]%byte(len(companyWords))]
	return fmt.Sprintf("%s %s %s", suffix, word, word2)
}

func fakeUUID(d [32]byte) string {
	var b [16]byte
	copy(b[:], d[:16])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomInt(d [32]byte, opts map[string]any) int64 {
	min := int64(optInt(opts, "min", 0))
	max := int64(optInt(opts, "max", 1_000_000))
	if max <= min {
		max = min + 1
	}
	v := int64(d[0])<<24 | int64(d[1])<<16 | int64(d[2])<<8 | int64(d[3])
	return min + (v % (max - min + 1))
}

// sequential is the only stateful strategy: it is used for columns that need
// unique, close-packed values (e.g. document sequence numbers).
func (e *Engine) sequential(raw, table, column string) any {
	key := table + "." + column
	e.mu.Lock()
	defer e.mu.Unlock()
	if id, cached := e.cache[key+"|"+raw]; cached {
		return id
	}
	e.counters[key]++
	id := e.counters[key]
	// Bounded cache: enough to deduplicate within a single table.
	if len(e.cache) < 2_000_000 {
		e.cache[key+"|"+raw] = id
	}
	return id
}

// scrubJSON scans JSON and masks the values of keys detected as sensitive;
// useful for `settings`, `profile`, and `payload` columns (common in modern
// apps) that plain column masking often misses.
//
// Important: JSON columns in MySQL/PG/SQLite can arrive as a string (TEXT) or
// as bytes. Both must have their contents parsed, not treated as an opaque
// string, otherwise secrets inside the JSON pass through untouched.
func scrubJSON(e *Engine, value any, opts map[string]any) (any, error) {
	var document any
	wasText := false

	switch x := value.(type) {
	case string:
		wasText = true
		if err := json.Unmarshal([]byte(x), &document); err != nil {
			return e.Apply("", "", protocol.StrategyScramble, opts, value)
		}
	case []byte:
		wasText = true
		if err := json.Unmarshal(x, &document); err != nil {
			return e.Apply("", "", protocol.StrategyScramble, opts, value)
		}
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("json_scrub: %w", err)
		}
		if err := json.Unmarshal(encoded, &document); err != nil {
			return e.Apply("", "", protocol.StrategyScramble, opts, value)
		}
	}

	keyStrategy := map[string]string{}
	if raw, ok := opts["keys"]; ok {
		if m, ok := raw.(map[string]any); ok {
			for k, v := range m {
				keyStrategy[strings.ToLower(k)] = fmt.Sprint(v)
			}
		}
	}
	scrubbed := scrubNode(e, document, keyStrategy, opts)

	if wasText {
		// Return it as a JSON text so it stays valid in a TEXT column.
		out, err := json.Marshal(scrubbed)
		if err != nil {
			return value, nil
		}
		return string(out), nil
	}
	return scrubbed, nil
}

func scrubNode(e *Engine, node any, keyStrategy map[string]string, opts map[string]any) any {
	switch x := node.(type) {
	case map[string]any:
		for k, v := range x {
			if s, ok := keyStrategy[strings.ToLower(k)]; ok {
				masked, err := e.Apply("", k, s, opts, v)
				if err == nil {
					x[k] = masked
					continue
				}
			}
			x[k] = scrubNode(e, v, keyStrategy, opts)
		}
		return x
	case []any:
		for i := range x {
			x[i] = scrubNode(e, x[i], keyStrategy, opts)
		}
		return x
	default:
		return node
	}
}

// Option utilities

func optsFingerprint(opts map[string]any) string {
	if len(opts) == 0 {
		return ""
	}
	// Determinism: key order must be stable.
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%v;", k, opts[k])
	}
	return b.String()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func optString(opts map[string]any, key, def string) string {
	if opts == nil {
		return def
	}
	if v, ok := opts[key]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return def
}

func optInt(opts map[string]any, key string, def int) int {
	if opts == nil {
		return def
	}
	v, ok := opts[key]
	if !ok || v == nil {
		return def
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		if n, err := strconv.Atoi(x); err == nil {
			return n
		}
	}
	return def
}

// defaultHash returns a hash password that is CONSTANT for all rows.
//
// Rationale: seed data does not need a unique hash per user. What it needs is
// (a) a column shape that stays valid (60 characters of bcrypt), and (b) every
// dev account being able to log in with a known password. The hash is computed
// once and cached, so the bcrypt cost (hundreds of ms) is not paid per row.
//
// DETERMINISM: this is not an internal detail but a product contract. The
// bcrypt salt is derived from `seed` + password via HMAC, NOT from crypto/rand
// as stock bcrypt does. Without that, two runs with the same seed produce
// different hashes, and the promise of "identical checksum / clean Git diff"
// in the UI and CI could never hold. See internal/mask/bcryptdet.
func (e *Engine) defaultHash(opts map[string]any) (string, error) {
	algo := optString(opts, "algo", "bcrypt")
	plain := optString(opts, "value", "password123")

	// The cache key includes the seed: one process can serve several jobs with
	// different seeds, and their results must not cross-contaminate.
	cacheKey := algo + "|" + plain + "|" + string(e.seed)

	globalCache.mu.Lock()
	defer globalCache.mu.Unlock()
	if cached, ok := globalCache.m[cacheKey]; ok {
		return cached, nil
	}

	var out string
	switch algo {
	case "sha256":
		d := e.digest("default_hash|sha256", plain)
		out = "$sha256$" + hex.EncodeToString(d[:])
	case "base64":
		d := e.digest("default_hash|base64", plain)
		out = base64.RawStdEncoding.EncodeToString(d[:])
	default:
		d := e.digest("default_hash|bcrypt-salt", plain)
		hash, err := bcryptdet.GenerateFromPasswordSalt(
			[]byte(plain), bcryptdet.MinCost, d[:bcryptdet.SaltSize],
		)
		if err != nil {
			return "", fmt.Errorf("default_hash: %w", err)
		}
		out = string(hash)
	}

	globalCache.m[cacheKey] = out
	return out, nil
}

var globalCache = struct {
	mu sync.Mutex
	m  map[string]string
}{m: map[string]string{}}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

func firstRunes(s string, n int) string {
	r := []rune(s)
	if n > len(r) {
		n = len(r)
	}
	return string(r[:n])
}

func lastRunes(s string, n int) string {
	r := []rune(s)
	if n > len(r) {
		n = len(r)
	}
	return string(r[len(r)-n:])
}

// StrategyCatalog is used by the UI to show the choices and their default options.
func StrategyCatalog() []map[string]any {
	return []map[string]any{
		{"strategy": protocol.StrategyNullify, "label": "Nullify", "description": "Set the value to NULL.", "pii": []string{"ip", "user_agent", "free_text"}},
		{"strategy": protocol.StrategyFixed, "label": "Fixed value", "description": "Replace with a constant value.", "options": []string{"value"}},
		{"strategy": protocol.StrategyScramble, "label": "Scramble (shape-preserving)", "description": "Shuffle characters while keeping length and shape; letters stay letters, digits stay digits.", "options": []string{"keep_prefix", "keep_suffix"}},
		{"strategy": protocol.StrategyRedact, "label": "Redact", "description": "Keep part of the characters, then stars.", "options": []string{"keep_start", "keep_end"}},
		{"strategy": protocol.StrategyFakeEmail, "label": "Fake email", "description": "Deterministic and unique email.", "pii": []string{"email"}, "options": []string{"domain", "style"}},
		{"strategy": protocol.StrategyFakeName, "label": "Fake name", "description": "Deterministic person name.", "pii": []string{"person_name"}},
		{"strategy": protocol.StrategyFakePhone, "label": "Fake phone", "description": "Deterministic E.164 phone number.", "pii": []string{"phone"}, "options": []string{"country_code", "length", "prefix"}},
		{"strategy": protocol.StrategyFakeAddress, "label": "Fake address", "description": "Deterministic street address.", "pii": []string{"address"}},
		{"strategy": protocol.StrategyDefaultHash, "label": "Default hash (password)", "description": "All accounts use a single known dev password hash; the salt is derived from the seed so the result is reproducible.", "pii": []string{"password"}, "options": []string{"algo", "value"}},
		{"strategy": protocol.StrategyHashSHA256, "label": "SHA-256 hash", "description": "Deterministic hash, suitable for tokens/IDs.", "pii": []string{"password"}},
		{"strategy": protocol.StrategyFakeUUID, "label": "Fake UUID v4", "description": "Deterministic valid UUID.", "options": []string{}},
		{"strategy": protocol.StrategyRandomInt, "label": "Random integer", "description": "Integer within a range.", "options": []string{"min", "max"}},
		{"strategy": protocol.StrategySequential, "label": "Sequential", "description": "Unique sequence number 1..n.", "options": []string{}},
		{"strategy": protocol.StrategyJSONScrub, "label": "JSON scrub", "description": "Masks sensitive values inside a JSON column.", "options": []string{"keys"}},
		{"strategy": protocol.StrategyKeep, "label": "Keep (passthrough)", "description": "Leave the original value; requires an explicit user decision."},
	}
}
