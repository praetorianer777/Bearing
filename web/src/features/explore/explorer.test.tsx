import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

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
  dottedTo: [],
  holders: [],
  roles: [],
  deputies: [],
  ...extra,
});

function serve() {
  const asked: string[] = [];
  fakeAPI({
    ...signedInAs(),
    "GET /units": {
      body: { asOf: "2026-11-02", units: [{ id: "q", name: "Quality", parentId: null, typeId: "t", shortName: null, description: "", archivedOn: null }] },
    },
    "GET /positions": {
      body: {
        asOf: "2026-11-02",
        positions: [
          position("hoq", "q", "Head of Quality", {
            holders: [{ id: "h", personId: "carol", positionId: "hoq", roleId: null, share: 100, from: "2025-01-01", to: null }],
          }),
          position("ana", "q", "Quality analyst", { reportsToId: "hoq" }),
        ],
      },
    },
    "GET /people": { body: { asOf: "2026-11-02", people: [{ id: "carol", name: "Carol Checker" }] } },
    "GET /roles": { body: { roles: [] } },
    "GET /responsibilities": (_, url) => {
      asked.push(url);
      return {
        body: {
          asOf: "2026-11-02",
          responsibilities: url.includes("personId=carol")
            ? [
                {
                  processId: "inv",
                  processKey: "invoice",
                  processName: "Supplier invoice",
                  stepKey: "approve",
                  stepName: "Approve the invoice",
                  stepKind: "step",
                  kind: "accountable",
                  version: 1,
                  people: [],
                },
              ]
            : [],
        },
      };
    },
  });
  return asked;
}

describe("the explorer", () => {
  it("starts from the node in the address, expands around it and lists the relations", async () => {
    const asked = serve();
    renderApp("/explore?node=position:hoq");
    const start = await screen.findByRole("article", { name: "Head of Quality" });
    expect(within(start).getByRole("button", { name: "Hide what is around Head of Quality" })).toHaveAttribute("aria-expanded", "true");
    const carol = await screen.findByRole("article", { name: "Carol Checker" });
    expect(screen.getByRole("article", { name: "Quality analyst" })).toBeVisible();
    expect(within(carol).getByRole("link", { name: "Carol Checker" })).toHaveAttribute("href", "/people/carol");
    await waitFor(() => expect(asked.some((u) => u.includes("positionId=hoq"))).toBe(true));

    await userEvent.click(within(carol).getByRole("button", { name: "Show what is around Carol Checker" }));
    const step = await screen.findByRole("article", { name: "Approve the invoice" });
    expect(within(step).getByRole("link", { name: "Approve the invoice" })).toHaveAttribute("href", "/processes/inv");
    expect(within(step).queryByRole("button")).toBeNull();

    await userEvent.click(screen.getByText(/relations as a list/));
    const list = screen.getByRole("list", { name: "Relations" });
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toContain("Carol Checker is accountable for Approve the invoice");
  });

  it("drops the lines of a relation switched off, and starts elsewhere on choosing", async () => {
    serve();
    const { router } = renderApp("/explore?node=position:hoq");
    expect(await screen.findByRole("article", { name: "Carol Checker" })).toBeVisible();
    await userEvent.click(screen.getByLabelText("Holdings"));
    await waitFor(() => expect(screen.queryByRole("article", { name: "Carol Checker" })).toBeNull());
    await userEvent.selectOptions(screen.getByLabelText("Start from"), "Carol Checker");
    await waitFor(() => expect(router.state.location.search).toEqual({ node: "person:carol" }));
    expect(await screen.findByRole("article", { name: "Head of Quality" })).toBeVisible();
  });

  it("asks where to start, and speaks German", async () => {
    serve();
    renderApp("/explore");
    expect(await screen.findByText("Choose a unit, position, person or role to start from.")).toBeVisible();
    cleanup();
    applyLanguage("de");
    serve();
    renderApp("/explore?node=unit:q");
    expect(await screen.findByRole("heading", { level: 1, name: "Erkunden" })).toBeVisible();
    expect(await screen.findByRole("article", { name: "Head of Quality" })).toBeVisible();
  });
});
