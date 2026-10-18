// Command relaydb-engine is RelayDB's Go sidecar.
//
// TWO USAGE MODES (deliberate):
//
//  1. Driven by Tauri over stdio RPC (default):
//     relaydb-engine
//  2. Headless for CI / scripting (with no GUI at all):
//     relaydb-engine --preset .relaydb.yaml --profile staging --out seeds/dev.sql
//
// ABSOLUTE RULE: stdout is ONLY for the JSON-RPC protocol. All logs, panic
// handlers, and diagnostics MUST go to stderr. A single stray byte on stdout
// breaks the framing and freezes the UI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/refliqx/relaydb/engine/internal/app"
	"github.com/refliqx/relaydb/engine/internal/config"
	"github.com/refliqx/relaydb/engine/internal/driver"
	"github.com/refliqx/relaydb/engine/internal/export"
	"github.com/refliqx/relaydb/engine/internal/introspect"
	"github.com/refliqx/relaydb/engine/internal/jsonrpc"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// version is overridden at build time: -ldflags "-X main.version=1.2.3"
var version = "0.1.0-dev"

func main() {
	var (
		presetPath  = flag.String("preset", "", "relaydb.yaml path for headless mode")
		profileID   = flag.String("profile", "", "profile id used in headless mode")
		outPath     = flag.String("out", "", "output .sql file path (headless mode)")
		seed        = flag.String("seed", "", "masking seed (defaults from the preset)")
		selftest    = flag.Bool("selftest", false, "run the end-to-end SQLite smoke test to stdout")
		checkPreset = flag.String("check-preset", "", "validate a preset file then exit (used in CI)")
		dumpSchema  = flag.Bool("dump-schema", false, "print the method catalogue & protocol version as JSON")
		showVersion = flag.Bool("version", false, "print the engine version")
		logLevel    = flag.String("log-level", "info", "debug|info|warn|error")
	)
	flag.Parse()

	log := newLogger(*logLevel)

	if *showVersion {
		fmt.Fprintln(os.Stderr, "relaydb-engine", version, "protocol", protocol.ProtocolVersion)
		return
	}
	if *dumpSchema {
		// Used by scripts/gen-protocol-doc.ts to keep the docs & schema in sync
		// with the code.
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"protocolVersion": protocol.ProtocolVersion,
			"engineVersion":   version,
			"methods": []string{
				protocol.MethodEngineHandshake, protocol.MethodEngineShutdown,
				protocol.MethodConnectionTest, protocol.MethodSchemaIntrospect,
				protocol.MethodExportPlan, protocol.MethodExportRun,
				protocol.MethodExportCancel, protocol.MethodConfigLoad,
				protocol.MethodConfigValidate, protocol.MethodMaskCatalog,
			},
			"notifications": []string{
				protocol.NotifyEngineReady, protocol.NotifyEngineLog,
				protocol.NotifyExportProgress, protocol.NotifyExportTableDone,
			},
			"errorCodes": map[string]int{
				"engineNotReady":   protocol.CodeEngineNotReady,
				"kindUnsupported":  protocol.CodeKindUnsupported,
				"connectionFailed": protocol.CodeConnectionFailed,
				"authFailed":       protocol.CodeAuthFailed,
				"introspectFailed": protocol.CodeIntrospectFailed,
				"exportFailed":     protocol.CodeExportFailed,
				"configInvalid":    protocol.CodeConfigInvalid,
				"cancelled":        protocol.CodeCancelled,
				"timeout":          protocol.CodeTimeout,
			},
			"strategies": config.StrategyList,
		})
		return
	}
	if *selftest {
		if err := runSelfTest(log); err != nil {
			log.Error("selftest failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *checkPreset != "" {
		file, issues, err := config.Load(*checkPreset)
		if err != nil {
			log.Error("preset could not be read", "err", err)
			os.Exit(1)
		}
		hasError := false
		for _, is := range issues {
			if is.Severity == "error" {
				hasError = true
			}
			fmt.Fprintf(os.Stdout, "%-7s %-28s %s\n", is.Severity, is.Path, is.Message)
		}
		profiles := file.AllProfiles()
		fmt.Fprintf(os.Stdout, "profiles=%d masking=%d pruning=%d export=%s\n",
			len(profiles), len(file.Masking), len(file.Pruning), file.Export.OutputPath)
		if hasError {
			os.Exit(1)
		}
		return
	}
	if *presetPath != "" {
		if err := runHeadless(*presetPath, *profileID, *outPath, *seed, log); err != nil {
			log.Error("headless export failed", "err", err)
			os.Exit(1)
		}
		return
	}

	serve(log)
}

// serve runs the RPC server on stdin/stdout.
func serve(log *logr) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := jsonrpc.NewServer(os.Stdout, log.slog)
	a := app.New(version, log.slog)
	a.Register(srv)

	log.slog.Info("engine ready", "version", version, "protocol", protocol.ProtocolVersion, "pid", os.Getpid())
	_ = srv.Notify(protocol.NotifyEngineReady, map[string]any{
		"protocolVersion": protocol.ProtocolVersion,
		"engineVersion":   version,
	})

	// Serve() stops when stdin hits EOF (the parent process died). This
	// matters: the sidecar must not outlive the Tauri app.
	if err := srv.Serve(ctx, os.Stdin); err != nil {
		log.slog.Error("rpc stopped with error", "err", err)
		os.Exit(1)
	}
	log.slog.Info("stdin closed, engine exiting")
}

// runHeadless is the CI path: no UI, no Tauri.
func runHeadless(presetPath, profileID, outPath, seed string, log *logr) error {
	file, issues, err := config.Load(presetPath)
	if err != nil {
		return err
	}
	for _, is := range issues {
		if is.Severity == "error" {
			return fmt.Errorf("invalid preset: %s: %s", is.Path, is.Message)
		}
		log.slog.Warn("preset issue", "path", is.Path, "message", is.Message)
	}
	config.ApplyDefaults(file)

	profiles := file.AllProfiles()
	if len(profiles) == 0 {
		return fmt.Errorf("no profiles in %s", presetPath)
	}
	profile := profiles[0]
	if profileID != "" {
		found := false
		for _, p := range profiles {
			if p.ID == profileID {
				profile, found = p, true
			}
		}
		if !found {
			return fmt.Errorf("profile %q not found in %s", profileID, presetPath)
		}
	}

	opts := file.Export
	if outPath != "" {
		opts.OutputPath = outPath
	}
	if seed != "" {
		opts.Seed = seed
	}

	ctx := context.Background()
	conn, err := driver.Open(ctx, profile)
	if err != nil {
		return err
	}
	defer conn.Close()

	snap, err := introspect.Snapshot(ctx, conn, introspect.Options{})
	if err != nil {
		return err
	}
	plan, err := export.BuildPlan(export.PlanInput{
		Conn:      conn,
		Snapshot:  snap,
		Selection: file.Selection,
		Masking:   file.Masking,
		Pruning:   file.Pruning,
		Options:   opts,
	})
	if err != nil {
		return err
	}
	for _, w := range plan.Warnings {
		log.slog.Warn("plan", "warning", w)
	}

	runner := &export.Runner{
		OnProgress: func(p protocol.ExportProgress) {
			if p.Phase == "streaming" && p.RowsProcessed > 0 {
				log.slog.Info("progress", "table", p.Table, "rows", p.RowsProcessed, "percent", p.Percent)
			}
		},
		OnLog: func(level, msg string) { log.slog.Warn(msg, "level", level) },
	}
	res, err := runner.Run(ctx, conn, plan, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr,
		"done: %d tables, %d rows, %.1f KB -> %s (sha256 %s)\n",
		len(res.Tables), res.TotalRows, float64(res.TotalBytes)/1024, res.OutputPath, short(res.Checksum))
	return nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// logr wraps slog for consistency.
type logr struct{ slog *slog.Logger }

func (l *logr) Error(msg string, args ...any) { l.slog.Error(msg, args...) }

func newLogger(level string) *logr {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	// MUST be stderr: stdout is the RPC channel.
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return &logr{slog: slog.New(h)}
}
