package driver

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/refliqx/relaydb/engine/internal/protocol"
	_ "github.com/tursodatabase/libsql-client-go/libsql" // registers "libsql"
)

// Turso is a SQLite fork accessed over HTTP/WebSocket (libsql). The
// libsql-client-go driver is a pure-Go implementation, with no cgo.
type Turso struct{}

func init() { Register(Turso{}) }

func (Turso) Kind() string  { return protocol.KindTurso }
func (Turso) Label() string { return "TursoDB (libsql)" }
func (Turso) DSNExample() string {
	return "libsql://my-db-org.turso.io?authToken=eyJ..."
}

func (Turso) Capabilities() []string {
	return []string{"schema", "foreign_keys", "row_estimate", "auth_token", "embedded_replica"}
}

func (Turso) BuildDSN(p protocol.Profile) (string, error) {
	if p.DSN != "" {
		if p.AuthToken != "" && !strings.Contains(p.DSN, "authToken=") {
			sep := "?"
			if strings.Contains(p.DSN, "?") {
				sep = "&"
			}
			return p.DSN + sep + "authToken=" + url.QueryEscape(p.AuthToken), nil
		}
		return p.DSN, nil
	}
	if p.Host == "" {
		return "", fmt.Errorf("field `host` is required (e.g. my-db-org.turso.io)")
	}
	host := strings.TrimPrefix(strings.TrimPrefix(p.Host, "libsql://"), "https://")
	u := &url.URL{Scheme: "libsql", Host: host}
	if p.AuthToken == "" {
		return "", fmt.Errorf("`authToken` is required for TursoDB")
	}
	q := url.Values{}
	q.Set("authToken", p.AuthToken)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (Turso) SQLDriver() string { return "libsql" }
func (Turso) Dialect() Dialect  { return TursoDialect }

func (Turso) Tune(_ context.Context, _ *sql.DB, _ protocol.Profile) error {
	// Remote libsql does not support PRAGMA foreign_keys across connections.
	return nil
}

func (Turso) ServerVersion(ctx context.Context, db *sql.DB) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var v string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&v); err != nil {
		return "", err
	}
	return "Turso/libSQL " + v, nil
}

var _ Connector = Turso{}
