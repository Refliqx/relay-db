<script lang="ts">
  import { GitPullRequest, HardDrive, ShieldAlert } from "lucide-svelte";

  const problems = [
    {
      icon: ShieldAlert,
      title: "Personal data on every laptop",
      body: "A production copy puts real names, emails and addresses on machines that were never in scope for that data."
    },
    {
      icon: HardDrive,
      title: "Gigabytes in every checkout",
      body: "Full dumps slow down CI, fill disks and make every environment slightly different from the next."
    },
    {
      icon: GitPullRequest,
      title: "Changes you cannot review",
      body: "A binary dump cannot be diffed. Nobody knows what changed until a test breaks."
    }
  ];
</script>

<section class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="max-w-2xl">
      <p class="kicker">The problem</p>
      <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
        Your staging database is a copy of production.
      </h2>
      <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
        That means real customer rows on developer laptops, in CI logs and in test
        fixtures. It is a compliance problem, a storage problem and a review
        problem at the same time.
      </p>
    </div>

    <div class="mt-10 grid gap-px overflow-hidden rounded-md border border-line bg-line sm:grid-cols-3">
      {#each problems as problem}
        {@const Icon = problem.icon}
        <div class="bg-surface p-6">
          <div class="text-danger">
            <Icon size={17} />
          </div>
          <h3 class="mt-4 text-[15px] font-semibold">{problem.title}</h3>
          <p class="mt-2 text-[13.5px] leading-relaxed text-muted">{problem.body}</p>
        </div>
      {/each}
    </div>
  </div>
</section>
