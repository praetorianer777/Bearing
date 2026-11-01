import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const role = { id: "aud", key: "auditor", name: "Auditor", description: "", positions: [], holders: [] };
const page = {
  "GET /roles/aud": { body: { asOf: "2026-11-02", role } },
  "GET /positions": { body: { asOf: "2026-11-02", positions: [] } },
  "GET /people": { body: { asOf: "2026-11-02", people: [] } },
  "GET /targets": { body: { targets: [] } },
  "GET /responsibilities": { body: { asOf: "2026-11-02", duties: [] } },
};

const gap = { subjectKind: "role", subjectId: "aud", name: "Auditor", from: "2026-11-10", to: "2026-11-10", days: 1 };

describe("nobody there", () => {
  it("lists the days a role has nobody there", async () => {
    fakeAPI({
      ...signedInAs(),
      ...page,
      "GET /roles/aud/absences": {
        body: {
          from: "2026-11-02",
          to: "2027-02-01",
          partner: { status: "ok" },
          hidden: false,
          gaps: [gap, { ...gap, from: "2026-11-16", to: "2026-11-18", days: 3 }],
        },
      },
    });
    renderApp("/roles/aud");
    const list = within(await screen.findByRole("list", { name: "Nobody there" }));
    expect(list.getByText(/2026-11-10/)).toBeInTheDocument();
    expect(list.getByText(/2026-11-16 to 2026-11-18/)).toBeInTheDocument();
  });

  it("says why Armature is not asked", async () => {
    fakeAPI({
      ...signedInAs(),
      ...page,
      "GET /roles/aud/absences": {
        body: {
          from: "2026-11-02",
          to: "2027-02-01",
          partner: { status: "not_connected", message: "You are not connected to Armature." },
          hidden: false,
          gaps: [],
        },
      },
    });
    renderApp("/roles/aud");
    expect(await screen.findByText("You are not connected to Armature.")).toBeInTheDocument();
  });

  it("shows nothing to somebody who may not see who is away", async () => {
    fakeAPI({
      ...signedInAs(),
      ...page,
      "GET /roles/aud/absences": { body: { from: "2026-11-02", to: "2027-02-01", partner: { status: "ok" }, hidden: true, gaps: [] } },
    });
    renderApp("/roles/aud");
    await screen.findByRole("heading", { level: 1, name: "Auditor" });
    expect(screen.queryByRole("heading", { name: "Nobody there" })).not.toBeInTheDocument();
  });

  it("names a step and the party answering for it", async () => {
    fakeAPI({
      ...signedInAs(),
      ...page,
      "GET /roles/aud/absences": {
        body: {
          from: "2026-11-02",
          to: "2027-02-01",
          partner: { status: "ok" },
          hidden: false,
          gaps: [{ ...gap, stepKey: "approve", stepName: "Approve", kind: "responsible" }],
        },
      },
    });
    renderApp("/roles/aud");
    expect(await screen.findByText("Approve: Auditor, responsible")).toBeInTheDocument();
  });
});
