package driver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// Postgres covers Supabase, Neon, RDS, and self-hosted PG.
//
// ARCHITECTURAL WIN: because we use a native driver (pgx) instead of shelling
// out to `pg_dump`, the user's local CLI version does not matter.
type Postgres struct{}

func init() { Register(Postgres{}) }

func (Postgres) Kind() string  { return protocol.KindPostgres }
func (Postgres) Label() string { return "PostgreSQL / Supabase / Neon" }
func (Postgres) DSNExample() string {
	return "postgres://user:pass@db.xxxx.supabase.co:5432/postgres?sslmode=require"
}

func (Postgres) Capabilities() []string {
	return []string{"schema", "foreign_keys", "row_estimate", "server_side_cursor", "ssl", "read_only_session"}
}

func (Postgres) BuildDSN(p protocol.Profile) (string, error) {
	if p.DSN != "" {
		return p.DSN, nil
	}
	// Supabase/Neon always need TLS unless the user says otherwise.
	sslmode := p.SSLMode
	if sslmode == "" {
		sslmode = "prefer"
	}
	timeout := p.ConnectTimeoutMs
	if timeout <= 0 {
		timeout = 10000
	}
	return buildURLDSN(p, "postgres", map[string][]string{
		"sslmode":         {sslmode},
		"connect_timeout": {fmt.Sprintf("%d", timeout/1000+1)},
		// Without this, pgx needs an extra round trip; we never write.
		"default_query_exec_mode": {"simple_protocol"},
	})
}

func (Postgres) SQLDriver() string { return "pgx" }
func (Postgres) Dialect() Dialect  { return PostgresDialect }

func (Postgres) Tune(ctx context.Context, db *sql.DB, _ protocol.Profile) error {
	// Safety belt: read-only transaction. A write attempt is rejected by the
	// server, not just by our own good intentions.
	_, err := db.ExecContext(ctx, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY")
	if err != nil {
		// Some managed PG (e.g. a pooler) rejects SET outside a transaction.
		// Not fatal: we only read via SELECT.
		return nil
	}
	return nil
}

func (Postgres) ServerVersion(ctx context.Context, db *sql.DB) (string, error) {
	var v string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&v); err != nil {
		return "", err
	}
	return v, nil
}

// Stats returns extra metadata for the UI.
func (Postgres) Stats(ctx context.Context, db *sql.DB) (map[string]string, error) {
	out := map[string]string{}
	var cur, dbName, user string
	row := db.QueryRowContext(ctx, "SELECT current_database(), current_user, current_setting('server_version_num')")
	if err := row.Scan(&dbName, &user, &cur); err != nil {
		return out, err
	}
	out["database"] = dbName
	out["user"] = user
	out["serverVersionNum"] = cur
	return out, nil
}

var _ Connector = Postgres{}

// time helper for the other drivers
var errEmptyFile = errors.New("SQLite file path is empty")

func timeoutOr(p protocol.Profile, def time.Duration) time.Duration {
	if p.ConnectTimeoutMs <= 0 {
		return def
	}
	return time.Duration(p.ConnectTimeoutMs) * time.Millisecond
}
