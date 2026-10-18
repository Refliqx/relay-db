<script lang="ts">
  /**
   * CommandPalette: uniform access to every application action (⌘/Ctrl+K).
   *
   * This is the pattern that separates a tool you use every day from one that
   * "looks good": power users should not have to hunt for buttons. Every command
   * uses the *same gating* as StepNav, so the palette cannot be used to jump to
   * export on top of configuration that has not been tested.
   */
  import type { LucideIcon } from "$lib/types/icon";
  import { ui } from "$lib/stores/ui.svelte";
  import { flow } from "$lib/stores/flow.svelte";
  import { engineStore } from "$lib/stores/engine.svelte";
  import {
    Ban,
    Database,
    ListOrdered,
    Moon,
    Play,
    Plug,
    RefreshCw,
    Search,
    ShieldCheck,
    Sun,
    Table2,
    Terminal,
    Workflow,
    Keyboard,
  } from "lucide-svelte";

  interface Cmd {
    id: string;
    label: string;
    group: string;
    icon: LucideIcon;
    hint?: string;
    run: () => void | Promise<void>;
    enabled: () => boolean;
  }

  const canStep2 = $derived(flow.testResult?.ok === true);
  const canStep3 = $derived(flow.snapshot !== null);

  const commands = $derived<Cmd[]>([
    {
      id: "go-1",
      label: "Connection",
      group: "Flow",
      icon: Plug,
      hint: "1",
      enabled: () => true,
      run: () => {
        flow.step = 1;
      },
    },
    {
      id: "go-2",
      label: "Schema & relationships",
      group: "Flow",
      icon: Table2,
      hint: "2",
      enabled: () => canStep2,
      run: () => {
        flow.step = 2;
      },
    },
    {
      id: "go-3",
      label: "Masking & pruning",
      group: "Flow",
      icon: ShieldCheck,
      hint: "3",
      enabled: () => canStep3,
      run: () => {
        flow.step = 3;
      },
    },
    {
      id: "go-4",
      label: "Export",
      group: "Flow",
      icon: Play,
      hint: "4",
      enabled: () => canStep3,
      run: () => {
        flow.step = 4;
      },
    },
    {
      id: "test",
      label: "Test connection",
      group: "Action",
      icon: Plug,
      enabled: () => !flow.testing,
      run: async () => {
        flow.step = 1;
        if (await flow.testConnection()) flow.loadSchema();
      },
    },
    {
      id: "schema",
      label: "Load schema & relationships",
      group: "Action",
      icon: Workflow,
      enabled: () => canStep2 && !flow.loadingSchema,
      run: () => {
        void flow.loadSchema();
      },
    },
    {
      id: "plan",
      label: "Build export plan",
      group: "Action",
      icon: ListOrdered,
      enabled: () => canStep3 && !flow.planning,
      run: () => {
        flow.step = 4;
        void flow.buildPlan();
      },
    },
    {
      id: "run",
      label: "Run export",
      group: "Action",
      icon: Play,
      enabled: () => canStep3 && !flow.running,
      run: () => {
        flow.step = 4;
        void flow.run();
      },
    },
    {
      id: "cancel",
      label: "Cancel export",
      group: "Action",
      icon: Ban,
      enabled: () => flow.running,
      run: () => flow.cancel(),
    },
    {
      id: "restart",
      label: "Restart engine",
      group: "Action",
      icon: RefreshCw,
      enabled: () => true,
      run: () => engineStore.restart(),
    },
    {
      id: "theme",
      label: ui.theme === "dark" ? "Switch to light theme" : "Switch to dark theme",
      group: "Appearance",
      icon: ui.theme === "dark" ? Sun : Moon,
      enabled: () => true,
      run: () => ui.toggleTheme(),
    },
    {
      id: "logs",
      label: "Engine log panel",
      group: "Appearance",
      icon: Terminal,
      enabled: () => true,
      run: () => ui.toggleLogs(),
    },
    {
      id: "shortcuts",
      label: "Keyboard shortcuts",
      group: "Help",
      icon: Keyboard,
      enabled: () => true,
      run: () => {
        ui.shortcutsOpen = true;
      },
    },
  ]);

  let query = $state("");
  let index = $state(0);
  let inputEl: HTMLInputElement | null = $state(null);

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return commands.filter(
      (c) => c.enabled() && (q === "" || c.label.toLowerCase().includes(q) || c.group.toLowerCase().includes(q)),
    );
  });

  $effect(() => {
    if (ui.paletteOpen) {
      query = "";
      index = 0;
      queueMicrotask(() => inputEl?.focus());
    }
  });

  $effect(() => {
    // keep the index valid when the filter results change
    if (index >= filtered.length) index = Math.max(0, filtered.length - 1);
  });

  function run(cmd: Cmd) {
    ui.closePalette();
    void cmd.run();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      index = Math.min(index + 1, filtered.length - 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      index = Math.max(index - 1, 0);
    } else if (e.key === "Enter") {
      e.preventDefault();
      const cmd = filtered[index];
      if (cmd) run(cmd);
    } else if (e.key === "Escape") {
      e.preventDefault();
      ui.closePalette();
    }
  }
</script>

{#if ui.paletteOpen}
  <div
    class="fixed inset-0 z-50 flex items-start justify-center bg-canvas/70 px-4 pt-[12vh]"
    role="presentation"
    onclick={(e) => e.currentTarget === e.target && ui.closePalette()}
  >
    <div
      class="w-full max-w-[560px] overflow-hidden rounded-lg border border-line bg-overlay"
      style="box-shadow: var(--shadow-pop)"
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
    >
      <div class="flex items-center gap-2 border-b border-line px-3">
        <Search size={15} class="shrink-0 text-faint" />
        <input
          bind:this={inputEl}
          bind:value={query}
          onkeydown={onKeydown}
          class="h-11 flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-faint"
          placeholder="Search commands or jump to a step…"
          autocomplete="off"
          spellcheck="false"
        />
        <span class="kbd">esc</span>
      </div>

      <div class="max-h-[52vh] overflow-y-auto scroll py-1">
        {#each filtered as cmd, i (cmd.id)}
          {@const showGroup = i === 0 || filtered[i - 1].group !== cmd.group}
          {@const Icon = cmd.icon}
          {#if showGroup}
            <p class="label px-3 pb-1 pt-2.5">{cmd.group}</p>
          {/if}
          <button
            class="flex w-full items-center gap-2.5 px-3 py-2 text-left text-[12.5px] {i === index
              ? 'bg-raised text-ink'
              : 'text-muted'}"
            onmouseenter={() => (index = i)}
            onclick={() => run(cmd)}
          >
            <Icon size={15} class="shrink-0 {i === index ? 'text-accent' : 'text-faint'}" />
            <span class="flex-1 truncate">{cmd.label}</span>
            {#if cmd.hint}<span class="kbd">{cmd.hint}</span>{/if}
          </button>
        {:else}
          <p class="px-3 py-6 text-center text-[12px] text-faint">No matching commands.</p>
        {/each}
      </div>

      <div class="flex items-center justify-between border-t border-line px-3 py-2 text-[10.5px] text-faint">
        <span class="flex items-center gap-3">
          <span class="flex items-center gap-1"><span class="kbd">↑</span><span class="kbd">↓</span> navigate</span>
          <span class="flex items-center gap-1"><span class="kbd">↵</span> run</span>
        </span>
        <span class="flex items-center gap-1.5"><Database size={12} /> RelayDB</span>
      </div>
    </div>
  </div>
{/if}
