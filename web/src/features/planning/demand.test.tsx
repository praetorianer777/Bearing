import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const org = {
  "GET /units": {
    body: { asOf: "2026-11-02", units: [{ id: "root", name: "Kestrel", typeId: "t", parentId: null, shortName: null, description: "", archivedOn: null }] },
  },
  "GET /positions": { body: { asOf: "2026-11-02", positions: [] } },
  "GET /people": { body: { asOf: "2026-11-02", people: [] } },
  "GET /gaps": { body: { gaps: [] } },
  "GET /units/root/capacity": { body: { periods: [] } },
};

const week = (start: string, fields: object) => ({
  start,
  fte: 2.5,
  target: 3,
  people: 3,
  allocatedFte: null,
  loadHours: null,
  nominalHours: null,
  daysAway: null,
  ...fields,
});

describe("demand on the planning page", () => {
  it("shows Armature's demand against the unit's capacity and who is given out beyond their week", async () => {
    fakeAPI({
      ...signedInAs(),
      ...org,
      "GET /units/root/demand": {
        body: {
          from: "2026-11-02",
          to: "2027-02-07",
          subUnits: true,
          partner: { status: "ok" },
          weeks: [week("2026-11-02", { allocatedFte: 3.2, loadHours: 45, nominalHours: 60, daysAway: 1 })],
          over: [
            { personId: "d", name: "Dev", week: "2026-11-02", reason: "over_allocated", allocationPercent: 120, loadHours: 30, nominalHours: 40, share: 100 },
            { personId: "a", name: "Ann", week: "2026-11-02", reason: "over_loaded", allocationPercent: 50, loadHours: 30, nominalHours: 40, share: 50 },
          ],
          warnings: 2,
          unmatched: [
            {
              personId: "e",
              name: "Ext",
              issuer: null,
              subject: null,
              positionId: "p",
              title: "Writer",
              unitId: "root",
              headOfUnit: false,
              holdingId: "h",
              share: 100,
            },
          ],
        },
      },
    });
    renderApp("/planning?from=2026-11-02");
    const section = within((await screen.findByRole("heading", { name: "Demand from Armature" })).closest("section") as HTMLElement);
    const row = within(await section.findByRole("row", { name: /2026-11-02/ }));
    expect(row.getByText("3.2")).toHaveClass("text-danger");
    expect(row.getByText("45")).toBeInTheDocument();
    expect(section.getByText("Dev, week of 2026-11-02: allocated 120 percent.")).toBeInTheDocument();
    expect(section.getByText("Ann, week of 2026-11-02: 30 hours of work for a 50 percent share of 40 hours.")).toBeInTheDocument();
    expect(section.getByText("Armature does not know: Ext.")).toBeInTheDocument();
  });

  it("says why Armature is not asked, naming what to update, and counts warnings for members", async () => {
    fakeAPI({
      ...signedInAs(),
      ...org,
      "GET /units/root/demand": {
        body: {
          from: "2026-11-02",
          to: "2027-02-07",
          subUnits: true,
          partner: { status: "outdated", message: "The connected Armature does not offer resource-load@1, which Bearing needs for this." },
          weeks: [week("2026-11-02", {})],
          over: [],
          warnings: 1,
          unmatched: [],
        },
      },
    });
    renderApp("/planning?from=2026-11-02");
    expect(await screen.findByText(/does not offer resource-load@1/)).toBeInTheDocument();
    expect(screen.getByText("1 person is given out beyond their week")).toBeInTheDocument();
    expect(screen.getByText("Editors see who.")).toBeInTheDocument();
  });

  it("says when the demand cannot be read", async () => {
    fakeAPI({ ...signedInAs(), ...org, "GET /units/root/demand": { status: 500, body: { error: { code: "internal", message: "No." } } } });
    renderApp("/planning?from=2026-11-02");
    expect(await screen.findByText("The demand could not be read. Try again in a moment.")).toBeInTheDocument();
  });
});
