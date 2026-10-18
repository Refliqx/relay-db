<script lang="ts">
  /**
   * +layout.svelte: application shell.
   *
   * The engine start point is here (not on each page): event listeners and the
   * handshake are attached ONCE, then every page reads the same status. The
   * global shortcut handler is also attached here, following the keyboard-first
   * principle.
   */
  import "../app.css";
  import EngineBadge from "$lib/components/EngineBadge.svelte";
  import CommandPalette from "$lib/components/CommandPalette.svelte";
  import ShortcutsDialog from "$lib/components/ShortcutsDialog.svelte";
  import LogsPanel from "$lib/components/LogsPanel.svelte";
  import { engineStore } from "$lib/stores/engine.svelte";
  import { flow, type Step } from "$lib/stores/flow.svelte";
  import { ui } from "$lib/stores/ui.svelte";
  import { onMount } from "svelte";
  import { Search, Sun, Moon, Terminal } from "lucide-svelte";

  let { children } = $props();

  const isMac =
    typeof navigator !== "undefined" && /mac/i.test(navigator.platform || navigator.userAgent);
  const modKey = isMac ? "⌘" : "Ctrl";

  function canOpen(step: Step): boolean {
    if (step === 1) return true;
    if (step === 2) return flow.testResult?.ok === true;
    return flow.snapshot !== null;
  }

  function onKeydown(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    const typing =
      !!t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
    const mod = e.metaKey || e.ctrlKey;

    if (mod && e.key.toLowerCase() === "k") {
      e.preventDefault();
      ui.paletteOpen ? ui.closePalette() : ui.openPalette();
      return;
    }
    if (e.key === "Escape") {
      if (ui.paletteOpen) ui.closePalette();
      else if (ui.shortcutsOpen) ui.shortcutsOpen = false;
      return;
    }
    if (typing) return;

    if (e.key === "?") {
      e.preventDefault();
      ui.toggleShortcuts();
      return;
    }
    if (["1", "2", "3", "4"].includes(e.key)) {
      const step = Number(e.key) as Step;
      if (canOpen(step)) {
        e.preventDefault();
        flow.step = step;
      }
    }
  }

  onMount(() => {
    ui.init();
    // start() attaches the listener then refreshes; idempotent.
    engineStore.start();
    window.addEventListener("keydown", onKeydown);
    return () => window.removeEventListener("keydown", onKeydown);
  });
</script>

<div class="flex h-screen flex-col">
  <header class="flex h-11 shrink-0 items-center gap-3 border-b border-line bg-surface px-3">
    <!-- Brand -->
    <div class="flex shrink-0 items-center gap-2">
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" class="text-accent">
        <path
          d="M2 4.25 6 8l-4 3.75"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
        <path
          d="M8 4.25 12 8l-4 3.75"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
          opacity="0.42"
        />
      </svg>
      <span class="text-[13.5px] font-semibold tracking-tight text-ink">
        Relay<span class="text-accent">DB</span>
      </span>
    </div>

    <!-- Command palette trigger -->
    <button
      class="ml-1 flex h-7 min-w-0 max-w-[340px] flex-1 items-center gap-2 rounded-sm border border-line bg-canvas px-2.5 text-left text-[12px] text-faint transition-colors hover:border-line-strong hover:text-muted"
      onclick={() => ui.openPalette()}
    >
      <Search size={13} class="shrink-0" />
      <span class="flex-1 truncate">Search commands or jump…</span>
      <span class="kbd hidden sm:inline-flex">{modKey} K</span>
    </button>

    <div class="ml-auto flex items-center gap-2">
      <button
        class="btn btn-ghost btn-icon btn-sm"
        onclick={() => ui.toggleTheme()}
        aria-label="Switch theme"
        title="Switch theme"
      >
        {#if ui.theme === "dark"}<Sun size={15} />{:else}<Moon size={15} />{/if}
      </button>
      <button
        class="btn btn-ghost btn-icon btn-sm"
        onclick={() => ui.toggleLogs()}
        aria-label="Log panel"
        title="Engine log panel"
        data-active={ui.logsOpen}
      >
        <Terminal size={15} />
      </button>
      <span class="h-4 w-px bg-line"></span>
      <EngineBadge />
    </div>
  </header>

  <main class="min-h-0 flex-1 overflow-auto scroll">
    <div class="mx-auto flex max-w-[1440px] flex-col gap-4 px-4 py-4">{@render children()}</div>
  </main>

  <LogsPanel />
</div>

<CommandPalette />
<ShortcutsDialog />
