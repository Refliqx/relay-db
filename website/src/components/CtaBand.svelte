<script lang="ts">
  import { ArrowRight } from "lucide-svelte";
  import { DOCS, REPO } from "../lib/links";
</script>

<section id="get-started" class="relative overflow-hidden border-b border-line">
  <div class="hero-glow opacity-60"></div>

  <div class="shell relative py-24 sm:py-28">
    <div class="mx-auto max-w-2xl text-center">
      <p class="kicker">Get started</p>
      <h2 class="mt-3 text-[clamp(2rem,5vw,3.25rem)] font-semibold">
        Stop copying production data.
      </h2>
      <p class="mx-auto mt-4 max-w-xl text-[16.5px] leading-relaxed text-muted">
        RelayDB is MIT licensed. Clone it, build the sidecar and run the desktop
        app. No account and no hosted service.
      </p>
    </div>

    <div class="card mx-auto mt-8 max-w-lg overflow-hidden text-left">
      <div class="flex items-center justify-between border-b border-line bg-raised px-4 py-2">
        <span class="mono text-[11px] text-faint">terminal</span>
        <span class="mono text-[11px] text-faint">bash</span>
      </div>
      <div class="mono overflow-x-auto p-4 text-[12.5px] leading-[1.9]">
        <p>
          <span class="text-faint">$</span>
          <span class="text-ink"> git clone https://github.com/Refliqx/relay-db.git</span>
        </p>
        <p>
          <span class="text-faint">$</span>
          <span class="text-ink"> cd relay-db</span>
        </p>
        <p>
          <span class="text-faint">$</span>
          <span class="text-ink"> bun install</span>
        </p>
        <p>
          <span class="text-faint">$</span>
          <span class="text-ink"> bun run dev:all</span>
        </p>
      </div>
    </div>

    <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
      <a class="btn btn-primary" href={REPO} target="_blank" rel="noreferrer">
        View on GitHub
        <ArrowRight size={16} />
      </a>
      <a class="btn btn-outline" href={DOCS} target="_blank" rel="noreferrer">
        Read the docs
      </a>
    </div>
  </div>
</section>
