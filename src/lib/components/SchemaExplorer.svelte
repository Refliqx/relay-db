<script lang="ts">
  /**
   * SchemaExplorer: Step 2.
   *
   * Shows the list of tables + columns + row estimates and the FK graph. The
   * "insert order" panel is the output of a topological sort, a direct answer
   * to the problem "a standard dump hits FK constraints on import".
   */
  import FkGraph from "./FkGraph.svelte";
  import { flow, tableKey } from "$lib/stores/flow.svelte";
  import { formatRows, formatNumber } from "$lib/utils/format";
  import { Columns3, KeyRound, Link2, ListOrdered, Search, TriangleAlert } from "lucide-svelte";

  const snapshot = $derived(flow.snapshot);
  const activeTable = $derived(flow.tables.find((t) => t.name === flow.activeTable));

  let query = $state("");
  const filtered = $derived(
    flow.tables.filter((t) => t.name.toLowerCase().includes(query.trim().toLowerCase())),
  );

  function toggleInclude(table: string, include: boolean) {
    flow.included = { ...flow.included, [table]: include };
    flow.plan = null;
  }

  function selectAll(include: boolean) {
    const all: Record<string, boolean> = {};
    for (const t of flow.tables) all[tableKey(t)] = include;
    flow.included = all;
    flow.plan = null;
  }

  const columnsOf = (table: string) => snapshot?.tables.find((t) => t.name === table)?.columns ?? [];

  /** Column -> referenced table map, for the FK badge on the column table. */
  const refByColumn = $derived.by(() => {
    const map = new Map<string, string>();
    if (!activeTable) return map;
    for (const e of flow.graph?.edges ?? []) {
      if (e.from === activeTable.name || e.from.endsWith(`.${activeTable.name}`)) {
        map.set(e.column, e.to.replace(/^.*\./, ""));
      }
    }
    return map;
  });
</script>

{#if !snapshot}
  <div class="panel panel-body text-center text-[12.5px] text-muted">
    Schema not loaded yet. Go back to step 1 and press <strong class="text-ink">Test connection</strong>.
  </div>
{:else}
  <div class="grid gap-4 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)]">
    <!-- Table list -->
    <section class="panel flex flex-col self-start overflow-hidden lg:sticky lg:top-4">
      <header class="panel-head">
        <h3 class="text-[12.5px] font-semibold text-ink">
          Tables <span class="tnum font-normal text-faint">{filtered.length}/{flow.tables.length}</span>
        </h3>
        <div class="flex items-center gap-0.5">
          <button class="btn btn-ghost btn-sm" onclick={() => selectAll(true)}>All</button>
          <button class="btn btn-ghost btn-sm" onclick={() => selectAll(false)}>None</button>
        </div>
      </header>

      <div class="border-b border-line p-2">
        <div class="relative">
          <Search size={13} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" />
          <input class="field pl-7" bind:value={query} placeholder="Search tables…" autocomplete="off" />
        </div>
      </div>

      <div class="max-h-[540px] overflow-auto scroll">
        {#each filtered as table (tableKey(table))}
          {@const key = tableKey(table)}
          {@const included = flow.included[key] !== false}
          <div
            class="flex items-center gap-2 border-b border-line/60 px-2.5 py-1.5"
            class:bg-raised={flow.activeTable === table.name}
          >
            <input
              type="checkbox"
              checked={included}
              onchange={(e) => toggleInclude(key, (e.currentTarget as HTMLInputElement).checked)}
              aria-label={`include ${table.name}`}
            />
            <button
              class="min-w-0 flex-1 text-left"
              onclick={() => (flow.activeTable = table.name)}
            >
              <span class="flex items-center gap-1.5">
                <span class="truncate mono text-[11.5px] {included ? 'text-ink' : 'text-faint line-through'}">
                  {table.name}
                </span>
                {#if table.primaryKey.length}
                  <KeyRound size={11} class="shrink-0 text-accent" />
                {/if}
              </span>
              <span class="tnum text-[10.5px] text-faint">
                {formatRows(table.estimatedRows)}{table.rowCountExact ? "" : "+"} rows ·
                {table.columns.length} columns
              </span>
            </button>
            {#if flow.pruning[key]?.limit}
              <span class="chip chip-accent shrink-0">limit {flow.pruning[key].limit}</span>
            {/if}
          </div>
        {:else}
          <p class="px-3 py-6 text-center text-[11.5px] text-faint">No matching tables.</p>
        {/each}
      </div>
    </section>

    <!-- Detail + graph -->
    <div class="grid min-w-0 gap-4">
      {#if flow.graph && flow.order}
        <FkGraph
          graph={flow.graph}
          order={flow.order}
          active={flow.activeTable}
          onselect={(id) => (flow.activeTable = id)}
        />
      {/if}

      <div class="grid gap-4 lg:grid-cols-2">
        <!-- insert order -->
        <section class="panel self-start overflow-hidden">
          <header class="panel-head">
            <div class="flex items-center gap-2">
              <ListOrdered size={14} class="text-faint" />
              <h3 class="text-[12.5px] font-semibold text-ink">Insert order</h3>
            </div>
            <span class="hint">topological</span>
          </header>
          <div class="panel-body">
            {#if flow.order}
              <ol class="space-y-0.5">
                {#each flow.order.order as id, i (id)}
                  <li class="flex items-center gap-2 rounded-sm px-1.5 py-1 text-[11.5px] hover:bg-raised">
                    <span class="tnum w-5 shrink-0 text-right mono text-faint">{i + 1}</span>
                    <button
                      class="flex-1 truncate text-left mono hover:text-accent {id === flow.activeTable
                        ? 'text-accent'
                        : 'text-muted'}"
                      onclick={() => (flow.activeTable = id)}
                    >
                      {id}
                    </button>
                  </li>
                {/each}
              </ol>
              {#if flow.order.deferred.length > 0}
                <p class="mt-3 flex items-start gap-1.5 text-[11.5px] text-warn">
                  <TriangleAlert size={12} class="mt-0.5 shrink-0" />
                  <span>Deferred (inside an FK cycle): <span class="mono">{flow.order.deferred.join(", ")}</span></span>
                </p>
              {/if}
            {:else}
              <p class="hint">No FK relationships, so any order is fine.</p>
            {/if}
          </div>
        </section>

        <!-- column detail -->
        <section class="panel self-start overflow-hidden">
          <header class="panel-head">
            <div class="flex items-center gap-2">
              <Columns3 size={14} class="text-faint" />
              <h3 class="text-[12.5px] font-semibold text-ink">
                {activeTable ? activeTable.name : "Choose a table"}
              </h3>
            </div>
            {#if activeTable}
              <span class="hint tnum">{formatNumber(activeTable.estimatedRows)} rows</span>
            {/if}
          </header>
          <div class="max-h-[300px] overflow-auto scroll">
            {#if activeTable}
              <table class="data-table">
                <thead>
                  <tr>
                    <th>Column</th>
                    <th>Type</th>
                    <th>PII</th>
                  </tr>
                </thead>
                <tbody>
                  {#each activeTable.columns as col (col.name)}
                    <tr>
                      <td class="mono text-ink">
                        <span class="flex items-center gap-1.5">
                          <span class="truncate selectable">{col.name}</span>
                          {#if col.isPrimaryKey}
                            <span class="chip chip-accent"><KeyRound size={9} />pk</span>
                          {/if}
                          {#if !col.nullable}
                            <span class="text-warn" title="NOT NULL">*</span>
                          {/if}
                          {#if refByColumn.get(col.name)}
                            <Link2 size={11} class="shrink-0 text-info" aria-label={`FK to ${refByColumn.get(col.name)}`} />
                          {/if}
                        </span>
                      </td>
                      <td class="mono">{col.dataType}</td>
                      <td>
                        {#if col.piiGuess}
                          <span
                            class="chip chip-warn"
                            title={`confidence ${Math.round((col.piiConfidence ?? 0) * 100)}%`}
                          >
                            {col.piiGuess}
                          </span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {:else}
              <p class="hint p-4">Click a table on the left or a node in the graph.</p>
            {/if}
          </div>
        </section>
      </div>

      <!-- PII column summary -->
      <section class="panel">
        <header class="panel-head">
          <div class="flex items-center gap-2">
            <TriangleAlert size={14} class="text-warn" />
            <h3 class="text-[12.5px] font-semibold text-ink">Sensitive columns detected</h3>
          </div>
          <span class="hint">automatic suggestions applied in step 3</span>
        </header>
        <div class="panel-body">
          <div class="flex flex-wrap gap-1.5">
            {#each flow.tables as table (tableKey(table))}
              {#each columnsOf(table.name) as col (col.name)}
                {#if col.piiGuess}
                  <button
                    class="chip chip-warn mono"
                    onclick={() => (flow.activeTable = table.name)}
                    title={`confidence ${Math.round((col.piiConfidence ?? 0) * 100)}%`}
                  >
                    {table.name}.{col.name} · {col.piiGuess}
                  </button>
                {/if}
              {/each}
            {/each}
          </div>
          <div class="mt-4 flex justify-end">
            <button class="btn btn-primary" onclick={() => (flow.step = 3)}>
              Continue to masking &amp; pruning
            </button>
          </div>
        </div>
      </section>
    </div>
  </div>
{/if}
