import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";
import { daysAfter } from "./GapList";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const gap = (kind: string, subjectKind: string, subjectId: string, name: string, from: string, to: string, size: number) => ({
  kind,
  subjectKind,
  subjectId,
  name,
  unitId: subjectKind === "role" ? null : "q",
  from,
  to,
  size,
});

const gaps = [
  gap("vacant", "position", "hoq", "Head of Quality", "2026-11-02", "2026-12-31", 1),
  gap("vacant", "position", "an", "Analyst", "2026-11-02", "2026-11-02", 1.5),
  gap("under_target", "unit", "q", "Quality", "2026-11-09", "2026-11-20", 2.5),
  gap("over_target", "position", "lead", "Lab lead", "2026-11-09", "2026-11-20", 0.5),
  gap("unheld", "role", "aud", "Auditor", "2026-11-10", "2026-11-12", 0),
  gap("single_holder", "role", "app", "Approver", "2026-11-12", "2026-11-30", 1),
];

describe("the planning page", () => {
  it("lists the organisation's gaps with what is missing and when, each leading to its subject", async () => {
    const asked: string[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /gaps": (_, url) => {
        asked.push(url);
        return { body: { from: "2026-11-02", to: "2027-01-31", gaps } };
      },
    });
    const { router } = renderApp("/planning?from=2026-11-02");
    const list = await screen.findByRole("list", { name: "Gaps" });
    const rows = within(list)
      .getAllByRole("listitem")
      .map((li) => li.textContent);
    expect(rows).toEqual([
      "Head of Quality · 1 seat vacant2026-11-02 to 2026-12-31",
      "Analyst · 1.5 seats vacant2026-11-02",
      "Quality · 2.5 FTE under target2026-11-09 to 2026-11-20",
      "Lab lead · 0.5 FTE over target2026-11-09 to 2026-11-20",
      "Auditor · nobody holds it2026-11-10 to 2026-11-12",
      "Approver · only one person holds it2026-11-12 to 2026-11-30",
    ]);
    expect(within(list).getByRole("link", { name: "Quality" })).toHaveAttribute("href", "/units/q");
    expect(within(list).getByRole("link", { name: "Auditor" })).toHaveAttribute("href", "/roles/aud");
    expect(within(list).getByRole("link", { name: "Analyst" })).toHaveAttribute("href", "/positions/an");
    expect(asked[0]).toContain("from=2026-11-02");
    expect(asked[0]).toContain(`to=${daysAfter("2026-11-02", 90)}`);
    expect(asked[0]).not.toContain("unitId");

    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-12-01" } });
    await waitFor(() => expect(router.state.location.search).toEqual({ from: "2026-11-02", to: "2026-12-01" }));
    await waitFor(() => expect(asked.at(-1)).toContain("to=2026-12-01"));
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "2026-11-05" } });
    await waitFor(() => expect(router.state.location.search).toEqual({ from: "2026-11-05", to: "2026-12-01" }));
  });

  it("is in the sidebar, and says when there are no gaps or they cannot be read", async () => {
    fakeAPI({ ...signedInAs(), "GET /gaps": (_, url) => (url.includes("2026-01") ? { status: 500, body: {} } : { body: { gaps: [] } }) });
    const { router } = renderApp("/planning");
    expect(within(await screen.findByRole("navigation", { name: "Main" })).getByRole("link", { name: "Planning" })).toHaveAttribute("aria-current", "page");
    expect(await screen.findByText("No gaps in these weeks.")).toBeVisible();
    await router.navigate({ to: "/planning", search: { from: "2026-01-01" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("The gaps could not be read.");
  });

  it("speaks German", async () => {
    applyLanguage("de");
    fakeAPI({ ...signedInAs(), "GET /gaps": { body: { gaps: gaps.slice(0, 3) } } });
    renderApp("/planning");
    const list = await screen.findByRole("list", { name: "Lücken" });
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual([
      "Head of Quality · 1 Platz unbesetzt2026-11-02 bis 2026-12-31",
      "Analyst · 1.5 Plätze unbesetzt2026-11-02",
      "Quality · 2.5 VZÄ unter Soll2026-11-09 bis 2026-11-20",
    ]);
  });
});

describe("the days after a day", () => {
  it("crosses months and years", () => {
    expect(daysAfter("2026-12-15", 20)).toBe("2027-01-04");
    expect(daysAfter("2026-02-27", 2)).toBe("2026-03-01");
  });
});
