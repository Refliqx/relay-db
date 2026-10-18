package driver

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Dialect captures the RDBMS syntax differences the SQL generator needs. All
// differences are funnelled here so the export writer stays plain.
type Dialect struct {
	Name      string
	QuoteChar string // `"` for pg/sqlite, backtick for mysql
	// Placeholder turns a 1-based index into a bind placeholder.
	Placeholder func(i int) string
	// EscapeStrings marks whether backslashes need escaping (MySQL without
	// NO_BACKSLASH_ESCAPES).
	EscapeStrings bool
	// FKDisable/FKEnable are used when there are FK cycles that the topological
	// sort cannot break.
	FKDisable []string
	FKEnable  []string
	// Features
	SupportsTx       bool
	SupportsSchema   bool   // has a schema namespace (pg/mysql) vs flat (sqlite)
	UpsertConflict   string // template ON CONFLICT
	TruncateTemplate string
}

var (
	PostgresDialect = Dialect{
		Name:             "postgres",
		QuoteChar:        `"`,
		Placeholder:      func(i int) string { return "$" + strconv.Itoa(i) },
		SupportsTx:       true,
		SupportsSchema:   true,
		UpsertConflict:   "ON CONFLICT DO NOTHING",
		TruncateTemplate: `TRUNCATE TABLE %s RESTART IDENTITY CASCADE;`,
		FKDisable:        []string{`SET session_replication_role = replica;`},
		FKEnable:         []string{`SET session_replication_role = DEFAULT;`},
	}
	MySQLDialect = Dialect{
		Name:             "mysql",
		QuoteChar:        "`",
		EscapeStrings:    true,
		Placeholder:      func(i int) string { return "?" },
		SupportsTx:       true,
		SupportsSchema:   true,
		UpsertConflict:   "ON DUPLICATE KEY UPDATE id = id",
		TruncateTemplate: "TRUNCATE TABLE %s;",
		FKDisable:        []string{"SET FOREIGN_KEY_CHECKS = 0;"},
		FKEnable:         []string{"SET FOREIGN_KEY_CHECKS = 1;"},
	}
	SQLiteDialect = Dialect{
		Name:             "sqlite",
		QuoteChar:        `"`,
		Placeholder:      func(i int) string { return "?" },
		SupportsTx:       true,
		UpsertConflict:   "ON CONFLICT DO NOTHING",
		TruncateTemplate: "DELETE FROM %s;",
		FKDisable:        []string{"PRAGMA foreign_keys = OFF;"},
		FKEnable:         []string{"PRAGMA foreign_keys = ON;"},
	}
	TursoDialect = Dialect{
		Name:             "turso",
		QuoteChar:        `"`,
		Placeholder:      func(i int) string { return "?" },
		SupportsTx:       true,
		UpsertConflict:   "ON CONFLICT DO NOTHING",
		TruncateTemplate: "DELETE FROM %s;",
		FKDisable:        []string{"PRAGMA foreign_keys = OFF;"},
		FKEnable:         []string{"PRAGMA foreign_keys = ON;"},
	}
)

// QuoteIdent safely escapes an identifier (doubling the quote character).
func (d Dialect) QuoteIdent(ident string) string {
	return d.QuoteChar + strings.ReplaceAll(ident, d.QuoteChar, d.QuoteChar+d.QuoteChar) + d.QuoteChar
}

// TableRef joins schema + table name when the dialect supports schemas.
func (d Dialect) TableRef(schema, table string) string {
	if d.SupportsSchema && schema != "" {
		return d.QuoteIdent(schema) + "." + d.QuoteIdent(table)
	}
	return d.QuoteIdent(table)
}

// Literal renders a single Go value as a dialect-specific SQL literal.
// Placeholders are deliberately not used for output because we GENERATE a
// .sql file rather than running a query.
func (d Dialect) Literal(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if d.Name == "postgres" {
			if x {
				return "TRUE"
			}
			return "FALSE"
		}
		if x {
			return "1"
		}
		return "0"
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case string:
		return d.quoteString(x)
	case []byte:
		return d.literalBytes(x)
	case time.Time:
		return d.quoteString(d.formatTime(x))
	default:
		return d.quoteString(fmt.Sprint(x))
	}
}

func (d Dialect) quoteString(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	if d.EscapeStrings {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, "\x00", `\0`)
	}
	return "'" + s + "'"
}

func (d Dialect) literalBytes(b []byte) string {
	switch d.Name {
	case "postgres":
		return `'\x` + strings.ToUpper(hexEncode(b)) + `'::bytea`
	case "mysql":
		return "0x" + strings.ToUpper(hexEncode(b))
	default: // sqlite / turso
		return "X'" + strings.ToUpper(hexEncode(b)) + "'"
	}
}

func (d Dialect) formatTime(t time.Time) string {
	t = t.UTC()
	switch d.Name {
	case "postgres":
		return t.Format("2006-01-02T15:04:05.999999Z07:00")
	default:
		return t.Format("2006-01-02 15:04:05.999999")
	}
}

const hexDigits = "0123456789abcdef"

func hexEncode(b []byte) string {
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexDigits[c>>4], hexDigits[c&0x0f])
	}
	return string(out)
}
