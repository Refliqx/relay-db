<script lang="ts">
  /**
   * EngineBadge: engine status in the header.
   *
   * The user must always be able to answer "is the engine alive or not?" and
   * "if it is dead, what do I do?". So the badge carries a recovery action
   * (Restart), not just a status light. The indicator is deliberately flat (no
   * decorative glow/pulse); animation is used only when it is genuinely
   * working.
   */
  import { engineStore } from "$lib/stores/engine.svelte";
  import { RefreshCw } from "lucide-svelte";

  // Note: the variable must NOT be named `state`, because it would clash with the
  // `$state` rune (the compiler treats it as a store subscription).
  const engineState = $derived(engineStore.status.state);

  const chipClass = $derived(
    engineState === "ready" ? "chip-ok" : engineState === "starting" ? "chip-warn" : "chip-danger",
  );

  const label = $derived(
    engineState === "ready"
      ? "Engine ready"
      : engineState === "starting"
        ? "Starting up"
        : engineState === "crashed"
          ? "Engine stopped"
          : "Engine down",
  );

  const detail = $derived(
    [
      `status: ${engineState}`,
      engineStore.status.engineVersion ? `version: ${engineStore.status.engineVersion}` : null,
      engineStore.status.protocolVersion ? `protocol: ${engineStore.status.protocolVersion}` : null,
      engineStore.status.platform ? `platform: ${engineStore.status.platform}` : null,
      engineStore.status.pid ? `pid: ${engineStore.status.pid}` : null,
      engineStore.drivers.length ? `driver: ${engineStore.drivers.map((d) => d.kind).join(", ")}` : null,
      engineStore.status.lastError ? `error: ${engineStore.status.lastError}` : null,
    ]
      .filter(Boolean)
      .join("\n"),
  );

  let busy = $state(false);
  async function restart() {
    busy = true;
    await engineStore.restart();
    busy = false;
  }
</script>

<div class="flex items-center gap-2">
  <span class="chip {chipClass}" title={detail}>
    {#if engineState === "starting"}
      <RefreshCw size={11} class="animate-spin" />
    {:else}
      <span class="size-1.5 rounded-full bg-current"></span>
    {/if}
    {label}
  </span>

  {#if engineState === "ready"}
    <span class="tnum hidden text-[11px] text-faint sm:inline">
      v{engineStore.status.engineVersion ?? "?"}
      {#if engineStore.status.pid}· pid {engineStore.status.pid}{/if}
    </span>
  {/if}

  {#if engineState !== "ready"}
    <button class="btn btn-sm" onclick={restart} disabled={busy}>
      <RefreshCw size={12} class={busy ? "animate-spin" : ""} />
      {busy ? "Starting…" : "Restart"}
    </button>
  {/if}
</div>
