# RelayDB: System Architecture

> **RelayDB**: *Seamlessly relay, sanitize, and prune live databases into clean local seed data.*

This document explains the division of responsibilities between layers, the IPC
topology, the engine lifecycle, the concurrency model, the security guarantees,
and the design decisions taken together with their trade-offs.

Related documents:

- [`IPC-PROTOCOL.md`](./IPC-PROTOCOL.md): wire protocol details (framing, envelope, methods, errors).
- [`CONFIG-SCHEMA.md`](./CONFIG-SCHEMA.md): `relaydb.yaml` preset format.

---

## 1. The problems it solves

Developers who want to build *local seed data* from a live database today face
four recurring problems:

| Problem | Root cause | RelayDB's answer |
| --- | --- | --- |
| **Broken dump / version mismatch** | `pg_dump` / `mysqldump` must match the server version exactly. Often not present in the local PATH. | **Native** drivers (pgx, go-sql-driver, modernc sqlite, libsql); never *shell out* to a version-specific tool. |
| **Import failures caused by FK** | The table order in the dump does not respect FK dependencies. | **Topological sort** (Kahn) + cycle detection (Tarjan SCC) + FK guard. |
| **Dirty & huge dump** | Carries every table, including logs and PII data. | **Pruning** (limit/where/order) + **masking** of sensitive columns, applied while streaming. |
| **Hard to visualise** | A raw `.sql` file gives no picture of the schema. | 4-step GUI: Schema & FK Explorer → Visual Masking & Pruning → Export. |

Output principle: a `.sql` file that is **clean, Git-friendly, and byte-reproducible**.

---

## 2. Layer map

RelayDB is a three-language desktop application with one strict separation rule:
**each layer may only do what the other layers cannot.**

```
┌──────────────────────────────────────────────────────────────────────┐
│  SVELTE + TypeScript + Tailwind            (WebView, renderer)       │
│  • UI / UX, presentation state, 4-step wizard                        │
│  • Strong types via src/lib/types/protocol.ts                        │
│  • NEVER touches DB / filesystem / processes                         │
└───────────────────────────────┬──────────────────────────────────────┘
                                │  Tauri `invoke` (commands)  ▲
                                │  Tauri `listen` (events)    │
┌───────────────────────────────▼──────────────────────────────────────┐
│  RUST: Tauri host                           (application process)    │
│  • Sidecar process lifecycle (spawn / restart / shutdown)            │
│  • JSON-RPC multiplexer (id → oneshot), timeout per method           │
│  • OS dialogs (open/save), atomic preset write                       │
│  • JSON-RPC error translation → AppError                             │
│  • "Thin": not the domain owner, knows no SQL/masking                │
└───────────────────────────────┬──────────────────────────────────────┘
                                │  NDJSON JSON-RPC 2.0 via stdin/stdout
                                │  (stdout = RPC, stderr = log)
┌───────────────────────────────▼──────────────────────────────────────┐
│  GO: engine sidecar                         (separate process)       │
│  • Native DB connectors + read-only session                          │
│  • Schema introspection, FK graph, topological sort, cycle detection │
│  • PII heuristics, deterministic masking engine, SQL generator       │
│  • Streaming runner + progress + cancellation                        │
│  • YAML preset parsing & validation (single source of truth)         │
└──────────────────────────────────────────────────────────────────────┘
```

### 2.1 Why three languages?

- **Rust/Tauri** provides native OS integration (window, dialog, tray) at a far
  smaller binary size than an Electron bundle, without giving up the web
  ecosystem for the UI.
- **Svelte** provides compiler-pure reactivity without a virtual DOM, a good
  fit for data UIs such as the FK graph and large column tables.
- **Go** provides native database drivers that are cheap to cross-compile
  (`CGO_ENABLED=0`), goroutines for streaming, and, most importantly,
  **headless capability**: the same engine is used by the GUI and CI.

### 2.2 Responsibility table (one source of truth per capability)

| Capability | Svelte | Rust | Go |
| --- | :---: | :---: | :---: |
| UI, wizard, input form validation | ✅ | | |
| Choose a file via an OS dialog | | ✅ | |
| Atomic write of output file & preset | | ✅ | |
| Start / stop / restart the engine | | ✅ | |
| Request multiplexing & timeout | | ✅ | |
| Error code translation | partial | ✅ | origin |
| Database connection & drivers | | | ✅ |
| Schema introspection & FK graph | | | ✅ |
| Topological sort & cycle detection | | | ✅ |
| PII heuristics | | | ✅ |
| All masking strategies | | | ✅ |
| Export planning & streaming | | | ✅ |
| YAML preset parsing & validation | | | ✅ |
| Headless / CI export | | | ✅ |

**Rule:** each row has **exactly one** ✅ in the implementation columns. When a
capability seems to "need" to live in two layers, that is a sign of incorrect
separation.

---

## 3. IPC topology

There are **two hops** of communication, deliberately kept separate:

```
Svelte ──(1)── Rust ──(2)── Go
```

### Hop 1: Svelte ↔ Rust (Tauri IPC)

- **Commands (request/response):** `invoke("test_connection", { profile })`.
  Each command is **one operation that is meaningful to the user**; there is no
  generic `invoke_engine(method, params)` passthrough; that would discard the
  benefit of an explicit contract.
- **Events (push):** `listen("relaydb://export-progress", …)`. Rust forwards
  engine notifications as Tauri events with the `relaydb://` prefix.
- **Errors:** always go through `AppError { code, message, stage?, detail? }`,
  normalised in `src/lib/ipc/client.ts` into `RelayError` (with a `hint` per
  code). The UI branches on the **code**, not on string matching.

### Hop 2: Rust ↔ Go (JSON-RPC 2.0 NDJSON)

- **Newline-delimited JSON** framing: one message = one line (`\n`).
  `tauri-plugin-shell` reads stdout **line-buffered**, so this framing fits
  without an adapter.
- **stdout = RPC channel. stderr = log.** This rule is absolute; one byte
  straying into stdout corrupts the framing and freezes the UI. The Go logger
  writes to `stderr` (`slog.NewTextHandler(os.Stderr, …)`).
- Requests from Rust have an ascending numeric `id`; notifications from Go have
  no `id` (they have a `method`). Disambiguated via `RpcMessage::is_notification`.

Full details of envelopes, methods, and error codes are in
[`IPC-PROTOCOL.md`](./IPC-PROTOCOL.md).

### Why is Go not embedded as a Rust library?

Choosing a separate process (sidecar) gives: crash isolation (engine dead ≠ app
dead), easy headless testing, separate cross-compilation per target triple, and
no need for an FFI/CGO bridge. The cost is transport complexity, which is
absorbed by the Rust supervisor.

---

## 4. Engine lifecycle

`Engine` is created in Tauri's `setup()` (`src-tauri/src/lib.rs`) because the
supervisor needs an `AppHandle` to access the shell plugin. Spawning is
**asynchronous**, so the window appears first and UI startup does not wait for
the Go process.

```
        ┌────────────┐  ensure_alive()   ┌───────────┐  handshake ok  ┌─────────┐
        │ starting   │ ────────────────▶ │  running  │ ─────────────▶ │  ready  │
        └─────┬──────┘                   └─────┬─────┘                └─────────┘
              │ spawn failed                    │ process Terminated
              ▼                                 ▼
        ┌───────────┐   restart()        ┌───────────┐
        │  crashed  │ ─────────────────▶ │  stopped  │
        └───────────┘                    └───────────┘
```

- **Spawn:** `app.shell().sidecar("relaydb-engine").spawn()` →
  `(Receiver<CommandEvent>, CommandChild)`. The reader task sticks to the
  receiver until `Terminated`.
- **Double-spawn safety:** `spawn_lock` (async `Mutex`) + a re-check in
  `ensure_alive` ensure that two requests arriving at the same time do not start
  two processes.
- **Handshake:** `engine.handshake` is idempotent; the result is cached. Frontend
  & Rust reject an engine whose **major** protocol version does not match.
- **Publish status:** every status change is emitted as a
  `relaydb://engine-status` event; the UI shows `EngineBadge`.
- **Crash:** when the process ends, all pending requests are `fail_all_pending`
  with the `engine_down` error, the status becomes `stopped` (or `crashed` if the
  spawn failed), and the UI offers a **Restart** button.
- **Shutdown:** `RunEvent::Exit` calls `engine.shutdown()` → `child.kill()`.
  Without this, closing the window leaves the Go process as a zombie (especially
  on Windows). In addition, the engine exits on its own at **stdin EOF**; the
  sidecar never outlives the application.

---

## 5. Concurrency model

### 5.1 Multiplexing (Rust side)

`Shared` holds `pending: Mutex<HashMap<u64, oneshot::Sender<…>>>`. Concurrent UI
requests are correlated via `id`. Without this, the export progress notification
stream would "steal" the Test Connection response.

A single reader task parses each NDJSON line and routes it to **exactly one
destination**:

- has an `id` → `pending.remove(id)` → send to the caller's `oneshot`;
- has a `method` without an `id` → notification → re-emitted as a Tauri event.

A non-JSON line does **not** kill the application; it is logged to stderr and
ignored.

### 5.2 Timeout per method

Timeouts are chosen per method (`default_timeout`), because the reasonable
duration of each operation differs greatly:

| Method | Timeout |
| --- | --- |
| `engine.handshake` | 15 s |
| `connection.test` | 30 s |
| `schema.introspect` | 180 s |
| `export.plan` | 180 s |
| `export.run` | 6 hours (cancelled via `export.cancel`) |
| others | 60 s |

A timeout produces the `-32099` error (`E_TIMEOUT`), distinguished from a user
cancellation (`-32010`), so the UI can choose the right message.

### 5.3 Export cancellation

`jobId` is **created by the frontend** (uuid) and sent in `export.run`. This
makes it possible to:

1. have the UI correlate `export.progress`/`export.table` notifications with the
   job card shown on screen (rather than just "there is some progress");
2. have `export.cancel { jobId }` target the right job when several exports are
   running (e.g. multiple windows).

### 5.4 Streaming (Go side)

The runner reads query results per batch (`fetchSize`) and writes per batch
(`batchSize` rows per `INSERT`). Progress (`rowsProcessed`, `percent`, `etaMs`)
is computed from the row estimate and does not force an expensive `COUNT(*)`.

---

## 6. Read-only guarantee & security

RelayDB **never** writes to the source database. This guarantee is layered:

| Layer | Mechanism |
| --- | --- |
| 1. Intent | Default profile `readOnly: true` (UI). |
| 2. Session | SQLite `mode=ro` + `PRAGMA query_only`; PostgreSQL `SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY`; MySQL `SET SESSION TRANSACTION READ ONLY`. |
| 3. `WHERE` guardrail | `inspectWhere` rejects pruning clauses that contain write commands / dangerous keywords. |
| 4. Escaping | All identifiers are escaped (`QuoteIdent`); all string literals are escaped correctly per dialect (`Literal`). |

### `WHERE` guardrail (layer 3)

`inspectWhere` is the **second** layer, not a replacement for the read-only
session. It rejects:

- write commands as **whole words**: `DROP`, `DELETE`, `UPDATE`, `INSERT`,
  `ALTER`, `TRUNCATE`, `GRANT`, `REVOKE`, `CREATE`, `ATTACH`, `VACUUM`,
  `PRAGMA`, `CALL`, `EXEC`, `EXECUTE`, `COPY`, `LOAD_FILE`, `PG_SLEEP`, `UNION`;
- **sequences** that cannot appear in a legitimate read filter: `;`, `--`,
  `/*`, `*/`, `INTO OUTFILE`, `INTO DUMPFILE`.

> **Important implementation note.** Keyword matching uses *word boundaries*
> (`\b…\b`), **not** `strings.Contains`. The reason: legitimate columns such as
> `created_at`, `updated_at`, and `deleted_at` contain
> `create`/`update`/`delete` as substrings. With `Contains`, an entirely
> legitimate read filter would be rejected, a bug that did occur and is now
> guarded by `TestWhereGuardrails`.

### Escaping guardrail (layer 4)

Because RelayDB **generates** a `.sql` file (rather than executing queries),
values are rendered as literals, not placeholders. One escaping bug = a corrupt
seed file or an injection. Rules per dialect:

- Single quotes are **always** doubled (`'` → `''`).
- MySQL (`EscapeStrings`) also escapes backslash (`\` → `\\`) and NUL.
- PostgreSQL/SQLite use standard literals: backslash is **not** escaped.
- Identifiers: the quote char inside an identifier is doubled
  (`we"ird` → `"we""ird"`).
- Timestamps are converted to **UTC** before rendering.

---

## 7. Deterministic masking (value-keyed)

This is RelayDB's core design decision.

**Output = HMAC-SHA256(seed, strategy ‖ hash(options) ‖ canonicalValue)**

That means the output value is determined by the **input value**, not by row
position or table order. The consequences:

- **Referential integrity is preserved.** `users.email` and `orders.billing_email`
  holding the same value are masked to the same value, so cross-table
  relationships (without explicit FKs) stay consistent.
- **UNIQUE stays safe.** Two different values will not collide (with very high
  probability), and the same value stays the same.
- **Reproducible.** Two runs with the same `seed` produce byte-identical output.

### Reproducibility end-to-end

- `headerTimestamp` **defaults to `false`** → the header carries no generation
  timestamp.
- Output is written to `*.tmp` then atomically `rename`d → no half-written file
  if the process fails.
- The runner returns a **SHA-256 checksum**; `ExportPanel` displays it for
  verification.
- **No per-process randomness.** The only strategy that needs randomness
  (`default_hash` → bcrypt) uses a **salt derived from the `seed`**, not
  `crypto/rand`. The implementation lives in `internal/mask/bcryptdet` (a
  faithful copy of `x/crypto/bcrypt` with the salt as an argument) and is
  cross-checked against `bcrypt.CompareHashAndPassword` in a unit test.

  This is not a cosmetic detail: with a random salt, two processes produce
  different hashes, so the checksum changes every time the application runs;
  the "byte-identical" promise above becomes a lie. An in-process selftest
  **cannot** detect this (a process-global cache makes two runs look the same),
  which is why `bun run engine:repro` exists: it runs the selftest in two
  separate processes and compares the checksums, and it is part of `check:all`
  too.

**Documented trade-off:** identical source values map to identical masked
values. For low-cardinality columns (e.g. `country`), this means all rows with
the same value get the same masked value; usually exactly what is wanted, but
worth knowing before use.

### PII heuristics

`introspect.GuessPII` scans column names & types and suggests masking strategies
(`email` → `fake_email`, `phone` → `fake_phone`, `password` →
`default_hash`, etc.). Step 3 becomes *audit*, not *author*: the user reviews
suggestions rather than writing rules from scratch. Not-yet-masked PII appears
as a **warning**, not a hard block, so the user can still export if it is
intentional.

---

## 8. FK order & cycles

`introspect.BuildGraph` builds graph nodes (`schema.table`) and edges
(child → parent). `TopoSort` uses **Kahn** (parents first), with a tie-break on
the **smallest row estimate** so small tables finish first.

A cycle (e.g. `users.manager_id → users.id`, or `a ↔ b`) cannot be broken by
Kahn. `findCycles` uses an **iterative Tarjan SCC** (not recursive, to avoid a
stack overflow on large schemas). Tables in a cycle are placed in `Deferred`,
and the SQL includes an **FK guard** (`SET FOREIGN_KEY_CHECKS = 0` /
`SET session_replication_role = replica` / `PRAGMA foreign_keys = OFF`) so the
import still succeeds.

---

## 9. Error taxonomy

All errors crossing into the frontend have the uniform shape
`{ code, message, stage?, detail? }`.

| Code | Name | Meaning | UI suggestion (`RelayError.hint`) |
| --- | --- | --- | --- |
| `-32602` | invalid params | Invalid arguments | Show in the form |
| `-32000` | engine not ready | Engine dead / pipe broken | **Restart engine** button |
| `-32001` | kind unsupported | Unknown DB kind | Choose another driver / fix the DSN |
| `-32002` | connection failed | Host unreachable | Check host/port/firewall |
| `-32003` | auth failed | Wrong credentials | Check user/password, use read-only |
| `-32004` | introspect failed | Failed to read the schema | Show details |
| `-32005` | export failed | Failed during export | Show details |
| `-32006` | config invalid | Invalid preset | Fix the file then reload |
| `-32010` | cancelled | Cancelled by the user | Temporary file already deleted |
| `-32099` | timeout | Exceeded the time limit | Increase Connect Timeout |

This code contract is maintained in three places: `engine/internal/protocol/protocol.go`,
`src-tauri/src/engine/protocol.rs`, and `src/lib/types/protocol.ts`, and
verified automatically by `bun run protocol:check`.

---

## 10. 4-step wizard data flow

```
Step 1  Connection Manager
        Svelte form ──invoke("test_connection")──▶ Rust ──connection.test──▶ Go
                                                       ◀── TestConnectionResult ──

Step 2  Schema & FK Explorer
        invoke("introspect_schema") ─▶ Rust ──schema.introspect──▶ Go
        Go: schema snapshot + FK graph + topo order + PII suggestions  (one response)
        ◀── IntrospectResult ──  ─▶ SchemaExplorer + FkGraph

Step 3  Visual Masking & Pruning
        PII suggestions → user reviews → MaskingWorkbench
        (no DB round-trip; purely local state + engine catalogue)

Step 4  Export
        invoke("plan_export")  ──export.plan──▶  Go  → ExportPlan (dry-run preview)
        invoke("run_export")   ──export.run───▶  Go  → streaming + progress
                 ▲                                    │
                 │  listen("relaydb://export-progress")│
                 └────────────────────────────────────┘
        ◀── ExportResult (checksum, per-table report) ──
```

`introspect_schema` deliberately combines snapshot + graph + order + masking
catalogue into **one** response, so Step 2 does not need 3 round-trips just to
draw a single page.

---

## 11. Decisions & trade-offs

| Decision | Rejected alternative | Reason |
| --- | --- | --- |
| **Sidecar Go** | Rust library / FFI | Crash isolation, headless CI, easy cross-compile, no CGO. |
| **NDJSON** | LSP-style `Content-Length` framing | `tauri-plugin-shell` is already line-buffered; simpler framing. |
| **Go parses YAML** | Rust parses YAML | One parser → GUI and CI behave identically; no drift. |
| **Thin Rust** | Mirror all domain structs in Rust | 40+ structs = two places that can drift, with no added safety. |
| **TS mirror + drift checker** | Auto-generate bindings | TS needs *types*, the engine needs *runtime*; the checker catches drift in CI. |
| **Value-keyed masking** | Row-index-keyed | Preserves referential integrity & UNIQUE, byte-reproducible. |
| **`modernc.org/sqlite`** | `mattn/go-sqlite3` (CGO) | `CGO_ENABLED=0` → cross-compile without a C toolchain. |
| **Native drivers** | Shell out to `pg_dump`/`mysqldump` | Eliminates the "tool version mismatch" class of bugs. |
| **`headerTimestamp=false` default** | Include the generation timestamp | Two runs = byte-identical → clean Git diff. |
| **Word-boundary guardrail** | `strings.Contains` | Avoids false positives on `created_at`/`deleted_at`. |

---

## 12. Non-goals

- **Not a migration/schema tool.** RelayDB never writes DDL to the source.
- **Not a synchroniser.** One run, one output; there is no incremental
  replication.
- **Not a query proxy.** The engine does not accept free-form SQL from the user;
  `WHERE` is restricted to read expressions.
- **Does not support databases other than PostgreSQL / MySQL / MariaDB / SQLite /
  Turso (libsql)** in this release.
