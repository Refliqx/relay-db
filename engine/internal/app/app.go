// Package app wires the driver, introspection, masking, and export together
// into JSON-RPC handlers. This is the only layer that knows about the wire
// protocol; the other packages stay purely domain.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/refliqx/relaydb/engine/internal/config"
	"github.com/refliqx/relaydb/engine/internal/driver"
	"github.com/refliqx/relaydb/engine/internal/export"
	"github.com/refliqx/relaydb/engine/internal/introspect"
	"github.com/refliqx/relaydb/engine/internal/jsonrpc"
	"github.com/refliqx/relaydb/engine/internal/mask"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

type App struct {
	Version string
	Log     *slog.Logger

	srv    *jsonrpc.Server
	jobsMu sync.Mutex
	jobs   map[string]context.CancelFunc
}

func New(version string, log *slog.Logger) *App {
	return &App{Version: version, Log: log, jobs: map[string]context.CancelFunc{}}
}

// Register registers all methods. The order follows the UI flow
// (Step 1 -> Step 4) so it is easy to audit.
func (a *App) Register(s *jsonrpc.Server) {
	a.srv = s

	s.Register(protocol.MethodEngineHandshake, a.handshake)
	s.Register(protocol.MethodEngineShutdown, a.shutdown)
	s.Register(protocol.MethodConnectionTest, a.testConnection)
	s.Register(protocol.MethodSchemaIntrospect, a.introspectSchema)
	s.Register(protocol.MethodExportPlan, a.exportPlan)
	s.Register(protocol.MethodExportRun, a.exportRun)
	s.Register(protocol.MethodExportCancel, a.cancelExport)
	s.Register(protocol.MethodConfigLoad, a.loadConfig)
	s.Register(protocol.MethodConfigValidate, a.validateConfig)
	s.Register(protocol.MethodMaskCatalog, a.maskCatalog)
}

// Step 0: handshake

func (a *App) handshake(_ context.Context, _ json.RawMessage) (any, error) {
	drivers := make([]protocol.DriverInfo, 0, 6)
	caps := map[string][]string{
		protocol.KindPostgres: {"schema", "ssl", "row_estimate", "server_side_cursor"},
		protocol.KindMySQL:    {"schema", "ssl", "row_estimate"},
		protocol.KindSQLite:   {"file", "offline"},
		protocol.KindTurso:    {"remote", "auth_token"},
	}
	for _, c := range driver.Supported() {
		drivers = append(drivers, protocol.DriverInfo{
			Kind:         c.Kind(),
			Label:        c.Label(),
			DSNExample:   c.DSNExample(),
			Capabilities: caps[c.Kind()],
		})
	}
	return protocol.HandshakeResult{
		ProtocolVersion: protocol.ProtocolVersion,
		EngineVersion:   a.Version,
		GoVersion:       runtime.Version(),
		Platform:        runtime.GOOS + "/" + runtime.GOARCH,
		PID:             pid(),
		MaxParallelism:  max(2, runtime.NumCPU()),
		Drivers:         drivers,
	}, nil
}

func (a *App) shutdown(_ context.Context, _ json.RawMessage) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (a *App) maskCatalog(_ context.Context, _ json.RawMessage) (any, error) {
	return map[string]any{"strategies": mask.StrategyCatalog()}, nil
}

// Step 1: Test Connection

func (a *App) testConnection(ctx context.Context, params json.RawMessage) (any, error) {
	var p protocol.TestConnectionParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if err := ensureProfile(p.Profile); err != nil {
		return nil, err
	}

	start := time.Now()
	conn, err := driver.Open(ctx, p.Profile)
	if err != nil {
		return nil, toRPC(err)
	}
	defer conn.Close()
	latency := time.Since(start).Milliseconds()

	// Lightweight probe that proves read access actually exists.
	tableCount := 0
	capName := "cannot count tables"
	switch conn.Kind {
	case protocol.KindPostgres:
		_ = conn.DB.QueryRowContext(ctx,
			`SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema')`).
			Scan(&tableCount)
		capName = ""
	case protocol.KindMySQL, protocol.KindMariaDB:
		_ = conn.DB.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = DATABASE()`).Scan(&tableCount)
		capName = ""
	default:
		_ = conn.DB.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount)
		capName = ""
	}

	c, _ := driver.Get(conn.Kind)
	warnings := []string{}
	if p.Profile.ReadOnly == false {
		warnings = append(warnings, "profile does not set readOnly=true; RelayDB still opens a read-only session, but prefer a DB user with SELECT-only rights")
	}
	if p.Profile.Kind == "" {
		warnings = append(warnings, fmt.Sprintf("kind inferred from the DSN as %q", conn.Kind))
	}

	res := protocol.TestConnectionResult{
		OK:            true,
		Kind:          conn.Kind,
		Driver:        conn.Dialect.Name,
		LatencyMs:     latency,
		ServerVersion: conn.ServerVersion(),
		Database:      conn.Database(),
		Host:          displayHost(p.Profile),
		ReadOnly:      true,
		TableCount:    tableCount,
		Warnings:      warnings,
		CheckedAt:     time.Now().UTC(),
	}
	if c != nil {
		res.Capabilities = c.Capabilities()
	}
	if capName != "" {
		res.Warnings = append(res.Warnings, capName)
	}
	a.Log.Info("test connection",
		"kind", conn.Kind, "ok", true, "latency_ms", latency,
		"server_version", truncate(conn.ServerVersion(), 60))
	return res, nil
}

func displayHost(p protocol.Profile) string {
	if p.Host != "" {
		return fmt.Sprintf("%s:%d", p.Host, p.Port)
	}
	if p.File != "" {
		return p.File
	}
	if p.DSN != "" {
		return "(from DSN)"
	}
	return ""
}

// Step 2: Schema & FK explorer

func (a *App) introspectSchema(ctx context.Context, params json.RawMessage) (any, error) {
	var p protocol.IntrospectParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	if err := ensureProfile(p.Profile); err != nil {
		return nil, err
	}

	start := time.Now()
	conn, err := driver.Open(ctx, p.Profile)
	if err != nil {
		return nil, toRPC(err)
	}
	defer conn.Close()

	snap, err := introspect.Snapshot(ctx, conn, introspect.Options{
		ExactRowCount: p.ExactRowCount,
		IncludeViews:  p.IncludeViews,
	})
	if err != nil {
		return nil, toRPC(err)
	}
	graph := introspect.BuildGraph(snap, nil)
	order := introspect.TopoSort(graph, snap.Tables)

	return protocol.IntrospectResult{
		Snapshot:    snap,
		Graph:       graph,
		Order:       order,
		MaskCatalog: mask.StrategyCatalog(),
		ProfileEcho: protocol.ProfileSummary{
			ID: p.Profile.ID, Name: p.Profile.Name, Kind: conn.Kind,
		},
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// Step 4: Export

func (a *App) exportPlan(ctx context.Context, params json.RawMessage) (any, error) {
	var p protocol.ExportRunParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	conn, snap, err := a.openWithSchema(ctx, p)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return export.BuildPlan(export.PlanInput{
		Conn:      conn,
		Snapshot:  snap,
		Selection: p.Selection,
		Masking:   p.Masking,
		Pruning:   p.Pruning,
		Options:   p.Options,
	})
}

func (a *App) exportRun(ctx context.Context, params json.RawMessage) (any, error) {
	var p protocol.ExportRunParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	conn, snap, err := a.openWithSchema(ctx, p)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	plan, err := export.BuildPlan(export.PlanInput{
		Conn:      conn,
		Snapshot:  snap,
		Selection: p.Selection,
		Masking:   p.Masking,
		Pruning:   p.Pruning,
		Options:   p.Options,
	})
	if err != nil {
		return nil, jsonrpc.NewError(protocol.CodeExportFailed, err.Error())
	}
	// JobID from the UI is used as the PlanID so progress & cancel can be
	// correlated exactly with the job shown on screen.
	if p.JobID != "" {
		plan.PlanID = p.JobID
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.jobsMu.Lock()
	a.jobs[plan.PlanID] = cancel
	a.jobsMu.Unlock()
	defer func() {
		a.jobsMu.Lock()
		delete(a.jobs, plan.PlanID)
		a.jobsMu.Unlock()
	}()

	runner := &export.Runner{
		OnProgress: func(pr protocol.ExportProgress) {
			_ = a.srv.Notify(protocol.NotifyExportProgress, pr)
		},
		OnTableDone: func(rep protocol.TableReport) {
			_ = a.srv.Notify(protocol.NotifyExportTableDone, rep)
		},
		OnLog: func(level, msg string) {
			_ = a.srv.Notify(protocol.NotifyEngineLog, map[string]any{
				"level": level, "message": msg, "at": time.Now().UTC(),
			})
		},
	}

	result, err := runner.Run(jobCtx, conn, plan, p.Options)
	if err != nil {
		if jobCtx.Err() != nil {
			return nil, jsonrpc.NewError(protocol.CodeCancelled, "export cancelled; the temporary file has already been removed")
		}
		a.Log.Error("export failed", "err", err)
		return nil, jsonrpc.NewError(protocol.CodeExportFailed, err.Error())
	}
	return result, nil
}

func (a *App) cancelExport(_ context.Context, params json.RawMessage) (any, error) {
	var p protocol.CancelParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	a.jobsMu.Lock()
	cancel, ok := a.jobs[p.JobID]
	a.jobsMu.Unlock()
	if !ok {
		return map[string]any{"ok": false, "reason": "job not found (it may already have finished)"}, nil
	}
	cancel()
	return map[string]any{"ok": true}, nil
}

// openWithSchema opens a connection + snapshot once, then shares them between
// export.plan and export.run.
func (a *App) openWithSchema(ctx context.Context, p protocol.ExportRunParams) (*driver.Conn, *protocol.SchemaSnapshot, error) {
	profile := p.Profile
	var masking []protocol.MaskRule
	var pruning []protocol.PruneRule

	// When the UI sends a preset path, the engine reads that file itself (one
	// parser, a single source of truth).
	if p.ProfilePath != "" && p.Preset == nil {
		file, issues, err := config.Load(p.ProfilePath)
		if err != nil {
			return nil, nil, jsonrpc.NewError(protocol.CodeConfigInvalid, err.Error())
		}
		for _, is := range issues {
			if is.Severity == "error" {
				return nil, nil, jsonrpc.NewError(protocol.CodeConfigInvalid,
					fmt.Sprintf("preset %s: %s", is.Path, is.Message))
			}
		}
		if len(file.AllProfiles()) > 0 {
			profile = file.AllProfiles()[0]
		}
		masking, pruning = file.Masking, file.Pruning
		if len(p.Selection) == 0 {
			p.Selection = file.Selection
		}
		if p.Options.OutputPath == "" {
			p.Options = file.Export
		}
	}
	if p.Preset != nil {
		masking, pruning = p.Preset.Masking, p.Preset.Pruning
	}
	// Rules from the request (unsaved UI edits) win over the file.
	if len(p.Masking) > 0 {
		masking = p.Masking
	}
	if len(p.Pruning) > 0 {
		pruning = p.Pruning
	}
	p.Masking, p.Pruning = masking, pruning

	if err := ensureProfile(profile); err != nil {
		return nil, nil, err
	}
	conn, err := driver.Open(ctx, profile)
	if err != nil {
		return nil, nil, toRPC(err)
	}
	snap, err := introspect.Snapshot(ctx, conn, introspect.Options{})
	if err != nil {
		conn.Close()
		return nil, nil, toRPC(err)
	}
	return conn, snap, nil
}

// Config

func (a *App) loadConfig(_ context.Context, params json.RawMessage) (any, error) {
	var p protocol.LoadConfigParams
	if err := decode(params, &p); err != nil {
		return nil, err
	}
	file, issues, err := config.Load(p.Path)
	if err != nil {
		return nil, jsonrpc.NewError(protocol.CodeConfigInvalid, err.Error())
	}
	config.ApplyDefaults(file)

	summaries := []protocol.ProfileSummary{}
	for _, prof := range file.AllProfiles() {
		summaries = append(summaries, protocol.ProfileSummary{ID: prof.ID, Name: prof.Name, Kind: prof.Kind})
	}

	var preset *protocol.Preset
	if len(file.AllProfiles()) > 0 {
		chosen := file.AllProfiles()[0]
		if p.ProfileID != "" {
			for _, prof := range file.AllProfiles() {
				if prof.ID == p.ProfileID {
					chosen = prof
				}
			}
		}
		preset = &protocol.Preset{
			Version:   file.Version,
			Profile:   chosen,
			Masking:   file.Masking,
			Pruning:   file.Pruning,
			Selection: file.Selection,
			Export:    file.Export,
		}
	}
	return protocol.LoadConfigResult{
		Preset:   preset,
		Path:     p.Path,
		Profiles: summaries,
		Issues:   issues,
	}, nil
}

func (a *App) validateConfig(_ context.Context, params json.RawMessage) (any, error) {
	// Accepts an inline preset from the UI (not yet saved to disk).
	var preset protocol.Preset
	if err := decode(params, &preset); err != nil {
		return nil, err
	}
	file := &config.File{
		Version:   preset.Version,
		Profile:   &preset.Profile,
		Masking:   preset.Masking,
		Pruning:   preset.Pruning,
		Selection: preset.Selection,
		Export:    preset.Export,
	}
	config.ApplyDefaults(file)
	issues := config.Validate(file)
	hasError := false
	for _, is := range issues {
		if is.Severity == "error" {
			hasError = true
		}
	}
	return map[string]any{"valid": !hasError, "issues": issues}, nil
}

// Helper

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return jsonrpc.NewError(protocol.CodeInvalidParams, "empty params")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return jsonrpc.NewError(protocol.CodeInvalidParams, "invalid params: "+err.Error())
	}
	return nil
}

func ensureProfile(p protocol.Profile) error {
	if p.Kind == "" && p.DSN == "" && p.File == "" && p.Host == "" {
		return jsonrpc.NewError(protocol.CodeInvalidParams,
			"empty profile: set `dsn`, or a combination of `kind`+`host`/`file`")
	}
	return nil
}

// toRPC translates a *driver.OpError into a JSON-RPC error with a domain code.
func toRPC(err error) error {
	var op *driver.OpError
	if errors.As(err, &op) {
		return jsonrpc.NewError(op.Code, op.Error())
	}
	return jsonrpc.NewError(protocol.CodeInternalError, err.Error())
}

// pid is used by the UI to show & verify the active sidecar process
// (useful when debugging a "zombie engine" on Windows).
func pid() int { return os.Getpid() }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
