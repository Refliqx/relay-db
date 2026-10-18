package export

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/refliqx/relaydb/engine/internal/driver"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// Writer assembles a "clean & Git-friendly" .sql file.
//
// Format decisions (made deliberately, not by default):
//   - 1 table = 1 block with a marker comment: easy to review in a PR.
//   - INSERTs are batched `BatchSize` rows: a small file, but still readable
//     (not one giant INSERT that is impossible to diff).
//   - Rows are separated by "\n", indented 2 spaces, with NO trailing
//     whitespace.
//   - The header omits a timestamp by default so two runs with the same seed
//     are byte-identical -> empty diff, stable checksum.
type Writer struct {
	buf     *bufio.Writer
	dialect driver.Dialect
	opts    protocol.ExportOptions

	pending []string
	prefix  string

	bytes int64
	rows  int64
}

func NewWriter(w io.Writer, d driver.Dialect, opts protocol.ExportOptions) *Writer {
	return &Writer{
		buf:     bufio.NewWriterSize(w, 1<<16),
		dialect: d,
		opts:    withDefaults(opts),
	}
}

func (w *Writer) BytesWritten() int64 { return w.bytes }

// WriteHeader writes the information preamble plus the FK guard option.
func (w *Writer) WriteHeader(plan *protocol.ExportPlan, snapshot *protocol.SchemaSnapshot, sourceLabel string) error {
	if !w.opts.IncludeHeader {
		if w.opts.FkGuard && len(plan.Cycles) > 0 {
			return w.writeDisableFK()
		}
		return nil
	}
	var b strings.Builder
	b.WriteString("-- RelayDB seed export\n")
	b.WriteString("-- ----------------------------------------------------------------\n")
	fmt.Fprintf(&b, "-- source      : %s (%s)\n", sourceLabel, plan.Dialect)
	fmt.Fprintf(&b, "-- database    : %s\n", snapshot.Database)
	fmt.Fprintf(&b, "-- tables      : %d (ordered by the FK topological sort)\n", len(plan.Steps))
	fmt.Fprintf(&b, "-- seed        : %s\n", plan.Seed)
	fmt.Fprintf(&b, "-- batch size  : %d rows per INSERT\n", w.opts.BatchSize)
	if w.opts.HeaderTimestamp {
		fmt.Fprintf(&b, "-- generated   : %s\n", plan.CreatedAt.Format("2006-01-02T15:04:05Z"))
	}
	b.WriteString("-- Tagline: Seamlessly relay, sanitize, and prune live databases.\n")
	if len(plan.Cycles) > 0 {
		fmt.Fprintf(&b, "-- WARNING: %d FK cycles detected; importing requires an FK guard.\n", len(plan.Cycles))
	}
	b.WriteString("-- ----------------------------------------------------------------\n\n")
	if err := w.writeString(b.String()); err != nil {
		return err
	}
	if w.opts.FkGuard && len(plan.Cycles) > 0 {
		return w.writeDisableFK()
	}
	return nil
}

func (w *Writer) writeDisableFK() error {
	return w.writeString("-- FK guard: cycle relationships cannot be ordered purely topologically.\n" +
		strings.Join(w.dialect.FKDisable, "\n") + "\n\n")
}

// BeginTable opens a table block and prepares the INSERT prefix.
func (w *Writer) BeginTable(step protocol.ExportStep) error {
	w.flushPending()

	table := step.Table
	if w.dialect.SupportsSchema && step.Schema != "" && step.Schema != "main" {
		table = step.Schema + "." + step.Table
	}
	cols := make([]string, 0, len(step.Columns))
	for _, c := range step.Columns {
		cols = append(cols, w.dialect.QuoteIdent(c))
	}

	var header strings.Builder
	fmt.Fprintf(&header, "-- ----------------------------------------------------------------\n")
	fmt.Fprintf(&header, "-- Table: %s\n", table)
	if step.RowEstimate > 0 {
		fmt.Fprintf(&header, "-- Rows : ~%d\n", step.RowEstimate)
	}
	if step.Prune != nil && step.Prune.Limit != nil {
		fmt.Fprintf(&header, "-- Prune: LIMIT %d", *step.Prune.Limit)
		if len(step.Prune.OrderBy) > 0 {
			terms := make([]string, 0, len(step.Prune.OrderBy))
			for _, o := range step.Prune.OrderBy {
				terms = append(terms, o.Column+" "+strings.ToUpper(o.Direction))
			}
			fmt.Fprintf(&header, " ORDER BY %s", strings.Join(terms, ", "))
		}
		if step.Prune.Where != "" {
			fmt.Fprintf(&header, " WHERE %s", step.Prune.Where)
		}
		header.WriteString("\n")
	}
	for _, m := range step.MaskedColumns {
		fmt.Fprintf(&header, "-- Mask : %s -> %s\n", m.Column, m.Strategy)
	}
	fmt.Fprintf(&header, "-- ----------------------------------------------------------------\n")

	insert := "INSERT INTO "
	if w.opts.Upsert && w.dialect.Name == "mysql" {
		insert = "INSERT IGNORE INTO "
	}
	ref := w.dialect.TableRef(step.Schema, step.Table)
	prefix := insert + ref + " (" + strings.Join(cols, ", ") + ") VALUES"
	if w.opts.Upsert && w.dialect.Name != "mysql" {
		prefix += "\n  " // ON CONFLICT is written at the end of the batch
	}
	w.prefix = prefix

	return w.writeString(header.String())
}

// WriteRow takes values that are ALREADY masked, still in unescaped literal
// form (any), and renders their tuple.
func (w *Writer) WriteRow(values []any) error {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, w.dialect.Literal(v))
	}
	w.pending = append(w.pending, "("+strings.Join(parts, ", ")+")")
	w.rows++
	if len(w.pending) >= w.opts.BatchSize {
		return w.flushPending()
	}
	return nil
}

func (w *Writer) flushPending() error {
	if len(w.pending) == 0 {
		return nil
	}
	sep := ", "
	if w.opts.PrettyPrint {
		sep = ",\n  "
	}
	var b strings.Builder
	b.WriteString(w.prefix)
	b.WriteString("\n  ")
	b.WriteString(strings.Join(w.pending, sep))
	if w.opts.Upsert && w.dialect.Name != "mysql" {
		b.WriteString("\n")
		b.WriteString(w.dialect.UpsertConflict)
	}
	b.WriteString(";\n")
	w.pending = w.pending[:0]
	return w.writeString(b.String())
}

// EndTable closes the table block.
func (w *Writer) EndTable() error {
	if err := w.flushPending(); err != nil {
		return err
	}
	return w.writeString("\n")
}

// WriteFooter flushes the final pending INSERT batch.
func (w *Writer) WriteFooter() error {
	if err := w.flushPending(); err != nil {
		return err
	}
	return nil
}

func (w *Writer) WriteTransactionBegin() error {
	if !w.opts.WrapTransaction {
		return nil
	}
	if w.dialect.Name == "postgres" {
		// One transaction + deferrable constraint = atomic and fast import.
		return w.writeString("BEGIN;\n\n")
	}
	return w.writeString("BEGIN;\n\n")
}

func (w *Writer) WriteTransactionEnd() error {
	if !w.opts.WrapTransaction {
		return nil
	}
	return w.writeString("COMMIT;\n")
}

func (w *Writer) WriteEnableFK() error {
	if !w.opts.FkGuard {
		return nil
	}
	return w.writeString("\n" + strings.Join(w.dialect.FKEnable, "\n") + "\n")
}

func (w *Writer) Close() error {
	if err := w.flushPending(); err != nil {
		return err
	}
	return w.buf.Flush()
}

func (w *Writer) writeString(s string) error {
	n, err := w.buf.WriteString(s)
	w.bytes += int64(n)
	return err
}
