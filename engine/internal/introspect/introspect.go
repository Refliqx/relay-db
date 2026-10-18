// Package introspect builds a portable schema snapshot (tables, columns, FKs)
// across dialects, then derives the relationship graph and an execution order
// that is safe with respect to foreign key constraints.
package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/refliqx/relaydb/engine/internal/driver"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

type Options struct {
	ExactRowCount bool
	IncludeViews  bool
}

// rowCountProbeLimit caps COUNT(*) on SQLite, which has no row estimate
// statistics: enough to tell "small" from "large".
const rowCountProbeLimit = 100_000

// Snapshot is the main entry point.
func Snapshot(ctx context.Context, c *driver.Conn, opts Options) (*protocol.SchemaSnapshot, error) {
	var (
		tables []protocol.Table
		fks    []protocol.ForeignKey
		err    error
	)
	switch c.Kind {
	case protocol.KindPostgres:
		tables, fks, err = snapshotPostgres(ctx, c, opts)
	case protocol.KindMySQL, protocol.KindMariaDB:
		tables, fks, err = snapshotMySQL(ctx, c, opts)
	default:
		tables, fks, err = snapshotSQLite(ctx, c, opts, c.Kind)
	}
	if err != nil {
		return nil, &driver.OpError{Stage: "introspect", Code: protocol.CodeIntrospectFailed, Err: err}
	}

	// Inject the PII heuristic so the UI can suggest masking rules.
	for ti := range tables {
		for ci := range tables[ti].Columns {
			col := &tables[ti].Columns[ci]
			if col.PiiGuess == "" {
				col.PiiGuess, col.PiiConfidence = GuessPII(col.Name, col.DataType)
			}
		}
	}
	sortTables(tables)

	return &protocol.SchemaSnapshot{
		Dialect:     c.Dialect.Name,
		Database:    c.Database(),
		Tables:      tables,
		ForeignKeys: fks,
		CapturedAt:  time.Now().UTC(),
	}, nil
}

func sortTables(tables []protocol.Table) {
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Schema != tables[j].Schema {
			return tables[i].Schema < tables[j].Schema
		}
		return tables[i].Name < tables[j].Name
	})
}

// PostgreSQL (Supabase / Neon / RDS)

func snapshotPostgres(ctx context.Context, c *driver.Conn, opts Options) ([]protocol.Table, []protocol.ForeignKey, error) {
	// reltuples is the estimate from autovacuum/ANALYZE; -1 means it has never
	// been analysed, so we fall back to an exact COUNT(*).
	const tableQuery = `
SELECT n.nspname AS schema,
       c.relname AS name,
       CASE c.relkind WHEN 'r' THEN 'table' WHEN 'p' THEN 'table'
                      WHEN 'v' THEN 'view'  WHEN 'm' THEN 'view' END AS kind,
       GREATEST(c.reltuples, -1)::bigint AS est
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p','v','m')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND n.nspname NOT LIKE 'pg\_temp%'
ORDER BY n.nspname, c.relname`

	rows, err := c.DB.QueryContext(ctx, tableQuery)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	index := map[string]int{}
	var tables []protocol.Table
	for rows.Next() {
		var t protocol.Table
		var est sql.NullInt64
		if err := rows.Scan(&t.Schema, &t.Name, &t.Kind, &est); err != nil {
			return nil, nil, err
		}
		if t.Kind == "view" && !opts.IncludeViews {
			continue
		}
		t.EstimatedRows = est.Int64
		index[t.Schema+"."+t.Name] = len(tables)
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	const columnQuery = `
SELECT table_schema, table_name, column_name, ordinal_position, data_type,
       is_nullable, column_default, character_maximum_length, is_identity, is_generated
FROM information_schema.columns
WHERE table_schema NOT IN ('pg_catalog','information_schema')
ORDER BY table_schema, table_name, ordinal_position`
	crows, err := c.DB.QueryContext(ctx, columnQuery)
	if err != nil {
		return nil, nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var schema, table, name, dataType, isNullable, isIdentity, isGenerated string
		var ordinal int
		var def sql.NullString
		var maxLen sql.NullInt64
		if err := crows.Scan(&schema, &table, &name, &ordinal, &dataType, &isNullable, &def, &maxLen, &isIdentity, &isGenerated); err != nil {
			return nil, nil, err
		}
		m, ok := index[schema+"."+table]
		if !ok {
			continue
		}
		col := protocol.Column{
			Name:            name,
			OrdinalPosition: ordinal,
			DataType:        dataType,
			Nullable:        strings.EqualFold(isNullable, "YES"),
			IsAutoIncrement: strings.EqualFold(isIdentity, "YES"),
			IsGenerated:     isGenerated != "NEVER" && isGenerated != "",
		}
		if def.Valid {
			d := def.String
			col.Default = &d
		}
		if maxLen.Valid {
			n := maxLen.Int64
			col.MaxLength = &n
		}
		tables[m].Columns = append(tables[m].Columns, col)
	}
	if err := crows.Err(); err != nil {
		return nil, nil, err
	}

	const pkQuery = `
SELECT tc.table_schema, tc.table_name, kcu.column_name, kcu.ordinal_position
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
WHERE tc.constraint_type = 'PRIMARY KEY'
ORDER BY tc.table_schema, tc.table_name, kcu.ordinal_position`
	prows, err := c.DB.QueryContext(ctx, pkQuery)
	if err != nil {
		return nil, nil, err
	}
	applyPrimaryKeys(tables, index, prows)

	// unnest(conkey, confkey) WITH ORDINALITY -> safe for composite FKs.
	const fkQuery = `
SELECT con.conname,
       nsp.nspname, rel.relname, att.attname,
       fnsp.nspname, frel.relname, fatt.attname,
       con.confdeltype::text, con.confupdtype::text
FROM pg_constraint con
JOIN pg_class rel   ON rel.oid = con.conrelid
JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
JOIN pg_class frel  ON frel.oid = con.confrelid
JOIN pg_namespace fnsp ON fnsp.oid = frel.relnamespace
JOIN LATERAL unnest(con.conkey, con.confkey) WITH ORDINALITY AS k(attnum, fattnum, ord) ON TRUE
JOIN pg_attribute att  ON att.attrelid = con.conrelid  AND att.attnum = k.attnum
JOIN pg_attribute fatt ON fatt.attrelid = con.confrelid AND fatt.attnum = k.fattnum
WHERE con.contype = 'f'
  AND nsp.nspname NOT IN ('pg_catalog','information_schema')
ORDER BY nsp.nspname, rel.relname, con.conname, k.ord`

	frows, err := c.DB.QueryContext(ctx, fkQuery)
	if err != nil {
		return nil, nil, err
	}
	defer frows.Close()
	var fks []protocol.ForeignKey
	for frows.Next() {
		var fk protocol.ForeignKey
		var delType, updType string
		if err := frows.Scan(&fk.ConstraintName, &fk.Schema, &fk.Table, &fk.Column,
			&fk.RefSchema, &fk.RefTable, &fk.RefColumn, &delType, &updType); err != nil {
			return nil, nil, err
		}
		fk.OnDelete = pgAction(delType)
		fk.OnUpdate = pgAction(updType)
		fks = append(fks, fk)
	}
	if err := frows.Err(); err != nil {
		return nil, nil, err
	}
	return tables, fks, nil
}

func pgAction(code string) string {
	switch code {
	case "c":
		return "CASCADE"
	case "n":
		return "SET NULL"
	case "d":
		return "SET DEFAULT"
	case "r":
		return "RESTRICT"
	default:
		return "NO ACTION"
	}
}

// MySQL / MariaDB

func snapshotMySQL(ctx context.Context, c *driver.Conn, opts Options) ([]protocol.Table, []protocol.ForeignKey, error) {
	schemaFilter := c.Profile.Database
	tableQuery := `
SELECT table_schema, table_name, table_type, COALESCE(table_rows, 0)
FROM information_schema.tables
WHERE table_schema NOT IN ('mysql','information_schema','performance_schema','sys')
  AND table_schema NOT LIKE '\_%'
ORDER BY table_schema, table_name`

	rows, err := c.DB.QueryContext(ctx, tableQuery)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	index := map[string]int{}
	var tables []protocol.Table
	for rows.Next() {
		var t protocol.Table
		var tableType string
		if err := rows.Scan(&t.Schema, &t.Name, &tableType, &t.EstimatedRows); err != nil {
			return nil, nil, err
		}
		t.Kind = "table"
		if strings.EqualFold(tableType, "VIEW") {
			t.Kind = "view"
			if !opts.IncludeViews {
				continue
			}
		}
		if schemaFilter != "" && t.Schema != schemaFilter {
			continue
		}
		index[t.Schema+"."+t.Name] = len(tables)
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	colQuery := `
SELECT table_schema, table_name, column_name, ordinal_position, column_type,
       is_nullable, column_default, character_maximum_length, column_key, extra
FROM information_schema.columns
WHERE table_schema NOT IN ('mysql','information_schema','performance_schema','sys')
ORDER BY table_schema, table_name, ordinal_position`
	crows, err := c.DB.QueryContext(ctx, colQuery)
	if err != nil {
		return nil, nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var schema, table, name, dataType, isNullable, columnKey, extra string
		var ordinal int
		var def sql.NullString
		var maxLen sql.NullInt64
		if err := crows.Scan(&schema, &table, &name, &ordinal, &dataType, &isNullable, &def, &maxLen, &columnKey, &extra); err != nil {
			return nil, nil, err
		}
		m, ok := index[schema+"."+table]
		if !ok {
			continue
		}
		col := protocol.Column{
			Name:            name,
			OrdinalPosition: ordinal,
			DataType:        dataType,
			Nullable:        strings.EqualFold(isNullable, "YES"),
			IsPrimaryKey:    columnKey == "PRI",
			IsAutoIncrement: strings.Contains(strings.ToLower(extra), "auto_increment"),
			IsGenerated:     strings.Contains(strings.ToLower(extra), "generated"),
		}
		if def.Valid {
			d := def.String
			col.Default = &d
		}
		if maxLen.Valid {
			n := maxLen.Int64
			col.MaxLength = &n
		}
		if col.IsPrimaryKey {
			tables[m].PrimaryKey = append(tables[m].PrimaryKey, name)
		}
		tables[m].Columns = append(tables[m].Columns, col)
	}
	if err := crows.Err(); err != nil {
		return nil, nil, err
	}

	fkQuery := `
SELECT kcu.constraint_name, kcu.table_schema, kcu.table_name, kcu.column_name,
       kcu.referenced_table_schema, kcu.referenced_table_name, kcu.referenced_column_name,
       rc.delete_rule, rc.update_rule
FROM information_schema.key_column_usage kcu
JOIN information_schema.referential_constraints rc
  ON rc.constraint_name = kcu.constraint_name AND rc.constraint_schema = kcu.constraint_schema
WHERE kcu.referenced_table_name IS NOT NULL
ORDER BY kcu.table_schema, kcu.table_name, kcu.constraint_name, kcu.ordinal_position`
	frows, err := c.DB.QueryContext(ctx, fkQuery)
	if err != nil {
		return nil, nil, err
	}
	defer frows.Close()
	var fks []protocol.ForeignKey
	for frows.Next() {
		var fk protocol.ForeignKey
		var del, upd sql.NullString
		if err := frows.Scan(&fk.ConstraintName, &fk.Schema, &fk.Table, &fk.Column,
			&fk.RefSchema, &fk.RefTable, &fk.RefColumn, &del, &upd); err != nil {
			return nil, nil, err
		}
		fk.OnDelete, fk.OnUpdate = del.String, upd.String
		if _, ok := index[fk.Schema+"."+fk.Table]; !ok {
			continue
		}
		fks = append(fks, fk)
	}
	return tables, fks, frows.Err()
}

// SQLite / Turso (libsql)

func snapshotSQLite(ctx context.Context, c *driver.Conn, opts Options, kind string) ([]protocol.Table, []protocol.ForeignKey, error) {
	masterQuery := `SELECT name, type, COALESCE(sql,'') FROM sqlite_master
	                WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%'
	                ORDER BY name`
	rows, err := c.DB.QueryContext(ctx, masterQuery)
	if err != nil {
		return nil, nil, err
	}
	type obj struct {
		name, kind, ddl string
	}
	var objs []obj
	for rows.Next() {
		var o obj
		if err := rows.Scan(&o.name, &o.kind, &o.ddl); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if o.kind == "view" && !opts.IncludeViews {
			continue
		}
		objs = append(objs, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var tables []protocol.Table
	var fks []protocol.ForeignKey

	for _, o := range objs {
		t := protocol.Table{Schema: "main", Name: o.name, Kind: o.kind, EstimatedRows: -1}
		if o.kind == "table" {
			count, exact := sqliteRowCount(ctx, c, o.name, opts.ExactRowCount)
			t.EstimatedRows = count
			t.RowCountExact = exact
		}

		// PRAGMA table_info -> cid, name, type, notnull, dflt_value, pk
		infoRows, err := c.DB.QueryContext(ctx, `PRAGMA table_info(`+quoteSQLite(o.name)+`)`)
		if err != nil {
			return nil, nil, err
		}
		for infoRows.Next() {
			var cid, notnull, pk int
			var name, dataType string
			var dflt sql.NullString
			if err := infoRows.Scan(&cid, &name, &dataType, &notnull, &dflt, &pk); err != nil {
				infoRows.Close()
				return nil, nil, err
			}
			col := protocol.Column{
				Name:            name,
				OrdinalPosition: cid + 1,
				DataType:        dataType,
				Nullable:        notnull == 0,
				IsPrimaryKey:    pk > 0,
				IsAutoIncrement: pk == 1 && strings.EqualFold(dataType, "INTEGER") &&
					strings.Contains(strings.ToUpper(o.ddl), "AUTOINCREMENT"),
			}
			if dflt.Valid {
				d := dflt.String
				col.Default = &d
			}
			if pk > 0 {
				t.PrimaryKey = append(t.PrimaryKey, name)
			}
			t.Columns = append(t.Columns, col)
		}
		infoRows.Close()

		// PRAGMA foreign_key_list -> id, seq, table, from, to, on_update, on_delete, match
		fkRows, err := c.DB.QueryContext(ctx, `PRAGMA foreign_key_list(`+quoteSQLite(o.name)+`)`)
		if err != nil {
			return nil, nil, err
		}
		for fkRows.Next() {
			var id, seq int
			var refTable, from string
			var to sql.NullString
			var onUpdate, onDelete, match string
			if err := fkRows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				fkRows.Close()
				return nil, nil, err
			}
			refCol := to.String
			if refCol == "" {
				refCol = "rowid"
			}
			fks = append(fks, protocol.ForeignKey{
				ConstraintName: fmt.Sprintf("fk_%s_%d_%d", o.name, id, seq),
				Schema:         "main", Table: o.name, Column: from,
				RefSchema: "main", RefTable: refTable, RefColumn: refCol,
				OnDelete: onDelete, OnUpdate: onUpdate,
			})
		}
		fkRows.Close()
		tables = append(tables, t)
	}
	_ = kind
	return tables, fks, nil
}

// sqliteRowCount uses a bounded probe so large tables do not block the UI.
func sqliteRowCount(ctx context.Context, c *driver.Conn, table string, exact bool) (int64, bool) {
	ref := quoteSQLite(table)
	if exact {
		var n int64
		if err := c.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+ref).Scan(&n); err == nil {
			return n, true
		}
		return -1, false
	}
	probe := fmt.Sprintf(`SELECT COUNT(*) FROM (SELECT 1 FROM %s LIMIT %d)`, ref, rowCountProbeLimit+1)
	var n int64
	if err := c.DB.QueryRowContext(ctx, probe).Scan(&n); err != nil {
		return -1, false
	}
	if n > rowCountProbeLimit {
		return -1, false // "100k+"
	}
	return n, true
}

func quoteSQLite(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// Shared helpers

func applyPrimaryKeys(tables []protocol.Table, index map[string]int, rows *sql.Rows) {
	defer rows.Close()
	for rows.Next() {
		var schema, table, column string
		var ordinal int
		if err := rows.Scan(&schema, &table, &column, &ordinal); err != nil {
			return
		}
		m, ok := index[schema+"."+table]
		if !ok {
			continue
		}
		tables[m].PrimaryKey = append(tables[m].PrimaryKey, column)
		for ci := range tables[m].Columns {
			if tables[m].Columns[ci].Name == column {
				tables[m].Columns[ci].IsPrimaryKey = true
			}
		}
	}
}
