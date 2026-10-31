import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const unit = (id: string, name: string, parentId: string | null, extra: object = {}) => ({
  id,
  name,
  parentId,
  typeId: "dept",
  shortName: null as string | null,
  description: "",
  archivedOn: null as string | null,
  ...extra,
});
const holding = (id: string, personId: string, positionId: string, from = "2025-01-01", to: string | null = null, share = 100) => ({
  id,
  personId,
  positionId,
  roleId: null,
  share,
  from,
  to,
});
const position = (id: string, unitId: string, title: string, extra: object = {}) => ({
  id,
  unitId,
  title,
  description: "",
  headOfUnit: false,
  seats: 1,
  existsFrom: "2025-01-01",
  archivedOn: null as string | null,
  reportsToId: null as string | null,
  dottedTo: [] as string[],
  holders: [] as ReturnType<typeof holding>[],
  roles: [] as string[],
  deputies: [] as string[],
  ...extra,
});
const person = (id: string, name: string) => ({
  id,
  name,
  email: null,
  phone: null,
  location: null,
  kind: "external" as const,
  accountId: null,
  issuer: null,
  subject: null,
  leftOn: null,
  hasPhoto: false,
});
const member = (personId: string, name: string, positionId: string, title: string, unitId: string, extra: object = {}) => ({
  personId,
  name,
  positionId,
  title,
  unitId,
  headOfUnit: false,
  holdingId: `h-${personId}`,
  share: 100,
  ...extra,
});

const units = [unit("ops", "Operations", null), unit("quality", "Quality", "ops", { shortName: "QA" }), unit("lab", "Test lab", "quality")];
const hoq = position("hoq", "quality", "Head of Quality", { headOfUnit: true, reportsToId: "hops", deputies: ["lead"], seats: 1 });
const analyst = position("analyst", "quality", "Quality analyst", {
  seats: 2,
  holders: [holding("h-ana", "ana", "analyst", "2025-02-01", null, 50)],
  reportsToId: "hoq",
  roles: ["auditor"],
  deputies: ["lead"],
  lines: [
    { reportsToId: "old", kind: "solid", from: "2025-01-01", to: "2026-01-01" },
    { reportsToId: "hoq", kind: "solid", from: "2026-01-01", to: null },
  ],
});
const lead = position("lead", "lab", "Test lab lead", { headOfUnit: true, reportsToId: "hoq", holders: [holding("h-carol", "carol", "lead")] });
const old = position("old", "ops", "Quality manager", { archivedOn: "2026-01-01" });

function serve(role = "administrator") {
  fakeAPI({
    ...signedInAs({ ...ADMIN, role }),
    "GET /units/quality": () => ({
      body: {
        asOf: "2026-10-31",
        unit: { ...units[1], description: "Keeps the sensors honest.", placements: [{ parentId: "ops", from: "2025-01-01", to: null }] },
      },
    }),
    "GET /units": () => ({ body: { asOf: "2026-10-31", units } }),
    "GET /unit-types": () => ({ body: { unitTypes: [{ id: "dept", name: "Department", icon: "department", colour: "#3b8c6e", rank: 2, units: 2 }] } }),
    "GET /positions": (_, url) => ({
      body: { asOf: "2026-10-31", positions: url.includes("unitId=quality") ? [hoq, analyst] : [hoq, analyst, lead] },
    }),
    "GET /units/quality/members": () => ({
      body: {
        asOf: "2026-10-31",
        members: [
          member("ana", "Ana Analyst", "analyst", "Quality analyst", "quality", { share: 50 }),
          member("carol", "Carol Checker", "lead", "Test lab lead", "lab", { headOfUnit: true }),
        ],
      },
    }),
    "GET /units/quality/audit": () => ({
      body: {
        events: [{ id: "e", at: "2026-01-01T09:00:00Z", actorId: null, actorName: null, table: "unit", operation: "insert", before: {}, after: {} }],
        next: null,
      },
    }),
    "GET /positions/analyst": () => ({ body: { asOf: "2026-10-31", position: analyst } }),
    "GET /positions/old": () => ({ body: { asOf: "2025-01-01", position: old } }),
    "GET /holdings": () => ({ body: { holdings: [holding("h-bo", "bo", "analyst", "2025-01-01", "2025-02-01"), ...analyst.holders] } }),
    "GET /positions/analyst/chain": () => ({
      body: {
        asOf: "2026-10-31",
        positions: [{ positionId: "hoq", title: "Head of Quality", unitId: "quality", from: "analyst", kind: "solid", depth: 1, holders: [] }],
      },
    }),
    "GET /positions/analyst/reports": () => ({ body: { asOf: "2026-10-31", positions: [] } }),
    "GET /people": (_, url) => ({
      body: { asOf: "2026-10-31", people: url.includes("left=true") ? [person("bo", "Bo Before")] : [person("ana", "Ana Analyst")] },
    }),
    "GET /roles": () => ({ body: { roles: [{ id: "auditor", key: "auditor", name: "Auditor", description: "" }] } }),
    "GET /positions/analyst/audit": () => ({ body: { events: [], next: null } }),
  });
}

const section = (name: string) => screen.getByRole("region", { name });

describe("the unit page", () => {
  it("shows the unit's type, place, head, positions, members and sub-units", async () => {
    serve();
    renderApp("/units/quality");
    expect(await screen.findByRole("heading", { level: 1, name: /Quality/ })).toHaveTextContent("QA");
    expect(screen.getByText("Keeps the sensors honest.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Operations" })).toHaveAttribute("href", "/units/ops");
    expect(within(section("Head")).getByRole("link", { name: "Head of Quality" })).toBeInTheDocument();
    expect(within(section("Head")).getByText("Vacant")).toBeInTheDocument();
    expect(within(section("Sub-units")).getByRole("link", { name: "Test lab" })).toHaveAttribute("href", "/units/lab");
    expect(within(section("Positions")).getByText("(50 %)")).toBeInTheDocument();
    expect(within(section("2 members, with its sub-units")).getByText(/Test lab lead · Test lab/)).toBeInTheDocument();
    expect(within(section("Place in the tree over time")).getByText("Inside Operations")).toBeInTheDocument();
    expect(await screen.findByText("Unit added")).toBeInTheDocument();
    expect(screen.getByText(/Bearing ·/)).toBeInTheDocument();
  });

  it("reads another day and keeps the changes from members", async () => {
    serve("member");
    renderApp("/units/quality?asOf=2026-03-31");
    expect(await screen.findByRole("heading", { level: 1, name: /Quality/ })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("As of"), { target: { value: "2025-06-01" } });
    await waitFor(() => {
      const urls = vi.mocked(globalThis.fetch).mock.calls.map(([input]) => (input instanceof Request ? input.url : String(input)));
      expect(urls.some((u) => u.includes("/units/quality?asOf=2025-06-01"))).toBe(true);
    });
    expect(screen.queryByRole("region", { name: "Changes" })).not.toBeInTheDocument();
  });

  it("says when it cannot be read", async () => {
    fakeAPI(signedInAs());
    renderApp("/units/nowhere");
    expect(await screen.findByText(/This unit could not be read/)).toBeInTheDocument();
  });
});

describe("the position page", () => {
  it("shows holders now and over time, lines, deputies, roles and whom it stands in for", async () => {
    serve();
    renderApp("/positions/analyst");
    expect(await screen.findByRole("heading", { level: 1, name: "Quality analyst" })).toBeInTheDocument();
    expect(screen.getByText(/2 seats · exists from 2025-01-01/)).toBeInTheDocument();
    expect(within(section("Held by")).getByRole("link", { name: "Ana Analyst" })).toBeInTheDocument();
    expect(within(section("Held by")).getByText("1 seat open")).toBeInTheDocument();
    expect(within(section("Roles")).getByText("Auditor")).toBeInTheDocument();
    expect(within(section("Reports to")).getByText("Vacant")).toBeInTheDocument();
    expect(within(section("Direct reports")).getByText("No position reports to it on this day.")).toBeInTheDocument();
    expect(within(section("Deputies")).getByRole("link", { name: "Test lab lead" })).toBeInTheDocument();
    expect(within(section("Holders over time")).getByRole("link", { name: "Bo Before" })).toBeInTheDocument();
    expect(within(section("Holders over time")).getByText("2025-01-01 until 2025-02-01")).toBeInTheDocument();
    expect(within(section("Reporting lines over time")).getByRole("link", { name: "Quality manager" })).toBeInTheDocument();
    expect(within(section("Stands in for")).getByText("Stands in for nobody.")).toBeInTheDocument();
    expect(await screen.findByText("No changes recorded.")).toBeInTheDocument();
  });

  it("says when it cannot be read", async () => {
    fakeAPI(signedInAs());
    renderApp("/positions/nowhere");
    expect(await screen.findByText(/This position could not be read/)).toBeInTheDocument();
  });
});
