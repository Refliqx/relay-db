package driver

import (
	"strings"
	"testing"
	"time"
)

// Escaping is the only place where user data becomes SQL syntax. A bug here
// means a corrupt seed file or, worse, injection.
func TestQuoteIdent(t *testing.T) {
	if got := PostgresDialect.QuoteIdent("users"); got != `"users"` {
		t.Fatalf("got %s", got)
	}
	// An identifier containing a quote must be escaped by doubling it.
	if got := PostgresDialect.QuoteIdent(`we"ird`); got != `"we""ird"` {
		t.Fatalf("quote inside identifier not escaped: %s", got)
	}
	if got := MySQLDialect.QuoteIdent("table"); got != "`table`" {
		t.Fatalf("got %s", got)
	}
	if got := MySQLDialect.QuoteIdent("back`tick"); got != "`back``tick`" {
		t.Fatalf("backtick not escaped: %s", got)
	}
}

func TestLiteralStringEscaping(t *testing.T) {
	// A single quote is doubled for every dialect.
	for _, d := range []Dialect{PostgresDialect, MySQLDialect, SQLiteDialect} {
		got := d.Literal("O'Brien")
		if !strings.Contains(got, "O''Brien") {
			t.Fatalf("%s: single quote not escaped: %s", d.Name, got)
		}
	}
	// MySQL without NO_BACKSLASH_ESCAPES also needs backslashes escaped.
	if got := MySQLDialect.Literal(`C:\path`); !strings.Contains(got, `C:\\path`) {
		t.Fatalf("mysql: backslash not escaped: %s", got)
	}
	// Postgres/SQLite use standard literals: backslashes are NOT escaped.
	if got := PostgresDialect.Literal(`C:\path`); strings.Contains(got, `\\`) {
		t.Fatalf("postgres: backslash must not be escaped: %s", got)
	}
}

func TestLiteralInjectAttempt(t *testing.T) {
	payload := "x'); DROP TABLE users; --"
	got := PostgresDialect.Literal(payload)
	// The payload must be wrapped and every quote inside it doubled.
	want := `'x''); DROP TABLE users; --'`
	if got != want {
		t.Fatalf("wrong escaping:\n got  %s\n want %s", got, want)
	}
	// The real invariant: the payload must not appear verbatim; every quote it
	// uses as a delimiter has become a doubled quote, so it cannot close the
	// literal.
	if strings.Contains(got, payload) {
		t.Fatalf("payload leaked verbatim (possible injection): %s", got)
	}
	if strings.Count(got, "'")%2 != 0 {
		t.Fatalf("an odd quote count means the literal is unbalanced: %s", got)
	}
}

func TestLiteralScalars(t *testing.T) {
	if got := PostgresDialect.Literal(nil); got != "NULL" {
		t.Fatalf("got %s", got)
	}
	if got := PostgresDialect.Literal(true); got != "TRUE" {
		t.Fatalf("got %s", got)
	}
	if got := MySQLDialect.Literal(true); got != "1" {
		t.Fatalf("got %s", got)
	}
	if got := SQLiteDialect.Literal(false); got != "0" {
		t.Fatalf("got %s", got)
	}
	if got := PostgresDialect.Literal(int64(42)); got != "42" {
		t.Fatalf("got %s", got)
	}
}

func TestLiteralBytesPerDialect(t *testing.T) {
	blob := []byte{0x00, 0xff, 0x10}
	if got := PostgresDialect.Literal(blob); got != `'\x00FF10'::bytea` {
		t.Fatalf("wrong postgres bytea: %s", got)
	}
	if got := MySQLDialect.Literal(blob); got != "0x00FF10" {
		t.Fatalf("wrong mysql blob: %s", got)
	}
	if got := SQLiteDialect.Literal(blob); got != "X'00FF10'" {
		t.Fatalf("wrong sqlite blob: %s", got)
	}
}

func TestLiteralTimeUsesUTC(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	ts := time.Date(2026, 3, 1, 10, 30, 0, 0, loc)
	pg := PostgresDialect.Literal(ts)
	if !strings.HasPrefix(pg, "'2026-03-01T03:30:00") {
		t.Fatalf("postgres must convert to UTC: %s", pg)
	}
	my := MySQLDialect.Literal(ts)
	if !strings.HasPrefix(my, "'2026-03-01 03:30:00") {
		t.Fatalf("wrong mysql format: %s", my)
	}
}

func TestPlaceholders(t *testing.T) {
	if got := PostgresDialect.Placeholder(3); got != "$3" {
		t.Fatalf("got %s", got)
	}
	if got := SQLiteDialect.Placeholder(3); got != "?" {
		t.Fatalf("got %s", got)
	}
}

func TestInferKind(t *testing.T) {
	cases := map[string]string{
		"postgres://u@h/db":                "postgres",
		"postgresql://u@h/db":              "postgres",
		"mysql://u@h/db":                   "mysql",
		"libsql://db.turso.io?authToken=x": "turso",
		"/home/user/app.sqlite":            "sqlite",
		"file:/tmp/app.db":                 "sqlite",
		"unknown-secret":                   "",
	}
	for dsn, want := range cases {
		if got := InferKind(dsn); got != want {
			t.Errorf("%s: want %q got %q", dsn, want, got)
		}
	}
}

func TestMySQLURLToDSN(t *testing.T) {
	got, err := mysqlURLToDSN("mysql://root:secret@127.0.0.1:3307/shop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "root:secret@tcp(127.0.0.1:3307)/shop") {
		t.Fatalf("wrong DSN: %s", got)
	}
	if !strings.Contains(got, "parseTime=true") {
		t.Fatalf("parseTime must be true so DATETIME is read as time.Time: %s", got)
	}
}

func TestSupportedDriversRegistered(t *testing.T) {
	for _, kind := range []string{"postgres", "mysql", "sqlite", "turso"} {
		if _, ok := Get(kind); !ok {
			t.Errorf("driver %s is not registered", kind)
		}
	}
}
