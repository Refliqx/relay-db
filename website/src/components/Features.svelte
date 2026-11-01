<script lang="ts">
  import { Fingerprint, GitBranch, HardDrive, Scissors, ShieldCheck, Shuffle } from "lucide-svelte";

  const features = [
    {
      icon: Fingerprint,
      title: "Deterministic by construction",
      body: "Every masked value is derived from the seed and the original value, not the row number. Export twice and you get the same bytes."
    },
    {
      icon: GitBranch,
      title: "Foreign keys stay intact",
      body: "Tables are ordered topologically and cycles are reported instead of silently dropped. Undeclared foreign keys survive because masking is value-keyed."
    },
    {
      icon: ShieldCheck,
      title: "Read-only, enforced three ways",
      body: "A read-only DSN, session-level enforcement and a statement guardrail. RelayDB has no write path to the source database."
    },
    {
      icon: Shuffle,
      title: "Sixteen masking strategies",
      body: "Email, name, phone, address, hash, redact, fixed value, sequential and more. Each column is configured once, in YAML."
    },
    {
      icon: Scissors,
      title: "Prune before you copy",
      body: "Keep the rows that matter with WHERE clauses, row limits and deterministic ordering. Smaller seeds, faster test runs."
    },
    {
      icon: HardDrive,
      title: "Local-first by design",
      body: "A Rust shell and a Go sidecar. No account, no cloud, no telemetry. Credentials stay on the machine."
    }
  ];
</script>

<section id="features" class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="max-w-2xl">
      <p class="kicker">Capabilities</p>
      <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
        Built for the awkward parts.
      </h2>
      <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
        Foreign keys, cycles, reproducible values and a read-only guarantee. The
        parts that usually go wrong are the parts RelayDB is designed around.
      </p>
    </div>

    <div
      class="mt-10 grid gap-px overflow-hidden rounded-md border border-line bg-line sm:grid-cols-2 lg:grid-cols-3"
    >
      {#each features as feature}
        {@const Icon = feature.icon}
        <div class="bg-surface p-6 transition-colors hover:bg-raised">
          <div
            class="flex h-9 w-9 items-center justify-center rounded-sm border border-line bg-canvas text-accent"
          >
            <Icon size={17} />
          </div>
          <h3 class="mt-4 text-[15px] font-semibold">{feature.title}</h3>
          <p class="mt-2 text-[13.5px] leading-relaxed text-muted">{feature.body}</p>
        </div>
      {/each}
    </div>
  </div>
</section>
