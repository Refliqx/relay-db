# RelayDB: Configuration Schema (`relaydb.yaml`)

A RelayDB preset is a single YAML file holding **connection profiles**,
**table selection**, **masking rules**, **pruning rules**, and **export
preferences**. The file is designed to be **committed** to the user's
application repo.

- Machine schema: [`schemas/relaydb.schema.json`](../schemas/relaydb.schema.json) (JSON Schema draft 2020-12).
- Full examples: [`examples/local-mysql.relaydb.yaml`](../examples/local-mysql.relaydb.yaml), [`examples/supabase-staging.relaydb.yaml`](../examples/supabase-staging.relaydb.yaml).
- Parser: `engine/internal/config/config.go` (**Go is the single source of truth**).

> **Why is YAML parsed in Go, not Rust?** So the same file can be used by the
> GUI *and* headless in CI without two parsers that could behave differently:
> `relaydb-engine --preset .relaydb.yaml --profile staging --out seeds/dev.sql`.

---

## 1. Top-level structure

```yaml
version: 1            # required, integer, currently must be 1

profile:  { … }       # ONE profile (compact form) …
# or
profiles: [ { … } ]   # … MANY profiles (local, staging, production)

selection: [ { table, include } ]         # optional; defaults to all tables
masking:   [ { table, column, strategy, options?, enabled? } ]
pruning:   [ { table, limit?, where?, orderBy?, mode? } ]
export:    { … }       # .sql output preferences
```

Rule: at least one of `profile` **or** `profiles` must be present.
`AllProfiles()` normalises both into a single list.

**Strict parsing:** the decoder uses `KnownFields(true)`. A misspelled key
(**typo**) is an **error**, not silently ignored. This prevents "configuration
that looks right but has no effect".

---

## 2. `profile` / `profiles[]`

```yaml
- id: supabase-staging       # required, unique within the file; pattern ^[a-zA-Z0-9._-]+$
  name: Supabase Staging     # optional, display name
  kind: postgres             # postgres | mysql | mariadb | sqlite | turso
  # Either a single DSN…
  dsn: postgres://${USER}:${PASS}@host:5432/db?sslmode=require
  # …or structured fields:
  host: 127.0.0.1
  port: 3306
  database: shop_dev
  user: root
  password: ${MYSQL_LOCAL_PASSWORD:-}   # NEVER literal
  file: ./storage/app.sqlite            # SQLite only
  authToken: ${TURSO_AUTH_TOKEN}        # Turso/libsql only
  sslMode: disable                      # disable | prefer | require | skip-verify
  params: { options: "-c ..." }         # forwarded to the driver
  readOnly: true                        # default; marks intent for audit
  connectTimeoutMs: 5000                # 500..120000
```

Notes:

- If `dsn` is set, the structured fields are ignored.
- `kind` may be left empty when `dsn` uses a recognised scheme
  (`postgres://`, `mysql://`, `libsql://`, a `.sqlite` path). The engine will
  guess it and add a **warning** in Test Connection.
- If `kind`/`dsn`/`file`/`host` are all empty → `invalid params` error.

---

## 3. `selection[]`

```yaml
selection:
  - { table: users,      include: true }
  - { table: activity_logs, include: false }
```

If `selection` is empty, all tables (except views, unless
`includeViews: true`) are exported.

---

## 4. `masking[]`

One rule = one column.

```yaml
masking:
  - table: users
    column: email
    strategy: fake_email
    options: { domain: example.com, style: first.last }
    enabled: true          # false = stored as a note, not executed
```

### 4.1 Available strategies

This catalogue is sent by the engine via `mask.catalog` and compared with TS by
`protocol:check`.

| Strategy | Description | Options |
| --- | --- | --- |
| `keep` | Passthrough; requires an explicit user decision. | *(none)* |
| `nullify` | `→ NULL`. | *(none)* |
| `fixed` | Constant value. | `value` |
| `scramble` | Scramble characters, keeping **length & shape** (letters stay letters, digits stay digits). | `keep_prefix`, `keep_suffix` |
| `redact` | Keep some characters then star them out. | `keep_start`, `keep_end` |
| `hash_sha256` | Deterministic hash; suitable for tokens/IDs. | *(none)* |
| `default_hash` | Constant dev password hash (every account can log in); the salt is derived from `seed` so the result is reproducible. | `algo` (`bcrypt`\|`sha256`\|`base64`), `value` |
| `fake_email` | Valid, deterministic, unique email. | `domain`, `style` |
| `fake_name` | Deterministic personal name. | *(none)* |
| `fake_phone` | Deterministic E.164 phone number. | `country_code`, `length`, `prefix` |
| `fake_uuid` | Deterministic UUID v4. | *(none)* |
| `random_int` | Deterministic integer within a range. | `min`, `max` |
| `sequential` | Unique sequential number 1..n. | *(none)* |
| `fake_address` | Deterministic street address. | *(none)* |
| `fake_company` | Deterministic company name. | *(none)* |
| `json_scrub` | Mask sensitive values **inside** JSON columns. | `keys` |

### 4.2 `json_scrub`: masking inside JSON

Columns such as `settings`, `payload`, `profile` often store secrets as JSON
(invisible to ordinary column masking). `json_scrub` sweeps the document and
masks the **values** at the specified keys:

```yaml
- table: user_settings
  column: payload
  strategy: json_scrub
  options:
    keys:
      token: hash_sha256
      refresh_token: hash_sha256
      phone: fake_phone
      email: fake_email
```

- Works for JSON typed as `TEXT` (string), `[]byte`, or already parsed.
- Keys are **case-insensitive** (normalised to lowercase).
- If the column content is **not** valid JSON, the value is `scramble`d as a
  fallback, so the secret still does not leak.
- If the input is `TEXT`, the output is returned as valid JSON text.

### 4.3 Determinism (important)

Masked output = **HMAC-SHA256(seed, strategy ‖ fingerprint(options) ‖ value)**,
determined by the **value**, not row position. As a result:

- the same value → the same output across **all tables and all runs**
  (relational integrity & UNIQUE preserved);
- changing `export.seed` produces a completely different fake dataset.

See [`ARCHITECTURE.md §7`](./ARCHITECTURE.md#7-deterministic-masking-value-keyed).

---

## 5. `pruning[]`

Prunes the number of rows per table.

```yaml
pruning:
  - table: activity_logs
    limit: 2000
    orderBy: [{ column: created_at, direction: desc }]
  - table: posts
    limit: 500
    where: "status = 'published'"
    mode: tail
```

| Field | Meaning |
| --- | --- |
| `limit` | Maximum number of rows. `null`/`0` = all rows. |
| `where` | Additional WHERE clause (read expressions only; see the guardrail). |
| `orderBy` | Deterministic order before the limit, e.g. take the newest 2,000 rows. |
| `mode` | `sample` \| `head` \| `tail` \| `all`. |

### 5.1 `WHERE` guardrail

`inspectWhere` (the second layer after the read-only session) rejects a clause
containing:

- **write keywords as whole words:** `DROP`, `DELETE`, `UPDATE`, `INSERT`,
  `ALTER`, `TRUNCATE`, `GRANT`, `REVOKE`, `CREATE`, `ATTACH`, `VACUUM`,
  `PRAGMA`, `CALL`, `EXEC`, `EXECUTE`, `COPY`, `LOAD_FILE`, `PG_SLEEP`, `UNION`;
- **dangerous sequences:** `;`, `--`, `/*`, `*/`, `INTO OUTFILE`, `INTO DUMPFILE`.

Matching uses *word boundaries*, so legitimate columns such as `created_at`,
`updated_at`, and `deleted_at` are **not** wrongly rejected.

```yaml
# ACCEPTED
where: "created_at >= '2026-01-01'"
where: "deleted_at IS NULL AND level IN ('error','warn')"

# REJECTED (validation error)
where: "1=1; DROP TABLE users"
where: "status = 'x' -- comment"
```

---

## 6. `export`

```yaml
export:
  outputPath: ./seeds/dev.seed.sql
  dialect: sqlite            # optional; defaults to the source
  batchSize: 100             # rows per INSERT
  fetchSize: 1000            # rows per cursor round-trip
  wrapTransaction: true
  fkGuard: true              # FK-disable when there is a cycle
  includeDdl: false
  includeHeader: true
  headerTimestamp: false     # false → byte-identical output across runs
  upsert: false
  seed: relaydb-local-seed
  splitPerTable: false
  prettyPrint: false
  progressEveryN: 500
```

| Field | Default | Note |
| --- | --- | --- |
| `outputPath` | `seeds/seed.sql` | If empty, the UI asks for it at export time. |
| `dialect` | follows the source | SQL syntax target for the output. |
| `batchSize` | `100` | Trade-off between file size and ease of reviewing the diff. |
| `fetchSize` | `1000` | Read batch size from the driver. |
| `wrapTransaction` | `true` | Wrap the output in a transaction. |
| `fkGuard` | `true` | Include the FK-disable command when the graph has cycles. |
| `includeDdl` | `false` | Include table-creation DDL. |
| `includeHeader` | `true` | Comment header for diff review. |
| `headerTimestamp` | **`false`** | `false` → two runs with the same seed are **byte-identical**. |
| `upsert` | `false` | `ON CONFLICT` / `INSERT IGNORE`. |
| `seed` | `relaydb-default-seed` | Determines the entire masked output. |
| `splitPerTable` | `false` | One file per table. |
| `prettyPrint` | `false` | `INSERT`s on separate lines so the diff stays tidy. |
| `progressEveryN` | `500` | Progress notification frequency. |

---

## 7. Environment variable expansion

String connection fields (`dsn`, `host`, `user`, `database`, `password`, `file`,
`authToken`) support:

```
${VAR}                 # the value of VAR; if absent → empty string
${VAR:-default}        # the value of VAR; if absent → "default"
```

Example:

```yaml
password: ${MYSQL_LOCAL_PASSWORD:-}          # empty when unset
dsn: postgres://${SB_USER}:${SB_PASS}@db.xxxx.supabase.co:5432/postgres
authToken: ${TURSO_AUTH_TOKEN}
```

This is what keeps the preset **safe to commit**: credentials are never stored
as literals in a file shared with the team.

---

## 8. Validation

`Validate` (run on `config.load`, `config.validate`, and `--check-preset`)
reports `ConfigIssue { severity: error|warning, path, message }`.

**Errors** (fail the load/export):

- `version` newer than the engine supports;
- empty or duplicate `profiles[].id`;
- a profile without `kind`/`dsn`, or a SQLite profile without `file`/`dsn`;
- `masking[]` without `table`/`column`/`strategy`, or an unknown strategy;
- `pruning[]` without `table`, a negative `limit`, or a `where` that breaks the
  guardrail.

**Warnings** (non-fatal):

- no profiles at all;
- empty `export.outputPath`;
- empty `export.seed` (results are not unique per project).

`Load` separates **I/O errors** from **validation issues**: a file that exists
but is wrong is still returned together with the issue list, so the UI can show
the editor + problem list at once.

---

## 9. Configuration precedence at export

When `export.run`/`export.plan` is called:

1. rules `masking`/`pruning` from the **request** (unsaved UI edits) **win**;
2. otherwise → use the inline `preset`;
3. otherwise → the engine reads `profilePath` itself via `config.Load`.

Thus the UI can preview changes before the user saves the preset.

---

## 10. Headless usage / CI

Validate a file (exit ≠ 0 on error):

```bash
relaydb-engine --check-preset .relaydb.yaml
```

Export without a GUI:

```bash
relaydb-engine --preset .relaydb.yaml --profile staging --out seeds/dev.sql
# optional: --seed <value> to override the seed
```

From this repo:

```bash
bun run engine:build                     # build the sidecar
./src-tauri/binaries/relaydb-engine-<target-triple> --check-preset examples/local-mysql.relaydb.yaml
```

Because the engine is the same binary used by the GUI, the validation gate in CI
behaves **identically** to what the user sees in the application.

---

## 11. Minimal example

```yaml
version: 1

profile:
  id: local-sqlite
  kind: sqlite
  file: ./storage/app.sqlite
  readOnly: true

masking:
  - { table: users, column: email, strategy: fake_email }
  - { table: users, column: password, strategy: default_hash }

export:
  outputPath: ./seeds/dev.seed.sql
  seed: my-team-seed
```
