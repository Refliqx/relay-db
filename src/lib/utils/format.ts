/** Display formatting utilities. All functions are pure -> easy to test. */

export function formatRows(n: number | null | undefined): string {
  if (n === null || n === undefined || n < 0) return "±?";
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`;
  return `${(n / 1_000_000).toFixed(1)}M`;
}

export function formatBytes(bytes: number | null | undefined): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  return `${(bytes / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatDuration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || ms < 0) return "not measured";
  if (ms === 0) return "< 1 ms";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  const minutes = Math.floor(ms / 60_000);
  const seconds = Math.round((ms % 60_000) / 1000);
  return `${minutes}m ${seconds}s`;
}

export function formatNumber(n: number | null | undefined): string {
  return (n ?? 0).toLocaleString("en-GB");
}

export function shortPath(path: string, max = 46): string {
  if (path.length <= max) return path;
  const parts = path.split(/[\\/]/);
  const file = parts.pop() ?? path;
  return `…/${file}`;
}

/** Suggested seed file name from the database name (used as the default output). */
export function defaultSeedFileName(database: string | undefined): string {
  const base = (database ?? "database").replace(/\.[a-z0-9]+$/i, "").replace(/[^\w-]+/g, "_");
  return `${base || "database"}.seed.sql`;
}
