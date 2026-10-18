/**
 * events.ts: typed wrapper for push events from the engine.
 *
 * All engine notifications (progress, log) pass through Rust and are re-emitted
 * as Tauri events. The UI subscribes through the helpers here, so event names
 * and payload shapes are not scattered as string literals across components.
 */

import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { EVENTS } from "$lib/types/protocol";
import type { EngineStatus, ExportProgress, TableReport } from "$lib/types/protocol";

export interface EngineLogEvent {
  level: string;
  message: string;
  at?: string;
}

export function onEngineStatus(cb: (status: EngineStatus) => void): Promise<UnlistenFn> {
  return listen<EngineStatus>(EVENTS.engineStatus, (e) => cb(e.payload));
}

export function onEngineLog(cb: (log: EngineLogEvent) => void): Promise<UnlistenFn> {
  return listen<EngineLogEvent>(EVENTS.engineLog, (e) => cb(e.payload));
}

export function onExportProgress(cb: (p: ExportProgress) => void): Promise<UnlistenFn> {
  return listen<ExportProgress>(EVENTS.exportProgress, (e) => cb(e.payload));
}

export function onExportTable(cb: (t: TableReport) => void): Promise<UnlistenFn> {
  return listen<TableReport>(EVENTS.exportTable, (e) => cb(e.payload));
}
