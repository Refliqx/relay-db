// Package protocol defines the shared wire (JSON) contract between the
// Go Engine <-> Tauri (Rust) <-> Svelte (frontend).
//
// GOLDEN RULE: this file is the SINGLE SOURCE OF TRUTH for the data shapes.
// Every change here MUST be copied to:
//   - src-tauri/src/engine/protocol.rs
//   - src/lib/types/protocol.ts
//
// Automated verification: `go run ./cmd/relaydb-engine --dump-schema` then
// compared with schemas/ipc.schema.json (see docs/IPC-PROTOCOL.md).
package protocol

import "time"

// ProtocolVersion is the IPC protocol version. The frontend and Rust perform a
// handshake and reject an engine whose major version does not match.
const ProtocolVersion = "1.0"

// Methods exposed by the engine. Grouped by domain prefix.
const (
	MethodEngineHandshake  = "engine.handshake"
	MethodEngineShutdown   = "engine.shutdown"
	MethodConnectionTest   = "connection.test"
	MethodSchemaIntrospect = "schema.introspect"
	MethodSchemaGraph      = "schema.graph"
	MethodExportPlan       = "export.plan"
	MethodExportRun        = "export.run"
	MethodExportCancel     = "export.cancel"
	MethodConfigValidate   = "config.validate"
	MethodConfigLoad       = "config.load"
	MethodMaskCatalog      = "mask.catalog"
)

// Notifications (server -> client, id == nil).
const (
	NotifyEngineReady     = "engine.ready"
	NotifyEngineLog       = "engine.log"
	NotifyExportProgress  = "export.progress"
	NotifyExportTableDone = "export.table"
)

// JSON-RPC 2.0 error codes (reserved) + domain RelayDB codes (-32000..-32099).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	CodeEngineNotReady   = -32000
	CodeKindUnsupported  = -32001
	CodeConnectionFailed = -32002
	CodeAuthFailed       = -32003
	CodeIntrospectFailed = -32004
	CodeExportFailed     = -32005
	CodeConfigInvalid    = -32006
	CodeCancelled        = -32010
	// CodeTimeout is used by the Rust layer (invoke timeout) and is kept
	// distinct from CodeCancelled so the UI can choose the right message.
	CodeTimeout = -32099
)

// Kind (supported database types).
const (
	KindPostgres = "postgres"
	KindMySQL    = "mysql"
	KindMariaDB  = "mariadb"
	KindSQLite   = "sqlite"
	KindTurso    = "turso"
)

// Masking strategies available in the engine.
const (
	StrategyKeep        = "keep"         // passthrough (explicit whitelist)
	StrategyNullify     = "nullify"      // -> NULL
	StrategyFixed       = "fixed"        // -> constant value
	StrategyScramble    = "scramble"     // deterministic character shuffle
	StrategyRedact      = "redact"       // keep N characters + stars
	StrategyHashSHA256  = "hash_sha256"  // deterministic hash
	StrategyDefaultHash = "default_hash" // password hash (bcrypt/argon2/scrypt)
	StrategyFakeEmail   = "fake_email"   // valid, deterministic email
	StrategyFakeName    = "fake_name"    // human name
	StrategyFakePhone   = "fake_phone"   // E.164 phone number
	StrategyFakeUUID    = "fake_uuid"    // deterministic UUID v4
	StrategyRandomInt   = "random_int"   // integer within a range
	StrategySequential  = "sequential"   // 1..n, preserves order
	StrategyFakeAddress = "fake_address"
	StrategyFakeCompany = "fake_company"
	StrategyJSONScrub   = "json_scrub" // masks sensitive keys inside JSON
)

// Configuration / preset

// Profile is a connection configuration. It can be written as a single DSN
// (`dsn:`) or as structured fields (host/port/user/...).
type Profile struct {
	ID       string `json:"id,omitempty" yaml:"id,omitempty"`
	Name     string `json:"name,omitempty" yaml:"name,omitempty"`
	Kind     string `json:"kind" yaml:"kind"`
	DSN      string `json:"dsn,omitempty" yaml:"dsn,omitempty"`
	Host     string `json:"host,omitempty" yaml:"host,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
	Database string `json:"database,omitempty" yaml:"database,omitempty"`
	User     string `json:"user,omitempty" yaml:"user,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	File     string `json:"file,omitempty" yaml:"file,omitempty"`
	// AuthToken is used by Turso/libsql (`authToken=`).
	AuthToken string            `json:"authToken,omitempty" yaml:"authToken,omitempty"`
	SSLMode   string            `json:"sslMode,omitempty" yaml:"sslMode,omitempty"`
	Params    map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
	// ReadOnly is set true by the UI by default: RelayDB never writes to the
	// source.
	ReadOnly         bool `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	ConnectTimeoutMs int  `json:"connectTimeoutMs,omitempty" yaml:"connectTimeoutMs,omitempty"`
}

// MaskRule describes masking on a single column.
type MaskRule struct {
	Table    string         `json:"table" yaml:"table"`
	Column   string         `json:"column" yaml:"column"`
	Strategy string         `json:"strategy" yaml:"strategy"`
	Options  map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
	// Enabled=false -> the rule is stored but not applied (preview feature).
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// OrderTerm is used for deterministic pruning (e.g. take the latest 1,000 rows).
type OrderTerm struct {
	Column    string `json:"column" yaml:"column"`
	Direction string `json:"direction,omitempty" yaml:"direction,omitempty"` // asc|desc
}

// PruneRule trims the number of rows per table.
type PruneRule struct {
	Table   string      `json:"table" yaml:"table"`
	Limit   *int        `json:"limit,omitempty" yaml:"limit,omitempty"`
	Where   string      `json:"where,omitempty" yaml:"where,omitempty"`
	OrderBy []OrderTerm `json:"orderBy,omitempty" yaml:"orderBy,omitempty"`
	// Mode: sample | head | tail | all
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// TableSelection chooses the tables included in the export.
type TableSelection struct {
	Schema  string `json:"schema,omitempty" yaml:"schema,omitempty"`
	Table   string `json:"table" yaml:"table"`
	Include bool   `json:"include" yaml:"include"`
}

// ExportOptions controls the shape of the .sql output.
type ExportOptions struct {
	OutputPath      string `json:"outputPath" yaml:"outputPath"`
	Dialect         string `json:"dialect,omitempty" yaml:"dialect,omitempty"`
	BatchSize       int    `json:"batchSize,omitempty" yaml:"batchSize,omitempty"` // rows per INSERT
	FetchSize       int    `json:"fetchSize,omitempty" yaml:"fetchSize,omitempty"` // rows per cursor round-trip
	WrapTransaction bool   `json:"wrapTransaction,omitempty" yaml:"wrapTransaction,omitempty"`
	FkGuard         bool   `json:"fkGuard,omitempty" yaml:"fkGuard,omitempty"` // SET FOREIGN_KEY_CHECKS / session_replication_role
	IncludeDdl      bool   `json:"includeDdl,omitempty" yaml:"includeDdl,omitempty"`
	IncludeHeader   bool   `json:"includeHeader,omitempty" yaml:"includeHeader,omitempty"`
	// HeaderTimestamp includes the generation time in the header. Defaults to
	// FALSE so two runs with the same seed produce a byte-identical file (clean
	// Git diff).
	HeaderTimestamp bool   `json:"headerTimestamp,omitempty" yaml:"headerTimestamp,omitempty"`
	Upsert          bool   `json:"upsert,omitempty" yaml:"upsert,omitempty"` // ON CONFLICT / INSERT IGNORE
	Seed            string `json:"seed,omitempty" yaml:"seed,omitempty"`     // seed for masking determinism
	SplitPerTable   bool   `json:"splitPerTable,omitempty" yaml:"splitPerTable,omitempty"`
	PrettyPrint     bool   `json:"prettyPrint,omitempty" yaml:"prettyPrint,omitempty"`
	ProgressEveryN  int    `json:"progressEveryN,omitempty" yaml:"progressEveryN,omitempty"`
}

// Preset is a single decoded RelayDB configuration file (relaydb.yaml).
type Preset struct {
	Version   int              `json:"version" yaml:"version"`
	Profile   Profile          `json:"profile" yaml:"profile"`
	Masking   []MaskRule       `json:"masking" yaml:"masking"`
	Pruning   []PruneRule      `json:"pruning" yaml:"pruning"`
	Selection []TableSelection `json:"selection,omitempty" yaml:"selection,omitempty"`
	Export    ExportOptions    `json:"export" yaml:"export"`
}

// Results: handshake & test connection

type DriverInfo struct {
	Kind         string   `json:"kind"`
	Label        string   `json:"label"`
	DSNExample   string   `json:"dsnExample"`
	Capabilities []string `json:"capabilities"`
}

type HandshakeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	EngineVersion   string       `json:"engineVersion"`
	GoVersion       string       `json:"goVersion"`
	Platform        string       `json:"platform"`
	PID             int          `json:"pid"`
	MaxParallelism  int          `json:"maxParallelism"`
	Drivers         []DriverInfo `json:"drivers"`
}

type TestConnectionResult struct {
	OK            bool      `json:"ok"`
	Kind          string    `json:"kind"`
	Driver        string    `json:"driver"`
	LatencyMs     int64     `json:"latencyMs"`
	ServerVersion string    `json:"serverVersion,omitempty"`
	Database      string    `json:"database,omitempty"`
	Host          string    `json:"host,omitempty"`
	ReadOnly      bool      `json:"readOnly"`
	Capabilities  []string  `json:"capabilities"`
	TableCount    int       `json:"tableCount,omitempty"`
	Warnings      []string  `json:"warnings,omitempty"`
	CheckedAt     time.Time `json:"checkedAt"`
}

// Results: schema

type Column struct {
	Name            string  `json:"name"`
	OrdinalPosition int     `json:"ordinalPosition"`
	DataType        string  `json:"dataType"`
	Nullable        bool    `json:"nullable"`
	Default         *string `json:"default,omitempty"`
	IsPrimaryKey    bool    `json:"isPrimaryKey"`
	IsAutoIncrement bool    `json:"isAutoIncrement,omitempty"`
	MaxLength       *int64  `json:"maxLength,omitempty"`
	IsGenerated     bool    `json:"isGenerated,omitempty"`
	// PII heuristics: "email", "phone", "person_name", "password", "address", ...
	PiiGuess      string  `json:"piiGuess,omitempty"`
	PiiConfidence float64 `json:"piiConfidence,omitempty"`
}

type Table struct {
	Schema        string   `json:"schema"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"` // table|view
	EstimatedRows int64    `json:"estimatedRows"`
	RowCountExact bool     `json:"rowCountExact"`
	Columns       []Column `json:"columns"`
	PrimaryKey    []string `json:"primaryKey"`
}

type ForeignKey struct {
	ConstraintName string `json:"constraintName"`
	Schema         string `json:"schema"`
	Table          string `json:"table"`
	Column         string `json:"column"`
	RefSchema      string `json:"refSchema"`
	RefTable       string `json:"refTable"`
	RefColumn      string `json:"refColumn"`
	OnDelete       string `json:"onDelete,omitempty"`
	OnUpdate       string `json:"onUpdate,omitempty"`
}

type SchemaSnapshot struct {
	Dialect     string       `json:"dialect"`
	Database    string       `json:"database"`
	Tables      []Table      `json:"tables"`
	ForeignKeys []ForeignKey `json:"foreignKeys"`
	CapturedAt  time.Time    `json:"capturedAt"`
}

type GraphNode struct {
	ID            string `json:"id"` // "schema.table"
	Label         string `json:"label"`
	EstimatedRows int64  `json:"estimatedRows"`
	InDegree      int    `json:"inDegree"`
	OutDegree     int    `json:"outDegree"`
}

type GraphEdge struct {
	From       string `json:"from"` // child (holds the FK)
	To         string `json:"to"`   // parent (referenced)
	Column     string `json:"column"`
	RefColumn  string `json:"refColumn"`
	Constraint string `json:"constraint"`
	Nullable   bool   `json:"nullable"`
}

type GraphResult struct {
	Nodes  []GraphNode `json:"nodes"`
	Edges  []GraphEdge `json:"edges"`
	Cycles [][]string  `json:"cycles"`
}

type OrderResult struct {
	Order    []string   `json:"order"`    // safe insert order
	Deferred []string   `json:"deferred"` // tables in a cycle (need an FK guard)
	Cycles   [][]string `json:"cycles"`
}

// Results: export

type ExportStep struct {
	Order         int        `json:"order"`
	Schema        string     `json:"schema"`
	Table         string     `json:"table"`
	RowEstimate   int64      `json:"rowEstimate"`
	Prune         *PruneRule `json:"prune,omitempty"`
	MaskedColumns []MaskRule `json:"maskedColumns"`
	Columns       []string   `json:"columns"`
	SelectSQL     string     `json:"selectSql"`
	DependsOn     []string   `json:"dependsOn"`
}

type ExportPlan struct {
	PlanID     string       `json:"planId"`
	Dialect    string       `json:"dialect"`
	Seed       string       `json:"seed"`
	Steps      []ExportStep `json:"steps"`
	Cycles     [][]string   `json:"cycles"`
	Warnings   []string     `json:"warnings"`
	OutputPath string       `json:"outputPath"`
	CreatedAt  time.Time    `json:"createdAt"`
}

type ExportProgress struct {
	JobID         string  `json:"jobId"`
	Phase         string  `json:"phase"` // planning|streaming|finalizing|done|error
	Table         string  `json:"table,omitempty"`
	TableIndex    int     `json:"tableIndex"`
	TableCount    int     `json:"tableCount"`
	RowsProcessed int64   `json:"rowsProcessed"`
	RowsTotal     int64   `json:"rowsTotal"`
	BytesWritten  int64   `json:"bytesWritten"`
	Percent       float64 `json:"percent"`
	ElapsedMs     int64   `json:"elapsedMs"`
	ETAMs         int64   `json:"etaMs,omitempty"`
	Message       string  `json:"message,omitempty"`
}

type TableReport struct {
	Schema       string `json:"schema"`
	Table        string `json:"table"`
	Rows         int64  `json:"rows"`
	MaskedCells  int64  `json:"maskedCells"`
	BytesWritten int64  `json:"bytesWritten"`
	DurationMs   int64  `json:"durationMs"`
}

type ExportResult struct {
	JobID       string        `json:"jobId"`
	OK          bool          `json:"ok"`
	OutputPath  string        `json:"outputPath"`
	TotalRows   int64         `json:"totalRows"`
	TotalBytes  int64         `json:"totalBytes"`
	DurationMs  int64         `json:"durationMs"`
	Tables      []TableReport `json:"tables"`
	Warnings    []string      `json:"warnings"`
	Checksum    string        `json:"checksum,omitempty"`
	CompletedAt time.Time     `json:"completedAt"`
}

// Params

type TestConnectionParams struct {
	Profile Profile `json:"profile"`
}

type IntrospectParams struct {
	Profile Profile `json:"profile"`
	// ExactRowCount forces an exact COUNT(*) (expensive on large tables).
	ExactRowCount bool `json:"exactRowCount,omitempty"`
	IncludeViews  bool `json:"includeViews,omitempty"`
}

// IntrospectResult combines the snapshot + graph + safe FK order into one
// response, so the UI does not need 3 round-trips just to draw Step 2.
type IntrospectResult struct {
	Snapshot    *SchemaSnapshot  `json:"snapshot"`
	Graph       GraphResult      `json:"graph"`
	Order       OrderResult      `json:"order"`
	MaskCatalog []map[string]any `json:"maskCatalog"`
	ProfileEcho ProfileSummary   `json:"profileEcho"`
	DurationMs  int64            `json:"durationMs"`
}

type ExportRunParams struct {
	// JobID is created by the frontend so the UI can call export.cancel and
	// correlate progress notifications with the on-screen job card.
	JobID       string           `json:"jobId,omitempty"`
	Profile     Profile          `json:"profile"`
	Preset      *Preset          `json:"preset,omitempty"`
	ProfilePath string           `json:"profilePath,omitempty"`
	Selection   []TableSelection `json:"selection,omitempty"`
	Masking     []MaskRule       `json:"masking,omitempty"`
	Pruning     []PruneRule      `json:"pruning,omitempty"`
	Options     ExportOptions    `json:"options"`
	DryRun      bool             `json:"dryRun,omitempty"`
}

type CancelParams struct {
	JobID string `json:"jobId"`
}

// LoadConfigParams: the preset is read by the ENGINE (not Rust) so the same
// YAML file can be used headless in CI without duplicating the parser.
type LoadConfigParams struct {
	Path string `json:"path,omitempty"`
	// ProfileID selects a profile from the registry when the file holds several.
	ProfileID string `json:"profileId,omitempty"`
}

type ConfigIssue struct {
	Severity string `json:"severity"` // error|warning
	Path     string `json:"path"`
	Message  string `json:"message"`
}

type LoadConfigResult struct {
	Preset   *Preset          `json:"preset,omitempty"`
	Path     string           `json:"path"`
	Profiles []ProfileSummary `json:"profiles"`
	Issues   []ConfigIssue    `json:"issues"`
}

type ProfileSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
