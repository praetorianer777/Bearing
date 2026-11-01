import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { THEME_STYLE_ID } from "@/config";
import { applyAppearance, applyThemeCSS, cachedThemeCSS, cacheThemeCSS, nextMode, reloadAppearance } from "./appearance";
import { type CompilableTheme, compileTheme, emptySpec, TOKEN_GROUPS, TOKEN_NAMES } from "./theme";

const harbour = JSON.parse(readFileSync(resolve(__dirname, "../../../backend/internal/theme/testdata/harbour.armature-theme.json"), "utf8"));

afterEach(() => {
  applyAppearance({ mode: "system" });
  applyThemeCSS(null);
  localStorage.clear();
});

describe("a theme", () => {
  it("may redefine exactly the colour tokens the stylesheet has", () => {
    const css = readFileSync(resolve(__dirname, "../styles/index.css"), "utf8");
    const declared = new Set([...css.matchAll(/--color-([a-z0-9-]+):/g)].map((m) => m[1]));
    expect(new Set(TOKEN_NAMES)).toEqual(declared);
    expect(TOKEN_NAMES).toHaveLength(new Set(TOKEN_NAMES).size);
    expect(TOKEN_GROUPS.every((group) => group.tokens.length > 0)).toBe(true);
  });

  it("asks for its moving picture outside the stylesheet, and an empty one compiles to nothing", () => {
    expect(compileTheme({ id: "t", spec: { ...emptySpec(), effect: "constellation" }, assets: [] })).toBe("");
  });

  it("compiles to only what it names, its files served from Bearing", () => {
    const theme: CompilableTheme = { id: "t1", spec: harbour.spec, assets: harbour.assets };
    const css = compileTheme(theme);
    expect(css).toContain(':root[data-theme="light"] {\n  --color-accent: #0e7490;');
    expect(css).toContain('@media (prefers-color-scheme: dark) {\n:root:not([data-theme="light"]) {\n  --color-accent: #22d3ee;');
    expect(css).toContain("--radius-control: 2px;");
    expect(css).toContain("--shadow-1: 0 1px 2px rgba(15, 23, 42, 0.25);");
    expect(css).toContain(`cursor: url("/api/v1/themes/t1/assets/${harbour.assets[0].id}") 4 2, default;`);
    expect(css).not.toContain("--font-sans");
  });

  it("leaves out tokens Bearing does not have, and is nothing when it names nothing", () => {
    const empty = { colors: { light: {}, dark: {} }, fonts: {}, shape: {}, shadows: {}, cursors: {}, icons: {}, css: "" };
    expect(compileTheme({ id: "t", spec: empty, assets: [] })).toBe("");
    const odd = { ...empty, colors: { light: { "not-a-token": "#fff" }, dark: {} } };
    expect(compileTheme({ id: "t", spec: odd, assets: [] })).toBe("");
  });

  it("draws fonts, icons, a backdrop and its own CSS", () => {
    const spec = {
      colors: { light: {}, dark: {} },
      fonts: { sans: { family: "Atkinson", assetId: "f1" }, mono: { family: "Plex Mono" } },
      shape: { radiusOverlay: 12 },
      shadows: { 9: "none" },
      cursors: {},
      icons: { search: { paths: ["M1 1L15 15"] }, gear: { assetId: "g1" }, empty: {} },
      backdrop: { assetId: "b1", fit: "tile" },
      css: " .x { color: red } ",
    };
    const css = compileTheme({ id: "t", spec, assets: [{ id: "f1", contentType: "font/woff2" }] });
    expect(css).toContain('src: url("/api/v1/themes/t/assets/f1") format("woff2");');
    expect(css).toContain('--font-mono: "Plex Mono", ui-monospace, monospace;');
    expect(css).toContain("--radius-overlay: 12px;");
    expect(css).not.toContain("--shadow-9");
    expect(css).toContain('svg[data-icon="search"]');
    expect(css).toContain("data:image/svg+xml,");
    expect(css).toContain('url("/api/v1/themes/t/assets/g1")');
    expect(css).not.toContain('svg[data-icon="empty"]');
    expect(css).toContain('.bg-backdrop {\n  background: url("/api/v1/themes/t/assets/b1") repeat;');
    expect(css.endsWith(".x { color: red }")).toBe(true);
    const cover = compileTheme({ id: "t", spec: { ...spec, backdrop: { assetId: "b1", fit: "cover" } }, assets: [] });
    expect(cover).toContain("center / cover no-repeat");
    expect(cover).toContain('format("woff")');
  });
});

describe("the appearance", () => {
  it("stamps the chosen mode on the document and keeps it for the next load", () => {
    applyAppearance({ mode: "dark" });
    expect(document.documentElement.dataset.theme).toBe("dark");
    applyAppearance({ mode: "system" });
    expect(document.documentElement.dataset.theme).toBeUndefined();
    applyAppearance({ mode: "light" });
    reloadAppearance();
    expect(document.documentElement.dataset.theme).toBe("light");
  });

  it("steps from the system to light to dark and round again", () => {
    expect(nextMode("system")).toBe("light");
    expect(nextMode("light")).toBe("dark");
    expect(nextMode("dark")).toBe("system");
  });

  it("puts one theme stylesheet on the page and remembers it for the next load", () => {
    applyThemeCSS(".a {}");
    applyThemeCSS(".b {}");
    expect(document.querySelectorAll(`#${THEME_STYLE_ID}`)).toHaveLength(1);
    expect(document.getElementById(THEME_STYLE_ID)?.textContent).toBe(".b {}");
    cacheThemeCSS({ key: "t1:1", css: ".b {}" });
    expect(cachedThemeCSS()).toEqual({ key: "t1:1", css: ".b {}" });
    cacheThemeCSS(null);
    expect(cachedThemeCSS()).toBeNull();
    applyThemeCSS(null);
    expect(document.getElementById(THEME_STYLE_ID)).toBeNull();
  });

  it("still applies in a browser that keeps nothing", () => {
    const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    applyAppearance({ mode: "dark" });
    expect(document.documentElement.dataset.theme).toBe("dark");
    spy.mockRestore();
  });
});
