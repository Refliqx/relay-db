/**
 * protocol.ts: typed mirror of `engine/internal/protocol/protocol.go`.
 *
 * WHY NOT JUST AUTO-GENERATE?
 * Ideally yes. Until that generator exists, this file is a "frozen contract":
 * it is hand-written but guarded by `scripts/check-protocol.ts`, which compares
 * the method/notification list against the output of
 * `relaydb-engine --dump-schema`. That way drift is caught in CI, not while the
 * app is in use.
 *
 * Naming convention: the engine uses camelCase in JSON (see the Go tags), so the
 * types here use camelCase as-is; there is no transformation at the boundary.
 */

export const PROTOCOL_VERSION = "1.0";

export const METHODS = {
  handshake: "engine.handshake",
  shutdown: "engine.shutdown",
  connectionTest: "connection.test",
  schemaIntrospect: "schema.introspect",
  exportPlan: "export.plan",
  exportRun: "export.run",
  exportCancel: "export.cancel",
  configLoad: "config.load",
  configValidate: "config.validate",
  maskCatalog: "mask.catalog",
} as const;

export const EVENTS = {
  engineStatus: "relaydb://engine-status",
  engineLog: "relaydb://engine-log",
  exportProgress: "relaydb://export-progress",
  exportTable: "relaydb://export-table",
} as const;

/**
 * Notifications sent by the ENGINE (over the JSON-RPC channel). These are
 * different from the EVENTS above, which are Tauri events forwarded by Rust. These two hops are
 * deliberately kept separate so the engine needs to know nothing about Tauri,
 * and so Rust can add/filter notifications.
 */
export const NOTIFICATIONS = {
  engineReady: "engine.ready",
  engineLog: "engine.log",
  exportProgress: "export.progress",
  exportTable: "export.table",
} as const;

/** Domain error codes, which must match the constants in protocol.go. */
export const ERR = {
  engineNotReady: -32000,
  kindUnsupported: -32001,
  connectionFailed: -32002,
  authFailed: -32003,
  introspectFailed: -32004,
  exportFailed: -32005,
  configInvalid: -32006,
  cancelled: -32010,
  timeout: -32099,
} as const;

export type ErrorCode = (typeof ERR)[keyof typeof ERR] | number;

// Connection

export type DbKind = "postgres" | "mysql" | "mariadb" | "sqlite" | "turso";

export interface Profile {
  id?: string;
  name?: string;
  kind: DbKind | "";
  dsn?: string;
  host?: string;
  port?: number;
  database?: string;
  user?: string;
  password?: string;
  file?: string;
  authToken?: string;
  sslMode?: "" | "disable" | "prefer" | "require" | "skip-verify";
  params?: Record<string, string>;
  readOnly?: boolean;
  connectTimeoutMs?: number;
}

export interface ProfileSummary {
  id: string;
  name: string;
  kind: DbKind;
}

export interface DriverInfo {
  kind: DbKind;
  label: string;
  dsnExample: string;
  capabilities: string[];
}

export interface TestConnectionResult {
  ok: boolean;
  kind: DbKind;
  driver: string;
  latencyMs: number;
  serverVersion?: string;
  database?: string;
  host?: string;
  readOnly: boolean;
  capabilities: string[];
  tableCount?: number;
  warnings?: string[];
  checkedAt: string;
}

// Schema

export type PiiCategory =
  | "email"
  | "password"
  | "phone"
  | "person_name"
  | "address"
  | "national_id"
  | "credit_card"
  | "date_of_birth"
  | "geo"
  | "ip"
  | "user_agent"
  | "free_text"
  | "";

export interface Column {
  name: string;
  ordinalPosition: number;
  dataType: string;
  nullable: boolean;
  default?: string | null;
  isPrimaryKey: boolean;
  isAutoIncrement?: boolean;
  isGenerated?: boolean;
  maxLength?: number | null;
  piiGuess?: PiiCategory;
  piiConfidence?: number;
}

export interface Table {
  schema: string;
  name: string;
  kind: "table" | "view";
  estimatedRows: number;
  rowCountExact: boolean;
  columns: Column[];
  primaryKey: string[];
}

export interface ForeignKey {
  constraintName: string;
  schema: string;
  table: string;
  column: string;
  refSchema: string;
  refTable: string;
  refColumn: string;
  onDelete?: string;
  onUpdate?: string;
}

export interface SchemaSnapshot {
  dialect: string;
  database: string;
  tables: Table[];
  foreignKeys: ForeignKey[];
  capturedAt: string;
}

export interface GraphNode {
  id: string;
  label: string;
  estimatedRows: number;
  inDegree: number;
  outDegree: number;
}

export interface GraphEdge {
  from: string;
  to: string;
  column: string;
  refColumn: string;
  constraint: string;
  nullable: boolean;
}

export interface GraphResult {
  nodes: GraphNode[];
  edges: GraphEdge[];
  cycles: string[][];
}

export interface OrderResult {
  order: string[];
  deferred: string[];
  cycles: string[][];
}

export interface MaskStrategyInfo {
  strategy: MaskStrategy;
  label: string;
  description: string;
  options?: string[];
  pii?: PiiCategory[];
}

// Masking & pruning

export type MaskStrategy =
  | "keep"
  | "nullify"
  | "fixed"
  | "scramble"
  | "redact"
  | "hash_sha256"
  | "default_hash"
  | "fake_email"
  | "fake_name"
  | "fake_phone"
  | "fake_uuid"
  | "random_int"
  | "sequential"
  | "fake_address"
  | "fake_company"
  | "json_scrub";

export interface MaskRule {
  table: string;
  column: string;
  strategy: MaskStrategy;
  options?: Record<string, unknown>;
  enabled?: boolean;
}

export interface OrderTerm {
  column: string;
  direction?: "asc" | "desc";
}

export interface PruneRule {
  table: string;
  limit?: number | null;
  where?: string;
  orderBy?: OrderTerm[];
  mode?: "sample" | "head" | "tail" | "all";
}

export interface TableSelection {
  schema?: string;
  table: string;
  include: boolean;
}

export interface IntrospectResult {
  snapshot: SchemaSnapshot;
  graph: GraphResult;
  order: OrderResult;
  maskCatalog: MaskStrategyInfo[];
  profileEcho: ProfileSummary;
  durationMs: number;
}

// Export

export interface ExportOptions {
  outputPath: string;
  dialect?: string;
  batchSize?: number;
  fetchSize?: number;
  wrapTransaction?: boolean;
  fkGuard?: boolean;
  includeDdl?: boolean;
  includeHeader?: boolean;
  headerTimestamp?: boolean;
  upsert?: boolean;
  seed?: string;
  splitPerTable?: boolean;
  prettyPrint?: boolean;
  progressEveryN?: number;
}

export interface ExportStep {
  order: number;
  schema: string;
  table: string;
  rowEstimate: number;
  prune?: PruneRule | null;
  maskedColumns: MaskRule[] | null;
  columns: string[];
  selectSql: string;
  dependsOn: string[] | null;
}

export interface ExportPlan {
  planId: string;
  dialect: string;
  seed: string;
  steps: ExportStep[];
  cycles: string[][];
  warnings: string[];
  outputPath: string;
  createdAt: string;
}

export interface ExportRunParams {
  jobId?: string;
  profile: Profile;
  profilePath?: string;
  selection?: TableSelection[];
  masking?: MaskRule[];
  pruning?: PruneRule[];
  options: ExportOptions;
  dryRun?: boolean;
}

export type ExportPhase = "planning" | "streaming" | "finalizing" | "done" | "error";

export interface ExportProgress {
  jobId: string;
  phase: ExportPhase;
  table?: string;
  tableIndex: number;
  tableCount: number;
  rowsProcessed: number;
  rowsTotal: number;
  bytesWritten: number;
  percent: number;
  elapsedMs: number;
  etaMs?: number;
  message?: string;
}

export interface TableReport {
  schema: string;
  table: string;
  rows: number;
  maskedCells: number;
  bytesWritten: number;
  durationMs: number;
}

export interface ExportResult {
  jobId: string;
  ok: boolean;
  outputPath: string;
  totalRows: number;
  totalBytes: number;
  durationMs: number;
  tables: TableReport[];
  warnings: string[];
  checksum?: string;
  completedAt: string;
}

// Preset (relaydb.yaml) + engine status

export interface Preset {
  version: number;
  profile: Profile;
  masking: MaskRule[];
  pruning: PruneRule[];
  selection?: TableSelection[];
  export: ExportOptions;
}

export interface ConfigIssue {
  severity: "error" | "warning";
  path: string;
  message: string;
}

export interface LoadConfigResult {
  preset?: Preset;
  path: string;
  profiles: ProfileSummary[];
  issues: ConfigIssue[];
}

export interface ValidateConfigResult {
  valid: boolean;
  issues: ConfigIssue[];
}

export type EngineState = "starting" | "ready" | "stopped" | "crashed";

export interface EngineStatus {
  state: EngineState;
  pid?: number;
  engineVersion?: string;
  protocolVersion?: string;
  platform?: string;
  drivers?: DriverInfo[];
  lastError?: string;
}

export interface HandshakeResult {
  protocolVersion: string;
  engineVersion: string;
  goVersion: string;
  platform: string;
  pid: number;
  maxParallelism: number;
  drivers: DriverInfo[];
}

export type LogLevel = "debug" | "info" | "warn" | "error" | "unhandledRejection";
