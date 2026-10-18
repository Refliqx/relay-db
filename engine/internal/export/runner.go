package export

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/refliqx/relaydb/engine/internal/driver"
	"github.com/refliqx/relaydb/engine/internal/mask"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// Runner executes the plan: streaming SELECT -> masking -> write .sql.
//
// Important: rows are NEVER buffered entirely in memory. We read row by row
// from the DB cursor and write straight through -> constant memory footprint
// (O(1) per row), so a ten-million-row table stays safe.
type Runner struct {
	OnProgress  func(protocol.ExportProgress)
	OnTableDone func(protocol.TableReport)
	OnLog       func(level, msg string)
}

func (r *Runner) progress(p protocol.ExportProgress) {
	if r.OnProgress != nil {
		r.OnProgress(p)
	}
}

func (r *Runner) log(level, format string, args ...any) {
	if r.OnLog != nil {
		r.OnLog(level, fmt.Sprintf(format, args...))
	}
}

// Run executes the plan and returns the aggregate report.
func (r *Runner) Run(ctx context.Context, conn *driver.Conn, plan *protocol.ExportPlan, opts protocol.ExportOptions) (*protocol.ExportResult, error) {
	opts = withDefaults(opts)
	started := time.Now()
	engine := mask.New(opts.Seed)

	result := &protocol.ExportResult{
		JobID:      plan.PlanID,
		OutputPath: opts.OutputPath,
		Warnings:   plan.Warnings,
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("plan is empty")
	}

	var rowsTotal int64
	for _, s := range plan.Steps {
		rowsTotal += max64(s.RowEstimate, 0)
	}

	r.progress(protocol.ExportProgress{
		JobID:      plan.PlanID,
		Phase:      "streaming",
		TableCount: len(plan.Steps),
		RowsTotal:  rowsTotal,
		Message:    fmt.Sprintf("starting export of %d tables", len(plan.Steps)),
	})

	sink, err := newSink(opts.OutputPath, conn.Dialect, opts)
	if err != nil {
		return nil, fmt.Errorf("preparing output file: %w", err)
	}
	defer sink.Cleanup()

	writer := sink.writer
	if err := writer.WriteHeader(plan, &protocol.SchemaSnapshot{Dialect: conn.Dialect.Name, Database: conn.Database()}, conn.Kind); err != nil {
		return nil, err
	}
	if err := writer.WriteTransactionBegin(); err != nil {
		return nil, err
	}

	var processedTotal int64
	for idx, step := range plan.Steps {
		if err := ctx.Err(); err != nil {
			return nil, err // -> JSON-RPC CodeCancelled
		}
		stepStarted := time.Now()
		bytesBefore := sink.writer.BytesWritten()
		report := protocol.TableReport{Schema: step.Schema, Table: step.Table}

		if sink.split {
			if err := sink.rotate(idx, step); err != nil {
				return nil, err
			}
			writer = sink.writer
		}
		if err := writer.BeginTable(step); err != nil {
			return nil, err
		}

		rows, err := streamTable(ctx, conn, step)
		if err != nil {
			return nil, fmt.Errorf("reading table %s: %w", step.Table, err)
		}

		maskIdx := map[int]protocol.MaskRule{}
		for i, col := range step.Columns {
			for _, rule := range step.MaskedColumns {
				if strings.EqualFold(rule.Column, col) {
					maskIdx[i] = rule
				}
			}
		}

		cols := make([]any, len(step.Columns))
		ptrs := make([]any, len(step.Columns))
		for i := range cols {
			ptrs[i] = &cols[i]
		}

		tableRows := int64(0)
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning rows of %s: %w", step.Table, err)
			}
			// Type normalisation + masking happen inline.
			for i := range cols {
				v := normalize(cols[i])
				if rule, ok := maskIdx[i]; ok && v != nil {
					mv, err := engine.Apply(step.Table, step.Columns[i], rule.Strategy, rule.Options, v)
					if err != nil {
						rows.Close()
						return nil, fmt.Errorf("masking %s.%s: %w", step.Table, step.Columns[i], err)
					}
					v = mv
					report.MaskedCells++
				}
				cols[i] = v
			}
			if err := writer.WriteRow(cols); err != nil {
				rows.Close()
				return nil, fmt.Errorf("writing rows of %s: %w", step.Table, err)
			}

			tableRows++
			processedTotal++
			if tableRows%int64(opts.ProgressEveryN) == 0 {
				r.emitProgress(plan, idx, step, processedTotal, rowsTotal, started, tableRows, sink.writer.BytesWritten())
			}
		}
		scanErr := rows.Err()
		rows.Close()
		if scanErr != nil {
			return nil, fmt.Errorf("iterating table %s: %w", step.Table, scanErr)
		}

		if err := writer.EndTable(); err != nil {
			return nil, err
		}

		report.Rows = tableRows
		report.DurationMs = time.Since(stepStarted).Milliseconds()
		report.BytesWritten = sink.writer.BytesWritten() - bytesBefore
		result.Tables = append(result.Tables, report)
		result.TotalRows += tableRows
		if r.OnTableDone != nil {
			r.OnTableDone(report)
		}
		if report.MaskedCells == 0 {
			r.log("warn", "table %s exported without masking (0 cells)", step.Table)
		}

		r.progress(protocol.ExportProgress{
			JobID:         plan.PlanID,
			Phase:         "streaming",
			Table:         step.Table,
			TableIndex:    idx + 1,
			TableCount:    len(plan.Steps),
			RowsProcessed: processedTotal,
			RowsTotal:     rowsTotal,
			BytesWritten:  sink.writer.BytesWritten(),
			Percent:       percent(processedTotal, rowsTotal, idx+1, len(plan.Steps)),
			ElapsedMs:     time.Since(started).Milliseconds(),
			Message:       fmt.Sprintf("%s done (%d rows)", step.Table, tableRows),
		})
	}

	r.progress(protocol.ExportProgress{JobID: plan.PlanID, Phase: "finalizing", Message: "closing file"})

	if err := writer.WriteEnableFK(); err != nil {
		return nil, err
	}
	if err := writer.WriteTransactionEnd(); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	// Atomic rename: a partial file is NEVER visible as the final result.
	if err := sink.Commit(); err != nil {
		return nil, err
	}

	result.OK = true
	result.DurationMs = time.Since(started).Milliseconds()
	result.TotalBytes = sink.BytesWritten()
	result.Checksum = sink.Checksum()
	result.CompletedAt = time.Now().UTC()
	r.progress(protocol.ExportProgress{
		JobID:         plan.PlanID,
		Phase:         "done",
		TableCount:    len(plan.Steps),
		TableIndex:    len(plan.Steps),
		RowsProcessed: processedTotal,
		RowsTotal:     rowsTotal,
		BytesWritten:  result.TotalBytes,
		Percent:       100,
		ElapsedMs:     result.DurationMs,
		Message:       fmt.Sprintf("done: %d rows, %.1f KB", result.TotalRows, float64(result.TotalBytes)/1024),
	})
	return result, nil
}

// streamTable opens the cursor. sql.Rows is generally already streaming
// (pgx simple protocol, mysql unbuffered, sqlite per-row), so there is no need
// for slow LIMIT/OFFSET paging on large tables.
func streamTable(ctx context.Context, conn *driver.Conn, step protocol.ExportStep) (*sql.Rows, error) {
	return conn.DB.QueryContext(ctx, step.SelectSQL)
}

func (r *Runner) emitProgress(plan *protocol.ExportPlan, idx int, step protocol.ExportStep,
	done, total int64, started time.Time, tableRows int64, bytes int64) {

	elapsed := time.Since(started).Milliseconds()
	eta := int64(0)
	if done > 0 && total > done {
		eta = elapsed * (total - done) / done
	}
	r.progress(protocol.ExportProgress{
		JobID:         plan.PlanID,
		Phase:         "streaming",
		Table:         step.Table,
		TableIndex:    idx + 1,
		TableCount:    len(plan.Steps),
		RowsProcessed: done,
		RowsTotal:     total,
		BytesWritten:  bytes,
		Percent:       percent(done, total, idx, len(plan.Steps)),
		ElapsedMs:     elapsed,
		ETAMs:         eta,
		Message:       fmt.Sprintf("%s: %d rows", step.Table, tableRows),
	})
}

func percent(done, total int64, tablesDone, tableCount int) float64 {
	if total > 0 {
		p := float64(done) / float64(total) * 100
		if p > 99.5 && tablesDone < tableCount {
			p = 99.5
		}
		return round1(p)
	}
	if tableCount > 0 {
		return round1(float64(tablesDone) / float64(tableCount) * 100)
	}
	return 0
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// normalize turns driver.Value values into Go types that Dialect.Literal() can
// render. Notably []byte becomes a string for text columns; otherwise every
// VARCHAR would come out as X'..' hex.
func normalize(v any) any {
	switch x := v.(type) {
	case []byte:
		if isPrintableText(x) {
			return string(x)
		}
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	case uint64:
		if x <= 1<<62 {
			return int64(x)
		}
		return x
	default:
		return v
	}
}

func isPrintableText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	// Light heuristic: reject when a NUL byte is present (a sign of binary/BLOB
	// data).
	for _, c := range b {
		if c == 0x00 {
			return false
		}
	}
	return true
}

// Sink: output file wrapper (atomic + checksum + optional per-table split)

type sink struct {
	split bool

	dir      string
	base     string
	ext      string
	final    string
	tmp      string
	hasher   hash.Hash
	file     *os.File
	writer   *Writer
	dialect  driver.Dialect
	opts     protocol.ExportOptions
	finalize func() error
}

func newSink(outputPath string, d driver.Dialect, opts protocol.ExportOptions) (*sink, error) {
	if outputPath == "" {
		return nil, fmt.Errorf("outputPath is required")
	}
	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return nil, err
	}
	s := &sink{
		split:   opts.SplitPerTable,
		dir:     filepath.Dir(abs),
		base:    strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)),
		ext:     filepath.Ext(abs),
		final:   abs,
		dialect: d,
		opts:    opts,
		hasher:  sha256.New(),
	}
	if s.ext == "" {
		s.ext = ".sql"
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	if s.split {
		s.dir = filepath.Join(s.dir, s.base)
		if err := os.MkdirAll(s.dir, 0o755); err != nil {
			return nil, err
		}
	}
	if err := s.open(abs, filepath.Join(filepath.Dir(abs), "."+filepath.Base(abs)+".tmp")); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *sink) open(final, tmp string) error {
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	s.file, s.tmp, s.final = f, tmp, final
	s.writer = NewWriter(io.MultiWriter(f, s.hasher), s.dialect, s.opts)
	return nil
}

func (s *sink) rotate(index int, step protocol.ExportStep) error {
	if s.writer != nil {
		if err := s.writer.Close(); err != nil {
			return err
		}
	}
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			return err
		}
	}
	_ = os.Remove(s.tmp)

	name := fmt.Sprintf("%03d_%s%s", index+1, sanitizeFile(step.Table), s.ext)
	final := filepath.Join(s.dir, name)
	prev := s.finalize
	s.finalize = func() error {
		if prev != nil {
			if err := prev(); err != nil {
				return err
			}
		}
		return os.Rename(s.tmp, final)
	}
	return s.open(final, final+".tmp")
}

func (s *sink) BytesWritten() int64 {
	if s.writer == nil {
		return 0
	}
	return s.writer.BytesWritten()
}

func (s *sink) Checksum() string { return hex.EncodeToString(s.hasher.Sum(nil)) }

// Commit closes the file and then moves it from .tmp to the final name.
func (s *sink) Commit() error {
	if s.file != nil {
		if err := s.file.Sync(); err != nil {
			return err
		}
		if err := s.file.Close(); err != nil {
			return err
		}
		s.file = nil
	}
	if err := os.Rename(s.tmp, s.final); err != nil {
		return err
	}
	if s.finalize != nil {
		return s.finalize()
	}
	return nil
}

// Cleanup discards temporary files when an error/cancel occurs.
func (s *sink) Cleanup() {
	if s.file != nil {
		_ = s.file.Close()
	}
	if s.tmp != "" {
		_ = os.Remove(s.tmp)
	}
}

func sanitizeFile(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
