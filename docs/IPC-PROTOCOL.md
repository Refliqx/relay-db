# RelayDB: IPC Protocol

This document is the reference for the JSON-RPC 2.0 wire protocol between the
**Go engine** and the **Rust host**, plus a summary of the Tauri hop between
**Rust** and **Svelte**.

The source of truth for the data shape is `engine/internal/protocol/protocol.go`.
This document explains it; if the two differ, **the code wins** and
`bun run protocol:check` will flag the drift in CI.

---

## 1. Transport & framing

- **Transport:** stdin/stdout of the Go sidecar process.
- **Framing:** **NDJSON**: one JSON message = one line, separated by `\n`.
- **stdout** = protocol channel. **stderr** = log/diagnostics. This rule is absolute.

Why NDJSON rather than LSP-style `Content-Length` framing:

1. Go's `encoding/json` always escapes newlines inside strings, so
   **1 frame = 1 line mathematically**; no special encoding.
2. `tauri-plugin-shell` already reads stdout **line-buffered**; this framing
   fits without an adapter.
3. Can be `tail`ed and `jq`ed like a log while debugging.

### Buffer limit

The Go reader uses `bufio.Scanner` with an initial buffer of 1 MiB and a
**maximum of 64 MiB** per line, because a single response (e.g. a schema
snapshot) can be tens of MB.

### Server concurrency

Each incoming line is `dispatch`ed on a **separate goroutine**. This matters: a
long-running `export.run` must not block `export.cancel`. The server waits for
all goroutines to finish (`wg.Wait`) before exiting at stdin EOF.

---

## 2. Envelope

Four message shapes, per JSON-RPC 2.0.

### Request (Rust → Go)

```json
{ "jsonrpc": "2.0", "id": 7, "method": "connection.test", "params": { "profile": { … } } }
```

### Successful response (Go → Rust)

```json
{ "jsonrpc": "2.0", "id": 7, "result": { … } }
```

### Error response (Go → Rust)

```json
{ "jsonrpc": "2.0", "id": 7, "error": { "code": -32003, "message": "authentication failed", "data": { … } } }
```

### Notification (Go → Rust, without `id`)

```json
{ "jsonrpc": "2.0", "method": "export.progress", "params": { "jobId": "…", "percent": 42.5 } }
```

**Disambiguation** (in Rust, `RpcMessage::is_notification`): a message is a
notification if it has a `method` **and** no `id`. A message with an `id` is
always treated as a response and correlated to the caller's `oneshot`.

`id` is written as an ascending integer (`u64`), but the Rust reader accepts
both numbers and numeric strings (tolerance).

---

## 3. Methods

The parameter/result formats refer to the structs in `protocol.go`.

| Method | Params | Result | Purpose |
| --- | --- | --- | --- |
| `engine.handshake` | `{}` | `HandshakeResult` | Protocol/engine/platform versions, driver list. Idempotent. |
| `engine.shutdown` | `{}` | `{ "ok": true }` | Marks the intent to stop (actual termination happens via kill/EOF). |
| `connection.test` | `TestConnectionParams` | `TestConnectionResult` | Open a connection, measure latency, get the server version, count tables, close. |
| `schema.introspect` | `IntrospectParams` | `IntrospectResult` | Schema snapshot + FK graph + topo order + masking catalogue in one response. |
| `export.plan` | `ExportRunParams` | `ExportPlan` | Planning without writing a file (dry-run preview). |
| `export.run` | `ExportRunParams` | `ExportResult` | Streaming export + progress + checksum. |
| `export.cancel` | `CancelParams` | `{ "ok": bool, "reason"?: string }` | Cancel an export job by `jobId`. |
| `config.load` | `LoadConfigParams` | `LoadConfigResult` | Read & validate a preset from disk (the engine does the YAML parsing). |
| `config.validate` | `Preset` (inline) | `{ "valid": bool, "issues": [] }` | Validate a preset that has not been saved. |
| `mask.catalog` | `{}` | `{ "strategies": [] }` | Masking strategy catalogue + PII metadata. |

> **Reserved:** `schema.graph` is declared as a constant
> (`MethodSchemaGraph`) but is **not yet registered** and is absent from the
> public `--dump-schema` catalogue. It is reserved for future granular use
> (requesting the graph without a full snapshot).

### Key parameters

**`TestConnectionParams`**

```json
{ "profile": { "kind": "postgres", "host": "…", "port": 5432, "database": "…", "user": "…", "password": "…" } }
```

**`IntrospectParams`**

```json
{ "profile": { … }, "exactRowCount": false, "includeViews": false }
```

**`ExportRunParams`**

```json
{
  "jobId": "uuid-from-frontend",
  "profile": { … },
  "profilePath": "/path/optional.relaydb.yaml",
  "selection": [{ "table": "users", "include": true }],
  "masking": [{ "table": "users", "column": "email", "strategy": "fake_email" }],
  "pruning": [{ "table": "logs", "limit": 100, "orderBy": [{ "column": "created_at", "direction": "desc" }] }],
  "options": { "outputPath": "seeds/seed.sql", "batchSize": 100, "seed": "…" },
  "dryRun": false
}
```

**Configuration precedence** (in `openWithSchema`):

1. rules `masking`/`pruning` from the **request** (unsaved UI edits) win;
2. otherwise, use the inline `preset`;
3. otherwise, the engine reads `profilePath` itself via `config.Load`.

This ensures the UI can preview changes before the user saves the preset.

---

## 4. Notifications

Sent by the engine to the host without an `id`; Rust forwards them as Tauri
events.

| Notification | Params | Tauri event | When |
| --- | --- | --- | --- |
| `engine.ready` | `{ protocolVersion, engineVersion }` | *(handled internally → `relaydb://engine-status`)* | When the engine is ready to serve. |
| `engine.log` | `{ level, message, at? }` | `relaydb://engine-log` | Structured log from the engine. |
| `export.progress` | `ExportProgress` | `relaydb://export-progress` | Periodically during streaming. |
| `export.table` | `TableReport` | `relaydb://export-table` | Each table finished. |

The notification → event mapping is done in `Shared::handle_line`
(`src-tauri/src/engine/mod.rs`):

```rust
N_ENGINE_READY     => publish_status(ready)   // not forwarded verbatim
N_EXPORT_PROGRESS  => emit("relaydb://export-progress")
N_EXPORT_TABLE     => emit("relaydb://export-table")
N_ENGINE_LOG       => emit("relaydb://engine-log")
```

### Two hops deliberately kept separate

`NOTIFICATIONS` (names on the JSON-RPC channel) ≠ `EVENTS` (Tauri event names).
This is deliberate: the engine must know nothing about Tauri, and Rust is free
to add/filter notifications without changing the engine contract.

---

## 5. Error codes

`-32xxx` codes are reserved JSON-RPC + RelayDB domain (`-32000..-32099`).

| Code | Name (dump) | Go constant | Rust constant | TS (`ERR`) |
| --- | --- | --- | --- | --- |
| `-32700` | parse error | *(inline)* | *(none)* | *(none)* |
| `-32600` | invalid request | `CodeInvalidRequest` | *(none)* | *(none)* |
| `-32601` | method not found | `CodeMethodNotFound` | *(none)* | *(none)* |
| `-32602` | invalid params | `CodeInvalidParams` | *(none)* | *(none)* |
| `-32603` | internal error | `CodeInternalError` | *(none)* | *(none)* |
| `-32000` | engineNotReady | `CodeEngineNotReady` | `E_ENGINE_NOT_READY` | `engineNotReady` |
| `-32001` | kindUnsupported | `CodeKindUnsupported` | `E_KIND_UNSUPPORTED` | `kindUnsupported` |
| `-32002` | connectionFailed | `CodeConnectionFailed` | `E_CONNECTION_FAILED` | `connectionFailed` |
| `-32003` | authFailed | `CodeAuthFailed` | `E_AUTH_FAILED` | `authFailed` |
| `-32004` | introspectFailed | `CodeIntrospectFailed` | `E_INTROSPECT_FAILED` | `introspectFailed` |
| `-32005` | exportFailed | `CodeExportFailed` | `E_EXPORT_FAILED` | `exportFailed` |
| `-32006` | configInvalid | `CodeConfigInvalid` | `E_CONFIG_INVALID` | `configInvalid` |
| `-32010` | cancelled | `CodeCancelled` | `E_CANCELLED` | `cancelled` |
| `-32099` | timeout | `CodeTimeout` | `E_TIMEOUT` | `timeout` |

The codes `-32000..-32006`/`-32010` are produced by the **engine**. `-32099`
(`timeout`) is produced by the **Rust layer** (the `invoke` timeout) and is
deliberately distinguished from `-32010` (user cancellation) so the UI picks the
right message.

### Error shape in the frontend

```ts
// AppError (Rust): normalised into RelayError in src/lib/ipc/client.ts
interface RelayError { code: number; message: string; stage?: string; detail?: string; hint?: string }
```

`RelayError.hint` maps a code → a suggested action (e.g. `authFailed` →
"Check user/password…"). The UI branches on `code`, **not** on matching the
message string.

---

## 6. Cancellation & timeout

There are **three** mechanisms, at three different levels:

| Mechanism | Level | Usage | Result |
| --- | --- | --- | --- |
| `export.cancel` | Application | `invoke("cancel_export", { jobId })` | `cancel()` the job context → runner stops, `.tmp` deleted, `-32010` error. |
| `$/cancelRequest` | Protocol | Send `{ "jsonrpc":"2.0", "method":"$/cancelRequest", "params": { "id": 7 } }` | Cancels the request with `id` 7 (`inflight` map). |
| `invoke` timeout | Transport | `Engine::call_with_timeout` in Rust | `-32099` error; the `pending` entry is removed. |

Rust uses `export.cancel` (not `$/cancelRequest`) because the frontend knows its
own `jobId` and does not need to know the numeric request `id`.

If the runner stops because the context was cancelled, `exportRun` returns
`-32010` "export cancelled; temporary file already deleted", and because the
output is written to `*.tmp` then atomically `rename`d, **there is no
half-written file**.

---

## 7. Versioning

- The current `protocolVersion` is **`1.0`** (`PROTOCOL_VERSION` in all three
  layers).
- The handshake (`engine.handshake`) returns `protocolVersion` +
  `engineVersion`.
- **Policy:** Rust & the frontend reject an engine whose **major** protocol
  version does not match; minor versions are additive (new fields are optional).
- **Breaking** changes (removing/changing the meaning of a field, adding a
  required method) bump the major.

---

## 8. Drift guard

Three layers share one contract. `scripts/check-protocol.ts` (`bun run
protocol:check`) runs the engine with `--dump-schema` and compares:

- `protocolVersion`;
- the `methods` list ↔ `METHODS` in TS;
- the `notifications` list ↔ `NOTIFICATIONS` in TS;
- `errorCodes` ↔ `ERR` in TS;
- `strategies` ↔ the `MaskStrategy` union in TS.

If something exists on only one side, the script exits with status ≠ 0. This
turns the "TypeError: undefined at runtime" class of bug into a **CI failure**.

Run the whole quality gate with:

```bash
bun run check:all      # go test + svelte-check + protocol:check
```

---

## 9. Complete session example

```
→ {"jsonrpc":"2.0","id":1,"method":"engine.handshake","params":{}}
← {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"1.0","engineVersion":"0.1.0","goVersion":"go1.27.2","platform":"linux/amd64","pid":4821,"maxParallelism":8,"drivers":[…]}}
← {"jsonrpc":"2.0","method":"engine.ready","params":{"protocolVersion":"1.0","engineVersion":"0.1.0"}}

→ {"jsonrpc":"2.0","id":2,"method":"connection.test","params":{"profile":{"kind":"sqlite","file":"./app.db","readOnly":true}}}
← {"jsonrpc":"2.0","id":2,"result":{"ok":true,"kind":"sqlite","driver":"sqlite","latencyMs":3,"readOnly":true,"tableCount":6,"capabilities":["file","offline"],"checkedAt":"2026-10-18T02:11:05Z"}}

→ {"jsonrpc":"2.0","id":3,"method":"export.run","params":{"jobId":"3f2a…","profile":{…},"masking":[…],"options":{"outputPath":"./seeds/seed.sql","seed":"demo"}}}
← {"jsonrpc":"2.0","method":"export.progress","params":{"jobId":"3f2a…","phase":"streaming","table":"users","tableIndex":0,"tableCount":6,"rowsProcessed":1200,"percent":18.4,…}}
← {"jsonrpc":"2.0","method":"export.table","params":{"table":"users","rows":1200,"maskedCells":2400,…}}
← {"jsonrpc":"2.0","id":3,"result":{"jobId":"3f2a…","ok":true,"outputPath":"./seeds/seed.sql","totalRows":5403,"checksum":"b7a2d44e5a4d…",…}}
```
