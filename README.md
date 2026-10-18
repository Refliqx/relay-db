# RelayDB

> **Seamlessly relay, sanitize, and prune live databases into clean local seed data.**

RelayDB is a desktop application for turning a live database (production/staging)
into clean, safe, commit-ready **local seed data**. It removes the three main
pain points of `pg_dump`/`mysqldump`:

- **Version mismatch**: no longer depends on a CLI tool that matches the
  server version. RelayDB uses **native database drivers**.
- **Import failures caused by foreign keys**: tables are ordered
  **topologically**, with cycle detection and an FK guard.
- **Leaked sensitive data**: PII is masked **deterministically** while
  streaming, and large tables are **pruned** according to rules.

The output is a **Git-friendly** and **byte-reproducible** `.sql` file:
two runs with the same seed produce an identical file.

---

## Features

- **4 visual steps:** Connection Manager → Schema & FK Explorer → Visual
  Masking & Pruning → Export Execution.
- **Supported databases:** PostgreSQL (Supabase/Neon/RDS), MySQL/MariaDB
  (XAMPP/Laragon), SQLite, and TursoDB (libsql).
- **16 masking strategies** including `fake_email`, `fake_name`, `fake_phone`,
  `default_hash`, and `json_scrub` (masking inside JSON columns).
- **Automatic PII suggestions**: the engine scans column names and suggests
  strategies, so the user only has to audit instead of writing rules from
  scratch.
- **Layered read-only**: RelayDB never writes to the source database.
- **Headless mode**: the same engine can be used in CI without a GUI.

---

## Architecture at a glance

```
Svelte + TypeScript + Tailwind   (UI/UX, strong typing)
        │  Tauri invoke / listen
Rust (Tauri host)                (process lifecycle, dialogs, RPC multiplexing)
        │  JSON-RPC 2.0 over NDJSON (stdin/stdout)
Go (engine sidecar)              (drivers, introspection, masking, SQL generator)
```

Full details, including design decisions and trade-offs, are in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

---

## Project structure

```
RelayDB-app/
├── engine/                        # Go sidecar (domain + headless CLI)
│   ├── cmd/relaydb-engine/         # entry point + --selftest, --dump-schema, etc.
│   └── internal/
│       ├── protocol/               # SINGLE SOURCE OF TRUTH for the wire contract
│       ├── jsonrpc/                # JSON-RPC 2.0 server (NDJSON)
│       ├── driver/                 # dialects + pg/mysql/sqlite/turso connectors (read-only)
│       ├── introspect/             # schema snapshot, FK graph, topo sort, PII heuristics
│       ├── mask/                   # 16 deterministic masking strategies
│       │   └── bcryptdet/          # bcrypt with a seed-derived salt (byte-reproducible)
│       ├── export/                 # planner, writer, streaming runner, atomic sink
│       ├── config/                 # relaydb.yaml parser & validator
│       └── app/                    # JSON-RPC handlers (method wiring)
├── src/                           # SvelteKit frontend
│   ├── lib/
│   │   ├── types/protocol.ts       # typed mirror of the Go contract
│   │   ├── ipc/{client,events}.ts  # the only bridge to Tauri
│   │   ├── stores/                 # engine & wizard state
│   │   └── components/             # ConnectionForm, SchemaExplorer, FkGraph, …
│   └── routes/                     # layout + wizard pages
├── src-tauri/                     # Rust/Tauri host
│   └── src/
│       ├── engine/                 # sidecar supervisor + RPC multiplexer
│       ├── commands/               # API surface per wizard step
│       ├── error.rs                # AppError (error contract to the frontend)
│       └── lib.rs                  # setup, spawn engine, shutdown
├── schemas/relaydb.schema.json     # JSON Schema for presets
├── examples/                       # example presets (local MySQL, Supabase, Turso)
├── scripts/                        # build-engine.ts, check-protocol.ts, check-reproducible.ts
└── docs/                           # ARCHITECTURE, IPC-PROTOCOL, CONFIG-SCHEMA, DESIGN
```

---

## Prerequisites

| Tool | Version | Purpose |
| --- | --- | --- |
| [Bun](https://bun.sh) | ≥ 1.2 | Package manager & script runtime |
| [Go](https://go.dev) | ≥ 1.24 | Build the engine sidecar |
| [Rust](https://rustup.rs) + Cargo | stable | Build the Tauri host |
| Tauri system dependencies | [see prerequisites](https://tauri.app/start/prerequisites/) | WebView, build tools |

---

## Quick start

```bash
# 1. Install frontend dependencies
bun install

# 2. Build the Go sidecar (suffixed with the Tauri target triple)
bun run engine:build

# 3. Run the desktop application
bun run dev:all          # engine:build + tauri dev
```

Quick verification without a GUI:

```bash
bun run engine:selftest  # end-to-end SQLite smoke test (masking + export)
```

---

## The 4-step flow

1. **Connection Manager**: choose the database kind, enter credentials (safely
   via `${ENV_VAR}`), then **Test Connection** (latency, server version, table
   count).
2. **Schema & FK Explorer**: view tables, columns, types, and the foreign key
   graph with a safe insert order; PII suggestions appear on sensitive columns.
3. **Visual Masking & Pruning**: review/set the masking strategy per column and
   the row pruning rules.
4. **Export Execution**: preview the plan (`export.plan`), then run the
   streaming export with per-table progress, ETA, and a SHA-256 checksum.

---

## Headless mode / CI

The same engine can be used without a GUI, useful for seeding in a pipeline:

```bash
# Validate a preset (exit ≠ 0 on error)
./src-tauri/binaries/relaydb-engine-<target-triple> --check-preset examples/local-mysql.relaydb.yaml

# Export directly
./src-tauri/binaries/relaydb-engine-<target-triple> \
  --preset examples/supabase-staging.relaydb.yaml \
  --profile supabase-staging \
  --out seeds/staging.sql
```

The preset file format is described in [`docs/CONFIG-SCHEMA.md`](docs/CONFIG-SCHEMA.md).

---

## Scripts

| Command | Function |
| --- | --- |
| `bun run dev` | Vite dev server (UI only) |
| `bun run dev:all` | Build the engine then `tauri dev` |
| `bun run engine:build` | Build the Go sidecar with a target-triple suffix |
| `bun run engine:selftest` | End-to-end SQLite smoke test |
| `bun run engine:test` | `go test ./...` |
| `bun run engine:repro` | Prove the output is reproducible across two separate processes (checksums must match) |
| `bun run protocol:check` | Go ↔ Rust ↔ TS contract drift guard |
| `bun run check` | `svelte-check` (Svelte types & lint) |
| `bun run check:all` | `engine:test` + `engine:repro` + `check` + `protocol:check` |
| `bun run tauri build` | Bundle the production application |

---

## Documentation

| Document | Contents |
| --- | --- |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Layer map, IPC topology, engine lifecycle, concurrency, read-only guarantees, error taxonomy, decisions & trade-offs. |
| [`docs/IPC-PROTOCOL.md`](docs/IPC-PROTOCOL.md) | NDJSON framing, envelopes, method & notification tables, error codes, cancellation, versioning. |
| [`docs/CONFIG-SCHEMA.md`](docs/CONFIG-SCHEMA.md) | `relaydb.yaml` preset format, env expansion, validation rules & guardrails, headless usage. |
| [`docs/DESIGN.md`](docs/DESIGN.md) | Design system: tokens, radius/text scale, component classes, accessibility, and the list of anti-patterns deliberately avoided. |

---

## Licence

MIT.
