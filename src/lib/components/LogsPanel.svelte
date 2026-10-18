<script lang="ts">
  /**
   * LogsPanel: engine log drawer at the bottom.
   *
   * Logs are the main diagnostic path when something fails on the Go/Rust side.
   * They are shown densely (monospace, tabular), with a level filter, so the
   * panel can be used for real debugging, not just a decorative "console".
   */
  import { engineStore } from "$lib/stores/engine.svelte";
  import { ui } from "$lib/stores/ui.svelte";
  import { Copy, X } from "lucide-svelte";

  type LevelFilter = "all" | "info" | "warn" | "error";
  let filter = $state<LevelFilter>("all");
  let copied = $state(false);
  let bodyEl: HTMLDivElement | null = $state(null);

  const rank: Record<string, number> = { debug: 0, info: 1, warn: 2, error: 3, unhandledRejection: 3 };

  const shown = $derived(
    engineStore.logs.filter((l) => filter === "all" || rank[l.level] >= rank[filter]),
  );

  function tone(level: string): string {
    if (level === "error" || level === "unhandledRejection") return "text-danger";
    if (level === "warn") return "text-warn";
    if (level === "debug") return "text-faint";
    return "text-muted";
  }

  $effect(() => {
    // Auto-scroll to the bottom whenever there is a new log.
    if (bodyEl && shown.length) bodyEl.scrollTop = bodyEl.scrollHeight;
  });

  async function copyAll() {
    const text = engineStore.logs.map((l) => `[${l.level}] ${l.message}`).join("\n");
    try {
      await navigator.clipboard.writeText(text);
      copied = true;
      setTimeout(() => (copied = false), 1200);
    } catch {
      /* clipboard unavailable: ignore */
    }
  }
</script>

{#if ui.logsOpen}
  <footer class="shrink-0 border-t border-line bg-surface">
    <div class="flex items-center justify-between gap-3 px-3 py-1.5">
      <div class="flex items-center gap-3">
        <span class="label">Engine log</span>
        <div class="flex items-center gap-1">
          {#each ["all", "info", "warn", "error"] as const as lvl (lvl)}
            <button
              class="rounded-sm px-1.5 py-0.5 text-[10.5px] font-medium {filter === lvl
                ? 'bg-raised text-ink'
                : 'text-faint hover:text-muted'}"
              onclick={() => (filter = lvl)}
            >
              {lvl === "all" ? "all" : lvl}
            </button>
          {/each}
        </div>
        <span class="tnum text-[10.5px] text-faint">{shown.length}/{engineStore.logs.length}</span>
      </div>

      <div class="flex items-center gap-1">
        <button class="btn btn-ghost btn-sm" onclick={copyAll} disabled={!engineStore.logs.length}>
          <Copy size={13} />{copied ? "copied" : "copy"}
        </button>
        <button class="btn btn-ghost btn-sm" onclick={() => engineStore.clearLogs()} disabled={!engineStore.logs.length}>
          clear
        </button>
        <button class="btn btn-ghost btn-icon btn-sm" onclick={() => ui.toggleLogs()} aria-label="Close log">
          <X size={14} />
        </button>
      </div>
    </div>

    <div bind:this={bodyEl} class="h-40 overflow-auto scroll border-t border-line bg-canvas px-3 py-2">
      {#each shown as log, i (i)}
        <p class="mono selectable whitespace-pre-wrap text-[11px] leading-[1.5] {tone(log.level)}">
          <span class="text-faint">{log.level.padEnd(5)}</span> {log.message}
        </p>
      {:else}
        <p class="text-[11px] text-faint">
          {engineStore.logs.length === 0
            ? "No engine logs yet."
            : "No logs at this level."}
        </p>
      {/each}
    </div>
  </footer>
{/if}
