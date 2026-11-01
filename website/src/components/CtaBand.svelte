<script lang="ts">
  import { ArrowRight, Download } from "lucide-svelte";
  import { DOCS, RELEASES } from "../lib/links";
</script>

<section class="relative overflow-hidden border-b border-line">
  <div class="hero-glow opacity-60"></div>

  <div class="shell relative py-24 text-center sm:py-28">
    <h2 class="text-[clamp(2rem,5vw,3.25rem)] font-semibold">
      Stop copying production data.
    </h2>
    <p class="mx-auto mt-4 max-w-xl text-[16.5px] leading-relaxed text-muted">
      Generate a seed file your team can commit, review and run anywhere.
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
  </div>
</section>
