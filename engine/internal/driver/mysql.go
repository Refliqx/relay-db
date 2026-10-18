package driver

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // registers "mysql"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// MySQL covers MySQL & MariaDB (XAMPP, Laragon, RDS, PlanetScale).
type MySQL struct{}

func init() { Register(MySQL{}) }

func (MySQL) Kind() string  { return protocol.KindMySQL }
func (MySQL) Label() string { return "MySQL / MariaDB" }
func (MySQL) DSNExample() string {
	return "user:pass@tcp(127.0.0.1:3306)/dbname?parseTime=true&tls=false"
}

func (MySQL) Capabilities() []string {
	return []string{"schema", "foreign_keys", "row_estimate", "ssl", "read_only_session"}
}

func (MySQL) BuildDSN(p protocol.Profile) (string, error) {
	if p.DSN != "" {
		// Support both the URL form (mysql://user:pass@host/db) and the native
		// go-sql-driver DSN. This matters because users often paste a URL from
		// a cloud dashboard.
		if strings.HasPrefix(p.DSN, "mysql://") || strings.HasPrefix(p.DSN, "mariadb://") {
			return mysqlURLToDSN(p.DSN)
		}
		return p.DSN, nil
	}
	if p.Host == "" {
		return "", fmt.Errorf("field `host` is required")
	}
	port := p.Port
	if port == 0 {
		port = 3306
	}
	// parseTime=true -> DATETIME/TIMESTAMP are scanned as time.Time so our
	// literal generator can format them correctly.
	q := url.Values{}
	q.Set("parseTime", "true")
	q.Set("interpolateParams", "false")
	q.Set("charset", "utf8mb4")
	q.Set("timeout", timeoutOr(p, 10*time.Second).String())
	q.Set("readTimeout", "0")
	q.Set("loc", "UTC")
	if tls := p.SSLMode; tls != "" {
		switch tls {
		case "disable":
			q.Set("tls", "false")
		case "require":
			q.Set("tls", "true")
		case "skip-verify":
			q.Set("tls", "skip-verify")
		}
	} else {
		q.Set("tls", "false")
	}
	for k, v := range p.Params {
		q.Set(k, v)
	}
	userinfo := p.User
	if p.Password != "" {
		userinfo += ":" + p.Password
	}
	return fmt.Sprintf("%s@tcp(%s:%d)/%s?%s", userinfo, p.Host, port, p.Database, q.Encode()), nil
}

func mysqlURLToDSN(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":3306"
	}
	q := u.Query()
	if q.Get("parseTime") == "" {
		q.Set("parseTime", "true")
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	auth := user
	if pass != "" {
		auth += ":" + pass
	}
	return fmt.Sprintf("%s@tcp(%s)/%s?%s", auth, host, dbName, q.Encode()), nil
}

func (MySQL) SQLDriver() string { return "mysql" }
func (MySQL) Dialect() Dialect  { return MySQLDialect }

func (MySQL) Tune(ctx context.Context, db *sql.DB, _ protocol.Profile) error {
	// Read-only session: prevents accidental UPDATE/DELETE from our own code.
	_, _ = db.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
	_, _ = db.ExecContext(ctx, "SET SESSION net_write_timeout = 600")
	return nil
}

func (MySQL) ServerVersion(ctx context.Context, db *sql.DB) (string, error) {
	var v string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err != nil {
		return "", err
	}
	return v, nil
}

var _ Connector = MySQL{}
