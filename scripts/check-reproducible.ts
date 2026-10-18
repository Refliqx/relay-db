#!/usr/bin/env bun
/**
 * check-reproducible.ts: guard for the "reproducible export" promise.
 *
 * PROBLEM IT PREVENTS: RelayDB's main promise is "same seed + timestamps off
 * = byte-identical file". This breaks easily and invisibly: one source of
 * per-process randomness is enough (e.g. a bcrypt salt from crypto/rand such as
 * the one `default_hash` used to use), and the checksum changes every time the
 * application runs; the Git diff becomes noisy and the guarantee shown in the
 * UI becomes a lie.
 *
 * HOW IT WORKS: `relaydb-engine --selftest` is run in TWO SEPARATE PROCESSES,
 * the SHA-256 checksum of the seed output is extracted, then compared.
 *
 * WHY MUST THEY BE SEPARATE PROCESSES? Because the selftest inside the engine
 * only compares two runs that share the same process; process-global caches and
 * RNG make both look identical. What the user faces is "today's export vs
 * tomorrow's export", i.e. genuinely new processes.
 */

import { $ } from "bun";

const root = new URL("..", import.meta.url).pathname;
const runs = Number(process.env.RELAYDB_REPRO_RUNS ?? 2);

async function selftestChecksum(): Promise<string> {
  const out = await $`go run ./cmd/relaydb-engine --selftest`.cwd(`${root}engine`).quiet().text();

  const match = out.match(/^checksum sha256:\s*([0-9a-f]{64})$/m);
  if (!match) {
    console.error("✗ The line `checksum sha256: <hex>` was not found in the selftest output.");
    console.error(out.split("\n").slice(-20).join("\n"));
    process.exit(1);
  }
  return match[1];
}

const sums: string[] = [];
for (let i = 0; i < runs; i++) {
  sums.push(await selftestChecksum());
}

const [first, ...rest] = sums;
const mismatch = rest.find((s) => s !== first);

if (mismatch) {
  console.error("✗ Export is NOT reproducible across peer processes:\n");
  sums.forEach((s, i) => console.error(`  process ${i + 1}: ${s}`));
  console.error(
    "\n  Look for a source of per-process randomness (crypto/rand, math/rand, map iteration,\n" +
      "  or a hash with a random salt such as the built-in bcrypt) in the masking/export path.",
  );
  process.exit(1);
}

console.log(`✓ Export reproducible across ${runs} processes (checksum ${first.slice(0, 12)}…)`);
