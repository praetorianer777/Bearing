import { useSyncExternalStore } from "react";
import { MODE_STORAGE_KEY, THEME_STORAGE_KEY } from "@/config";

/** Light or dark, or whatever the system says. */
export type Mode = "system" | "light" | "dark";
export const MODES: Mode[] = ["system", "light", "dark"];

/** The installation's theme, Bearing's own look, or one theme by id. */
export const INSTALLATION_THEME = "";
export const BUILT_IN_THEME = "built-in";

export interface Appearance {
  mode: Mode;
  theme: string;
}

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string): void {
  try {
    if (value) localStorage.setItem(key, value);
    else localStorage.removeItem(key);
  } catch {
    // A browser that keeps nothing still shows the choice until the page reloads.
  }
}

function readAppearance(): Appearance {
  const mode = read(MODE_STORAGE_KEY);
  return { mode: mode === "light" || mode === "dark" ? mode : "system", theme: read(THEME_STORAGE_KEY) ?? INSTALLATION_THEME };
}

let current = readAppearance();
const listeners = new Set<() => void>();

/** Sets data-theme on the document, which the stylesheet's dark tokens and every theme's answer to. */
function stampMode(mode: Mode): void {
  if (typeof document === "undefined") return;
  if (mode === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = mode;
}

stampMode(current.mode);

/** Changes how Bearing looks in this browser, and remembers it for the next load. */
export function applyAppearance(change: Partial<Appearance>): void {
  current = { ...current, ...change };
  write(MODE_STORAGE_KEY, current.mode === "system" ? "" : current.mode);
  write(THEME_STORAGE_KEY, current.theme);
  stampMode(current.mode);
  for (const listener of listeners) listener();
}

/** Rereads what the browser keeps, as a fresh page load would; for tests. */
export function reloadAppearance(): void {
  current = readAppearance();
  stampMode(current.mode);
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** How Bearing looks in this browser, rerendering when it changes. */
export function useAppearance(): Appearance {
  return useSyncExternalStore(subscribe, () => current);
}

/** Which of the installation's themes is shown: the chosen one while it exists, else the installation's. */
export function shownTheme<T extends { id: string; default: boolean }>(themes: readonly T[], choice: string): T | undefined {
  if (choice === BUILT_IN_THEME) return undefined;
  return themes.find((each) => each.id === choice) ?? themes.find((each) => each.default);
}
