<script lang="ts">
  /**
   * StepNav: 4-step stepper.
   *
   * Gating: the next step stays locked until its prerequisite is proven (a live
   * connection, a loaded schema). This stops the user pressing "Export" on top
   * of a configuration that has never been tested.
   */
  import { flow, STEP_TITLES, type Step } from "$lib/stores/flow.svelte";
  import { Check } from "lucide-svelte";

  const canOpen = $derived.by(() => ({
    1: true,
    2: flow.testResult?.ok === true,
    3: flow.snapshot !== null,
    4: flow.snapshot !== null,
  })) as unknown as Record<Step, boolean>;

  const steps = [1, 2, 3, 4] as const;

  function go(step: Step) {
    if (!canOpen[step]) return;
    flow.step = step;
  }
</script>

<nav class="panel overflow-hidden" aria-label="Seed build steps">
  <ol class="flex">
    {#each steps as step, i (step)}
      {@const enabled = canOpen[step]}
      {@const active = flow.step === step}
      {@const done = enabled && step < flow.step}
      <li class="flex min-w-0 flex-1 items-center">
        <button
          class="group flex min-w-0 flex-1 items-center gap-2.5 px-3.5 py-2.5 text-left transition-colors
            {active ? 'bg-raised' : enabled ? 'hover:bg-raised/60' : 'cursor-not-allowed'}"
          onclick={() => go(step)}
          disabled={!enabled}
          aria-current={active ? "step" : undefined}
        >
          <span
            class="grid size-6 shrink-0 place-items-center rounded-sm border text-[11px] font-semibold transition-colors
              {active
              ? 'border-accent bg-accent text-accent-ink'
              : done
                ? 'border-accent/40 bg-accent/10 text-accent'
                : enabled
                  ? 'border-line text-muted group-hover:border-line-strong'
                  : 'border-line/60 text-faint/60'}"
          >
            {#if done}<Check size={13} strokeWidth={2.5} />{:else}{step}{/if}
          </span>
          <span class="min-w-0 flex-1">
            <span
              class="block truncate text-[12.5px] font-medium {active
                ? 'text-ink'
                : enabled
                  ? 'text-muted group-hover:text-ink'
                  : 'text-faint/60'}">{STEP_TITLES[step - 1]}</span
            >
          </span>
        </button>
        {#if i < steps.length - 1}
          <span class="h-px w-5 shrink-0 {done ? 'bg-accent/50' : 'bg-line'}"></span>
        {/if}
      </li>
    {/each}
  </ol>
</nav>
