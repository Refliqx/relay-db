<script lang="ts">
  import type { Snippet } from "svelte";
  import { Check, Copy } from "lucide-svelte";

  let {
    title,
    meta,
    copy,
    children
  }: { title: string; meta?: string; copy?: string; children: Snippet } = $props();

  let copied = $state(false);

  async function copyCode() {
    if (!copy) return;
    try {
      await navigator.clipboard.writeText(copy);
    } catch {
      /* Async clipboard blocked: fall back to a hidden textarea. */
      const area = document.createElement("textarea");
      area.value = copy;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      const ok = document.execCommand("copy");
      area.remove();
      if (!ok) return;
    }
    copied = true;
    setTimeout(() => (copied = false), 1400);
  }
</script>

<div class="code-panel">
  <div class="code-bar">
    <span class="code-dots" aria-hidden="true">
      <span></span>
      <span></span>
      <span></span>
    </span>
    <span class="mono text-[11px] text-faint">{title}</span>
    <span class="ml-auto flex items-center gap-2">
      {#if meta}
        <span class="mono text-[11px] text-faint">{meta}</span>
      {/if}
      {#if copy}
        <button
          class="code-copy"
          onclick={copyCode}
          aria-label={copied ? "Copied" : "Copy code"}
        >
          {#if copied}<Check size={12} />{:else}<Copy size={12} />{/if}
          {copied ? "Copied" : "Copy"}
        </button>
      {/if}
    </span>
  </div>
  <div class="code-body">
    {@render children()}
  </div>
</div>
