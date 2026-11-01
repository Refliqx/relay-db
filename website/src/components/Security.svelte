<script lang="ts">
  import { Lock, ShieldCheck, Terminal } from "lucide-svelte";

  const layers = [
    {
      n: "01",
      icon: Lock,
      title: "Read-only DSN",
      body: "The connection is opened in a read-only mode the driver honours, so a stray write fails before it ever reaches the server."
    },
    {
      n: "02",
      icon: ShieldCheck,
      title: "Session enforcement",
      body: "Every session is set to read-only for the duration of introspection and export, then closed."
    },
    {
      n: "03",
      icon: Terminal,
      title: "Statement guardrail",
      body: "The engine rejects anything that is not a read before it is sent. Defence in depth, not a promise."
    }
  ];
</script>

<section id="security" class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="max-w-2xl">
      <p class="kicker">Security</p>
      <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
        Built to be pointed at production.
      </h2>
      <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
        RelayDB is a read tool. It has no write path to the source database, and
        that guarantee is enforced in three independent layers.
      </p>
    </div>

    <div class="mt-10 grid gap-4 sm:grid-cols-3">
      {#each layers as layer}
        {@const Icon = layer.icon}
        <div class="card p-6">
          <div class="flex items-center gap-2 text-accent">
            <Icon size={16} />
            <span class="mono text-[11px]">{layer.n}</span>
          </div>
          <h3 class="mt-3 text-[15px] font-semibold">{layer.title}</h3>
          <p class="mt-2 text-[13.5px] leading-relaxed text-muted">{layer.body}</p>
        </div>
      {/each}
    </div>

    <p class="mt-6 text-[13px] text-faint">
      The engine talks JSON-RPC over stdio. No network service, no telemetry, no
      account.
    </p>
  </div>
</section>
