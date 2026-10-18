<script lang="ts">
  /**
   * ConnectionForm: Step 1 (Connection Manager).
   *
   * Complete example of the "Test Connection" flow end-to-end:
   *   form -> flow.testConnection() -> IPC client -> Rust command
   *        -> JSON-RPC engine -> Go driver -> result back to the result panel.
   *
   * UX decision: the next button is only enabled once the connection is PROVEN;
   * errors are shown together with the `hint` from RelayError, not just the raw
   * message.
   */
  import { flow } from "$lib/stores/flow.svelte";
  import { formatNumber } from "$lib/utils/format";
  import type { DbKind } from "$lib/types/protocol";
  import type { LucideIcon } from "$lib/types/icon";
  import {
    CircleCheck,
    Cloud,
    Database,
    Eye,
    EyeOff,
    HardDrive,
    Lock,
    Plug,
    Server,
    TriangleAlert,
    Workflow,
  } from "lucide-svelte";

  const kinds: { value: DbKind; label: string; hint: string; icon: LucideIcon }[] = [
    { value: "postgres", label: "PostgreSQL", hint: "Supabase · Neon · RDS", icon: Server },
    { value: "mysql", label: "MySQL", hint: "MariaDB · XAMPP", icon: Database },
    { value: "sqlite", label: "SQLite", hint: "local file", icon: HardDrive },
    { value: "turso", label: "Turso", hint: "libsql remote", icon: Cloud },
  ];

  const p = $derived(flow.profile);
  const isFile = $derived(p.kind === "sqlite");
  const needsHost = $derived(p.kind === "postgres" || p.kind === "mysql" || p.kind === "turso");
  const defaultPort = $derived(p.kind === "postgres" ? 5432 : p.kind === "mysql" ? 3306 : 0);

  let showPassword = $state(false);

  function setKind(kind: DbKind) {
    flow.profile = {
      ...flow.profile,
      kind,
      port: kind === "postgres" ? 5432 : kind === "mysql" ? 3306 : undefined,
      sslMode: kind === "postgres" ? "require" : undefined,
    };
    flow.testResult = null;
    flow.testError = null;
    flow.invalidateSchema();
  }

  async function test() {
    const ok = await flow.testConnection();
    if (ok) flow.loadSchema();
  }
</script>

<div class="grid items-start gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
  <!-- Form -->
  <section class="panel self-start">
    <header class="panel-head">
      <div class="flex items-center gap-2">
        <Plug size={15} class="text-accent" />
        <h2 class="text-[13px] font-semibold text-ink">Data source</h2>
      </div>
      <span class="chip chip-ok"><Lock size={10} /> read-only</span>
    </header>

    <div class="panel-body space-y-4">
      <!-- Choose database kind -->
      <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {#each kinds as k (k.value)}
          {@const Icon = k.icon}
          {@const selected = p.kind === k.value}
          <button
            class="flex flex-col items-start gap-1.5 rounded-sm border px-2.5 py-2 text-left transition-colors
              {selected
              ? 'border-accent bg-accent/[0.08]'
              : 'border-line hover:border-line-strong hover:bg-raised'}"
            onclick={() => setKind(k.value)}
            aria-pressed={selected}
          >
            <Icon size={15} class={selected ? "text-accent" : "text-faint"} />
            <span class="text-[12px] font-medium text-ink">{k.label}</span>
            <span class="text-[10.5px] leading-tight text-faint">{k.hint}</span>
          </button>
        {/each}
      </div>

      {#if isFile}
        <div>
          <span class="label mb-1 block">Database file</span>
          <div class="flex gap-2">
            <input
              class="field mono text-[11.5px]"
              readonly
              value={p.file ?? ""}
              placeholder="no file chosen yet"
            />
            <button class="btn shrink-0" onclick={() => flow.chooseSqliteFile()}>Choose…</button>
          </div>
          <p class="hint mt-1.5">
            Opened with <code class="mono selectable">mode=ro</code> +
            <code class="mono selectable">query_only</code>; the file is never modified.
          </p>
        </div>
      {:else}
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="block">
            <span class="label mb-1 block">Host</span>
            <input
              class="field mono text-[11.5px]"
              bind:value={p.host}
              placeholder={p.kind === "turso" ? "my-db-org.turso.io" : "127.0.0.1"}
              autocomplete="off"
            />
          </label>
          {#if p.kind !== "turso"}
            <label class="block">
              <span class="label mb-1 block">Port</span>
              <input class="field" type="number" bind:value={p.port} placeholder={String(defaultPort || "")} />
            </label>
            <label class="block">
              <span class="label mb-1 block">User</span>
              <input class="field" bind:value={p.user} autocomplete="off" />
            </label>
            <label class="block">
              <span class="label mb-1 block">Password</span>
              <div class="relative">
                <input
                  class="field pr-8"
                  type={showPassword ? "text" : "password"}
                  bind:value={p.password}
                  autocomplete="off"
                />
                <button
                  class="absolute inset-y-0 right-0 grid w-8 place-items-center text-faint hover:text-muted"
                  type="button"
                  onclick={() => (showPassword = !showPassword)}
                  aria-label={showPassword ? "Hide password" : "Show password"}
                >
                  {#if showPassword}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
                </button>
              </div>
            </label>
          {/if}
          <label class="block {p.kind === 'turso' ? 'sm:col-span-2' : ''}">
            <span class="label mb-1 block">Database</span>
            <input
              class="field mono text-[11.5px]"
              bind:value={p.database}
              placeholder={p.kind === "postgres" ? "postgres" : "app_db"}
            />
          </label>
          {#if p.kind === "turso"}
            <label class="block sm:col-span-2">
              <span class="label mb-1 block">Auth token</span>
              <input class="field mono text-[11.5px]" type="password" bind:value={p.authToken} autocomplete="off" />
            </label>
          {/if}
          {#if p.kind === "postgres"}
            <label class="block">
              <span class="label mb-1 block">SSL mode</span>
              <select class="field" bind:value={p.sslMode}>
                <option value="require">require</option>
                <option value="prefer">prefer</option>
                <option value="disable">disable</option>
              </select>
            </label>
          {/if}
        </div>
      {/if}

      <details class="rounded-sm border border-line bg-canvas/60">
        <summary class="cursor-pointer px-3 py-2 text-[12px] text-muted hover:text-ink">
          Use DSN / connection string
        </summary>
        <div class="border-t border-line px-3 py-3">
          <textarea
            class="field mono text-[11.5px]"
            rows="2"
            bind:value={p.dsn}
            placeholder="postgres://user:pass@host:5432/db?sslmode=require"
          ></textarea>
          <p class="hint mt-1.5">
            If a DSN is provided, the structured fields above are ignored. The DB kind is inferred
            from the DSN scheme if none is chosen.
          </p>
        </div>
      </details>

      <div class="flex items-center gap-2 pt-1">
        <button class="btn btn-primary" onclick={test} disabled={flow.testing}>
          {#if flow.testing}
            <Plug size={14} class="animate-pulse" />
            Testing…
          {:else}
            <Plug size={14} />
            Test connection
          {/if}
        </button>
        {#if flow.testResult?.ok}
          <button class="btn" onclick={() => flow.loadSchema()} disabled={flow.loadingSchema}>
            <Workflow size={14} />
            {flow.loadingSchema ? "Reading schema…" : "Load schema"}
          </button>
        {/if}
      </div>
    </div>
  </section>

  <!-- Result -->
  <section class="panel self-start">
    <header class="panel-head">
      <h2 class="text-[13px] font-semibold text-ink">Diagnostics</h2>
    </header>

    <div class="panel-body">
      {#if flow.testing}
        <div class="flex items-center gap-2.5 text-[12.5px] text-muted">
          <span
            class="size-3.5 shrink-0 animate-spin rounded-full border-2 border-line border-t-accent"
          ></span>
          Contacting server & measuring latency…
        </div>
      {:else if flow.testError}
        <div class="rounded-sm border border-danger/40 bg-danger/[0.08] p-3.5">
          <p class="flex items-center gap-2 text-[12.5px] font-medium text-danger">
            <TriangleAlert size={15} />
            Connection failed
          </p>
          <p class="selectable mt-2 text-[12px] text-ink/90">{flow.testError.message}</p>
          {#if flow.testError.hint}
            <p class="mt-2 border-t border-danger/20 pt-2 text-[11.5px] text-muted">{flow.testError.hint}</p>
          {/if}
          {#if flow.testError.code !== undefined}
            <p class="mono tnum mt-1.5 text-[10.5px] text-faint">domain code {flow.testError.code}</p>
          {/if}
        </div>
      {:else if flow.testResult}
        <div class="rounded-sm border border-ok/35 bg-ok/[0.07] p-3.5">
          <p class="flex items-center gap-2 text-[12.5px] font-medium text-ok">
            <CircleCheck size={15} />
            Connection succeeded
            <span class="tnum font-normal text-muted">· {flow.testResult.latencyMs} ms</span>
          </p>
          <dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-[12px]">
            <dt class="text-faint">Driver</dt>
            <dd class="mono selectable text-ink">{flow.testResult.driver}</dd>
            <dt class="text-faint">Server version</dt>
            <dd class="mono selectable truncate text-ink" title={flow.testResult.serverVersion}>
              {flow.testResult.serverVersion ?? "unknown"}
            </dd>
            <dt class="text-faint">Database</dt>
            <dd class="mono selectable truncate text-ink">{flow.testResult.database || "unknown"}</dd>
            {#if flow.testResult.tableCount !== undefined}
              <dt class="text-faint">Tables</dt>
              <dd class="tnum text-ink">{formatNumber(flow.testResult.tableCount)}</dd>
            {/if}
            <dt class="text-faint">Session</dt>
            <dd class="text-ok">{flow.testResult.readOnly ? "read-only" : "read-write"}</dd>
          </dl>

          {#if flow.testResult.capabilities?.length}
            <div class="mt-3 flex flex-wrap gap-1">
              {#each flow.testResult.capabilities as cap (cap)}
                <span class="chip mono">{cap}</span>
              {/each}
            </div>
          {/if}

          {#if flow.testResult.warnings?.length}
            {#each flow.testResult.warnings as w (`${w}`)}
              <p class="mt-2 flex items-start gap-1.5 text-[11.5px] text-warn">
                <TriangleAlert size={12} class="mt-0.5 shrink-0" />
                <span>{w}</span>
              </p>
            {/each}
          {/if}
        </div>
      {:else}
        <div class="py-6 text-center">
          <p class="text-[12.5px] text-muted">No connection tested yet.</p>
          <p class="hint mx-auto mt-1 max-w-xs">
            Fill in the form on the left and press <strong class="text-muted">Test connection</strong>.
            The connection is opened read-only and closed again immediately.
          </p>
        </div>
      {/if}

      {#if flow.testResult?.ok && !flow.snapshot && !flow.loadingSchema}
        <p class="hint mt-3">
          Continue with <strong class="text-muted">Load schema</strong> to build the foreign key graph
          and insert order.
        </p>
      {/if}

      {#if flow.snapshot}
        <div class="mt-3 flex items-center gap-2 rounded-sm border border-ok/30 bg-ok/[0.06] px-3 py-2 text-[12px]">
          <CircleCheck size={14} class="shrink-0 text-ok" />
          <span class="text-ink">Schema loaded</span>
          <span class="tnum text-faint">
            · {flow.snapshot.tables.length} tables · {flow.snapshot.foreignKeys.length} FK relationships ·
            <span class="mono">{flow.snapshot.dialect}</span>
          </span>
        </div>
      {/if}

      {#if flow.schemaError}
        <p class="selectable mt-3 text-[12px] text-danger">{flow.schemaError}</p>
      {/if}
    </div>
  </section>
</div>
