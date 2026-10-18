#!/usr/bin/env bun
/**
 * check-protocol.ts: anti-drift guard for the IPC contract.
 *
 * PROBLEM IT PREVENTS: three layers (Go, Rust, TypeScript) share one protocol.
 * Without a check it is very easy to add a method in Go and forget to add it in
 * TS; the bug only shows up as "TypeError: undefined" at runtime.
 *
 * HOW IT WORKS: the engine is asked to print its catalogue (`--dump-schema`),
 * then it is compared against the constants in `src/lib/types/protocol.ts`.
 * This check runs in CI (`bun run protocol:check`).
 */

import { $ } from "bun";

type Dump = {
  protocolVersion: string;
  engineVersion: string;
  methods: string[];
  notifications: string[];
  errorCodes: Record<string, number>;
  strategies: string[];
};

const root = new URL("..", import.meta.url).pathname;

// The binary name carries the target triple (Tauri externalBin convention), so
// we find it by pattern, not a pinned path.
const binariesDir = `${root}src-tauri/binaries`;
const glob = new Bun.Glob("relaydb-engine*");
const candidates = [...glob.scanSync(binariesDir)].filter((f) => !f.endsWith(".exe.map"));
const engineFile = candidates.sort()[0];
if (!engineFile) {
  console.error(`✗ Sidecar not built in ${binariesDir}`);
  console.error("  Run `bun run engine:build` first.");
  process.exit(1);
}
const engineBin = `${binariesDir}/${engineFile}`;

const ts = (await import(`${root}src/lib/types/protocol.ts`)) as {
  PROTOCOL_VERSION: string;
  METHODS: Record<string, string>;
  NOTIFICATIONS: Record<string, string>;
  ERR: Record<string, number>;
};

const binaryExists = await Bun.file(engineBin).exists();
if (!binaryExists) {
  console.error(`✗ Cannot run the binary: ${engineBin}`);
  process.exit(1);
}

const raw = await $`${engineBin} --dump-schema`.quiet().text();
const dump: Dump = JSON.parse(raw);

const problems: string[] = [];

function compareSet(label: string, engineValues: string[], tsValues: string[]) {
  const onlyEngine = engineValues.filter((v) => !tsValues.includes(v));
  const onlyTs = tsValues.filter((v) => !engineValues.includes(v));
  if (onlyEngine.length) problems.push(`${label}: present in the engine but not yet in TypeScript -> ${onlyEngine.join(", ")}`);
  if (onlyTs.length) problems.push(`${label}: present in TypeScript but not yet in the engine -> ${onlyTs.join(", ")}`);
}

if (dump.protocolVersion !== ts.PROTOCOL_VERSION) {
  problems.push(`protocolVersion differs: engine=${dump.protocolVersion} ts=${ts.PROTOCOL_VERSION}`);
}

compareSet("methods", dump.methods, Object.values(ts.METHODS));
compareSet("notifications", dump.notifications, Object.values(ts.NOTIFICATIONS));

for (const [name, code] of Object.entries(dump.errorCodes)) {
  const tsCode = ts.ERR[name];
  if (tsCode === undefined) {
    problems.push(`errorCode "${name}" (${code}) is not defined in the TypeScript ERR`);
  } else if (tsCode !== code) {
    problems.push(`errorCode "${name}" differs: engine=${code} ts=${tsCode}`);
  }
}

// Masking strategies are part of the contract too (used by the dropdown in the UI).
const tsStrategies = (await Bun.file(`${root}src/lib/types/protocol.ts`).text())
  .match(/export type MaskStrategy =([\s\S]*?);/)?.[1]
  ?.match(/"([a-z0-9_]+)"/g)
  ?.map((s) => s.replace(/"/g, ""));
compareSet("masking strategies", dump.strategies, tsStrategies ?? []);

if (problems.length) {
  console.error("✗ Protocol contract out of sync:\n");
  for (const p of problems) console.error(`  - ${p}`);
  console.error("\n  Fix one side, then run again.");
  process.exit(1);
}

console.log(`✓ Protocol in sync (v${dump.protocolVersion}, engine ${dump.engineVersion})`);
console.log(
  `  ${dump.methods.length} methods · ${dump.notifications.length} notifications · ${dump.strategies.length} masking strategies`,
);
