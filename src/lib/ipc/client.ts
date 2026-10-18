/**
 * client.ts: the single frontend -> Tauri bridge.
 *
 * Principles:
 *  1. The UI never calls `invoke` raw. Everything goes through typed functions
 *     here, so command names and argument shapes are centralised.
 *  2. Tauri errors (which arrive as a `{code,message,stage,detail}` object) are
 *     normalised into `RelayError` so the UI can branch on `code` instead of
 *     matching strings.
 */

import { invoke } from "@tauri-apps/api/core";
import { ERR, type ErrorCode } from "$lib/types/protocol";
import type {
  EngineStatus,
  ExportPlan,
  ExportResult,
  ExportRunParams,
  HandshakeResult,
  IntrospectResult,
  LoadConfigResult,
  MaskStrategyInfo,
  Preset,
  Profile,
  TestConnectionResult,
  ValidateConfigResult,
} from "$lib/types/protocol";

export class RelayError extends Error {
  readonly code: ErrorCode;
  readonly stage?: string;
  readonly detail?: string;

  constructor(message: string, code: ErrorCode = -1, stage?: string, detail?: string) {
    super(message);
    this.name = "RelayError";
    this.code = code;
    this.stage = stage;
    this.detail = detail;
  }

  /** Suggested action for the user, based on the engine domain code. */
  get hint(): string | undefined {
    switch (this.code) {
      case ERR.authFailed:
        return "Check the user/password, or use a read-only connection (SELECT).";
      case ERR.connectionFailed:
        return "Make sure the host/port is open and the firewall is not blocking it.";
      case ERR.kindUnsupported:
        return "Choose another database kind, or provide a DSN with a recognised scheme.";
      case ERR.configInvalid:
        return "Fix the preset file, then reload.";
      case ERR.cancelled:
        return "Export cancelled. Temporary files have been removed.";
      case ERR.timeout:
        return "The host is responding too slowly. Try increasing Connect Timeout.";
      default:
        return this.detail;
    }
  }
}

function normalize(raw: unknown): RelayError {
  if (raw instanceof RelayError) return raw;

  // Tauri sends an AppError object (Serialize), but we still handle the JSON
  // string form in case the Rust side changes.
  if (typeof raw === "string") {
    try {
      const parsed = JSON.parse(raw);
      return new RelayError(parsed.message ?? raw, parsed.code ?? -1, parsed.stage, parsed.detail);
    } catch {
      return new RelayError(raw);
    }
  }
  if (raw && typeof raw === "object") {
    const o = raw as Record<string, unknown>;
    return new RelayError(
      String(o.message ?? "Unknown error"),
      (o.code as number) ?? -1,
      o.stage as string | undefined,
      o.detail as string | undefined,
    );
  }
  return new RelayError(String(raw));
}

async function call<T>(command: string, args?: Record<string, unknown>): Promise<T> {
  try {
    return await invoke<T>(command, args);
  } catch (raw) {
    throw normalize(raw);
  }
}

export const engine = {
  status: () => call<EngineStatus>("engine_status"),
  handshake: () => call<HandshakeResult>("engine_handshake"),
  restart: () => call<HandshakeResult>("engine_restart"),
  ping: () => call<{ ok: boolean }>("ping_engine"),
};

export const connection = {
  test: (profile: Profile) => call<TestConnectionResult>("test_connection", { profile }),
  pickSqliteFile: () => call<string | null>("pick_sqlite_file"),
};

export const schema = {
  introspect: (profile: Profile, opts?: { exactRowCount?: boolean; includeViews?: boolean }) =>
    call<IntrospectResult>("introspect_schema", {
      profile,
      exactRowCount: opts?.exactRowCount ?? false,
      includeViews: opts?.includeViews ?? false,
    }),
  maskCatalog: () => call<{ strategies: MaskStrategyInfo[] }>("mask_catalog"),
};

export const exporter = {
  plan: (params: ExportRunParams) => call<ExportPlan>("plan_export", { params }),
  run: (params: ExportRunParams, jobId: string) =>
    call<ExportResult>("run_export", { params, jobId }),
  cancel: (jobId: string) => call<{ ok: boolean }>("cancel_export", { jobId }),
  pickSaveFile: (defaultName = "seed.sql") =>
    call<string | null>("pick_save_file", { defaultName }),
  reveal: (path: string) => call<void>("reveal_in_os", { path }),
};

export const preset = {
  load: (path: string, profileId?: string) =>
    call<LoadConfigResult>("load_preset", { path, profileId: profileId ?? null }),
  validate: (value: Preset) => call<ValidateConfigResult>("validate_preset", { preset: value }),
  read: (path: string) => call<string>("read_text_file", { path }),
  write: (path: string, content: string) =>
    call<string>("write_text_file", { path, content }),
};
