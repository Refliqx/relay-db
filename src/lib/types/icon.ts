import type { Database } from "lucide-svelte";

/**
 * Lucide icon component type.
 *
 * lucide-svelte v1 exports icons as legacy-style classes
 * (`SvelteComponentTyped`), not Svelte 5 functions, so Svelte's `Component`
 * type does not fit. All icons share the same class shape, so the `typeof` of
 * one of them is used as a shared type.
 */
export type LucideIcon = typeof Database;
