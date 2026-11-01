<script lang="ts">
  const questions = [
    {
      q: "Does RelayDB upload my data?",
      a: "No. The engine runs as a local sidecar process and talks to the app over stdio. There is no network service and no telemetry."
    },
    {
      q: "Which databases are supported?",
      a: "PostgreSQL, MySQL, MariaDB, SQLite and Turso (libSQL). The driver layer is small, so adding another engine does not require touching the masking or export code."
    },
    {
      q: "How can the seed be reproducible if bcrypt uses random salts?",
      a: "The default_hash strategy derives its bcrypt salt from the seed with HMAC-SHA256, using a vendored salt-injectable bcrypt. Randomness is removed from the pipeline rather than hidden."
    },
    {
      q: "Can I leave some tables unmasked?",
      a: "Yes. Masking is configured per column, so anything you leave alone is copied as-is. Pruning is separate, which means you can trim rows without changing any values."
    },
    {
      q: "Is it really read-only?",
      a: "Three layers: a read-only DSN, session-level enforcement and a statement guardrail that rejects anything that is not a read."
    },
    {
      q: "Is it free?",
      a: "RelayDB is MIT licensed. Build it from source with Bun, Rust and Go."
    }
  ];
</script>

<section id="faq" class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="grid gap-10 lg:grid-cols-[0.9fr_1.4fr] lg:gap-16">
      <div>
        <p class="kicker">FAQ</p>
        <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
          Questions, answered.
        </h2>
        <p class="mt-4 text-[15px] leading-relaxed text-muted">
          Anything missing? Open an issue on GitHub.
        </p>
      </div>

      <div class="border-t border-line">
        {#each questions as item, i}
          <details class="faq" open={i === 0}>
            <summary>{item.q}</summary>
            <div class="faq-body">{item.a}</div>
          </details>
        {/each}
      </div>
    </div>
  </div>
</section>
