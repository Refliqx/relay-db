<script lang="ts">
  let { size = 18 }: { size?: number } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 16 16"
  aria-hidden="true"
  class="shrink-0 text-accent"
>
  <path
    d="M2 4.25 6 8l-4 3.75"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
  />
  <path
    d="M8 4.25 12 8l-4 3.75"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    opacity="0.42"
  />
</svg>
