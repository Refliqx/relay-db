#!/usr/bin/env bun
/**
 * build-engine.ts: builds the Go sidecar into `src-tauri/binaries/`.
 *
 * Why is a dedicated build step needed? Tauri `externalBin` requires the file
 * name to end with the TARGET TRIPLE (e.g. `relaydb-engine-x86_64-unknown-linux-gnu`),
 * because one bundle must be able to carry binaries for different architectures.
 * This script bridges the (ordinary) Go artefact name with the Tauri convention.
 *
 * Usage:
 *   bun run scripts/build-engine.ts                     # for the current host
 *   bun run scripts/build-engine.ts --target aarch64-apple-darwin
 *   bun run scripts/build-engine.ts --debug             # without -s -w, with symbols
 */

import { $ } from "bun";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";

type GoTarget = { goos: string; goarch: string; arm?: string };

/** Map of Rust target triple -> Go environment. */
const TRIPLE_TO_GO: Record<string, GoTarget> = {
  "x86_64-unknown-linux-gnu": { goos: "linux", goarch: "amd64" },
  "aarch64-unknown-linux-gnu": { goos: "linux", goarch: "arm64" },
  "x86_64-apple-darwin": { goos: "darwin", goarch: "amd64" },
  "aarch64-apple-darwin": { goos: "darwin", goarch: "arm64" },
  "x86_64-pc-windows-msvc": { goos: "windows", goarch: "amd64" },
  "aarch64-pc-windows-msvc": { goos: "windows", goarch: "arm64" },
  "x86_64-pc-windows-gnu": { goos: "windows", goarch: "amd64" },
};

const args = Bun.argv.slice(2);
const targetFlagIndex = args.indexOf("--target");
const debug = args.includes("--debug");

const root = join(import.meta.dir, "..");
const engineDir = join(root, "engine");
const outDir = join(root, "src-tauri", "binaries");

/** Detect the host target triple via rustc (rather than guessing from process.platform). */
async function detectTriple(): Promise<string> {
  const out = await $`rustc -vV`.text();
  const match = out.match(/^host:\s*(\S+)/m);
  if (!match) throw new Error("could not detect the target triple from `rustc -vV`");
  return match[1];
}

const triple = targetFlagIndex >= 0 ? args[targetFlagIndex + 1] : await detectTriple();
const goTarget = TRIPLE_TO_GO[triple];

if (!goTarget) {
  console.error(`Unknown target triple: ${triple}`);
  console.error(`Available: ${Object.keys(TRIPLE_TO_GO).join(", ")}`);
  process.exit(1);
}

const isWindows = goTarget.goos === "windows";
const binaryName = `relaydb-engine-${triple}${isWindows ? ".exe" : ""}`;
const outputPath = join(outDir, binaryName);

// Version injected at build time so `--version` & the handshake need no extra file.
const version = (await Bun.file(join(root, "package.json")).json()).version ?? "0.0.0";
const ldflags = debug
  ? [`-X main.version=${version}`]
  : [`-s`, `-w`, `-X main.version=${version}`];

await mkdir(outDir, { recursive: true });

console.log(`▸ building Go sidecar   : ${goTarget.goos}/${goTarget.goarch}`);
console.log(`▸ Rust target triple    : ${triple}`);
console.log(`▸ output                : ${outputPath.replace(root + "/", "")}`);

const started = performance.now();

await $`go build -trimpath -o ${outputPath} -ldflags ${ldflags.join(" ")} ./cmd/relaydb-engine`
  .cwd(engineDir)
  .env({
    ...process.env,
    GOOS: goTarget.goos,
    GOARCH: goTarget.goarch,
    // Pure Go binary: no cgo -> cross-compile needs no C toolchain.
    CGO_ENABLED: "0",
  })
  .quiet();

const size = (await Bun.file(outputPath).size) / (1024 * 1024);
const elapsed = ((performance.now() - started) / 1000).toFixed(1);

// Quick check: the binary must answer --version.
const check = await $`${outputPath} --version`.quiet().nothrow();
if (check.exitCode !== 0) {
  console.error("The built binary could not be run.");
  process.exit(1);
}

console.log(`✓ done in ${elapsed}s (${size.toFixed(1)} MB)`);
console.log(`  ${check.stderr.toString().trim()}`);
