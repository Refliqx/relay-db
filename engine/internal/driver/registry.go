package driver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// OpError marks a failed driver operation together with the JSON-RPC code
// worth returning to the UI, so the frontend can show a specific message.
type OpError struct {
	Stage string // "dsn" | "open" | "ping" | "auth" | "version"
	Code  int
	Err   error
}

func (e *OpError) Error() string { return fmt.Sprintf("%s: %v", e.Stage, e.Err) }
func (e *OpError) Unwrap() error { return e.Err }

// Connector is the contract for one kind of database.
type Connector interface {
	Kind() string
	Label() string
	DSNExample() string
	Capabilities() []string
	BuildDSN(p protocol.Profile) (string, error)
	SQLDriver() string
	Dialect() Dialect
	// Tune is called once the connection is established (PRAGMAs, timezone,
	// and so on).
	Tune(ctx context.Context, db *sql.DB, p protocol.Profile) error
	ServerVersion(ctx context.Context, db *sql.DB) (string, error)
}

// Conn is an active connection plus the metadata the introspect and export
// packages need.
type Conn struct {
	DB      *sql.DB
	Kind    string
	Dialect Dialect
	Profile protocol.Profile

	serverVersion string
	database      string
}

func (c *Conn) ServerVersion() string { return c.serverVersion }
func (c *Conn) Database() string      { return c.database }
func (c *Conn) Close() error          { return c.DB.Close() }

var registry = map[string]Connector{}

// Register is called from each driver's init().
func Register(c Connector) { registry[c.Kind()] = c }

// Supported returns the sorted driver list for the handshake.
func Supported() []Connector {
	out := make([]Connector, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind() < out[j].Kind() })
	return out
}

func Get(kind string) (Connector, bool) {
	c, ok := registry[kind]
	return c, ok
}

// InferKind guesses the database kind from the DSN when the user has not
// chosen one.
func InferKind(dsn string) string {
	d := strings.ToLower(strings.TrimSpace(dsn))
	switch {
	case strings.HasPrefix(d, "postgres://"), strings.HasPrefix(d, "postgresql://"):
		return protocol.KindPostgres
	case strings.HasPrefix(d, "mysql://"), strings.HasPrefix(d, "mariadb://"):
		return protocol.KindMySQL
	case strings.HasPrefix(d, "libsql://"), strings.HasPrefix(d, "turso://"):
		return protocol.KindTurso
	case strings.HasPrefix(d, "sqlite:"), strings.HasPrefix(d, "file:"):
		return protocol.KindSQLite
	case strings.HasSuffix(d, ".db"), strings.HasSuffix(d, ".sqlite"), strings.HasSuffix(d, ".sqlite3"):
		return protocol.KindSQLite
	default:
		return ""
	}
}

func NormalizeKind(p protocol.Profile) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(p.Kind))
	if kind == "postgresql" || kind == "pg" {
		kind = protocol.KindPostgres
	}
	if kind == "" {
		kind = InferKind(p.DSN)
	}
	if kind == "" {
		return "", &OpError{Stage: "dsn", Code: protocol.CodeInvalidParams,
			Err: errors.New("cannot determine the database kind; set `kind` or use a DSN with an explicit scheme")}
	}
	if _, ok := registry[kind]; !ok {
		return "", &OpError{Stage: "dsn", Code: protocol.CodeKindUnsupported,
			Err: fmt.Errorf("kind %q is not supported (available: %s)", kind, strings.Join(kinds(), ", "))}
	}
	return kind, nil
}

func kinds() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Open is the single entry point for creating a connection.
func Open(ctx context.Context, p protocol.Profile) (*Conn, error) {
	kind, err := NormalizeKind(p)
	if err != nil {
		return nil, err
	}
	c := registry[kind]

	dsn, err := c.BuildDSN(p)
	if err != nil {
		return nil, &OpError{Stage: "dsn", Code: protocol.CodeInvalidParams, Err: err}
	}

	db, err := sql.Open(c.SQLDriver(), dsn)
	if err != nil {
		return nil, &OpError{Stage: "open", Code: protocol.CodeConnectionFailed, Err: err}
	}
	// RelayDB only reads: small pool, connections released quickly.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	timeout := time.Duration(p.ConnectTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, classify(kind, err)
	}
	if err := c.Tune(ctx, db, p); err != nil {
		_ = db.Close()
		return nil, &OpError{Stage: "tune", Code: protocol.CodeConnectionFailed, Err: err}
	}
	ver, err := c.ServerVersion(ctx, db)
	if err != nil {
		// A missing server version is not fatal.
		ver = "unknown"
	}

	return &Conn{
		DB:            db,
		Kind:          kind,
		Dialect:       c.Dialect(),
		Profile:       p,
		serverVersion: ver,
		database:      databaseName(kind, p),
	}, nil
}

func databaseName(kind string, p protocol.Profile) string {
	if p.Database != "" {
		return p.Database
	}
	if kind == protocol.KindSQLite {
		base := filepath.Base(p.File)
		if base == "" {
			base = filepath.Base(p.DSN)
		}
		return base
	}
	return ""
}

// classify maps a driver error to a code and message the UI can use.
func classify(kind string, err error) error {
	msg := strings.ToLower(err.Error())

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &OpError{Stage: "ping", Code: protocol.CodeConnectionFailed,
			Err: fmt.Errorf("connection timed out: host/port did not respond (%v)", err)}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &OpError{Stage: "ping", Code: protocol.CodeConnectionFailed,
			Err: fmt.Errorf("host not found: %s", dnsErr.Name)}
	}

	authMarkers := []string{
		"password authentication failed", "authentication failed",
		"access denied for user", "role \"", "invalid password",
		"no such user", "auth token", "unauthorized", "401",
	}
	for _, m := range authMarkers {
		if strings.Contains(msg, m) {
			return &OpError{Stage: "auth", Code: protocol.CodeAuthFailed, Err: err}
		}
	}

	dbMarkers := []string{"does not exist", "unknown database", "no such host"}
	for _, m := range dbMarkers {
		if strings.Contains(msg, m) {
			return &OpError{Stage: "open", Code: protocol.CodeConnectionFailed,
				Err: fmt.Errorf("database/schema not found: %v", err)}
		}
	}

	fileMarkers := []string{"no such file", "cannot open"}
	for _, m := range fileMarkers {
		if strings.Contains(msg, m) {
			return &OpError{Stage: "open", Code: protocol.CodeConnectionFailed,
				Err: fmt.Errorf("cannot open database file: %v", err)}
		}
	}

	return &OpError{Stage: "ping", Code: protocol.CodeConnectionFailed, Err: err}
}

// buildURLDSN helps URL-based drivers (postgres, turso).
func buildURLDSN(p protocol.Profile, scheme string, defaults url.Values) (string, error) {
	host := p.Host
	if host == "" {
		return "", errors.New("field `host` is required")
	}
	port := p.Port
	if port != 0 {
		host = fmt.Sprintf("%s:%d", host, port)
	}
	u := &url.URL{Scheme: scheme, Host: host, Path: "/" + strings.TrimPrefix(p.Database, "/")}
	if p.User != "" {
		if p.Password != "" {
			u.User = url.UserPassword(p.User, p.Password)
		} else {
			u.User = url.User(p.User)
		}
	}
	q := url.Values{}
	for k, v := range defaults {
		q[k] = v
	}
	for k, v := range p.Params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
