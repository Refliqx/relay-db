<script lang="ts">
  /**
   * ShortcutsDialog: shortcuts list. It only contains shortcuts that are
   * actually implemented, so the documentation does not lie.
   */
  import { ui } from "$lib/stores/ui.svelte";
  import { X } from "lucide-svelte";

  const groups: { title: string; items: { keys: string[]; label: string }[] }[] = [
    {
      title: "Navigation",
      items: [
        { keys: ["⌘", "K"], label: "Open command palette" },
        { keys: ["1"], label: "Step 1: Connection" },
        { keys: ["2"], label: "Step 2: Schema & relationships" },
        { keys: ["3"], label: "Step 3: Masking & pruning" },
        { keys: ["4"], label: "Step 4: Export" },
      ],
    },
    {
      title: "General",
      items: [
        { keys: ["?"], label: "Show these shortcuts" },
        { keys: ["Esc"], label: "Close dialog / palette" },
      ],
    },
  ];
</script>

{#if ui.shortcutsOpen}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-canvas/70 px-4"
    role="presentation"
    onclick={(e) => e.currentTarget === e.target && (ui.shortcutsOpen = false)}
  >
    <div
      class="w-full max-w-[440px] overflow-hidden rounded-lg border border-line bg-overlay"
      style="box-shadow: var(--shadow-pop)"
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
    >
      <div class="panel-head">
        <h2 class="text-[13px] font-semibold text-ink">Keyboard shortcuts</h2>
        <button class="btn btn-ghost btn-icon btn-sm" onclick={() => (ui.shortcutsOpen = false)} aria-label="Close">
          <X size={15} />
        </button>
      </div>
      <div class="divide-y divide-line">
        {#each groups as g (g.title)}
          <div class="px-4 py-3">
            <p class="label mb-2">{g.title}</p>
            <ul class="space-y-1.5">
              {#each g.items as it (it.label)}
                <li class="flex items-center justify-between gap-3 text-[12.5px] text-muted">
                  <span>{it.label}</span>
                  <span class="flex items-center gap-1">
                    {#each it.keys as k (k)}<span class="kbd">{k}</span>{/each}
                  </span>
                </li>
              {/each}
            </ul>
          </div>
        {/each}
      </div>
    </div>
  </div>
{/if}
