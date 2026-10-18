package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/refliqx/relaydb/engine/internal/driver"
	"github.com/refliqx/relaydb/engine/internal/export"
	"github.com/refliqx/relaydb/engine/internal/introspect"
	"github.com/refliqx/relaydb/engine/internal/protocol"
	_ "modernc.org/sqlite"
)

// runSelfTest is a GUI-free smoke test: it creates a temporary SQLite database,
// fills it with data, then runs the full path (introspect -> FK graph ->
// topological sort -> masking -> streaming writer). Used in CI and while
// developing the engine, so the contract between layers can be verified faster
// than opening the desktop app.
func runSelfTest(log *logr) error {
	dir, err := os.MkdirTemp("", "relaydb-selftest-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	dbPath := filepath.Join(dir, "shop.sqlite")
	if err := seedSQLite(dbPath); err != nil {
		return fmt.Errorf("preparing fixture: %w", err)
	}
	log.slog.Info("fixture ready", "path", dbPath)

	ctx := context.Background()
	profile := protocol.Profile{Kind: protocol.KindSQLite, File: dbPath, ReadOnly: true}

	conn, err := driver.Open(ctx, profile)
	if err != nil {
		return err
	}
	defer conn.Close()

	snap, err := introspect.Snapshot(ctx, conn, introspect.Options{})
	if err != nil {
		return err
	}
	graph := introspect.BuildGraph(snap, nil)
	order := introspect.TopoSort(graph, snap.Tables)

	fmt.Fprintf(os.Stdout, "== TESTS 1: handshake/registry =================================\n")
	for _, c := range driver.Supported() {
		fmt.Fprintf(os.Stdout, "driver: %-9s %s\n", c.Kind(), c.Label())
	}

	fmt.Fprintf(os.Stdout, "\n== TESTS 2: introspect (dialect=%s, db=%s) =====================\n", snap.Dialect, snap.Database)
	for _, t := range snap.Tables {
		pii := []string{}
		for _, c := range t.Columns {
			if c.PiiGuess != "" {
				pii = append(pii, fmt.Sprintf("%s:%s(%.2f)", c.Name, c.PiiGuess, c.PiiConfidence))
			}
		}
		fmt.Fprintf(os.Stdout, "%-10s rows=%-6d pk=%v pii=%v\n", t.Name, t.EstimatedRows, t.PrimaryKey, pii)
	}
	fmt.Fprintf(os.Stdout, "FK edges: %d\n", len(graph.Edges))
	for _, e := range graph.Edges {
		fmt.Fprintf(os.Stdout, "  %s.%s -> %s.%s (nullable=%v)\n", e.From, e.Column, e.To, e.RefColumn, e.Nullable)
	}
	fmt.Fprintf(os.Stdout, "topological order : %v\n", order.Order)
	fmt.Fprintf(os.Stdout, "deferred          : %v\n", order.Deferred)

	fmt.Fprintf(os.Stdout, "\n== TESTS 3: plan + masking ====================================\n")
	limit := 3
	outPath := filepath.Join(dir, "seed.sql")
	opts := protocol.ExportOptions{
		OutputPath:      outPath,
		BatchSize:       2, // deliberately small so the multi-row INSERT is visible
		Seed:            "ci-seed",
		IncludeHeader:   true,
		WrapTransaction: true,
		PrettyPrint:     true,
	}
	plan, err := export.BuildPlan(export.PlanInput{
		Conn:     conn,
		Snapshot: snap,
		Masking: []protocol.MaskRule{
			{Table: "users", Column: "email", Strategy: protocol.StrategyFakeEmail,
				Options: map[string]any{"domain": "example.com"}},
			{Table: "users", Column: "phone", Strategy: protocol.StrategyFakePhone},
			{Table: "users", Column: "password", Strategy: protocol.StrategyDefaultHash},
			{Table: "users", Column: "settings", Strategy: protocol.StrategyJSONScrub,
				Options: map[string]any{"keys": map[string]any{"token": "hash_sha256", "phone": "fake_phone"}}},
			{Table: "comments", Column: "body", Strategy: protocol.StrategyScramble},
		},
		Pruning: []protocol.PruneRule{
			{Table: "activity_logs", Limit: &limit,
				OrderBy: []protocol.OrderTerm{{Column: "created_at", Direction: "desc"}}},
		},
		Options: opts,
	})
	if err != nil {
		return err
	}
	for _, s := range plan.Steps {
		fmt.Fprintf(os.Stdout, "step %d %-14s est=%-5d masked=%d sql=%s\n",
			s.Order+1, s.Table, s.RowEstimate, len(s.MaskedColumns), s.SelectSQL)
	}
	for _, w := range plan.Warnings {
		fmt.Fprintf(os.Stdout, "warning: %s\n", w)
	}

	runner := &export.Runner{
		OnProgress: func(p protocol.ExportProgress) {
			if p.Phase == "streaming" && p.TableIndex > 0 {
				fmt.Fprintf(os.Stdout, "progress: %s -> %.1f%% (%d rows, %d B)\n",
					p.Table, p.Percent, p.RowsProcessed, p.BytesWritten)
			}
		},
	}
	res, err := runner.Run(ctx, conn, plan, opts)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "\n== TESTS 4: export result =====================================\n")
	fmt.Fprintf(os.Stdout, "rows=%d bytes=%d avg duration=%dms\n", res.TotalRows, res.TotalBytes, res.DurationMs)
	fmt.Fprintf(os.Stdout, "checksum sha256: %s\n", res.Checksum)
	for _, t := range res.Tables {
		fmt.Fprintf(os.Stdout, "  %-14s rows=%-5d masked_cells=%-5d bytes=%d\n",
			t.Table, t.Rows, t.MaskedCells, t.BytesWritten)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "\n---- %s ----\n%s\n", filepath.Base(outPath), content)

	// Determinism check: a second run with the same seed must be byte-identical.
	second := filepath.Join(dir, "seed2.sql")
	opts.OutputPath = second
	plan2, err := export.BuildPlan(export.PlanInput{
		Conn: conn, Snapshot: snap,
		Masking: []protocol.MaskRule{
			{Table: "users", Column: "email", Strategy: protocol.StrategyFakeEmail, Options: map[string]any{"domain": "example.com"}},
			{Table: "users", Column: "phone", Strategy: protocol.StrategyFakePhone},
			{Table: "users", Column: "password", Strategy: protocol.StrategyDefaultHash},
			{Table: "users", Column: "settings", Strategy: protocol.StrategyJSONScrub,
				Options: map[string]any{"keys": map[string]any{"token": "hash_sha256", "phone": "fake_phone"}}},
			{Table: "comments", Column: "body", Strategy: protocol.StrategyScramble},
		},
		Pruning: []protocol.PruneRule{
			{Table: "activity_logs", Limit: &limit,
				OrderBy: []protocol.OrderTerm{{Column: "created_at", Direction: "desc"}}},
		},
		Options: opts,
	})
	if err != nil {
		return err
	}
	runner2 := &export.Runner{}
	res2, err := runner2.Run(ctx, conn, plan2, opts)
	if err != nil {
		return err
	}
	if res2.Checksum != res.Checksum {
		return fmt.Errorf("DETERMINISM FAILED: checksums differ (%s vs %s)", res.Checksum, res2.Checksum)
	}
	fmt.Fprintf(os.Stdout, "\nOK: export reproducible (identical checksum across two runs): %s\n", short(res.Checksum))

	// Make sure no original sensitive data leaked into the seed file.
	blob := string(content)
	for _, secret := range []string{"budi.santoso@mail.com", "081234567890"} {
		if strings.Contains(blob, secret) {
			return fmt.Errorf("PII LEAK: original value %q found in the output", secret)
		}
	}
	return nil
}

func seedSQLite(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	ddl := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL UNIQUE,
			name TEXT,
			phone TEXT,
			password TEXT,
			settings TEXT,
			created_at TEXT
		)`,
		`CREATE TABLE posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			title TEXT,
			body TEXT,
			created_at TEXT
		)`,
		`CREATE TABLE comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id INTEGER NOT NULL REFERENCES posts(id),
			user_id INTEGER NOT NULL REFERENCES users(id),
			body TEXT
		)`,
		`CREATE TABLE tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			label TEXT UNIQUE
		)`,
		`CREATE TABLE post_tags (
			post_id INTEGER NOT NULL REFERENCES posts(id),
			tag_id INTEGER NOT NULL REFERENCES tags(id),
			PRIMARY KEY (post_id, tag_id)
		)`,
		`CREATE TABLE activity_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER REFERENCES users(id),
			action TEXT,
			created_at TEXT
		)`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("ddl: %w", err)
		}
	}

	users := [][]any{
		{"budi.santoso@mail.com", "Budi Santoso", "081234567890", "supersecret1",
			`{"token":"tok_abc123","phone":"081234567890","theme":"dark"}`, "2026-01-02T10:00:00Z"},
		{"siti.anggraini@mail.com", "Siti Anggraini", "081298765432", "supersecret2",
			`{"token":"tok_def456","phone":"081298765432","theme":"light"}`, "2026-01-03T11:00:00Z"},
		{"agus@mail.com", "Agus Nugroho", "081311112222", "supersecret3",
			`{"token":"tok_ghi789","phone":"081311112222"}`, "2026-01-04T12:00:00Z"},
		{"dewi@mail.com", "Dewi Kusuma", "081344445555", "supersecret4", `{}`, "2026-01-05T13:00:00Z"},
	}
	for _, u := range users {
		if _, err := db.Exec(`INSERT INTO users (email,name,phone,password,settings,created_at) VALUES (?,?,?,?,?,?)`, u...); err != nil {
			return err
		}
	}

	for i := 1; i <= 6; i++ {
		if _, err := db.Exec(`INSERT INTO posts (user_id,title,body,created_at) VALUES (?,?,?,?)`,
			(i%4)+1, fmt.Sprintf("Post %d", i), "Confidential post body", "2026-02-0"+fmt.Sprint((i%9)+1)+"T08:00:00Z"); err != nil {
			return err
		}
	}
	for i := 1; i <= 8; i++ {
		if _, err := db.Exec(`INSERT INTO comments (post_id,user_id,body) VALUES (?,?,?)`,
			(i%6)+1, (i%4)+1, fmt.Sprintf("Comment %d from a confidential user", i)); err != nil {
			return err
		}
	}
	for _, tag := range []string{"go", "svelte", "rust", "sqlite", "tauri"} {
		if _, err := db.Exec(`INSERT INTO tags (label) VALUES (?)`, tag); err != nil {
			return err
		}
	}
	for i := 1; i <= 8; i++ {
		if _, err := db.Exec(`INSERT INTO post_tags (post_id,tag_id) VALUES (?,?)`, (i%6)+1, (i%5)+1); err != nil {
			return err
		}
	}
	for i := 1; i <= 25; i++ {
		if _, err := db.Exec(`INSERT INTO activity_logs (user_id,action,created_at) VALUES (?,?,?)`,
			(i%4)+1, fmt.Sprintf("login-%d", i),
			fmt.Sprintf("2026-03-%02dT07:00:00Z", i)); err != nil {
			return err
		}
	}
	return nil
}
