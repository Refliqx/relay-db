/**
 * flow.svelte.ts: RelayDB 4-step flow state.
 *
 * THIS IS THE SINGLE HOME OF APPLICATION STATE. Components only read/write this
 * store; no component keeps its own copy of domain state. As a consequence:
 * moving back and forth between steps loses no configuration, and the exported
 * preset always reflects what is on screen.
 */

import { connection, exporter, schema } from "$lib/ipc/client";
import type {
  ExportOptions,
  ExportPlan,
  ExportProgress,
  ExportResult,
  GraphResult,
  MaskRule,
  MaskStrategy,
  MaskStrategyInfo,
  OrderResult,
  PruneRule,
  SchemaSnapshot,
  Table,
  TestConnectionResult,
  Profile,
} from "$lib/types/protocol";

export const STEP_TITLES = [
  "Connection",
  "Schema & Relationships",
  "Masking & Pruning",
  "Export",
] as const;
export type Step = 1 | 2 | 3 | 4;

/** Rule key: case-insensitive, following the SQL table-naming convention. */
export function ruleKey(table: string, column: string): string {
  return `${table}.${column}`.toLowerCase();
}

export function tableKey(table: Table): string {
  return table.schema && !["main", "public"].includes(table.schema)
    ? `${table.schema}.${table.name}`
    : table.name;
}

class FlowStore {
  step = $state<Step>(1);

  // Step 1
  profile = $state<Profile>({
    id: "local",
    name: "Local database",
    kind: "sqlite",
    readOnly: true,
    connectTimeoutMs: 8000,
  });
  testing = $state(false);
  testResult = $state<TestConnectionResult | null>(null);
  testError = $state<{ message: string; hint?: string; code?: number } | null>(null);

  // Step 2
  loadingSchema = $state(false);
  snapshot = $state<SchemaSnapshot | null>(null);
  graph = $state<GraphResult | null>(null);
  order = $state<OrderResult | null>(null);
  maskCatalog = $state<MaskStrategyInfo[]>([]);
  schemaError = $state<string | null>(null);
  activeTable = $state<string | null>(null);

  // Step 3
  /** Kept separate from the Table object so rules survive a reopened connection. */
  rules = $state<Record<string, MaskRule>>({});
  pruning = $state<Record<string, PruneRule>>({});
  included = $state<Record<string, boolean>>({});

  // Step 4
  options = $state<ExportOptions>({
    outputPath: "seeds/seed.sql",
    batchSize: 100,
    includeHeader: true,
    headerTimestamp: false,
    wrapTransaction: true,
    fkGuard: true,
    upsert: false,
    splitPerTable: false,
    prettyPrint: false,
    seed: "relaydb-local-seed",
  });
  plan = $state<ExportPlan | null>(null);
  planning = $state(false);
  running = $state(false);
  jobId = $state<string | null>(null);
  progress = $state<ExportProgress | null>(null);
  result = $state<ExportResult | null>(null);
  exportError = $state<string | null>(null);
  cancelled = $state(false);

  get tables(): Table[] {
    return (this.snapshot?.tables ?? []).filter((t) => t.kind === "table");
  }

  get selectedTables(): Table[] {
    return this.tables.filter((t) => this.included[tableKey(t)] !== false);
  }

  get maskingRules(): MaskRule[] {
    return Object.values(this.rules).filter((r) => r.enabled !== false);
  }

  get pruningRules(): PruneRule[] {
    return Object.values(this.pruning).filter(
      (p) => (p.limit ?? 0) > 0 || (p.where ?? "").length > 0,
    );
  }

  get unmaskedPii(): { table: string; column: string; category: string; confidence: number }[] {
    const out: { table: string; column: string; category: string; confidence: number }[] = [];
    for (const table of this.selectedTables) {
      for (const col of table.columns) {
        if (!col.piiGuess) continue;
        const rule = this.rules[ruleKey(table.name, col.name)];
        if (!rule || rule.enabled === false || rule.strategy === "keep") {
          out.push({
            table: table.name,
            column: col.name,
            category: col.piiGuess,
            confidence: col.piiConfidence ?? 0,
          });
        }
      }
    }
    return out;
  }

  // Step 1: Test Connection

  async testConnection(): Promise<boolean> {
    this.testing = true;
    this.testError = null;
    this.testResult = null;
    try {
      this.testResult = await connection.test($state.snapshot(this.profile));
      return true;
    } catch (err) {
      const e = err as { message: string; hint?: string; code?: number };
      this.testError = { message: e.message, hint: e.hint, code: e.code };
      return false;
    } finally {
      this.testing = false;
    }
  }

  async chooseSqliteFile(): Promise<void> {
    const path = await connection.pickSqliteFile();
    if (!path) return;
    this.profile = { ...this.profile, kind: "sqlite", file: path, dsn: "" };
    this.invalidateSchema();
  }

  /** Any change to the connection identity invalidates the introspected schema. */
  invalidateSchema(): void {
    this.snapshot = null;
    this.graph = null;
    this.order = null;
    this.plan = null;
    this.result = null;
  }

  // Step 2: Schema

  async loadSchema(opts?: { exactRowCount?: boolean; includeViews?: boolean }): Promise<boolean> {
    this.loadingSchema = true;
    this.schemaError = null;
    try {
      const res = await schema.introspect($state.snapshot(this.profile), opts);
      this.snapshot = res.snapshot;
      this.graph = res.graph;
      this.order = res.order;
      this.maskCatalog = res.maskCatalog;

      // Default: every table is included, and PII columns get a suggested rule.
      const included: Record<string, boolean> = {};
      for (const t of res.snapshot.tables) {
        if (t.kind === "table") included[tableKey(t)] = true;
      }
      this.included = included;
      this.suggestRules();
      this.activeTable = res.order.order[0] ?? res.snapshot.tables[0]?.name ?? null;
      this.step = 2;
      return true;
    } catch (err) {
      this.schemaError = (err as Error).message;
      return false;
    } finally {
      this.loadingSchema = false;
    }
  }

  // Step 3: Masking & Pruning

  /**
   * Fills in rules from the engine's PII heuristics. This is what makes Step 3
   * feel "ready to use": the user audits suggestions rather than writing from
   * scratch.
   */
  suggestRules(): void {
    if (!this.snapshot) return;
    const next: Record<string, MaskRule> = { ...this.rules };
    for (const table of this.tables) {
      for (const col of table.columns) {
        const key = ruleKey(table.name, col.name);
        if (next[key]) continue; // don't overwrite the user's manual decision
        const category = col.piiGuess;
        if (!category) continue;
        const strategy = suggestedStrategy(category);
        if (!strategy) continue;
        next[key] = {
          table: table.name,
          column: col.name,
          strategy,
          options: defaultOptions(strategy),
          enabled: true,
        };
      }
    }
    this.rules = next;
  }

  setStrategy(table: string, column: string, strategy: MaskStrategy, options?: Record<string, unknown>): void {
    const key = ruleKey(table, column);
    if (strategy === "keep") {
      const { [key]: _removed, ...rest } = this.rules;
      this.rules = rest;
      return;
    }
    this.rules = {
      ...this.rules,
      [key]: {
        table,
        column,
        strategy,
        options: options ?? defaultOptions(strategy),
        enabled: true,
      },
    };
    this.plan = null;
  }

  toggleRule(table: string, column: string, enabled: boolean): void {
    const key = ruleKey(table, column);
    const existing = this.rules[key];
    if (!existing) return;
    this.rules = { ...this.rules, [key]: { ...existing, enabled } };
    this.plan = null;
  }

  setPrune(table: string, prune: PruneRule | null): void {
    if (!prune || ((prune.limit ?? 0) <= 0 && !prune.where)) {
      const { [table]: _removed, ...rest } = this.pruning;
      this.pruning = rest;
      return;
    }
    this.pruning = { ...this.pruning, [table]: prune };
    this.plan = null;
  }

  tidy(): void {
    const next: Record<string, MaskRule> = {};
    for (const rule of Object.values(this.rules)) {
      const table = this.tables.find((t) => t.name === rule.table);
      if (table?.columns.some((c) => c.name === rule.column)) next[ruleKey(rule.table, rule.column)] = rule;
    }
    this.rules = next;
  }

  // Step 4: Export

  get runParams() {
    return {
      profile: $state.snapshot(this.profile),
      selection: this.tables.map((t) => ({
        schema: t.schema,
        table: t.name,
        include: this.included[tableKey(t)] !== false,
      })),
      masking: $state.snapshot(this.maskingRules),
      pruning: $state.snapshot(this.pruningRules),
      options: $state.snapshot(this.options),
    };
  }

  async buildPlan(): Promise<ExportPlan | null> {
    this.planning = true;
    this.exportError = null;
    try {
      this.plan = await exporter.plan(this.runParams);
      return this.plan;
    } catch (err) {
      this.exportError = (err as Error).message;
      return null;
    } finally {
      this.planning = false;
    }
  }

  async chooseOutput(): Promise<void> {
    const path = await exporter.pickSaveFile(fileNameFrom(this.options.outputPath));
    if (!path) return;
    this.options = { ...this.options, outputPath: path };
    this.plan = null;
  }

  async run(): Promise<ExportResult | null> {
    // The jobId is created here (not in the engine) so the UI can correlate
    // progress notifications immediately, before the request is even built.
    const jobId = crypto.randomUUID();
    this.jobId = jobId;
    this.running = true;
    this.cancelled = false;
    this.exportError = null;
    this.result = null;
    this.progress = null;
    try {
      const res = await exporter.run(this.runParams, jobId);
      this.result = res;
      return res;
    } catch (err) {
      this.exportError = (err as Error).message;
      return null;
    } finally {
      this.running = false;
      this.jobId = null;
    }
  }

  async cancel(): Promise<void> {
    if (!this.jobId) return;
    this.cancelled = true;
    await exporter.cancel(this.jobId).catch(() => undefined);
  }

  /** Builds a Preset (relaydb.yaml) from the current UI state. */
  toPreset() {
    return {
      version: 1,
      profile: $state.snapshot(this.profile),
      masking: $state.snapshot(this.maskingRules),
      pruning: $state.snapshot(this.pruningRules),
      selection: this.tables.map((t) => ({
        schema: t.schema,
        table: t.name,
        include: this.included[tableKey(t)] !== false,
      })),
      export: $state.snapshot(this.options),
    };
  }
}

function fileNameFrom(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || "seed.sql";
}

export function suggestedStrategy(category: string): MaskStrategy | null {
  switch (category) {
    case "email":
      return "fake_email";
    case "password":
      return "default_hash";
    case "phone":
      return "fake_phone";
    case "person_name":
      return "fake_name";
    case "national_id":
    case "credit_card":
      return "redact";
    case "address":
    case "geo":
    case "date_of_birth":
      return "scramble";
    case "ip":
    case "user_agent":
      return "nullify";
    case "free_text":
      return "scramble";
    default:
      return null;
  }
}

export function defaultOptions(strategy: MaskStrategy): Record<string, unknown> {
  switch (strategy) {
    case "fake_email":
      return { domain: "example.com", style: "first.last" };
    case "fake_phone":
      return { country_code: "+62", length: 11 };
    case "default_hash":
      return { algo: "bcrypt", value: "password123" };
    case "redact":
      return { keep_start: 2, keep_end: 2 };
    case "scramble":
      return { keep_prefix: 0, keep_suffix: 0 };
    case "hash_sha256":
      return { prefix: "" };
    case "random_int":
      return { min: 1, max: 1_000_000 };
    case "json_scrub":
      return { keys: { token: "hash_sha256", password: "default_hash", phone: "fake_phone", email: "fake_email" } };
    default:
      return {};
  }
}

export const flow = new FlowStore();
