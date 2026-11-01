import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const target = (id: string, fte: number, from: string, to: string | null = null) => ({ id, unitId: null, positionId: null, roleId: "aud", fte, from, to });

function rolePage(answers: { capacity: { fte: number; target: number | null }; targets: object[]; account?: object }) {
  const posted: unknown[] = [];
  const removed: string[] = [];
  const asked: string[] = [];
  fakeAPI({
    ...signedInAs(answers.account ?? ADMIN),
    "GET /roles/aud": { body: { asOf: "2026-11-02", role: { id: "aud", key: "auditor", name: "Auditor", description: "", positions: [], holders: [] } } },
    "GET /positions": { body: { asOf: "2026-11-02", positions: [] } },
    "GET /people": { body: { asOf: "2026-11-02", people: [] } },
    "GET /responsibilities": { body: { asOf: "2026-11-02", responsibilities: [] } },
    "GET /roles/aud/capacity": (_, url) => {
      asked.push(url);
      return {
        body: {
          from: "2026-11-02",
          to: "2026-11-02",
          by: "day",
          subUnits: false,
          periods: [{ from: "2026-11-02", to: "2026-11-02", people: 1, ...answers.capacity }],
        },
      };
    },
    "GET /targets": { body: { targets: answers.targets } },
    "POST /targets": (body) => {
      posted.push(body);
      return { status: 201, body: { target: target("new", 3, "2026-11-02") } };
    },
    "DELETE /targets/old": () => {
      removed.push("old");
      return { status: 204 };
    },
  });
  renderApp("/roles/aud?asOf=2026-11-02");
  return { posted, removed, asked };
}

describe("a target on a page", () => {
  it("says what the subject should have against what it has, and lists its targets newest first", async () => {
    const { asked } = rolePage({
      capacity: { fte: 1.5, target: 2.25 },
      targets: [target("old", 1, "2026-01-01", "2026-06-01"), target("now", 2.25, "2026-06-01")],
    });
    const section = await screen.findByRole("region", { name: "Target" });
    expect(await within(section).findByText("Should have 2.25 FTE")).toBeVisible();
    expect(within(section).getByText(/0\.75 FTE short/)).toBeVisible();
    expect(within(section).getByText("Has 1.5 FTE on this day, from its holdings' shares.")).toBeVisible();
    const rows = within(within(section).getByRole("list", { name: "Targets over time" })).getAllByRole("listitem");
    expect(rows.map((r) => r.textContent)).toEqual([expect.stringContaining("from 2026-06-01"), expect.stringContaining("2026-01-01 until 2026-06-01")]);
    expect(asked[0]).toContain("from=2026-11-02");
    expect(asked[0]).toContain("by=day");
  });

  it("says when a target is met, and when there is none", async () => {
    rolePage({ capacity: { fte: 3, target: 2 }, targets: [] });
    const section = await screen.findByRole("region", { name: "Target" });
    expect(await within(section).findByText(/met/)).toBeVisible();
    cleanup();
    rolePage({ capacity: { fte: 0, target: null }, targets: [] });
    expect(await screen.findByText("No target on this day.")).toBeVisible();
  });

  it("lets an editor set a target for a span and remove one", async () => {
    const { posted, removed } = rolePage({ capacity: { fte: 1, target: 1 }, targets: [target("old", 1, "2026-01-01")] });
    const section = await screen.findByRole("region", { name: "Target" });
    const value = await within(section).findByLabelText("Full-time equivalents");
    expect(within(section).getByLabelText("From")).toHaveValue("2026-11-02");
    await userEvent.type(value, "2,5");
    await userEvent.type(within(section).getByLabelText("Until (optional)"), "2027-01-01");
    await userEvent.click(within(section).getByRole("button", { name: "Set" }));
    await waitFor(() => expect(posted).toEqual([{ roleId: "aud", fte: 2.5, from: "2026-11-02", to: "2027-01-01" }]));
    await waitFor(() => expect(value).toHaveValue(""));
    await userEvent.click(within(section).getByRole("button", { name: /Remove: 1 FTE from 2026-01-01/ }));
    await waitFor(() => expect(removed).toEqual(["old"]));
  });

  it("shows a refused target's reason by its field", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /roles/aud": { body: { asOf: "2026-11-02", role: { id: "aud", key: "auditor", name: "Auditor", description: "", positions: [], holders: [] } } },
      "GET /positions": { body: { asOf: "2026-11-02", positions: [] } },
      "GET /people": { body: { asOf: "2026-11-02", people: [] } },
      "GET /targets": { body: { targets: [] } },
      "POST /targets": {
        status: 422,
        body: { error: { code: "validation_failed", message: "Some fields need attention.", fields: { fte: "Give full-time equivalents from 0 to 100000." } } },
      },
    });
    renderApp("/roles/aud?asOf=2026-11-02");
    const section = await screen.findByRole("region", { name: "Target" });
    await userEvent.type(await within(section).findByLabelText("Full-time equivalents"), "-1");
    await userEvent.click(within(section).getByRole("button", { name: "Set" }));
    expect(await within(section).findByText("Give full-time equivalents from 0 to 100000.")).toBeVisible();
  });

  it("is read only for a member", async () => {
    rolePage({ capacity: { fte: 1, target: 1 }, targets: [target("old", 1, "2026-01-01")], account: { ...ADMIN, role: "member", ownRole: "member" } });
    const section = await screen.findByRole("region", { name: "Target" });
    expect(await within(section).findByText("Should have 1 FTE")).toBeVisible();
    expect(within(section).queryByRole("button")).toBeNull();
    expect(within(section).queryByLabelText("Full-time equivalents")).toBeNull();
  });

  it("speaks German", async () => {
    applyLanguage("de");
    rolePage({ capacity: { fte: 1, target: 2 }, targets: [] });
    const section = await screen.findByRole("region", { name: "Soll" });
    expect(await within(section).findByText("Soll 2 VZÄ")).toBeVisible();
    expect(within(section).getByText(/1 VZÄ fehlen/)).toBeVisible();
  });
});
