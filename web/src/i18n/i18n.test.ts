import { afterEach, describe, expect, it } from "vitest";
import { LANGUAGE_STORAGE_KEY } from "@/config";
import { de } from "./de";
import { en } from "./en";
import { applyLanguage, browserLanguage, language, resolveLanguage, t } from "./index";

afterEach(() => {
  applyLanguage("");
  localStorage.clear();
});

type Leaf = { path: string; kind: string };

function leaves(value: unknown, path = ""): Leaf[] {
  if (typeof value === "function") return [{ path, kind: `function of ${value.length}` }];
  if (Array.isArray(value)) return [{ path, kind: `array of ${value.length}` }, ...value.flatMap((each, i) => leaves(each, `${path}[${i}]`))];
  if (value && typeof value === "object")
    return Object.keys(value).flatMap((key) => leaves((value as Record<string, unknown>)[key], path ? `${path}.${key}` : key));
  return [{ path, kind: typeof value }];
}

function strings(value: unknown): string[] {
  if (typeof value === "string") return [value];
  if (typeof value === "function") {
    const out = (value as (...a: unknown[]) => unknown)("X", "Y");
    return typeof out === "string" ? [out] : [];
  }
  if (value && typeof value === "object") return Object.values(value).flatMap(strings);
  return [];
}

describe("the catalogues", () => {
  it("have the same keys, of the same kind", () => {
    expect(leaves(de)).toEqual(leaves(en));
  });

  it("are German in German, not a copy of the English", () => {
    const english = new Set(strings(en));
    const german = strings(de);
    const same = german.filter((each) => english.has(each));
    // Names and version strings read the same in both languages; sentences must not.
    expect(same.length).toBeLessThan(german.length / 5);
  });

  it("keep to plain typography: no dashes, curly quotes or ellipsis characters", () => {
    for (const each of [...strings(en), ...strings(de)]) {
      expect(each, each).not.toMatch(/[–—“”„‘’‚«»…]/);
    }
  });
});

describe("the language", () => {
  it("follows the browser's first language the interface speaks", () => {
    expect(browserLanguage(["fr-FR", "de-AT", "en"])).toBe("de");
    expect(browserLanguage(["fr"])).toBe("en");
    expect(resolveLanguage("", ["de-DE"])).toBe("de");
    expect(resolveLanguage("en", ["de-DE"])).toBe("en");
  });

  it("switches the strings and the document, and is remembered", () => {
    applyLanguage("de");
    expect(language()).toBe("de");
    expect(t.nav.people).toBe("Personen");
    expect(document.documentElement.lang).toBe("de");
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe("de");
    applyLanguage("", ["en-GB"]);
    expect(t.nav.people).toBe("People");
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBeNull();
  });
});
