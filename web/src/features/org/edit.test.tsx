import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
const holding = (id: string, personId: string, positionId: string, from = "2025-01-01", to: string | null = null) => ({
  id,
  personId,
  positionId,
  roleId: null,
  share: 100,
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
  lines: [],
  ...extra,
});
const person = (id: string, name: string, extra: object = {}) => ({
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
  ...extra,
});

const units = [unit("ops", "Operations", null), unit("quality", "Quality", "ops"), unit("lab", "Test lab", "quality"), unit("sales", "Sales", "ops")];
const analyst = position("analyst", "quality", "Quality analyst", { seats: 2, holders: [holding("h-ana", "ana", "analyst")] });
const hoq = position("hoq", "quality", "Head of Quality", { headOfUnit: true });
const role = { id: "aud", key: "auditor", name: "Auditor", description: "Checks the books.", positions: [], holders: [] };

type Write = { method: string; path: string; body: unknown };

function serve(account: object = ADMIN, extra: Record<string, unknown> = {}) {
  const writes: Write[] = [];
  const record = (method: string, path: string, answer: object) => (body: unknown) => {
    writes.push({ method, path, body });
    return answer;
  };
  fakeAPI({
    ...signedInAs(account),
    "GET /units/quality": { body: { asOf: "2026-10-31", unit: { ...units[1], placements: [] } } },
    "GET /units": { body: { asOf: "2026-10-31", units } },
    "GET /unit-types": {
      body: {
        unitTypes: [
          { id: "dept", name: "Department", icon: "department", colour: "#3b8c6e", rank: 2, units: 2 },
          { id: "team", name: "Team", icon: "team", colour: "#3b8c6e", rank: 3, units: 0 },
        ],
      },
    },
    "GET /positions": (_, url) => ({ body: { asOf: "2026-10-31", positions: url.includes("unitId=quality") ? [hoq, analyst] : [hoq, analyst] } }),
    "GET /units/quality/members": { body: { asOf: "2026-10-31", members: [] } },
    "GET /positions/analyst": { body: { asOf: "2026-10-31", position: analyst } },
    "GET /holdings": { body: { holdings: [holding("h-ana", "ana", "analyst"), holding("h-new", "bo", "analyst", "2099-01-01")] } },
    "GET /positions/analyst/chain": { body: { asOf: "2026-10-31", positions: [] } },
    "GET /positions/analyst/reports": { body: { asOf: "2026-10-31", positions: [] } },
    "GET /people": (_, url) => ({
      body: { asOf: "2026-10-31", people: url.includes("left=true") ? [] : [person("ana", "Ana Analyst"), person("bo", "Bo Builder")] },
    }),
    "GET /roles": { body: { roles: [{ id: "aud", key: "auditor", name: "Auditor", description: "Checks the books." }] } },
    "GET /roles/aud": { body: { asOf: "2026-10-31", role } },
    "GET /people/ana": { body: { asOf: "2026-10-31", person: person("ana", "Ana Analyst", { email: "ana@kestrel.test" }), holdings: [] } },
    "GET /people/ana/chain": { body: { asOf: "2026-10-31", positions: [] } },
    "GET /people/ana/reports": { body: { asOf: "2026-10-31", positions: [] } },
    "PATCH /units/quality": record("PATCH", "/units/quality", { body: { unit: units[1] } }),
    "POST /units": record("POST", "/units", { status: 201, body: { unit: unit("new", "Field lab", "quality") } }),
    "POST /units/quality/move": record("POST", "/units/quality/move", { body: { unit: units[1] } }),
    "POST /units/quality/archive": record("POST", "/units/quality/archive", { body: { unit: units[1] } }),
    "POST /positions": record("POST", "/positions", { status: 201, body: { position: position("p", "quality", "Inspector") } }),
    "PATCH /positions/analyst": record("PATCH", "/positions/analyst", { body: { position: analyst } }),
    "PUT /positions/analyst/reports-to": record("PUT", "/positions/analyst/reports-to", { body: { position: analyst } }),
    "POST /positions/analyst/archive": record("POST", "/positions/analyst/archive", { body: { position: analyst } }),
    "POST /holdings": record("POST", "/holdings", { status: 201, body: { holding: holding("h", "bo", "analyst") } }),
    "POST /holdings/h-ana/end": record("POST", "/holdings/h-ana/end", { body: { holding: holding("h-ana", "ana", "analyst") } }),
    "DELETE /holdings/h-new": record("DELETE", "/holdings/h-new", { status: 204 }),
    "PATCH /people/ana": record("PATCH", "/people/ana", { body: { person: person("ana", "Ana A.") } }),
    "POST /people/ana/leave": record("POST", "/people/ana/leave", { body: { person: person("ana", "Ana Analyst") } }),
    "POST /people": record("POST", "/people", { status: 201, body: { person: person("cy", "Cy New") } }),
    "GET /people/cy": { body: { asOf: "2026-10-31", person: person("cy", "Cy New"), holdings: [] } },
    "PATCH /roles/aud": record("PATCH", "/roles/aud", { body: { role } }),
    "DELETE /roles/aud": record("DELETE", "/roles/aud", { status: 204 }),
    "POST /roles": record("POST", "/roles", { status: 201, body: { role: { ...role, id: "new-role", name: "Approver" } } }),
    "GET /roles/new-role": { body: { asOf: "2026-10-31", role: { ...role, id: "new-role", name: "Approver" } } },
    ...extra,
  });
  return writes;
}

const open = async (name: string) => {
  await userEvent.click(await screen.findByText(name, { selector: "summary" }));
  return screen.getByText(name, { selector: "summary" }).closest("details") as HTMLElement;
};
const today = () => new Date().toISOString().slice(0, 10);

describe("changing a unit", () => {
  it("edits its details, adds a sub-unit and a position, moves and archives it", async () => {
    const writes = serve();
    renderApp("/units/quality");
    const details = await open("Name, type and description");
    await userEvent.clear(within(details).getByLabelText("Name"));
    await userEvent.type(within(details).getByLabelText("Name"), "Quality and safety");
    await userEvent.selectOptions(within(details).getByLabelText("Unit type"), "Team");
    await userEvent.click(within(details).getByRole("button", { name: "Save" }));
    expect(await within(details).findByRole("status")).toHaveTextContent("Saved.");
    expect(writes.at(-1)).toEqual({
      method: "PATCH",
      path: "/units/quality",
      body: { name: "Quality and safety", shortName: "", description: "", typeId: "team" },
    });

    const sub = await open("Add a sub-unit");
    await userEvent.type(within(sub).getByLabelText("Name"), "Field lab");
    await userEvent.click(within(sub).getByRole("button", { name: "Add" }));
    expect(await within(sub).findByRole("status")).toHaveTextContent("Field lab added.");
    expect(writes.at(-1)).toEqual({ method: "POST", path: "/units", body: { name: "Field lab", typeId: "dept", from: today(), parentId: "quality" } });

    const post = await open("Add a position");
    await userEvent.type(within(post).getByLabelText("Title"), "Inspector");
    await userEvent.clear(within(post).getByLabelText("Seats"));
    await userEvent.type(within(post).getByLabelText("Seats"), "3");
    await userEvent.click(within(post).getByLabelText("Heads the unit"));
    await userEvent.click(within(post).getByRole("button", { name: "Add" }));
    expect(await within(post).findByRole("status")).toHaveTextContent("Inspector added.");
    expect(writes.at(-1)).toEqual({
      method: "POST",
      path: "/positions",
      body: { unitId: "quality", title: "Inspector", seats: 3, headOfUnit: true, from: today() },
    });

    const move = await open("Move into another unit");
    const into = within(move).getByLabelText("Into");
    expect(
      within(into)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Choose a unit", "Operations", "Sales"]);
    await userEvent.selectOptions(into, "Sales");
    fireEvent.change(within(move).getByLabelText("From"), { target: { value: "2026-12-01" } });
    await userEvent.click(within(move).getByRole("button", { name: "Move" }));
    expect(await within(move).findByRole("status")).toHaveTextContent("Moved from 2026-12-01.");
    expect(writes.at(-1)).toEqual({ method: "POST", path: "/units/quality/move", body: { parentId: "sales", from: "2026-12-01" } });

    const archive = await open("Archive the unit");
    fireEvent.change(within(archive).getByLabelText("On"), { target: { value: "2027-01-01" } });
    await userEvent.click(within(archive).getByRole("button", { name: "Archive" }));
    expect(await within(archive).findByRole("status")).toHaveTextContent("Archived from 2027-01-01.");
  });

  it("shows a refusal by its field, and offers nothing to a member", async () => {
    serve(ADMIN, {
      "POST /units": {
        status: 422,
        body: { error: { code: "validation_failed", message: "Some fields need attention.", fields: { name: "Name the unit." } } },
      },
    });
    renderApp("/units/quality");
    const sub = await open("Add a sub-unit");
    await userEvent.type(within(sub).getByLabelText("Name"), " ");
    await userEvent.click(within(sub).getByRole("button", { name: "Add" }));
    expect(await within(sub).findByText("Name the unit.")).toBeVisible();
    cleanup();
    serve({ ...ADMIN, role: "member", ownRole: "member" });
    renderApp("/units/quality");
    expect(await screen.findByRole("heading", { level: 1, name: /Quality/ })).toBeVisible();
    expect(screen.queryByRole("region", { name: "Change" })).toBeNull();
  });
});

describe("changing a position", () => {
  it("edits its details and line, gives it, ends and removes holdings, and archives it", async () => {
    const writes = serve();
    renderApp("/positions/analyst");
    const details = await open("Title, seats and description");
    await userEvent.clear(within(details).getByLabelText("Seats"));
    await userEvent.type(within(details).getByLabelText("Seats"), "4");
    await userEvent.click(within(details).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(writes.at(-1)).toEqual({
        method: "PATCH",
        path: "/positions/analyst",
        body: { title: "Quality analyst", description: "", seats: 4, headOfUnit: false },
      }),
    );

    const line = await open("Reports to");
    expect(within(within(line).getByLabelText("Position it reports to")).queryByRole("option", { name: "Quality analyst" })).toBeNull();
    await userEvent.selectOptions(within(line).getByLabelText("Position it reports to"), "Head of Quality");
    await userEvent.click(within(line).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes.at(-1)).toEqual({ method: "PUT", path: "/positions/analyst/reports-to", body: { reportsToId: "hoq", from: today() } }));

    const give = await open("Give the position to somebody");
    await userEvent.selectOptions(within(give).getByLabelText("Person"), "Bo Builder");
    await userEvent.clear(within(give).getByLabelText("Share of the week, in percent"));
    await userEvent.type(within(give).getByLabelText("Share of the week, in percent"), "40");
    fireEvent.change(within(give).getByLabelText("Until (optional)"), { target: { value: "2027-06-01" } });
    await userEvent.click(within(give).getByRole("button", { name: "Give" }));
    expect(await within(give).findByRole("status")).toHaveTextContent(`Bo Builder holds it from ${today()}.`);
    expect(writes.at(-1)).toEqual({
      method: "POST",
      path: "/holdings",
      body: { personId: "bo", positionId: "analyst", share: 40, from: today(), to: "2027-06-01" },
    });

    const holdings = await open("End or remove a holding");
    expect(within(holdings).queryByRole("button", { name: "End Bo Builder's holding" })).toBeNull();
    fireEvent.change(within(holdings).getByLabelText("On"), { target: { value: "2026-12-31" } });
    await userEvent.click(within(holdings).getByRole("button", { name: "End Ana Analyst's holding" }));
    expect(await within(holdings).findByRole("status")).toHaveTextContent("Ana Analyst's holding ends on 2026-12-31.");
    expect(writes.at(-1)).toEqual({ method: "POST", path: "/holdings/h-ana/end", body: { on: "2026-12-31" } });
    await userEvent.click(within(holdings).getByRole("button", { name: "Remove Bo Builder's holding as if it never was" }));
    await waitFor(() => expect(writes.at(-1)).toEqual({ method: "DELETE", path: "/holdings/h-new", body: undefined }));

    const archive = await open("Archive the position");
    await userEvent.click(within(archive).getByRole("button", { name: "Archive" }));
    await waitFor(() => expect(writes.at(-1)).toEqual({ method: "POST", path: "/positions/analyst/archive", body: { on: today() } }));
  });
});

describe("changing people and roles", () => {
  it("edits a person's details and marks them gone", async () => {
    const writes = serve();
    renderApp("/people/ana");
    const details = await open("Name and contact");
    expect(within(details).getByLabelText("Address (optional)")).toHaveValue("ana@kestrel.test");
    await userEvent.type(within(details).getByLabelText("Location (optional)"), "Leeds");
    await userEvent.click(within(details).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(writes.at(-1)).toEqual({
        method: "PATCH",
        path: "/people/ana",
        body: { name: "Ana Analyst", email: "ana@kestrel.test", phone: "", location: "Leeds" },
      }),
    );
    const leave = await open("Mark as gone");
    await userEvent.click(within(leave).getByRole("button", { name: "Mark as gone" }));
    expect(await within(leave).findByRole("status")).toHaveTextContent(`Gone from ${today()}.`);
  });

  it("adds a person from the list and opens their page", async () => {
    const writes = serve();
    const { router } = renderApp("/people");
    const add = await open("Add a person");
    await userEvent.type(within(add).getByLabelText("Name"), "Cy New");
    await userEvent.click(within(add).getByRole("button", { name: "Add" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/people/cy"));
    expect(writes.at(-1)).toEqual({ method: "POST", path: "/people", body: { name: "Cy New" } });
  });

  it("lists roles, adds one, edits and deletes it", async () => {
    const writes = serve();
    const { router } = renderApp("/roles");
    expect(await screen.findByRole("link", { name: "Auditor" })).toHaveAttribute("href", "/roles/aud");
    const add = await open("Add a role");
    await userEvent.type(within(add).getByLabelText("Name"), "Approver");
    await userEvent.type(within(add).getByLabelText(/Key/), "approver");
    await userEvent.click(within(add).getByRole("button", { name: "Add" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/roles/new-role"));
    expect(writes.at(-1)).toEqual({ method: "POST", path: "/roles", body: { name: "Approver", key: "approver" } });

    await router.navigate({ to: "/roles/$roleId", params: { roleId: "aud" } });
    const details = await open("Name and description");
    await userEvent.clear(within(details).getByLabelText("Description (optional)"));
    await userEvent.click(within(details).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes.at(-1)).toEqual({ method: "PATCH", path: "/roles/aud", body: { name: "Auditor", description: "" } }));
    const remove = await open("Delete the role");
    await userEvent.click(within(remove).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/roles"));
  });

  it("restores an archived unit and takes back that somebody left", async () => {
    const writes = serve(ADMIN, {
      "GET /units/quality": { body: { asOf: "2026-10-31", unit: { ...units[1], archivedOn: "2026-06-01", placements: [] } } },
      "POST /units/quality/restore": (body: unknown) => {
        writes.push({ method: "POST", path: "/units/quality/restore", body });
        return { body: { unit: units[1] } };
      },
      "GET /people/ana": { body: { asOf: "2026-10-31", person: person("ana", "Ana Analyst", { leftOn: "2026-06-01" }), holdings: [] } },
      "POST /people/ana/return": (body: unknown) => {
        writes.push({ method: "POST", path: "/people/ana/return", body });
        return { body: { person: person("ana", "Ana Analyst") } };
      },
    });
    const { router } = renderApp("/units/quality");
    const restore = await open("Restore");
    await userEvent.click(within(restore).getByRole("button", { name: "Restore" }));
    expect(await within(restore).findByRole("status")).toHaveTextContent("Restored.");
    await router.navigate({ to: "/people/$personId", params: { personId: "ana" } });
    const back = await open("Take back that they left");
    await userEvent.click(within(back).getByRole("button", { name: "Take back" }));
    expect(await within(back).findByRole("status")).toHaveTextContent("They are back.");
    expect(writes.map((w) => w.path)).toEqual(["/units/quality/restore", "/people/ana/return"]);
  });

  it("speaks German", async () => {
    applyLanguage("de");
    serve();
    renderApp("/units/quality");
    expect(await screen.findByRole("region", { name: "Ändern" })).toBeVisible();
    expect(screen.getByText("Untereinheit hinzufügen")).toBeVisible();
  });
});
