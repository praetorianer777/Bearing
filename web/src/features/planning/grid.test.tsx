import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";
import { periodsOf, toCsv } from "./periods";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const unit = (id: string, name: string, parentId: string | null) => ({ id, name, parentId, typeId: "t", shortName: null, description: "", archivedOn: null });
const position = (id: string, unitId: string, title: string) => ({
  id,
  unitId,
  title,
  description: "",
  headOfUnit: false,
  seats: 1,
  existsFrom: "2025-01-01",
  archivedOn: null,
  reportsToId: null,
  dottedTo: [],
  holders: [],
  roles: [],
  deputies: [],
});
const person = (id: string, name: string) => ({
  id,
  name,
  email: null,
  phone: null,
  location: null,
  kind: "external",
  accountId: null,
  issuer: null,
  subject: null,
  leftOn: null,
  hasPhoto: false,
});
const period = (from: string, to: string, fte: number, extra: object = {}) => ({ from, to, fte, target: null, people: 1, ...extra });

// Two weeks from Monday 2 November 2026.
const weeks = [period("2026-11-02", "2026-11-08", 0), period("2026-11-09", "2026-11-15", 0)];

function planning(account: object = ADMIN, hidden = false) {
  const posted: { path: string; body: unknown }[] = [];
  fakeAPI({
    ...signedInAs(account),
    "GET /units": { body: { asOf: "2026-11-02", units: [unit("org", "Kestrel", null), unit("q", "Quality", "org"), unit("lab", "Test lab", "q")] } },
    "GET /positions": { body: { asOf: "2026-11-02", positions: [position("hoq", "q", "Head of Quality"), position("ceo", "org", "Chief executive")] } },
    "GET /people": { body: { asOf: "2026-11-02", people: [person("ana", "Ana Analyst"), person("cy", "Cy Checker")] } },
    "GET /gaps": { body: { gaps: [] } },
    "GET /units/org/capacity": {
      body: {
        periods: [
          period("2026-11-02", "2026-11-08", 4, { target: 5, seats: 6, filled: 4, vacant: 2 }),
          period("2026-11-09", "2026-11-15", 5, { target: 5, seats: 6, filled: 5, vacant: 1 }),
        ],
      },
    },
    "GET /units/q/capacity": {
      body: {
        periods: [
          period("2026-11-02", "2026-11-08", 1, { seats: 1, filled: 1, vacant: 0 }),
          period("2026-11-09", "2026-11-15", 1, { seats: 1, filled: 1, vacant: 0 }),
        ],
      },
    },
    "GET /units/lab/capacity": { status: 500, body: {} },
    "GET /positions/hoq/capacity": {
      body: {
        periods: [
          period("2026-11-02", "2026-11-08", 0.5, { seats: 1, filled: 0.5, vacant: 0.5 }),
          period("2026-11-09", "2026-11-15", 1, { seats: 1, filled: 1, vacant: 0 }),
        ],
      },
    },
    "GET /positions/ceo/capacity": { body: { periods: weeks } },
    "GET /people/ana/capacity": hidden
      ? { status: 403, body: { error: { code: "forbidden", message: "No." } } }
      : { body: { periods: [period("2026-11-02", "2026-11-08", 0.5), period("2026-11-09", "2026-11-15", 0.5)] } },
    "GET /people/new/capacity": { body: { periods: weeks } },
    "GET /holdings": {
      body: {
        holdings: [
          { id: "h1", personId: "ana", positionId: "hoq", roleId: null, share: 50, from: "2025-01-01", to: null },
          { id: "h0", personId: "cy", positionId: "hoq", roleId: null, share: 100, from: "2024-01-01", to: "2025-01-01" },
        ],
      },
    },
    "POST /people": (body) => {
      posted.push({ path: "/people", body });
      return { status: 201, body: { person: person("new", "Nia New") } };
    },
    "POST /holdings": (body) => {
      posted.push({ path: "/holdings", body });
      return { status: 201, body: { holding: { id: "h2", personId: "new", positionId: "hoq", roleId: null, share: 100, from: "2026-11-09", to: null } } };
    },
  });
  const rendered = renderApp("/planning?from=2026-11-02&to=2026-11-15");
  return { ...rendered, posted };
}

const rowNamed = (name: string) => screen.getByRole("rowheader", { name: new RegExp(name) }).closest("tr") as HTMLElement;

describe("the planning grid", () => {
  it("shows units over weeks, capacity against target, and marks the cells that miss people", async () => {
    planning();
    const grid = await screen.findByRole("table", { name: /Full-time equivalents each unit/ });
    expect(
      within(grid)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Unit, position or person", "Week of Nov 2", "Week of Nov 9"]);
    await waitFor(() =>
      expect(
        within(rowNamed("Kestrel"))
          .getAllByRole("cell")
          .map((c) => c.textContent),
      ).toEqual(["4 / 5: 1 FTE short, 2 seats vacant", "5 / 5: 1 seat vacant"]),
    );
    expect(within(rowNamed("Kestrel")).getAllByRole("cell")[0]).toHaveClass("bg-warning-subtle");
    expect(screen.getByRole("rowheader", { name: /Chief executive/ })).toBeVisible();
    expect(screen.queryByRole("rowheader", { name: /Head of Quality/ })).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Show what is inside Quality" }));
    expect(await screen.findByRole("rowheader", { name: /Head of Quality/ })).toBeVisible();
    expect(screen.getByRole("rowheader", { name: /Test lab/ })).toBeVisible();
    expect(await within(rowNamed("Test lab")).findByText("This row could not be read.")).toBeVisible();
    await waitFor(() => expect(within(rowNamed("Head of Quality")).getAllByRole("cell")[0]).toHaveTextContent("0.5: 0.5 seats vacant"));
    expect(within(rowNamed("Head of Quality")).getAllByRole("cell")[1]).not.toHaveClass("bg-warning-subtle");

    await userEvent.click(screen.getByRole("button", { name: "Show what is inside Head of Quality" }));
    expect(await screen.findByRole("rowheader", { name: "Ana Analyst" })).toBeVisible();
    expect(screen.queryByRole("rowheader", { name: "Cy Checker" })).toBeNull();
    await waitFor(() => expect(within(rowNamed("Ana Analyst")).getAllByRole("cell")[0]).toHaveTextContent("0.5"));
    expect(within(rowNamed("Ana Analyst")).getAllByRole("cell")[0]).not.toHaveClass("bg-warning-subtle");
    expect(screen.getByRole("link", { name: "Ana Analyst" })).toHaveAttribute("href", "/people/ana");

    await userEvent.click(screen.getByRole("button", { name: "Hide what is inside Quality" }));
    expect(screen.queryByRole("rowheader", { name: /Head of Quality/ })).toBeNull();
  });

  it("lets an editor plan a hire into a position, which adds somebody new and their holding", async () => {
    const { posted } = planning();
    await userEvent.click(await screen.findByRole("button", { name: "Show what is inside Quality" }));
    await userEvent.click(await screen.findByRole("button", { name: "Plan a holder for Head of Quality" }));
    const form = screen.getByRole("region", { name: "Plan a holder for Head of Quality" });
    expect(within(form).getByRole("heading", { name: "Plan a holder for Head of Quality" })).toHaveFocus();
    await userEvent.type(within(form).getByLabelText("Name"), "Nia New");
    fireEvent.change(within(form).getByLabelText("From"), { target: { value: "2026-11-09" } });
    await userEvent.click(within(form).getByRole("button", { name: "Plan the holding" }));
    expect(await within(form).findByRole("status")).toHaveTextContent("Nia New holds Head of Quality from 2026-11-09.");
    expect(posted).toEqual([
      { path: "/people", body: { name: "Nia New" } },
      { path: "/holdings", body: { personId: "new", positionId: "hoq", from: "2026-11-09", share: 100 } },
    ]);
    await userEvent.click(within(form).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("region", { name: "Plan a holder for Head of Quality" })).toBeNull();
  });

  it("plans a handover to somebody in the organisation for a part of their week until a day", async () => {
    const { posted } = planning();
    await userEvent.click(await screen.findByRole("button", { name: "Show what is inside Quality" }));
    await userEvent.click(await screen.findByRole("button", { name: "Plan a holder for Head of Quality" }));
    const form = screen.getByRole("region", { name: "Plan a holder for Head of Quality" });
    await userEvent.click(within(form).getByLabelText("Somebody in the organisation"));
    await userEvent.selectOptions(within(form).getByLabelText("Person"), "Cy Checker");
    fireEvent.change(within(form).getByLabelText("Until (optional)"), { target: { value: "2027-01-01" } });
    await userEvent.clear(within(form).getByLabelText("Share of the week, in percent"));
    await userEvent.type(within(form).getByLabelText("Share of the week, in percent"), "50");
    await userEvent.click(within(form).getByRole("button", { name: "Plan the holding" }));
    expect(await within(form).findByRole("status")).toHaveTextContent("Cy Checker holds Head of Quality from 2026-11-02.");
    expect(posted).toEqual([{ path: "/holdings", body: { personId: "cy", positionId: "hoq", from: "2026-11-02", share: 50, to: "2027-01-01" } }]);
  });

  it("offers no planning to a member, and counts by month on asking", async () => {
    const { router } = planning({ ...ADMIN, role: "member", ownRole: "member" }, true);
    await userEvent.click(await screen.findByRole("button", { name: "Show what is inside Quality" }));
    expect(await screen.findByRole("rowheader", { name: /Head of Quality/ })).toBeVisible();
    expect(screen.queryByRole("button", { name: /Plan a holder/ })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Show what is inside Head of Quality" }));
    expect(await within(rowNamed("Ana Analyst")).findByText("Only the person, their managers and editors see a person's capacity.")).toBeVisible();
    await userEvent.selectOptions(screen.getByLabelText("Count by"), "month");
    await waitFor(() => expect(router.state.location.search).toEqual({ from: "2026-11-02", by: "month" }));
    expect(await screen.findByRole("columnheader", { name: "Nov 2026" })).toBeVisible();
  });

  it("downloads what it shows as CSV", async () => {
    planning();
    await waitFor(() => expect(within(rowNamed("Kestrel")).getAllByRole("cell")[0]).toHaveTextContent("4"));
    const created: Blob[] = [];
    URL.createObjectURL = vi.fn((blob: Blob) => {
      created.push(blob);
      return "blob:planning";
    });
    URL.revokeObjectURL = vi.fn();
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    await userEvent.click(screen.getByRole("button", { name: "Download as CSV" }));
    expect(click).toHaveBeenCalled();
    // FileReader, as the test DOM's Blob cannot read itself.
    const text = await new Promise<string>((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.readAsText(created[0] as Blob);
    });
    expect(text.split("\n").slice(0, 2)).toEqual(["kind,name,from,to,fte,target,seats,vacant,people", "unit,Kestrel,2026-11-02,2026-11-08,4,5,6,2,1"]);
  });

  it("starts from a unit when asked, with that unit's gaps", async () => {
    const asked: string[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /units": { body: { asOf: "2026-11-02", units: [unit("org", "Kestrel", null), unit("q", "Quality", "org")] } },
      "GET /positions": { body: { asOf: "2026-11-02", positions: [position("hoq", "q", "Head of Quality")] } },
      "GET /people": { body: { asOf: "2026-11-02", people: [] } },
      "GET /gaps": (_, url) => {
        asked.push(url);
        return { body: { gaps: [] } };
      },
      "GET /units/q/capacity": { body: { periods: weeks } },
      "GET /positions/hoq/capacity": { body: { periods: weeks } },
    });
    const { router } = renderApp("/planning?unit=q&from=2026-11-02&to=2026-11-15");
    expect(await screen.findByRole("rowheader", { name: /Head of Quality/ })).toBeVisible();
    expect(screen.queryByRole("rowheader", { name: /Kestrel/ })).toBeNull();
    await waitFor(() => expect(asked[0]).toContain("unitId=q"));
    await userEvent.click(screen.getByRole("link", { name: "Plan the whole organisation" }));
    await waitFor(() => expect(router.state.location.search).toEqual({ from: "2026-11-02", to: "2026-11-15" }));
    expect(await screen.findByRole("rowheader", { name: /Kestrel/ })).toBeVisible();
  });

  it("speaks German", async () => {
    applyLanguage("de");
    planning();
    expect(await screen.findByRole("columnheader", { name: "Woche ab 2. Nov." })).toBeVisible();
    expect(screen.getByRole("button", { name: "Zeigen, was in Quality ist" })).toBeVisible();
  });
});

describe("periods", () => {
  it("are weeks from Monday or months, cut by the range", () => {
    expect(periodsOf("2026-11-04", "2026-11-17", "week")).toEqual([
      { from: "2026-11-04", to: "2026-11-08" },
      { from: "2026-11-09", to: "2026-11-15" },
      { from: "2026-11-16", to: "2026-11-17" },
    ]);
    expect(periodsOf("2026-12-15", "2027-02-02", "month")).toEqual([
      { from: "2026-12-15", to: "2026-12-31" },
      { from: "2027-01-01", to: "2027-01-31" },
      { from: "2027-02-01", to: "2027-02-02" },
    ]);
  });

  it("go to CSV with names quoted where they need it, and rows not read yet left out", () => {
    const csv = toCsv([
      { kind: "position", name: 'Head of "R, D"', periods: [period("2026-11-02", "2026-11-08", 1, { target: 2, seats: 1, vacant: 0 })] },
      { kind: "person", name: "Ana", periods: [period("2026-11-02", "2026-11-08", 0.5)] },
      { kind: "unit", name: "Lab", periods: undefined },
    ]);
    expect(csv).toBe(
      'kind,name,from,to,fte,target,seats,vacant,people\nposition,"Head of ""R, D""",2026-11-02,2026-11-08,1,2,1,0,1\nperson,Ana,2026-11-02,2026-11-08,0.5,,,,1\n',
    );
  });
});
