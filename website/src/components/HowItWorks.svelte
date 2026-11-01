<script lang="ts">
  const steps = [
    {
      n: "01",
      title: "Connect",
      body: "Point RelayDB at Postgres, MySQL, MariaDB, SQLite or Turso. The connection opens read-only and closes as soon as introspection finishes."
    },
    {
      n: "02",
      title: "Introspect",
      body: "RelayDB maps every table, column and foreign key, then works out an insert order that will actually import."
    },
    {
      n: "03",
      title: "Mask and prune",
      body: "Choose a strategy per column, keep only the rows you need, and preview the result before anything is written."
    },
    {
      n: "04",
      title: "Export",
      body: "Stream one .sql file with a manifest and a checksum. Commit the seed, share it, run it anywhere."
    }
  ];
</script>

<section id="how" class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="max-w-2xl">
      <p class="kicker">How it works</p>
      <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
        Four steps, one file.
      </h2>
      <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
        No server to deploy and no pipeline to maintain. The engine runs as a
        local sidecar, so the whole flow happens on your machine.
      </p>
    </div>

    <ol
      class="mt-10 grid gap-px overflow-hidden rounded-md border border-line bg-line sm:grid-cols-2 lg:grid-cols-4"
    >
      {#each steps as step}
        <li class="bg-surface p-6">
          <span class="mono text-[11px] text-accent">{step.n}</span>
          <h3 class="mt-3 text-[15px] font-semibold">{step.title}</h3>
          <p class="mt-2 text-[13.5px] leading-relaxed text-muted">{step.body}</p>
        </li>
      {/each}
    </ol>
  </div>
</section>
