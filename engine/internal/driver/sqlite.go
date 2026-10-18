package driver

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/refliqx/relaydb/engine/internal/protocol"
	_ "modernc.org/sqlite" // pure-Go driver (no cgo) -> easy cross-compile
)

// SQLite uses the modernc.org/sqlite driver (pure Go) so the sidecar binary
// can still be cross-compiled to macOS/Windows without a C toolchain.
//
// | Driver          | CGO  | Cross-compile | Note                 |
// |-----------------|------|---------------|----------------------|
// | mattn/go-sqlite3| yes  | hard          | needs gcc per target |
// | modernc/sqlite  | no   | easy          | chosen by RelayDB    |
type SQLite struct{}

func init() { Register(SQLite{}) }

func (SQLite) Kind() string  { return protocol.KindSQLite }
func (SQLite) Label() string { return "SQLite (.db / .sqlite / .sqlite3)" }
func (SQLite) DSNExample() string {
	return `sqlite:///home/user/app.db  (or choose a file via the dialog)`
}

func (SQLite) Capabilities() []string {
	return []string{"file", "foreign_keys", "no_row_estimate", "attach"}
}

func (SQLite) BuildDSN(p protocol.Profile) (string, error) {
	path := p.File
	if path == "" {
		path = strings.TrimPrefix(p.DSN, "sqlite://")
		path = strings.TrimPrefix(path, "sqlite:")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errEmptyFile
	}
	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("file not found: %s", abs)
	} else if st.IsDir() {
		return "", fmt.Errorf("path is a directory, not a SQLite file: %s", abs)
	}
	// mode=ro -> open read-only at the OS level; this is the strongest
	// guarantee that the user's database will not be modified.
	q := url.Values{}
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Set("_time_format", "sqlite")
	return "file:" + filepath.ToSlash(abs) + "?" + q.Encode(), nil
}

func (SQLite) SQLDriver() string { return "sqlite" }
func (SQLite) Dialect() Dialect  { return SQLiteDialect }

func (SQLite) Tune(ctx context.Context, db *sql.DB, _ protocol.Profile) error {
	// Read FK metadata so the topological sort knows the graph.
	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	return err
}

func (SQLite) ServerVersion(ctx context.Context, db *sql.DB) (string, error) {
	var v string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&v); err != nil {
		return "", err
	}
	return "SQLite " + v, nil
}

var _ Connector = SQLite{}
