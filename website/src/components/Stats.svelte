<script lang="ts">
  const stats = [
    { value: "5", label: "databases supported" },
    { value: "16", label: "masking strategies" },
    { value: "0", label: "network calls" },
    { value: "1", label: "reviewable seed file" }
  ];
</script>

<section class="border-b border-line">
  <div class="shell py-10 sm:py-12">
    <dl class="grid grid-cols-2 gap-px overflow-hidden rounded-md border border-line bg-line sm:grid-cols-4">
      {#each stats as stat}
        <div class="bg-surface px-6 py-5">
          <dt class="sr-only">{stat.label}</dt>
          <dd>
            <span class="block text-[clamp(1.75rem,3vw,2.25rem)] font-semibold tracking-tight">
              {stat.value}
            </span>
            <span class="mt-1 block text-[13px] text-muted">{stat.label}</span>
          </dd>
        </div>
      {/each}
    </dl>
  </div>
</section>
