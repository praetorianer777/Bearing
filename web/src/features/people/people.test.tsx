import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";
import { personView } from "./view";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const unit = (id: string, name: string, parentId: string | null) => ({ id, name, parentId, typeId: "t", shortName: null, description: "", archivedOn: null });
const position = (id: string, unitId: string, title: string, extra: object = {}) => ({
  id,
  unitId,
  title,
  description: "",
  headOfUnit: false,
  seats: 1,
  existsFrom: "2025-01-01",
  archivedOn: null,
  reportsToId: null,
  dottedTo: [] as string[],
  holders: [] as ReturnType<typeof holding>[],
  roles: [] as string[],
  deputies: [] as string[],
  ...extra,
});
const holding = (id: string, personId: string, at: { positionId?: string; roleId?: string }, from = "2025-01-01", to: string | null = null, share = 100) => ({
  id,
  personId,
  positionId: at.positionId ?? null,
  roleId: at.roleId ?? null,
  share,
  from,
  to,
});
const person = (id: string, name: string, extra: object = {}) => ({
  id,
  name,
  email: `${id}@kestrel.example`,
  phone: null,
  location: "Hamburg",
  kind: "external" as const,
  accountId: null,
  issuer: null,
  subject: null,
  leftOn: null,
  hasPhoto: false,
  ...extra,
});

// Carol leads the test lab, which stands in for the empty Head of Quality chair; she was a quality engineer before,
// in a position since archived, is a first aider in her own right, and the lab lead carries the auditor role.
const lead = position("lead", "lab", "Test lab lead", {
  headOfUnit: true,
  reportsToId: "hoq",
  holders: [holding("h1", "carol", { positionId: "lead" }, "2026-01-01")],
  roles: ["auditor"],
  deputies: ["eng"],
});
const data = {
  person: person("carol", "Carol Checker"),
  current: [holding("h1", "carol", { positionId: "lead" }, "2026-01-01"), holding("h2", "carol", { roleId: "aid" }, "2025-03-01", null, 5)],
  history: [
    holding("h0", "carol", { positionId: "old" }, "2025-01-01", "2026-01-01"),
    holding("h1", "carol", { positionId: "lead" }, "2026-01-01"),
    holding("h2", "carol", { roleId: "aid" }, "2025-03-01", null, 5),
  ],
  units: [unit("quality", "Quality", null), unit("lab", "Test lab", "quality")],
  positions: [
    position("hoq", "quality", "Head of Quality", { headOfUnit: true, deputies: ["lead"] }),
    lead,
    position("eng", "lab", "Quality engineer", { reportsToId: "lead", holders: [holding("h3", "tariq", { positionId: "eng" })] }),
    position("old", "lab", "Junior engineer", { archivedOn: "2026-01-01" }),
  ],
  people: [person("carol", "Carol Checker"), person("tariq", "Tariq Hassan")],
  roles: [
    { id: "aid", key: "first-aider", name: "First aider", description: "" },
    { id: "auditor", key: "auditor", name: "Auditor", description: "" },
  ],
};

describe("what the person page makes of the answers", () => {
  it("names holdings, roles, deputies and whom they stand in for", () => {
    const view = personView(data);
    expect(view.current.map((h) => [h.title, h.unitName ?? null, h.kind])).toEqual([
      ["Test lab lead", "Test lab", "position"],
      ["First aider", null, "role"],
    ]);
    expect(view.history.map((h) => h.title)).toEqual(["Test lab lead", "First aider", "Junior engineer"]);
    expect(view.roles).toEqual([
      { roleId: "auditor", name: "Auditor", through: "Test lab lead" },
      { roleId: "aid", name: "First aider" },
    ]);
    expect(view.deputies).toEqual([{ for: "Test lab lead", by: [{ positionId: "eng", title: "Quality engineer", holders: ["Tariq Hassan"] }] }]);
    expect(view.standsInFor).toEqual([{ positionId: "hoq", title: "Head of Quality", holders: [] }]);
  });

  it("leaves out what a person without holdings has none of", () => {
    const view = personView({ ...data, current: [], history: [] });
    expect(view).toEqual({ current: [], history: [], roles: [], deputies: [], standsInFor: [] });
  });
});

function serve(role = "administrator") {
  fakeAPI({
    ...signedInAs({ ...ADMIN, role }),
    "GET /people": () => ({ body: { asOf: "2026-10-31", people: data.people } }),
    "GET /people/carol": () => ({ body: { asOf: "2026-10-31", person: data.person, holdings: data.current } }),
    "GET /holdings": () => ({ body: { holdings: data.history } }),
    "GET /people/carol/chain": () => ({
      body: {
        asOf: "2026-10-31",
        positions: [
          { positionId: "hoq", title: "Head of Quality", unitId: "quality", from: "lead", kind: "solid", depth: 1, holders: [] },
          {
            positionId: "ops",
            title: "Head of Operations",
            unitId: "ops",
            from: "hoq",
            kind: "solid",
            depth: 2,
            holders: [{ personId: "gabe", name: "Gabriel Santos", share: 100 }],
          },
        ],
      },
    }),
    "GET /people/carol/reports": () => ({
      body: {
        asOf: "2026-10-31",
        positions: [
          {
            positionId: "eng",
            title: "Quality engineer",
            unitId: "lab",
            from: "lead",
            kind: "dotted",
            depth: 1,
            holders: [{ personId: "tariq", name: "Tariq Hassan", share: 100 }],
          },
        ],
      },
    }),
    "GET /units": () => ({ body: { asOf: "2026-10-31", units: data.units } }),
    "GET /positions": () => ({ body: { asOf: "2026-10-31", positions: data.positions.filter((p) => p.id !== "old") } }),
    "GET /positions/old": () => ({ body: { asOf: "2025-01-01", position: data.positions.find((p) => p.id === "old") } }),
    "GET /roles": () => ({ body: { roles: data.roles } }),
    "GET /responsibilities": (_, url) => ({
      body: {
        asOf: "2026-10-31",
        responsibilities: url.includes("personId=carol")
          ? [
              {
                processId: "complaint",
                processKey: "complaint-handling",
                processName: "Customer complaint",
                version: 1,
                stepKey: "cause",
                stepName: "Find the root cause",
                stepKind: "step",
                kind: "accountable",
                positionId: "hoq",
                people: [{ personId: "carol", name: "Carol Checker", share: 100, positionId: "lead", deputyFor: "hoq" }],
              },
              {
                processId: "run",
                processKey: "production-run",
                processName: "Production run",
                version: 1,
                stepKey: "test",
                stepName: "Test the batch",
                stepKind: "step",
                kind: "accountable",
                positionId: "lead",
                people: [{ personId: "carol", name: "Carol Checker", share: 100, positionId: "lead", deputyFor: null }],
              },
            ]
          : [],
      },
    }),
    "GET /people/carol/audit": () => ({
      body: {
        events: [{ id: "e1", at: "2026-01-01T09:00:00Z", actorId: "a", actorName: "Ada Admin", table: "holding", operation: "insert", before: {}, after: {} }],
        next: null,
      },
    }),
  });
}

describe("the people pages", () => {
  it("list people, narrow them and lead to each one's page", async () => {
    serve();
    const user = userEvent.setup();
    renderApp("/people");
    const list = await screen.findByRole("list", { name: "People" });
    expect(within(list).getByText("tariq@kestrel.example · Hamburg")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Find by name or address"), "carol");
    await user.click(screen.getByLabelText("Those who have left"));
    await waitFor(() => {
      const urls = vi.mocked(globalThis.fetch).mock.calls.map(([input]) => (input instanceof Request ? input.url : String(input)));
      expect(urls.some((u) => u.includes("q=carol") && u.includes("left=true"))).toBe(true);
    });
    await user.click(within(screen.getByRole("list", { name: "People" })).getByRole("link", { name: "Carol Checker" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Carol Checker" })).toBeInTheDocument();
  });

  it("show everything about a person, with the changes for editors", async () => {
    serve();
    renderApp("/people/carol");
    expect(await screen.findByRole("heading", { level: 1, name: "Carol Checker" })).toBeInTheDocument();
    const section = (name: string) => screen.getByRole("region", { name });
    expect(within(section("Holds")).getByText("Test lab lead")).toBeInTheDocument();
    expect(within(section("Holds")).getByText(/5 %/)).toBeInTheDocument();
    expect(within(section("Roles")).getByText("through Test lab lead", { exact: false })).toBeInTheDocument();
    expect(within(section("Reports to")).getByText("Vacant")).toBeInTheDocument();
    expect(within(section("Reports to")).getByRole("link", { name: "Gabriel Santos" })).toBeInTheDocument();
    expect(within(section("Direct reports")).getByText(/Dotted line/)).toBeInTheDocument();
    expect(within(section("Deputies")).getByText("For Test lab lead, in order:")).toBeInTheDocument();
    expect(within(section("Stands in for")).getByText("Head of Quality")).toBeInTheDocument();
    expect(within(section("Positions and roles over time")).getByText("2025-01-01 until 2026-01-01")).toBeInTheDocument();
    expect(within(section("Positions and roles over time")).getByText("Junior engineer")).toBeInTheDocument();
    expect(await screen.findByText("Holding added")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Show on the org chart" })).toHaveAttribute("href", "/organisation?focus=person%3Acarol");
    const duties = await screen.findByRole("region", { name: "Responsible for" });
    expect(within(duties).getByText("Accountable")).toBeInTheDocument();
    expect(within(duties).getByRole("link", { name: "Find the root cause" })).toHaveAttribute("href", "/processes/complaint");
    expect(within(duties).getByText(/standing in for/)).toBeInTheDocument();
    expect(within(duties).getByRole("link", { name: "Head of Quality" })).toHaveAttribute("href", "/positions/hoq");
    expect(within(section("Roles")).getAllByRole("link")[0]).toHaveAttribute("href", expect.stringMatching(/^\/roles\//));
  });

  it("say so where a person has nothing to show", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /people/carol": () => ({ body: { asOf: "2026-10-31", person: data.person, holdings: [] } }),
      "GET /holdings": () => ({ body: { holdings: [] } }),
      "GET /people/carol/chain": () => ({ body: { asOf: "2026-10-31", positions: [] } }),
      "GET /people/carol/reports": () => ({ body: { asOf: "2026-10-31", positions: [] } }),
      "GET /units": () => ({ body: { asOf: "2026-10-31", units: [] } }),
      "GET /positions": () => ({ body: { asOf: "2026-10-31", positions: [] } }),
      "GET /people": () => ({ body: { asOf: "2026-10-31", people: [] } }),
      "GET /roles": () => ({ body: { roles: [] } }),
      "GET /people/carol/audit": () => ({ body: { events: [], next: null } }),
    });
    renderApp("/people/carol");
    expect(await screen.findByText("Holds nothing on this day.")).toBeInTheDocument();
    for (const empty of ["No roles on this day.", "Reports to nobody on this day.", "Has never held a position or a role.", "Stands in for nobody."]) {
      expect(screen.getByText(empty)).toBeInTheDocument();
    }
    expect(await screen.findByText("No changes recorded.")).toBeInTheDocument();
  });

  it("read the person as of another day, and keep the changes from members", async () => {
    serve("member");
    renderApp("/people/carol?asOf=2026-03-31");
    expect(await screen.findByRole("heading", { level: 1, name: "Carol Checker" })).toBeInTheDocument();
    expect(screen.getByLabelText("As of")).toHaveValue("2026-03-31");
    fireEvent.change(screen.getByLabelText("As of"), { target: { value: "2025-06-01" } });
    await waitFor(() => {
      const urls = vi.mocked(globalThis.fetch).mock.calls.map(([input]) => (input instanceof Request ? input.url : String(input)));
      expect(urls.some((u) => u.includes("/people/carol?asOf=2025-06-01"))).toBe(true);
    });
    expect(screen.queryByRole("region", { name: "Changes" })).not.toBeInTheDocument();
  });

  it("say when nothing is there to show", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /people": () => ({ body: { asOf: "2026-10-31", people: [] } }),
    });
    renderApp("/people");
    expect(await screen.findByText("Nobody matches.")).toBeInTheDocument();
    cleanup();
    renderApp("/people/nobody");
    expect(await screen.findByText(/could not be read/)).toBeInTheDocument();
  });
});
