import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";
import { keyFor } from "./ProcessesPage";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const node = (key: string, kind: string, name: string, extra: object = {}) => ({
  key,
  kind,
  name,
  description: "",
  x: 0,
  y: 0,
  subprocessId: null,
  responsibilities: [] as object[],
  ...extra,
});
const arrow = (key: string, from: string, to: string, label = "") => ({ key, from, to, label });

const listed = (id: string, name: string, area: string, latest: number | null, changed: boolean) => ({
  id,
  key: keyFor(name),
  name,
  area,
  description: "",
  ownerId: null,
  latest,
  publishedAt: latest ? "2026-10-01T09:00:00Z" : null,
  changed,
});

const invoice = {
  ...listed("inv", "Supplier invoice", "Finance", 1, false),
  ownerId: "hof",
  draft: {
    nodes: [
      node("in", "start", "Invoice arrives"),
      node("big", "decision", "Over the limit?", { y: 100 }),
      node("book", "step", "Book it", {
        y: 150,
        responsibilities: [
          { kind: "accountable", positionId: "hof" },
          { kind: "informed", unitId: "fin" },
        ],
      }),
      node("out", "end", "Paid", { y: 200 }),
    ],
    arrows: [arrow("a1", "in", "big"), arrow("a2", "big", "out", "No"), arrow("a3", "big", "book", "Yes"), arrow("a4", "book", "out")],
    revision: 4,
    savedAt: "2026-10-01T09:00:00Z",
    problems: [
      { code: "decision_ways", message: "A decision needs two ways out or more.", node: "big", warning: false },
      { code: "no_responsible", message: "Nobody is responsible for this step.", node: "book", warning: true },
    ],
  },
  versions: [{ number: 1, publishedAt: "2026-10-01T09:00:00Z", publishedBy: null, publisherName: "Ada Admin", note: "First version." }],
};

function serve(role = "editor", answers: Record<string, unknown> = {}) {
  const saves: unknown[] = [];
  const publishes: unknown[] = [];
  fakeAPI({
    ...signedInAs({ ...ADMIN, role }),
    "GET /processes": () => ({
      body: {
        processes: [
          listed("inv", "Supplier invoice", "Finance", 1, false),
          listed("rel", "Product release", "", null, true),
          listed("ord", "Order to delivery", "Sales", 2, true),
        ],
      },
    }),
    "GET /processes/inv": () => ({ body: { process: invoice } }),
    "PUT /processes/inv/draft": (body) => {
      saves.push(body);
      const b = body as { nodes: unknown[]; arrows: unknown[] };
      const warnings = invoice.draft.problems.filter((p) => p.warning);
      return { body: { process: { ...invoice, changed: true, draft: { ...invoice.draft, ...b, revision: 5, problems: warnings } } } };
    },
    "POST /processes/inv/publish": (body) => {
      publishes.push(body);
      return { status: 201, body: { version: { number: 2, publishedAt: "2026-10-31T09:00:00Z", publishedBy: null, publisherName: "Ada", note: "x" } } };
    },
    "GET /processes/inv/versions/1": () => ({
      body: {
        version: {
          ...invoice.versions[0],
          nodes: [node("in", "start", "Invoice arrives"), node("old", "step", "Old check"), node("out", "end", "Paid")],
          arrows: [],
        },
      },
    }),
    "GET /units": () => ({
      body: { asOf: "2026-10-31", units: [{ id: "fin", name: "Finance", parentId: null, typeId: "d", shortName: null, description: "", archivedOn: null }] },
    }),
    "GET /roles": () => ({ body: { roles: [{ id: "clerk", key: "clerk", name: "Invoice clerk", description: "" }] } }),
    "GET /positions": () => ({
      body: {
        asOf: "2026-10-31",
        positions: [
          {
            id: "hof",
            unitId: "fin",
            title: "Head of Finance",
            description: "",
            headOfUnit: true,
            seats: 1,
            existsFrom: "2025-01-01",
            archivedOn: null,
            reportsToId: null,
            dottedTo: [],
            holders: [],
            roles: [],
            deputies: [],
          },
        ],
      },
    }),
    "GET /people": () => ({ body: { asOf: "2026-10-31", people: [] } }),
    ...(answers as object),
  });
  return { saves, publishes };
}

const nodeCard = (label: string) => screen.findByRole("article", { name: label });

describe("the processes page", () => {
  it("lists processes by area, those without one last, with where each stands", async () => {
    serve();
    renderApp("/processes");
    const finance = await screen.findByRole("region", { name: "Finance" });
    expect(within(finance).getByRole("link", { name: "Supplier invoice" })).toHaveAttribute("href", "/processes/inv");
    expect(within(finance).getByText("Version 1")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Sales" })).getByText(/unpublished changes/)).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Without an area" })).getByText("Draft only")).toBeInTheDocument();
    const areas = screen.getAllByRole("region").map((r) => r.querySelector("h2")?.textContent);
    expect(areas.slice(0, 3)).toEqual(["Finance", "Sales", "Without an area"]);
  });

  it("adds a process with a key from its name and opens it", async () => {
    let posted: unknown;
    serve("editor", {
      "POST /processes": (body: unknown) => {
        posted = body;
        return { status: 201, body: { process: { ...invoice, id: "inv" } } };
      },
    });
    const user = userEvent.setup();
    renderApp("/processes");
    const form = await screen.findByRole("region", { name: "Add a process" });
    await user.type(within(form).getByLabelText("Name"), "Größe prüfen!");
    expect(within(form).getByLabelText("Key")).toHaveValue("grosse-prufen");
    await user.type(within(form).getByLabelText("Area"), "Quality");
    await user.click(within(form).getByRole("button", { name: "Add" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Supplier invoice" })).toBeInTheDocument();
    expect(posted).toEqual({ name: "Größe prüfen!", key: "grosse-prufen", area: "Quality" });
  });

  it("offers members nothing to add, and the example data where there are no processes", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /processes": () => ({ body: { processes: [] } }),
      "GET /demo": () => ({ body: { organisation: { loaded: true, empty: false }, processes: { loaded: false, empty: true } } }),
    });
    renderApp("/processes");
    expect(await screen.findByText("No processes yet.")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Load example data" })).toBeInTheDocument();
    cleanup();
    serve("member");
    renderApp("/processes");
    expect(await screen.findByRole("link", { name: "Supplier invoice" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Add a process" })).not.toBeInTheDocument();
  });
});

describe("the process editor", () => {
  it("draws the draft, names its problems, and saves an edit from the revision read", async () => {
    const { saves, publishes } = serve();
    const user = userEvent.setup();
    renderApp("/processes/inv");
    expect(await screen.findByRole("heading", { level: 1, name: "Supplier invoice" })).toBeInTheDocument();
    expect(await nodeCard("Decision: Over the limit?")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "Head of Finance" })).toBeInTheDocument();
    const before = screen.getByRole("region", { name: "Before publishing" });
    expect(within(before).getByRole("button", { name: /Over the limit\?: A decision needs two ways/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish as the next version" })).toBeDisabled();

    await user.click(within(before).getByRole("button", { name: /Over the limit\?/ }));
    const name = within(screen.getByRole("region", { name: "Node" })).getByLabelText("Name");
    await user.clear(name);
    await user.type(name, "Over 10 000 EUR?");
    expect(await nodeCard("Decision: Over 10 000 EUR?")).toBeInTheDocument();
    expect(screen.getByText(/Unsaved changes/)).toBeInTheDocument();
    expect(screen.getByText("Save the draft to publish it.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save draft" }));
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]).toMatchObject({ revision: 4, arrows: invoice.draft.arrows });
    expect((saves[0] as { nodes: { name: string }[] }).nodes[1]?.name).toBe("Over 10 000 EUR?");
    expect(await screen.findByText("Nothing keeps the draft from being published.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Saved" })).toBeDisabled();

    await user.type(screen.getByLabelText("What changed"), "Named the limit");
    await user.click(screen.getByRole("button", { name: "Publish as the next version" }));
    await waitFor(() => expect(publishes).toEqual([{ note: "Named the limit" }]));

    await user.click(screen.getByRole("button", { name: "Undo" }));
    expect(await nodeCard("Decision: Over the limit?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Redo" }));
    expect(await nodeCard("Decision: Over 10 000 EUR?")).toBeInTheDocument();
  });

  it("adds nodes from the palette, removes what is picked, and arranges", async () => {
    serve();
    const user = userEvent.setup();
    renderApp("/processes/inv");
    await nodeCard("Start: Invoice arrives");
    await user.click(screen.getByRole("button", { name: "Step" }));
    expect(await nodeCard("Step: New step")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "z", ctrlKey: true });
    await waitFor(() => expect(screen.queryByRole("article", { name: "Step: New step" })).not.toBeInTheDocument());
    fireEvent.keyDown(window, { key: "z", ctrlKey: true, shiftKey: true });
    expect(await nodeCard("Step: New step")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Subprocess" }));
    fireEvent.click(await nodeCard("Subprocess: Subprocess"));
    const picked = screen.getByRole("region", { name: "Node" });
    await user.selectOptions(within(picked).getByLabelText("Stands for the process"), "ord");
    expect(await screen.findByRole("link", { name: "Order to delivery" })).toHaveAttribute("href", "/processes/ord");
    await user.click(within(picked).getByRole("button", { name: "Remove the node" }));
    await waitFor(() => expect(screen.queryByRole("article", { name: "Subprocess: Subprocess" })).not.toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "Arrange" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Save draft" })).toBeEnabled());
  });

  it("reads a published version without editing it, and leads back to the draft", async () => {
    serve();
    const user = userEvent.setup();
    const { router } = renderApp("/processes/inv");
    await nodeCard("Start: Invoice arrives");
    await user.click(within(screen.getByRole("region", { name: "Versions" })).getByRole("button", { name: "Version 1" }));
    expect(await nodeCard("Step: Old check")).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ version: 1 });
    expect(screen.queryByRole("button", { name: "Save draft" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Before publishing" })).not.toBeInTheDocument();
    expect(screen.getByText("Pick a node or an arrow to read it.")).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Showing"), "");
    expect(await nodeCard("Decision: Over the limit?")).toBeInTheDocument();
  });

  it("lets members read and not edit", async () => {
    serve("member");
    renderApp("/processes/inv");
    await nodeCard("Start: Invoice arrives");
    expect(screen.queryByRole("button", { name: "Save draft" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Step" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Publish" })).not.toBeInTheDocument();
  });

  it("changes the process's details", async () => {
    let patched: unknown;
    serve("editor", {
      "PATCH /processes/inv": (body: unknown) => {
        patched = body;
        return { body: { process: { ...invoice, name: "Invoices" } } };
      },
    });
    const user = userEvent.setup();
    renderApp("/processes/inv");
    await user.click(await screen.findByRole("button", { name: "Details" }));
    const name = screen.getByLabelText("Name", { selector: "input[name=name]" });
    await user.clear(name);
    await user.type(name, "Invoices");
    await user.selectOptions(screen.getByLabelText("Owner"), "");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Invoices" })).toBeInTheDocument();
    expect(patched).toMatchObject({ name: "Invoices", ownerId: "", key: "supplier-invoice", area: "Finance" });
  });

  it("says when the process cannot be read", async () => {
    fakeAPI(signedInAs());
    renderApp("/processes/nowhere");
    expect(await screen.findByText(/This process could not be read/)).toBeInTheDocument();
  });
});

describe("responsibilities on steps", () => {
  it("names who is accountable on the canvas, adds and removes parties, and warns without keeping the draft from publishing", async () => {
    const { saves } = serve();
    const user = userEvent.setup();
    renderApp("/processes/inv");
    const card = await nodeCard("Step: Book it");
    expect(await within(card).findByText("A · Head of Finance")).toBeInTheDocument();
    const warnings = screen.getByRole("region", { name: "Before publishing" });
    expect(within(warnings).getByText(/Worth a look/)).toBeInTheDocument();

    await user.click(within(warnings).getByRole("button", { name: /Book it: Nobody is responsible/ }));
    const picked = within(screen.getByRole("region", { name: "Node" })).getByRole("region", { name: "Responsibilities" });
    const named = within(picked).getByRole("list");
    expect(within(named).getByText("Head of Finance")).toBeInTheDocument();
    expect(within(named).getByText("Finance")).toBeInTheDocument();
    await user.selectOptions(within(picked).getByLabelText("How they stand to it"), "responsible");
    await user.selectOptions(within(picked).getByLabelText("Who"), "role:clerk");
    await user.click(within(picked).getByRole("button", { name: "Add" }));
    expect(within(named).getByText("Invoice clerk")).toBeInTheDocument();
    await user.click(within(picked).getByRole("button", { name: "Remove Informed: Finance" }));
    expect(within(named).queryByText("Finance")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save draft" }));
    await waitFor(() => expect(saves).toHaveLength(1));
    const book = (saves[0] as { nodes: { key: string; responsibilities: object[] }[] }).nodes.find((n) => n.key === "book");
    expect(book?.responsibilities).toEqual([
      { kind: "accountable", positionId: "hof" },
      { kind: "responsible", roleId: "clerk" },
    ]);
    expect(await screen.findByText("Nothing keeps the draft from being published.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish as the next version" })).toBeEnabled();
  });

  it("shows the RACI matrix and leads from a step back to the canvas", async () => {
    serve("member");
    const user = userEvent.setup();
    const { router } = renderApp("/processes/inv?view=matrix");
    const table = await screen.findByRole("table");
    expect(await within(table).findByRole("columnheader", { name: /Head of Finance/ })).toBeInTheDocument();
    const row = within(table).getByRole("row", { name: /Book it/ });
    expect(within(row).getByTitle("Accountable")).toHaveTextContent("A");
    expect(within(row).getByTitle("Informed")).toHaveTextContent("I");
    expect(within(row).getByText("incomplete")).toBeInTheDocument();
    await user.click(within(row).getByRole("button", { name: "Book it" }));
    expect(await nodeCard("Step: Book it")).toBeInTheDocument();
    expect(router.state.location.search).toEqual({});
    expect(within(within(screen.getByRole("region", { name: "Node" })).getByRole("list")).getByText("Head of Finance")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "RACI matrix" }));
    expect(await screen.findByRole("table")).toBeInTheDocument();
  });
});
