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

      <div class="terminal">
        <div class="terminal-bar">
          <span class="mono text-[11px] text-faint">relaydb export --check</span>
          <span class="mono ml-auto text-[11px] text-faint">2 processes</span>
        </div>
        <div class="mono overflow-x-auto p-5 text-[12.5px] leading-[2]">
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
        </div>
      </div>
    </div>

    <div class="mt-20 grid items-center gap-12 lg:grid-cols-2 lg:gap-16">
      <div class="card order-2 overflow-hidden lg:order-1">
        <div class="border-b border-line px-5 py-3">
          <span class="mono text-[11px] text-faint">ada@example.com · masked</span>
        </div>
        <div class="mono divide-y divide-line text-[12.5px]">
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-3">
            <span class="w-32 shrink-0 text-faint">users.email</span>
            <span class="text-muted">ada@example.com</span>
            <span class="text-faint">→</span>
            <span class="text-accent">ada.k@example.net</span>
          </div>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-3">
            <span class="w-32 shrink-0 text-faint">orders.email</span>
            <span class="text-muted">ada@example.com</span>
            <span class="text-faint">→</span>
            <span class="text-accent">ada.k@example.net</span>
          </div>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-3">
            <span class="w-32 shrink-0 text-faint">support.email</span>
            <span class="text-muted">ada@example.com</span>
            <span class="text-faint">→</span>
            <span class="text-accent">ada.k@example.net</span>
          </div>
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
