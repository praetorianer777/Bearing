import { useSyncExternalStore } from "react";
import { MODE_STORAGE_KEY, THEME_CACHE_KEY, THEME_STYLE_ID } from "@/config";

/** Light or dark, or whatever the system says. */
export type Mode = "system" | "light" | "dark";
export const MODES: Mode[] = ["system", "light", "dark"];

/** How Bearing looks in this browser. The theme is the person's on the server; the mode is the browser's own. */
export interface Appearance {
  mode: Mode;
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
  return { mode: mode === "light" || mode === "dark" ? mode : "system" };
}

let current = readAppearance();
const listeners = new Set<() => void>();

/** Sets data-theme on the document, which the stylesheet's dark tokens and every theme's answer to. */
function stampMode(mode: Mode): void {
  if (typeof document === "undefined") return;
  if (mode === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = mode;
}

/** Changes how Bearing looks in this browser, and remembers it for the next load. */
export function applyAppearance(change: Partial<Appearance>): void {
  current = { ...current, ...change };
  write(MODE_STORAGE_KEY, current.mode === "system" ? "" : current.mode);
  stampMode(current.mode);
  for (const listener of listeners) listener();
}

/** The mode after this one, as the sidebar's button steps through them. */
export function nextMode(mode: Mode): Mode {
  return MODES[(MODES.indexOf(mode) + 1) % MODES.length] ?? "system";
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

/** Puts a compiled theme on the page, after the built-in stylesheet so it wins, or takes it off with null. */
export function applyThemeCSS(css: string | null): void {
  if (typeof document === "undefined") return;
  let style = document.getElementById(THEME_STYLE_ID);
  if (css === null) {
    style?.remove();
    return;
  }
  if (!style) {
    style = document.createElement("style");
    style.id = THEME_STYLE_ID;
    document.head.appendChild(style);
  }
  if (style.textContent !== css) style.textContent = css;
}

/** Remembers the compiled theme, so the next load paints it before the server answers; null forgets it. */
export function cacheThemeCSS(entry: { key: string; css: string } | null): void {
  write(THEME_CACHE_KEY, entry ? JSON.stringify(entry) : "");
}

/** What the last load left behind, if anything. */
export function cachedThemeCSS(): { key: string; css: string } | null {
  try {
    const parsed = JSON.parse(read(THEME_CACHE_KEY) ?? "null") as { key?: unknown; css?: unknown } | null;
    if (typeof parsed?.key === "string" && typeof parsed.css === "string") return { key: parsed.key, css: parsed.css };
  } catch {
    // A cache written by an older version is no cache.
  }
  return null;
}

// The mode, and the theme this browser showed last, go on before anything renders, so no page flashes the wrong
// look; the loader replaces the theme once the server has answered.
stampMode(current.mode);
applyThemeCSS(cachedThemeCSS()?.css ?? null);
