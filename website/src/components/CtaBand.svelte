<script lang="ts">
  import { ArrowRight } from "lucide-svelte";
  import { DOCS, REPO } from "../lib/links";
  import CodePanel from "./CodePanel.svelte";

  const commands = [
    "git clone https://github.com/Refliqx/relay-db.git",
    "cd relay-db",
    "bun install",
    "bun run dev:all"
  ].join("\n");
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

    <div class="mx-auto mt-8 max-w-lg text-left">
      <CodePanel title="terminal" meta="bash" copy={commands}>
        <p class="grid grid-cols-[0.75rem_1fr] gap-x-2">
          <span class="text-faint">$</span>
          <span class="text-ink">git clone https://github.com/Refliqx/relay-db.git</span>
        </p>
        <p class="grid grid-cols-[0.75rem_1fr] gap-x-2">
          <span class="text-faint">$</span>
          <span class="text-ink">cd relay-db</span>
        </p>
        <p class="grid grid-cols-[0.75rem_1fr] gap-x-2">
          <span class="text-faint">$</span>
          <span class="text-ink">bun install</span>
        </p>
        <p class="grid grid-cols-[0.75rem_1fr] gap-x-2">
          <span class="text-faint">$</span>
          <span class="text-ink">bun run dev:all</span>
        </p>
      </CodePanel>
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
