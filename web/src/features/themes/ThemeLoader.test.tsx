import { act, render } from "@testing-library/react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { THEME_CACHE_KEY, THEME_STYLE_ID } from "@/config";
import { applyThemeCSS, cachedThemeCSS } from "@/lib/appearance";
import { compileTheme, emptySpec } from "@/lib/theme";
import { setThemePreview, ThemeLoader } from "./ThemeLoader";

const active = vi.fn();
vi.mock("@/api/themes", () => ({ useActiveTheme: () => ({ data: active() }) }));

const magenta = {
  id: "11111111-1111-1111-1111-111111111111",
  updatedAt: "2026-10-29T08:00:00Z",
  spec: { ...emptySpec(), colors: { light: { accent: "#ff0066" }, dark: {} } },
  assets: [],
};

const styleText = () => document.getElementById(THEME_STYLE_ID)?.textContent ?? null;

describe("the theme loader", () => {
  beforeEach(() => {
    localStorage.clear();
    applyThemeCSS(null);
  });
  afterEach(() => act(() => setThemePreview(null)));

  it("keeps what the last load cached on the page until the server answers, a preview ended included", () => {
    localStorage.setItem(THEME_CACHE_KEY, JSON.stringify({ key: "t1:1", css: ".cached {}" }));
    applyThemeCSS(".cached {}");
    active.mockReturnValue(undefined);
    render(<ThemeLoader />);
    expect(styleText()).toBe(".cached {}");
    act(() => setThemePreview(".draft {}"));
    expect(styleText()).toBe(".draft {}");
    act(() => setThemePreview(null));
    expect(styleText()).toBe(".cached {}");
  });

  it("applies the chosen theme and caches it under its id and version for the next load", () => {
    active.mockReturnValue({ theme: magenta, source: "chosen" });
    render(<ThemeLoader />);
    const css = compileTheme(magenta);
    expect(styleText()).toBe(css);
    expect(cachedThemeCSS()).toEqual({ key: `${magenta.id}:${magenta.updatedAt}`, css });
  });

  it("takes the theme off and forgets the cache when the built-in look is the answer", () => {
    applyThemeCSS(".stale {}");
    localStorage.setItem(THEME_CACHE_KEY, JSON.stringify({ key: "old", css: ".stale {}" }));
    active.mockReturnValue({ theme: null, source: "" });
    render(<ThemeLoader />);
    expect(styleText()).toBeNull();
    expect(cachedThemeCSS()).toBeNull();
  });

  it("shows the editor's draft instead while one is previewed, and the chosen theme after", () => {
    active.mockReturnValue({ theme: magenta, source: "chosen" });
    render(<ThemeLoader />);
    act(() => setThemePreview(".draft {}", null));
    expect(styleText()).toBe(".draft {}");
    act(() => setThemePreview(null));
    expect(styleText()).toBe(compileTheme(magenta));
  });

  it("takes the theme off when the shell goes", () => {
    active.mockReturnValue({ theme: magenta, source: "chosen" });
    render(<ThemeLoader />).unmount();
    expect(styleText()).toBeNull();
  });

  it("reads no cache from what an older version left", () => {
    localStorage.setItem(THEME_CACHE_KEY, "not json");
    expect(cachedThemeCSS()).toBeNull();
    localStorage.setItem(THEME_CACHE_KEY, JSON.stringify({ key: 1 }));
    expect(cachedThemeCSS()).toBeNull();
  });
});
