<script lang="ts">
  import { ArrowRight, Download } from "lucide-svelte";
  import { DOCS, RELEASES } from "../lib/links";
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
        <a class="btn btn-primary" href={RELEASES}>
          <Download size={16} />
          Download RelayDB
        </a>
        <a class="btn btn-outline" href={DOCS} target="_blank" rel="noreferrer">
          Read the docs
          <ArrowRight size={16} />
        </a>
      </div>

      <p class="mt-4 text-[13px] text-faint">
        macOS, Windows and Linux · Built with Tauri, Rust and Go
      </p>
    </div>

    <div class="terminal mx-auto mt-14 max-w-3xl text-left">
      <div class="terminal-bar">
        <span class="terminal-dot"></span>
        <span class="terminal-dot"></span>
        <span class="terminal-dot"></span>
        <span class="mono ml-2 text-[11px] text-faint">relaydb export</span>
        <span class="mono ml-auto text-[11px] text-faint">exit 0</span>
      </div>

      <div class="mono overflow-x-auto p-5 text-[12.5px] leading-[2]">
        <p class="text-ink">$ relaydb export --config relaydb.yaml</p>

        <div class="mt-3">
          <p>
            <span class="inline-block w-[5.75rem] text-accent">connect</span>
            <span class="text-muted">postgres://…@staging</span>
            <span class="ml-3 text-faint">read-only</span>
          </p>
          <p>
            <span class="inline-block w-[5.75rem] text-accent">schema</span>
            <span class="text-muted">24 tables · 38 relationships</span>
          </p>
          <p>
            <span class="inline-block w-[5.75rem] text-accent">order</span>
            <span class="text-muted">acyclic · 2 cycles reported</span>
          </p>
          <p>
            <span class="inline-block w-[5.75rem] text-accent">mask</span>
            <span class="text-muted">1,204,882 values · 16 strategies</span>
          </p>
          <p>
            <span class="inline-block w-[5.75rem] text-accent">prune</span>
            <span class="text-muted">18,400 rows kept of 2.1M</span>
          </p>
          <p>
            <span class="inline-block w-[5.75rem] text-accent">export</span>
            <span class="text-muted">seed.sql · 48.2 MB</span>
          </p>
        </div>

        <p class="mt-3">
          <span class="text-ok">✓</span>
          <span class="text-muted"> reproducible</span>
          <span class="text-faint"> sha256 63e61f81b08c…</span>
        </p>
      </div>
    </div>
  </div>
</section>
