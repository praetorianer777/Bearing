import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";
import { hitKey, hitTarget } from "./hits";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const hit = (kind: string, id: string, name: string, detail = "", extra: object = {}) => ({ kind, id, name, detail, ...extra });

const quick = [
  hit("person", "zoe", "Zoë Wójcik", "Head of Quality"),
  hit("unit", "q", "Quality", "Department"),
  hit("step", "complaint", "Check the quality", "Customer complaint", { stepKey: "check", version: 2 }),
];

const grouped = {
  asOf: "2026-10-31",
  query: "quality",
  people: [quick[0]],
  positions: [hit("position", "hoq", "Head of Quality", "Quality")],
  units: [quick[1]],
  roles: [],
  processes: [hit("process", "complaint", "Customer complaint", "")],
  steps: [quick[2]],
};

function answers(seen: string[] = []) {
  return {
    ...signedInAs(),
    "GET /search/quick": (_: unknown, url: string) => {
      seen.push(url);
      return { body: { asOf: "2026-10-31", query: "", hits: url.includes("q=nothing") ? [] : quick } };
    },
    "GET /search": (_: unknown, url: string) => {
      seen.push(url);
      return url.includes("q=broken") ? { status: 500, body: { error: { code: "internal", message: "No." } } } : { body: grouped };
    },
  };
}

describe("the search box along the top", () => {
  it("offers the best few hits while typing and opens the one picked with the keys", async () => {
    fakeAPI(answers());
    const { router } = renderApp("/");
    const box = await screen.findByRole("combobox", { name: /search/i });
    await userEvent.type(box, "qual");
    const list = await screen.findByRole("listbox", { name: "Suggestions" });
    expect(await within(list).findByRole("option", { name: /Zoë Wójcik/ })).toBeVisible();
    expect(within(list).getByRole("option", { name: /Check the quality.*Customer complaint/ })).toBeVisible();

    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    expect(box).toHaveAttribute("aria-activedescendant", within(list).getByRole("option", { name: /^Unit/ }).id);
    await userEvent.keyboard("{ArrowUp}{Enter}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/people/zoe"));
    expect(box).toHaveValue("");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("goes round from the box through the options and back", async () => {
    fakeAPI(answers());
    renderApp("/");
    const box = await screen.findByRole("combobox", { name: /search/i });
    await userEvent.type(box, "qual");
    await screen.findByRole("option", { name: /Zoë/ });
    await userEvent.keyboard("{ArrowUp}");
    expect(box).toHaveAttribute("aria-activedescendant", screen.getByRole("option", { name: 'Every result for "qual"' }).id);
    await userEvent.keyboard("{ArrowDown}");
    expect(box).not.toHaveAttribute("aria-activedescendant");
  });

  it("opens every result on the search page with Enter, and a step in its version with the pointer", async () => {
    fakeAPI(answers());
    const { router } = renderApp("/");
    const box = await screen.findByRole("combobox", { name: /search/i });
    await userEvent.type(box, "quality{Enter}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/search"));
    expect(router.state.location.search).toEqual({ q: "quality" });
    expect(await screen.findByRole("heading", { name: "Search", level: 1 })).toBeVisible();

    await userEvent.type(box, "check");
    fireEvent.mouseDown(await screen.findByRole("option", { name: /Check the quality/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/processes/complaint"));
    expect(router.state.location.search).toEqual({ version: 2 });
  });

  it("is reached with / and let go with Escape", async () => {
    fakeAPI(answers());
    renderApp("/");
    const box = await screen.findByRole("combobox", { name: /search/i });
    await userEvent.keyboard("/");
    expect(box).toHaveFocus();
    await userEvent.keyboard("nothing");
    expect(await screen.findByText("Nothing matches.")).toBeVisible();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(box).toHaveValue("nothing");
    await userEvent.keyboard("{Escape}");
    expect(box).toHaveValue("");
    expect(box).not.toHaveFocus();
  });

  it("leaves / alone where it is typed", async () => {
    fakeAPI(answers());
    renderApp("/search");
    const field = await screen.findByLabelText("Search for");
    await userEvent.type(field, "a/b");
    expect(field).toHaveValue("a/b");
    expect(screen.getByRole("combobox", { name: /search/i })).not.toHaveFocus();
  });

  it("asks nothing for a single letter", async () => {
    const seen: string[] = [];
    fakeAPI(answers(seen));
    renderApp("/");
    const box = await screen.findByRole("combobox", { name: /search/i });
    await userEvent.type(box, "q{ArrowDown}{Enter}");
    expect(screen.queryByRole("listbox")).toBeNull();
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(seen).toEqual([]);
  });
});

describe("the search page", () => {
  it("lists what was found by kind, each leading to its page on the day asked", async () => {
    fakeAPI(answers());
    renderApp("/search?q=quality&asOf=2026-10-31");
    const people = await screen.findByRole("region", { name: /People/ });
    expect(within(people).getByRole("link", { name: "Zoë Wójcik" })).toHaveAttribute("href", "/people/zoe?asOf=2026-10-31");
    expect(within(people).getByText("Head of Quality")).toBeVisible();
    expect(within(screen.getByRole("region", { name: /Positions/ })).getByRole("link", { name: "Head of Quality" })).toHaveAttribute(
      "href",
      "/positions/hoq?asOf=2026-10-31",
    );
    expect(within(screen.getByRole("region", { name: /Units/ })).getByRole("link", { name: "Quality" })).toHaveAttribute("href", "/units/q?asOf=2026-10-31");
    expect(within(screen.getByRole("region", { name: /Steps/ })).getByRole("link", { name: "Check the quality" })).toHaveAttribute(
      "href",
      "/processes/complaint?asOf=2026-10-31&version=2",
    );
    expect(screen.queryByRole("region", { name: /Roles/ })).toBeNull();
  });

  it("follows the field into the address, and hints until there is enough to search", async () => {
    const seen: string[] = [];
    fakeAPI(answers(seen));
    const { router } = renderApp("/search");
    expect(await screen.findByText("Type two letters or more.")).toBeVisible();
    await userEvent.type(screen.getByLabelText("Search for"), "quality");
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "quality" }));
    expect(await screen.findByRole("region", { name: /Processes/ })).toBeVisible();
    expect(seen.some((url) => url.includes("/search?q=quality"))).toBe(true);
  });

  it("says when nothing matches and when the search fails", async () => {
    fakeAPI({
      ...answers(),
      "GET /search": (_: unknown, url: string) =>
        url.includes("q=broken") ? { status: 500, body: {} } : { body: { ...grouped, people: [], positions: [], units: [], processes: [], steps: [] } },
    });
    const { router } = renderApp("/search?q=zzz");
    expect(await screen.findByText("Nothing matches.")).toBeVisible();
    await router.navigate({ to: "/search", search: { q: "broken" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("The search could not be run.");
  });

  it("speaks German", async () => {
    applyLanguage("de");
    fakeAPI(answers());
    renderApp("/search?q=quality");
    expect(await screen.findByRole("heading", { name: "Suche", level: 1 })).toBeVisible();
    expect(await screen.findByRole("region", { name: /Personen/ })).toBeVisible();
  });
});

describe("where a hit leads", () => {
  it("is the page of its kind, a step's process in its version", () => {
    expect(hitTarget(hit("role", "r", "Approver"))).toEqual({ to: "/roles/$roleId", params: { roleId: "r" }, search: {} });
    expect(hitTarget(hit("process", "p", "Invoice"), "2026-01-01")).toEqual({
      to: "/processes/$processId",
      params: { processId: "p" },
      search: { asOf: "2026-01-01" },
    });
    expect(hitTarget(hit("step", "p", "Draft step", "", { stepKey: "s" }))).toEqual({ to: "/processes/$processId", params: { processId: "p" }, search: {} });
    expect(hitKey(hit("step", "p", "", "", { stepKey: "s" }))).toBe("step-p-s");
  });
});
