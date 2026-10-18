<script lang="ts">
  /**
   * ExportPanel: Step 4.
   *
   * Three phases the user sees:
   *   1. Preview  : plan (table order, row counts, masked columns).
   *                 Pure computation, writes no file.
   *   2. Execution: streaming progress from the engine + Cancel button.
   *   3. Result   : summary + checksum (reproducibility proof) + Reveal.
   *
   * Progress notifications arrive via Tauri events and are filtered by jobId,
   * so they cannot get mixed up if the user runs two exports.
   */
  import { onMount } from "svelte";
  import { onExportProgress } from "$lib/ipc/events";
  import { flow } from "$lib/stores/flow.svelte";
  import { exporter } from "$lib/ipc/client";
  import {
    defaultSeedFileName,
    formatBytes,
    formatDuration,
    formatNumber,
    shortPath,
  } from "$lib/utils/format";
  import type { UnlistenFn } from "@tauri-apps/api/event";
  import {
    Ban,
    CircleCheck,
    Copy,
    FileCode,
    FolderOpen,
    ListOrdered,
    Play,
    TriangleAlert,
  } from "lucide-svelte";

  let unlisten: UnlistenFn | null = null;
  let copied = $state(false);

  $effect(() => {
    let disposed = false;
    onExportProgress((p) => {
      if (!flow.jobId || p.jobId === flow.jobId || p.phase === "done") flow.progress = p;
    }).then((fn) => {
      if (disposed) fn();
      else unlisten = fn;
    });
    return () => {
      disposed = true;
      unlisten?.();
      unlisten = null;
    };
  });

  onMount(() => {
    if (flow.snapshot && !flow.options.outputPath.startsWith("/")) {
      const db = flow.snapshot.database ?? "";
      if (db) {
        flow.options = {
          ...flow.options,
          outputPath: flow.options.outputPath.replace(/[^/\\]+$/, defaultSeedFileName(db)),
        };
      }
    }
  });

  const progress = $derived(flow.progress);
  const percent = $derived(Math.min(100, Math.max(0, progress?.percent ?? 0)));
  const maskedCells = $derived(flow.result?.tables.reduce((a, t) => a + t.maskedCells, 0) ?? 0);

  const toggles = [
    { key: "wrapTransaction", label: "Wrap transaction", hint: "BEGIN … COMMIT so the import is atomic" },
    { key: "fkGuard", label: "FK guard", hint: "use when there are FK cycles" },
    { key: "upsert", label: "Upsert", hint: "ON CONFLICT / INSERT IGNORE" },
    { key: "prettyPrint", label: "One tuple per line", hint: "Git diffs are easier to read" },
    { key: "splitPerTable", label: "Split file per table", hint: "one .sql per table" },
    { key: "includeHeader", label: "Include header", hint: "metadata comment at the top of the file" },
    {
      key: "headerTimestamp",
      label: "Timestamp in header",
      hint: "breaks byte-level reproducibility, so leave off for a clean diff",
    },
  ] as const;

  function toggle(key: keyof typeof flow.options) {
    flow.options = { ...flow.options, [key]: !flow.options[key] };
    flow.plan = null;
  }

  async function copyChecksum() {
    if (!flow.result?.checksum) return;
    try {
      await navigator.clipboard.writeText(flow.result.checksum);
      copied = true;
      setTimeout(() => (copied = false), 1200);
    } catch {
      /* clipboard unavailable */
    }
  }
</script>

<div class="grid gap-4">
  <!-- Output & options -->
  <section class="panel overflow-hidden">
    <header class="panel-head">
      <div class="flex items-center gap-2">
        <FileCode size={14} class="text-accent" />
        <h2 class="text-[12.5px] font-semibold text-ink">Output &amp; options</h2>
      </div>
      {#if flow.snapshot}
        <span class="chip mono">{flow.options.dialect || flow.snapshot.dialect}</span>
      {/if}
    </header>

    <div class="panel-body space-y-4">
      <div class="grid gap-3 lg:grid-cols-[1fr_auto]">
        <label class="block">
          <span class="label mb-1 block">Seed file (.sql)</span>
          <input class="field mono text-[11.5px]" bind:value={flow.options.outputPath} />
        </label>
        <div class="flex items-end">
          <button class="btn" onclick={() => flow.chooseOutput()}>
            <FolderOpen size={14} /> Choose location…
          </button>
        </div>
      </div>

      <div class="grid gap-3 sm:grid-cols-3">
        <label class="block">
          <span class="label mb-1 block">Rows per INSERT</span>
          <input class="field" type="number" min="1" bind:value={flow.options.batchSize} />
        </label>
        <label class="block">
          <span class="label mb-1 block">Masking seed</span>
          <input class="field mono text-[11.5px]" bind:value={flow.options.seed} />
        </label>
        <label class="block">
          <span class="label mb-1 block">Output SQL dialect</span>
          <select class="field" bind:value={flow.options.dialect}>
            <option value="">follow source ({flow.snapshot?.dialect ?? "?"})</option>
            <option value="postgres">postgres</option>
            <option value="mysql">mysql</option>
            <option value="sqlite">sqlite</option>
          </select>
        </label>
      </div>

      <div class="grid gap-x-6 gap-y-2.5 border-t border-line pt-3.5 sm:grid-cols-2 lg:grid-cols-3">
        {#each toggles as opt (opt.key)}
          <label class="flex cursor-pointer items-start gap-2.5">
            <input
              class="mt-0.5"
              type="checkbox"
              checked={Boolean(flow.options[opt.key])}
              onchange={() => toggle(opt.key)}
            />
            <span class="min-w-0">
              <span class="block text-[12px] text-ink">{opt.label}</span>
              <span class="block text-[10.5px] leading-snug text-faint">{opt.hint}</span>
            </span>
          </label>
        {/each}
      </div>

      <p class="hint border-t border-line pt-3">
        With the same <span class="mono">seed</span> and the timestamp off, two exports produce
        identical bytes; the checksum below can be used as proof.
      </p>
    </div>
  </section>

  <!-- Plan -->
  <section class="panel overflow-hidden">
    <header class="panel-head">
      <div class="flex items-center gap-2">
        <ListOrdered size={14} class="text-faint" />
        <h3 class="text-[12.5px] font-semibold text-ink">Export plan</h3>
        {#if flow.plan}
          <span class="chip tnum">{flow.plan.steps.length} tables</span>
        {/if}
      </div>
      <button class="btn btn-sm" onclick={() => flow.buildPlan()} disabled={flow.planning}>
        {flow.planning ? "Building…" : flow.plan ? "Rebuild plan" : "Build plan"}
      </button>
    </header>

    {#if flow.plan}
      <div class="max-h-[320px] overflow-auto scroll">
        <table class="data-table">
          <thead>
            <tr>
              <th class="w-10">#</th>
              <th>Table</th>
              <th class="text-right">Rows</th>
              <th>Masking</th>
              <th>Prune</th>
            </tr>
          </thead>
          <tbody>
            {#each flow.plan.steps as step (step.schema + step.table)}
              <tr>
                <td class="tnum mono text-faint">{step.order + 1}</td>
                <td class="mono text-ink">{step.table}</td>
                <td class="tnum text-right">{step.rowEstimate >= 0 ? formatNumber(step.rowEstimate) : "?"}</td>
                <td class="truncate">
                  {step.maskedColumns?.length
                    ? step.maskedColumns.map((m) => `${m.column}:${m.strategy}`).join(", ")
                    : "none"}
                </td>
                <td>{step.prune?.limit ? `limit ${step.prune.limit}` : "no limit"}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if flow.plan.warnings.length > 0}
        <div class="space-y-1 border-t border-warn/30 bg-warn/[0.07] px-3.5 py-2">
          {#each flow.plan.warnings as w (w)}
            <p class="flex items-start gap-1.5 text-[11.5px] text-warn">
              <TriangleAlert size={12} class="mt-0.5 shrink-0" /><span>{w}</span>
            </p>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="panel-body py-8 text-center">
        <p class="text-[12.5px] text-muted">No plan yet.</p>
        <p class="hint mt-1">
          The plan is pure computation on the engine side; it does not touch disk, so it is safe
          to build first.
        </p>
      </div>
    {/if}
  </section>

  <!-- Execution -->
  <section class="panel overflow-hidden">
    <header class="panel-head">
      <div class="flex items-center gap-2">
        <Play size={14} class="text-faint" />
        <h3 class="text-[12.5px] font-semibold text-ink">Execution</h3>
        {#if progress?.phase}
          <span class="chip chip-accent">{progress.phase}</span>
        {/if}
      </div>
      <span class="hint">{flow.selectedTables.length} tables selected</span>
    </header>

    <div class="panel-body space-y-4">
      <div class="flex flex-wrap items-center gap-2">
        <button class="btn btn-primary" onclick={() => flow.run()} disabled={flow.running || !flow.snapshot}>
          {#if flow.running}
            <Play size={14} class="animate-pulse" /> Running…
          {:else}
            <Play size={14} /> Run export
          {/if}
        </button>
        {#if flow.running}
          <button class="btn btn-danger" onclick={() => flow.cancel()}>
            <Ban size={14} /> Cancel
          </button>
        {/if}
        {#if flow.exportError}
          <span class="selectable text-[12px] text-danger">{flow.exportError}</span>
        {/if}
      </div>

      {#if flow.running || progress}
        <div>
          <div class="mb-1.5 flex items-center justify-between text-[11.5px] text-muted">
            <span>
              {progress?.phase === "finalizing"
                ? "Closing file…"
                : progress?.table
                  ? `${progress.table} · table ${progress.tableIndex}/${progress.tableCount}`
                  : "Preparing…"}
            </span>
            <span class="tnum mono text-faint">
              {formatNumber(progress?.rowsProcessed)} rows · {formatBytes(progress?.bytesWritten)}
              {#if progress?.etaMs} · ETA {formatDuration(progress.etaMs)}{/if}
            </span>
          </div>
          <div class="h-1.5 overflow-hidden rounded-full bg-canvas">
            <div
              class="h-full rounded-full bg-accent transition-[width] duration-200"
              style={`width:${percent}%`}
            ></div>
          </div>
          <p class="tnum mono mt-1 text-right text-[10.5px] text-faint">{percent.toFixed(1)}%</p>
        </div>
      {/if}

      {#if flow.result}
        <div class="rounded-md border border-ok/35 bg-ok/[0.07] p-3.5">
          <p class="flex items-center gap-2 text-[12.5px] font-medium text-ok">
            <CircleCheck size={15} /> Seed created
          </p>
          <dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-[12px]">
            <dt class="text-faint">File</dt>
            <dd class="truncate mono selectable text-ink" title={flow.result.outputPath}>
              {shortPath(flow.result.outputPath, 64)}
            </dd>
            <dt class="text-faint">Total rows</dt>
            <dd class="tnum text-ink">{formatNumber(flow.result.totalRows)}</dd>
            <dt class="text-faint">Size</dt>
            <dd class="tnum text-ink">{formatBytes(flow.result.totalBytes)}</dd>
            <dt class="text-faint">Duration</dt>
            <dd class="tnum text-ink">{formatDuration(flow.result.durationMs)}</dd>
            <dt class="text-faint">Masked cells</dt>
            <dd class="tnum text-ink">{formatNumber(maskedCells)}</dd>
            <dt class="text-faint">SHA-256</dt>
            <dd class="flex items-center gap-1.5">
              <span class="truncate mono selectable text-[11px] text-ink" title={flow.result.checksum}>
                {flow.result.checksum ?? "unavailable"}
              </span>
              {#if flow.result.checksum}
                <button class="btn btn-ghost btn-icon btn-sm" onclick={copyChecksum} aria-label="Copy checksum" title="Copy checksum">
                  <Copy size={12} />
                </button>
              {/if}
            </dd>
          </dl>
          {#if copied}
            <p class="mt-1 text-right text-[10.5px] text-ok">checksum copied</p>
          {/if}
          <div class="mt-3 flex items-center gap-2">
            <button class="btn btn-sm" onclick={() => exporter.reveal(flow.result!.outputPath)}>
              <FolderOpen size={13} /> Reveal in folder
            </button>
            <span class="hint">identical checksum across two runs with the same seed → Git diff stays clean</span>
          </div>
        </div>
      {/if}
    </div>
  </section>
</div>
