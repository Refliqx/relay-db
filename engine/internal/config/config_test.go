package config

import (
	"os"
	"strings"
	"testing"

	"github.com/refliqx/relaydb/engine/internal/protocol"
)

func TestParseValidPreset(t *testing.T) {
	raw := []byte(`
version: 1
profile:
  id: local
  kind: sqlite
  file: ./app.db
masking:
  - { table: users, column: email, strategy: fake_email }
pruning:
  - { table: logs, limit: 100 }
export:
  outputPath: ./seeds/seed.sql
  seed: s1
`)
	f, issues, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range issues {
		if is.Severity == "error" {
			t.Fatalf("there must be no errors: %+v", is)
		}
	}
	if len(f.Masking) != 1 || f.Masking[0].Strategy != protocol.StrategyFakeEmail {
		t.Fatalf("masking was not parsed: %+v", f.Masking)
	}
	if len(f.AllProfiles()) != 1 {
		t.Fatalf("a single profile must be normalised into AllProfiles()")
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	// KnownFields(true): a typo must FAIL, not be silently ignored.
	raw := []byte("version: 1\nprofile: { id: a, kind: sqlite, file: ./x.db }\nmaskingg: []\n")
	if _, _, err := Parse(raw); err == nil {
		t.Fatal("an unknown key should raise an error")
	}
}

func TestEnvExpansion(t *testing.T) {
	t.Setenv("RELAYDB_TEST_PASS", "s3cret")
	t.Setenv("RELAYDB_TEST_HOST", "db.internal")
	raw := []byte(`
version: 1
profiles:
  - id: a
    kind: postgres
    host: ${RELAYDB_TEST_HOST}
    user: app
    password: ${RELAYDB_TEST_PASS}
  - id: b
    kind: mysql
    host: localhost
    user: root
    password: ${RELAYDB_MISSING:-default-secret}
`)
	f, _, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	profiles := f.AllProfiles()
	if profiles[0].Password != "s3cret" || profiles[0].Host != "db.internal" {
		t.Fatalf("env expansion failed: %+v", profiles[0])
	}
	if profiles[1].Password != "default-secret" {
		t.Fatalf("env default failed: %+v", profiles[1])
	}
}

func TestEnvExpansionLeavesUnknownBare(t *testing.T) {
	os.Unsetenv("RELAYDB_UNSET")
	raw := []byte("version: 1\nprofile: { id: a, kind: mysql, host: h, user: u, password: '${RELAYDB_UNSET}' }\n")
	f, _, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.AllProfiles()[0].Password; got != "" {
		t.Fatalf("a variable without a default must become an empty string, not a literal: %q", got)
	}
}

func TestValidateCatchesProblems(t *testing.T) {
	f := &File{
		Version: 1,
		Profiles: []protocol.Profile{
			{ID: "dup", Kind: protocol.KindMySQL, Host: "h"},
			{ID: "dup", Kind: protocol.KindMySQL, Host: "h"},
			{ID: "sqlite-no-file", Kind: protocol.KindSQLite},
			{ID: "empty-kind"},
		},
		Masking: []protocol.MaskRule{
			{Table: "users", Column: "email", Strategy: "garbage-strategy"},
			{Table: "", Column: "", Strategy: protocol.StrategyNullify},
		},
		Pruning: []protocol.PruneRule{
			{Table: "logs", Where: "1=1; DROP TABLE users"},
		},
	}
	issues := Validate(f)

	need := map[string]bool{
		"duplicate": false,
		"file":      false,
		"kind":      false,
		"unknown":   false,
		"forbidden": false,
	}
	for _, is := range issues {
		if is.Severity != "error" {
			continue
		}
		for key := range need {
			if strings.Contains(strings.ToLower(is.Message), key) || strings.Contains(strings.ToLower(is.Path), key) {
				need[key] = true
			}
		}
	}
	for key, found := range need {
		if !found {
			t.Errorf("validation did not catch problem %q; issues=%+v", key, issues)
		}
	}
}

func TestWhereGuardrails(t *testing.T) {
	bad := []string{
		"1=1; DROP TABLE users",
		"status = 'x' -- comment",
		"id IN (SELECT 1) UNION UPDATE t SET x=1",
		"1=1;/*",
		"active = 1; DELETE FROM t",
		"pg_sleep(10)",
	}
	for _, w := range bad {
		if _, isBad := inspectWhere(w); !isBad {
			t.Errorf("a dangerous clause must be rejected: %q", w)
		}
	}
	good := []string{
		"status = 'active'",
		"created_at >= '2026-01-01'",
		"updated_at < '2026-06-01' AND deleted_at IS NULL",
		"level IN ('error','warn') AND retry_count > 3",
		"deleted_at IS NULL",
	}
	for _, w := range good {
		if msg, isBad := inspectWhere(w); isBad {
			t.Errorf("a legitimate read filter must be accepted: %q (%s)", w, msg)
		}
	}
}

func TestApplyDefaults(t *testing.T) {
	f := &File{Version: 1, Masking: []protocol.MaskRule{{Table: "u", Column: "e", Strategy: protocol.StrategyFakeEmail}}}
	ApplyDefaults(f)
	if f.Export.BatchSize != 100 {
		t.Fatalf("wrong batchSize default: %d", f.Export.BatchSize)
	}
	if !f.Export.IncludeHeader {
		t.Fatalf("the header default must be enabled")
	}
	if f.Export.HeaderTimestamp {
		t.Fatalf("the timestamp default must be DISABLED so output is reproducible")
	}
	if f.Masking[0].Options["domain"] != "example.com" {
		t.Fatalf("strategy default options were not applied: %+v", f.Masking[0].Options)
	}
}
