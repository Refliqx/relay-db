<script lang="ts">
  import { Check, X } from "lucide-svelte";

  const rows = [
    { label: "Personal data", dump: "Present", seed: "Masked or removed" },
    { label: "Size", dump: "Gigabytes", seed: "Only the rows you keep" },
    { label: "Review", dump: "Opaque binary", seed: "Readable SQL in a diff" },
    { label: "Reproducible", dump: "No", seed: "Byte-identical with a fixed seed" },
    { label: "Source access", dump: "Write credentials", seed: "Read-only by design" }
  ];
</script>

<section id="comparison" class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="max-w-2xl">
      <p class="kicker">Dump vs seed</p>
      <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
        What you commit is not a database dump.
      </h2>
      <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
        A seed file is small, reviewable and safe to share. The same workflow that
        produced a dump now produces something a reviewer can actually read.
      </p>
    </div>

    <div class="mt-10 overflow-hidden rounded-lg border border-line bg-surface">
      <div class="overflow-x-auto">
        <table class="w-full min-w-[640px] border-collapse text-left text-[13.5px]">
          <thead>
            <tr class="border-b border-line bg-raised">
              <th class="px-5 py-3 text-[11px] font-semibold text-faint"> </th>
              <th class="px-5 py-3 text-[11px] font-semibold text-muted">Production dump</th>
              <th class="border-l border-line bg-accent/5 px-5 py-3 text-[11px] font-semibold text-ink">
                RelayDB seed
              </th>
            </tr>
          </thead>
          <tbody>
            {#each rows as row}
              <tr class="border-b border-line transition-colors last:border-b-0 hover:bg-raised/50">
                <th class="px-5 py-3 font-normal text-faint">{row.label}</th>
                <td class="px-5 py-3 text-muted">
                  <span class="flex items-center gap-2">
                    <X size={14} class="shrink-0 text-danger" />
                    {row.dump}
                  </span>
                </td>
                <td class="border-l border-line bg-accent/5 px-5 py-3 text-ink">
                  <span class="flex items-center gap-2">
                    <Check size={14} class="shrink-0 text-ok" />
                    {row.seed}
                  </span>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  </div>
</section>
