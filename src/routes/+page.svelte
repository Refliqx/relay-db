<script lang="ts">
  /**
   * +page.svelte: 4-step flow orchestrator.
   *
   * This page only chooses which component is shown based on `flow.step`. All
   * domain state lives in the store, so there is no props drilling and no data
   * lost when the user moves between steps.
   */
  import StepNav from "$lib/components/StepNav.svelte";
  import ConnectionForm from "$lib/components/ConnectionForm.svelte";
  import SchemaExplorer from "$lib/components/SchemaExplorer.svelte";
  import MaskingWorkbench from "$lib/components/MaskingWorkbench.svelte";
  import ExportPanel from "$lib/components/ExportPanel.svelte";
  import { engineStore } from "$lib/stores/engine.svelte";
  import { flow } from "$lib/stores/flow.svelte";
  import { CircleAlert } from "lucide-svelte";
</script>

<StepNav />

{#if !engineStore.ready}
  <div class="panel border-warn/30 bg-warn/[0.06]">
    <div class="flex items-start gap-3 px-4 py-3">
      <CircleAlert size={16} class="mt-0.5 shrink-0 text-warn" />
      <div class="min-w-0">
        <p class="text-[12.5px] font-medium text-ink">Engine not ready</p>
        <p class="hint mt-0.5">
          RelayDB uses a local engine (Go) to talk to the database. The engine starts automatically
          when the app opens; if it fails, use the <strong class="text-muted">Restart</strong> button in
          the top right.
        </p>
        {#if engineStore.bootError}
          <pre
            class="selectable mono mt-2 max-h-24 overflow-auto rounded-sm border border-line bg-canvas px-2.5 py-2 text-[11px] text-danger">{engineStore.bootError}</pre>
        {/if}
      </div>
    </div>
  </div>
{/if}

{#if flow.step === 1}
  <ConnectionForm />
{:else if flow.step === 2}
  <SchemaExplorer />
{:else if flow.step === 3}
  <MaskingWorkbench />
{:else}
  <ExportPanel />
{/if}
