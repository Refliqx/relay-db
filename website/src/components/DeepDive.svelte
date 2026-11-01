<script lang="ts">
  import CodePanel from "./CodePanel.svelte";
</script>

<section id="reproducible" class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="grid items-center gap-12 lg:grid-cols-2 lg:gap-16">
      <div>
        <p class="kicker">Reproducible exports</p>
        <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
          The same seed always produces the same file.
        </h2>
        <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
          With timestamps off and the seed fixed, two runs in two separate
          processes produce byte-identical output. A CI script proves it on every
          change, because an in-process check cannot catch per-process randomness.
        </p>
        <p class="mt-4 text-[14.5px] leading-relaxed text-muted">
          Even <span class="mono text-ink">default_hash</span> follows the rule: its
          bcrypt salt is derived from the seed with HMAC-SHA256, using a vendored
          salt-injectable bcrypt. Randomness is removed from the pipeline rather
          than hidden.
        </p>
      </div>

      <CodePanel title="relaydb export --check" meta="2 processes">
        <p>
          <span class="inline-block w-14 text-faint">run 1</span>
          <span class="text-muted">63e61f81b08c1d0e…</span>
        </p>
        <p>
          <span class="inline-block w-14 text-faint">run 2</span>
          <span class="text-muted">63e61f81b08c1d0e…</span>
        </p>
        <p class="mt-2">
          <span class="text-ok">✓</span>
          <span class="text-muted"> byte-identical</span>
        </p>
      </CodePanel>
    </div>

    <div class="mt-20 grid items-center gap-12 lg:grid-cols-2 lg:gap-16">
      <div class="card order-2 overflow-hidden lg:order-1">
        <div class="flex items-center justify-between border-b border-line bg-raised px-5 py-2.5">
          <span class="mono text-[11px] text-faint">masking preview</span>
          <span class="mono text-[11px] text-faint">value-keyed</span>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[420px] border-collapse text-left text-[12.5px]">
            <thead>
              <tr class="border-b border-line text-[10.5px] text-faint">
                <th class="px-5 py-2 font-semibold">Column</th>
                <th class="px-5 py-2 font-semibold">Source</th>
                <th class="px-5 py-2 font-semibold">Masked</th>
              </tr>
            </thead>
            <tbody class="mono divide-y divide-line">
              <tr class="transition-colors hover:bg-raised/50">
                <td class="px-5 py-2.5 text-faint">users.email</td>
                <td class="px-5 py-2.5 text-muted">ada@example.com</td>
                <td class="px-5 py-2.5 text-accent">ada.k@example.net</td>
              </tr>
              <tr class="transition-colors hover:bg-raised/50">
                <td class="px-5 py-2.5 text-faint">orders.email</td>
                <td class="px-5 py-2.5 text-muted">ada@example.com</td>
                <td class="px-5 py-2.5 text-accent">ada.k@example.net</td>
              </tr>
              <tr class="transition-colors hover:bg-raised/50">
                <td class="px-5 py-2.5 text-faint">support.email</td>
                <td class="px-5 py-2.5 text-muted">ada@example.com</td>
                <td class="px-5 py-2.5 text-accent">ada.k@example.net</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="order-1 lg:order-2">
        <p class="kicker">Value-keyed masking</p>
        <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
          One value, one pseudonym, everywhere.
        </h2>
        <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
          Masking is keyed by value rather than by row. The same address becomes
          the same pseudonym in every table it appears in, even where no foreign
          key is declared, so relationships keep their meaning after masking.
        </p>
        <p class="mt-4 text-[14.5px] leading-relaxed text-muted">
          Changing the seed changes every pseudonym at once, which makes it easy to
          keep separate environments from colliding.
        </p>
      </div>
    </div>
  </div>
</section>
