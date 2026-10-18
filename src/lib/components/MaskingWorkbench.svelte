<script lang="ts">
  /**
   * MaskingWorkbench: Step 3.
   *
   * Layout: table list + search on the left, per-column editor on the right.
   *
   * Important product decision: rules are NOT forced onto every column.
   * Sensitive columns that are not yet masked are shown as a warning the user
   * can see before export, not blocked. Reason: some columns with a "sensitive
   * name" should be left alone (e.g. `product_name`), and a tool that nags too
   * much gets its warnings turned off by the user.
   */
  import { flow, ruleKey, tableKey, defaultOptions } from "$lib/stores/flow.svelte";
  import { formatRows } from "$lib/utils/format";
  import type { MaskStrategy, PiiCategory } from "$lib/types/protocol";
  import { ArrowLeft, ArrowRight, RotateCcw, Search, Scissors, ShieldCheck, TriangleAlert } from "lucide-svelte";

  const active = $derived(flow.tables.find((t) => t.name === flow.activeTable) ?? flow.tables[0]);

  const strategies = $derived(
    flow.maskCatalog.length > 0
      ? flow.maskCatalog
      : ([
          { strategy: "keep", label: "Keep (passthrough)", description: "leave the original value" },
          { strategy: "nullify", label: "Nullify", description: "set to NULL" },
          { strategy: "scramble", label: "Scramble", description: "randomised, shape preserved" },
          { strategy: "fake_email", label: "Fake email", description: "deterministic email" },
        ] as unknown as typeof flow.maskCatalog),
  );

  let tableQuery = $state("");
  let columnQuery = $state("");

  const tables = $derived(
    flow.tables.filter((t) => t.name.toLowerCase().includes(tableQuery.trim().toLowerCase())),
  );

  const columns = $derived(
    (active?.columns ?? []).filter((c) =>
      c.name.toLowerCase().includes(columnQuery.trim().toLowerCase()),
    ),
  );

  const strategyOf = (table: string, column: string): MaskStrategy =>
    flow.rules[ruleKey(table, column)]?.strategy ?? "keep";
  const enabledOf = (table: string, column: string): boolean =>
    flow.rules[ruleKey(table, column)]?.enabled !== false;

  function labelOf(strategy: MaskStrategy): string {
    return strategies.find((s) => s.strategy === strategy)?.label ?? strategy;
  }

  function optionValue(table: string, column: string, key: string): string {
    const raw = flow.rules[ruleKey(table, column)]?.options?.[key];
    if (raw === undefined || raw === null) return "";
    return typeof raw === "object" ? JSON.stringify(raw) : String(raw);
  }

  function setOption(table: string, column: string, key: string, value: string, numeric: boolean) {
    const k = ruleKey(table, column);
    const rule = flow.rules[k];
    if (!rule) return;
    let parsed: unknown = value;
    if (numeric && value !== "") parsed = Number(value);
    else if (value.startsWith("{") || value.startsWith("[")) {
      try {
        parsed = JSON.parse(value);
      } catch {
        parsed = value; // keep it raw until the JSON is valid
      }
    }
    flow.rules = { ...flow.rules, [k]: { ...rule, options: { ...(rule.options ?? {}), [key]: parsed } } };
    flow.plan = null;
  }

  function setPruneLimit(table: string, value: string) {
    const existing = flow.pruning[table];
    flow.setPrune(table, {
      table,
      limit: value === "" ? null : Number(value),
      orderBy: existing?.orderBy ?? [],
      where: existing?.where ?? "",
    });
  }

  function setPruneWhere(table: string, where: string) {
    const existing = flow.pruning[table];
    flow.setPrune(table, {
      table,
      limit: existing?.limit ?? null,
      orderBy: existing?.orderBy ?? [],
      where,
    });
  }

  function setPruneOrder(table: string, column: string) {
    const existing = flow.pruning[table];
    flow.setPrune(table, {
      table,
      limit: existing?.limit ?? null,
      where: existing?.where ?? "",
      orderBy: column ? [{ column, direction: "desc" }] : [],
    });
  }

  const OPTION_LABEL: Record<string, string> = {
    domain: "Domain",
    style: "Style",
    country_code: "Country code",
    length: "Length",
    algo: "Algorithm",
    value: "Replacement value",
    keep_start: "Keep start",
    keep_end: "Keep end",
    keep_prefix: "Keep prefix",
    keep_suffix: "Keep suffix",
    prefix: "Prefix",
    min: "Minimum",
    max: "Maximum",
    keys: "Key mapping (JSON)",
  };

  const piiLabel: Record<PiiCategory, string> = {
    email: "email",
    password: "password/secret",
    phone: "phone",
    person_name: "person name",
    address: "address",
    national_id: "national ID (NIK/KTP)",
    credit_card: "credit card",
    date_of_birth: "date of birth",
    geo: "coordinates",
    ip: "IP address",
    user_agent: "user agent",
    free_text: "free text",
    "": "",
  };

  const maskedCount = $derived(
    (active?.columns ?? []).filter((c) => flow.rules[ruleKey(active?.name ?? "", c.name)]).length,
  );
</script>

{#if !flow.snapshot}
  <div class="panel panel-body text-center text-[12.5px] text-muted">
    Load the schema first (steps 1–2).
  </div>
{:else}
  <div class="grid gap-4 lg:grid-cols-[minmax(0,300px)_minmax(0,1fr)]">
    <!-- Table list -->
    <section class="panel flex flex-col self-start overflow-hidden lg:sticky lg:top-4">
      <header class="panel-head">
        <h3 class="text-[12.5px] font-semibold text-ink">
          Tables <span class="tnum font-normal text-faint">{tables.length}/{flow.tables.length}</span>
        </h3>
      </header>
      <div class="border-b border-line p-2">
        <div class="relative">
          <Search size={13} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" />
          <input class="field pl-7" bind:value={tableQuery} placeholder="Search tables…" autocomplete="off" />
        </div>
      </div>
      <div class="max-h-[540px] overflow-auto scroll">
        {#each tables as table (tableKey(table))}
          {@const key = tableKey(table)}
          {@const ruleCount = table.columns.filter((c) => flow.rules[ruleKey(table.name, c.name)]).length}
          <button class="row-btn" data-active={active?.name === table.name} onclick={() => (flow.activeTable = table.name)}>
            <span class="min-w-0 flex-1">
              <span class="block truncate mono text-[11.5px] text-ink">{table.name}</span>
              <span class="tnum text-[10.5px] text-faint">{formatRows(table.estimatedRows)} rows</span>
            </span>
            {#if ruleCount > 0}
              <span class="chip chip-accent shrink-0">{ruleCount}</span>
            {/if}
            {#if flow.pruning[key]?.limit}
              <span class="chip chip-warn shrink-0">≤{flow.pruning[key].limit}</span>
            {/if}
          </button>
        {:else}
          <p class="px-3 py-6 text-center text-[11.5px] text-faint">No matching tables.</p>
        {/each}
      </div>
    </section>

    <!-- Editor -->
    <div class="grid min-w-0 gap-4">
      {#if flow.unmaskedPii.length > 0}
        <div class="flex items-start gap-2.5 rounded-md border border-warn/35 bg-warn/[0.07] px-3.5 py-2.5">
          <TriangleAlert size={15} class="mt-0.5 shrink-0 text-warn" />
          <div class="min-w-0">
            <p class="text-[12.5px] font-medium text-warn">
              {flow.unmaskedPii.length} sensitive columns not yet masked
            </p>
            <p class="mt-0.5 text-[11px] text-muted">
              {flow.unmaskedPii
                .slice(0, 6)
                .map((p) => `${p.table}.${p.column} (${piiLabel[p.category as PiiCategory] ?? p.category})`)
                .join(" · ")}{flow.unmaskedPii.length > 6
                ? ` · +${flow.unmaskedPii.length - 6} more`
                : ""}
            </p>
          </div>
        </div>
      {/if}

      {#if active}
        <!-- pruning -->
        <section class="panel overflow-hidden">
          <header class="panel-head">
            <div class="flex items-center gap-2">
              <Scissors size={14} class="text-faint" />
              <h3 class="text-[12.5px] font-semibold text-ink">
                Sampling &amp; pruning · <span class="mono font-normal text-muted">{active.name}</span>
              </h3>
            </div>
            <span class="hint">LIMIT · WHERE · ORDER BY</span>
          </header>
          <div class="panel-body grid gap-3 sm:grid-cols-3">
            <label class="block">
              <span class="label mb-1 block">Row limit</span>
              <input
                class="field"
                type="number"
                min="0"
                placeholder="all"
                value={flow.pruning[active.name]?.limit ?? ""}
                oninput={(e) => setPruneLimit(active.name, e.currentTarget.value)}
              />
            </label>
            <label class="block">
              <span class="label mb-1 block">Order by</span>
              <select
                class="field"
                value={flow.pruning[active.name]?.orderBy?.[0]?.column ?? ""}
                onchange={(e) => setPruneOrder(active.name, e.currentTarget.value)}
              >
                <option value="">default (primary key)</option>
                {#each active.columns as col (col.name)}
                  <option value={col.name}>{col.name}</option>
                {/each}
              </select>
            </label>
            <label class="block">
              <span class="label mb-1 block">WHERE filter (optional)</span>
              <input
                class="field mono text-[11.5px]"
                placeholder="status = 'active'"
                value={flow.pruning[active.name]?.where ?? ""}
                oninput={(e) => setPruneWhere(active.name, e.currentTarget.value)}
              />
            </label>
          </div>
          <div class="border-t border-line px-3.5 py-2">
            <p class="hint">
              Order &amp; filter are applied per table during export. Rows that are pruned do not
              affect other tables, but a dangling child relationship becomes
              <span class="mono"> NULL</span> if its FK is nullable.
            </p>
          </div>
        </section>

        <!-- columns -->
        <section class="panel overflow-hidden">
          <header class="panel-head">
            <div class="flex items-center gap-2">
              <ShieldCheck size={14} class="text-accent" />
              <h3 class="text-[12.5px] font-semibold text-ink">
                Columns <span class="tnum font-normal text-faint">{maskedCount}/{active.columns.length} masked</span>
              </h3>
            </div>
            <div class="relative w-44">
              <Search size={12} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" />
              <input class="field field-xs pl-7" bind:value={columnQuery} placeholder="Search columns…" autocomplete="off" />
            </div>
          </header>

          <div class="max-h-[520px] divide-y divide-line/60 overflow-auto scroll">
            {#each columns as col (col.name)}
              {@const rule = flow.rules[ruleKey(active.name, col.name)]}
              {@const catalog = strategies.find((s) => s.strategy === strategyOf(active.name, col.name))}
              <div class="px-3.5 py-2.5">
                <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2">
                      <span class="truncate mono text-[11.5px] text-ink">{col.name}</span>
                      <span class="mono text-[10.5px] text-faint">{col.dataType}</span>
                    </div>
                    {#if col.piiGuess || rule}
                      <div class="mt-1 flex flex-wrap items-center gap-1.5">
                        {#if col.piiGuess}
                          <span class="chip chip-warn">
                            {piiLabel[col.piiGuess] || col.piiGuess}
                            <span class="tnum opacity-70">{Math.round((col.piiConfidence ?? 0) * 100)}%</span>
                          </span>
                        {/if}
                        {#if rule}
                          <span class="chip chip-accent">{labelOf(rule.strategy)}</span>
                        {/if}
                      </div>
                    {/if}
                  </div>

                  <select
                    class="field w-auto min-w-[190px]"
                    value={strategyOf(active.name, col.name)}
                    onchange={(e) =>
                      flow.setStrategy(
                        active.name,
                        col.name,
                        e.currentTarget.value as MaskStrategy,
                        defaultOptions(e.currentTarget.value as MaskStrategy),
                      )}
                  >
                    {#each strategies as s (s.strategy)}
                      <option value={s.strategy}>{s.label}</option>
                    {/each}
                  </select>

                  {#if rule}
                    <label class="flex shrink-0 items-center gap-1.5 text-[11px] text-muted">
                      <input
                        type="checkbox"
                        checked={enabledOf(active.name, col.name)}
                        onchange={(e) => flow.toggleRule(active.name, col.name, e.currentTarget.checked)}
                      />
                      enabled
                    </label>
                    <button
                      class="btn btn-ghost btn-icon btn-sm shrink-0"
                      title="Revert to keep"
                      aria-label="Revert to keep"
                      onclick={() => flow.setStrategy(active.name, col.name, "keep")}
                    >
                      <RotateCcw size={13} />
                    </button>
                  {/if}
                </div>

                {#if catalog?.options?.length && rule}
                  <div class="mt-2.5 grid gap-2.5 sm:grid-cols-3">
                    {#each catalog.options as optKey (optKey)}
                      {@const numeric = ["keep_prefix", "keep_suffix", "keep_start", "keep_end", "length", "min", "max"].includes(optKey)}
                      <label class="block">
                        <span class="label mb-1 block">{OPTION_LABEL[optKey] ?? optKey}</span>
                        <input
                          class="field {numeric ? '' : 'mono text-[11.5px]'}"
                          type={numeric ? "number" : "text"}
                          value={optionValue(active.name, col.name, optKey)}
                          placeholder={optKey === "domain"
                            ? "example.com"
                            : optKey === "country_code"
                              ? "+62"
                              : optKey === "value"
                                ? "password123"
                                : ""}
                          oninput={(e) => setOption(active.name, col.name, optKey, e.currentTarget.value, numeric)}
                        />
                      </label>
                    {/each}
                  </div>
                {/if}
              </div>
            {:else}
              <p class="px-3.5 py-6 text-center text-[11.5px] text-faint">No matching columns.</p>
            {/each}
          </div>
        </section>

        <div class="flex items-center justify-between">
          <button class="btn" onclick={() => (flow.step = 2)}>
            <ArrowLeft size={14} /> Schema
          </button>
          <button class="btn btn-primary" onclick={() => (flow.step = 4)}>
            Continue to export <ArrowRight size={14} />
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}
