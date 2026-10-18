/**
 * engine.svelte.ts: engine status as a reactive store (Svelte 5 runes).
 *
 * This store is deliberately "thin": it only holds what the UI needs and passes
 * decisions to the Rust/Go layer. There is no retry logic in the UI, so if the
 * engine dies, the UI shows the facts + a restart button, rather than trying to
 * guess on its own.
 */

import { engine as api } from "$lib/ipc/client";
import { onEngineLog, onEngineStatus, type EngineLogEvent } from "$lib/ipc/events";
import type { EngineStatus, HandshakeResult } from "$lib/types/protocol";
import type { UnlistenFn } from "@tauri-apps/api/event";

const MAX_LOGS = 300;

class EngineStore {
  status = $state<EngineStatus>({ state: "starting" });
  handshake = $state<HandshakeResult | null>(null);
  logs = $state<EngineLogEvent[]>([]);
  bootError = $state<string | null>(null);

  #unlisten: UnlistenFn[] = [];
  #started = false;

  get ready(): boolean {
    return this.status.state === "ready";
  }

  get drivers() {
    return this.handshake?.drivers ?? this.status.drivers ?? [];
  }

  /** Called once from +layout.svelte. */
  async start(): Promise<void> {
    if (this.#started) return;
    this.#started = true;

    // Attach the listener BEFORE the handshake so the "ready" status is not
    // missed (a common race: the event is sent by the engine before the UI is
    // listening).
    this.#unlisten.push(
      await onEngineStatus((s) => {
        this.status = s;
      }),
      await onEngineLog((log) => {
        this.logs = [...this.logs.slice(-MAX_LOGS + 1), log];
      }),
    );

    await this.refresh();
  }

  async refresh(): Promise<void> {
    try {
      this.status = await api.status();
      if (this.status.state === "ready") {
        this.handshake = await api.handshake();
      }
      this.bootError = null;
    } catch (err) {
      // The handshake can fail if the engine has not finished starting; the UI
      // keeps showing "starting" without throwing an error at the user.
      this.status = await api.status().catch(() => this.status);
      if (this.status.state === "crashed" || this.status.state === "stopped") {
        this.bootError = this.status.lastError ?? "Engine is not running";
      }
    }
  }

  /** Warm-up handshake + bounded status polling (approx. 6 seconds). */
  async waitUntilReady(timeoutMs = 6000): Promise<boolean> {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      await this.refresh();
      if (this.ready && this.handshake) return true;
      await new Promise((r) => setTimeout(r, 250));
    }
    return this.ready;
  }

  async restart(): Promise<void> {
    this.status = { state: "starting" };
    try {
      this.handshake = await api.restart();
      this.status = await api.status();
    } catch (err) {
      this.bootError = (err as Error).message;
      await this.refresh();
    }
  }

  clearLogs(): void {
    this.logs = [];
  }

  dispose(): void {
    for (const un of this.#unlisten) un();
    this.#unlisten = [];
    this.#started = false;
  }
}

export const engineStore = new EngineStore();
