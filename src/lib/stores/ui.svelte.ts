/**
 * ui.svelte.ts: interface state unrelated to the DB domain.
 *
 * Kept separate from `flow`/`engine` because changes here (theme, dialogs, log
 * panel) must not trigger any side effect on the export pipeline.
 */

export type Theme = "dark" | "light";

const THEME_KEY = "relaydb.theme";

class UiStore {
  theme = $state<Theme>("dark");
  paletteOpen = $state(false);
  logsOpen = $state(false);
  shortcutsOpen = $state(false);

  #initialised = false;

  /** Called once from +layout.svelte (after mount, so DOM access is safe). */
  init(): void {
    if (this.#initialised) return;
    this.#initialised = true;
    const saved = localStorage.getItem(THEME_KEY);
    if (saved === "light" || saved === "dark") this.theme = saved;
    this.#apply();
  }

  toggleTheme(): void {
    this.theme = this.theme === "dark" ? "light" : "dark";
    localStorage.setItem(THEME_KEY, this.theme);
    this.#apply();
  }

  toggleLogs(): void {
    this.logsOpen = !this.logsOpen;
  }

  openPalette(): void {
    this.paletteOpen = true;
  }
  closePalette(): void {
    this.paletteOpen = false;
  }
  toggleShortcuts(): void {
    this.shortcutsOpen = !this.shortcutsOpen;
  }

  #apply(): void {
    document.documentElement.dataset.theme = this.theme;
  }
}

export const ui = new UiStore();
