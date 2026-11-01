<script lang="ts">
  import { ArrowRight } from "lucide-svelte";
  import { DOCS } from "../lib/links";
  import CodePanel from "./CodePanel.svelte";
</script>

<section id="top" class="relative overflow-hidden border-b border-line">
  <div class="hero-grid"></div>
  <div class="hero-glow"></div>

  <div class="shell relative pt-20 pb-16 sm:pt-28 sm:pb-20">
    <div class="mx-auto max-w-3xl text-center">
      <span class="eyebrow">
        <span class="eyebrow-dot"></span>
        v0.1.0 · Open source · MIT
      </span>

      <h1 class="mt-6 text-[clamp(2.75rem,7vw,5rem)] font-semibold">
        Live data in.<br />
        <span class="text-faint">Clean seeds out.</span>
      </h1>

      <p class="mx-auto mt-6 max-w-xl text-[17px] leading-relaxed text-muted">
        RelayDB relays, masks and prunes a live database into a reproducible seed
        file your team can run locally. Production rows never leave your machine.
      </p>

      <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
        <a class="btn btn-primary" href="#get-started">
          Get started
          <ArrowRight size={16} />
        </a>
        <a class="btn btn-outline" href={DOCS} target="_blank" rel="noreferrer">
          Read the docs
        </a>
      </div>

      <p class="mt-4 text-[13px] text-faint">
        macOS, Windows and Linux · Built with Tauri, Rust and Go
      </p>
    </div>

    <div class="mx-auto mt-14 max-w-3xl text-left">
      <CodePanel title="relaydb export" meta="exit 0">
        <p class="text-ink">$ relaydb export --config relaydb.yaml</p>

        <div class="mt-3">
          <p class="grid grid-cols-[4.75rem_1fr] gap-x-3 sm:grid-cols-[5.75rem_1fr]">
            <span class="text-accent">connect</span>
            <span class="text-muted">postgres://…@staging <span class="text-faint">read-only</span></span>
          </p>
          <p class="grid grid-cols-[4.75rem_1fr] gap-x-3 sm:grid-cols-[5.75rem_1fr]">
            <span class="text-accent">schema</span>
            <span class="text-muted">24 tables · 38 relationships</span>
          </p>
          <p class="grid grid-cols-[4.75rem_1fr] gap-x-3 sm:grid-cols-[5.75rem_1fr]">
            <span class="text-accent">order</span>
            <span class="text-muted">acyclic · 2 cycles reported</span>
          </p>
          <p class="grid grid-cols-[4.75rem_1fr] gap-x-3 sm:grid-cols-[5.75rem_1fr]">
            <span class="text-accent">mask</span>
            <span class="text-muted">1,204,882 values · 16 strategies</span>
          </p>
          <p class="grid grid-cols-[4.75rem_1fr] gap-x-3 sm:grid-cols-[5.75rem_1fr]">
            <span class="text-accent">prune</span>
            <span class="text-muted">18,400 rows kept of 2.1M</span>
          </p>
          <p class="grid grid-cols-[4.75rem_1fr] gap-x-3 sm:grid-cols-[5.75rem_1fr]">
            <span class="text-accent">export</span>
            <span class="text-muted">seed.sql · 48.2 MB</span>
          </p>
        </div>

        <p class="mt-3">
          <span class="text-ok">✓</span>
          <span class="text-muted"> reproducible</span>
          <span class="text-faint"> sha256 63e61f81b08c…</span>
        </p>
      </CodePanel>
    </div>
  </div>
</section>
