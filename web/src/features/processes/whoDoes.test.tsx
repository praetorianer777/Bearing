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

const people = (...list: [string, string, string | null][]) =>
  list.map(([personId, name, deputyFor]) => ({ personId, name, share: 100, positionId: "p", deputyFor }));

const organisation = {
  "GET /units": () => ({ body: { asOf: "2026-10-31", units: [] } }),
  "GET /positions": () => ({
    body: {
      asOf: "2026-10-31",
      positions: [
        {
          id: "hoq",
          unitId: "q",
          title: "Head of Quality",
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
  "GET /people": () => ({ body: { asOf: "2026-10-31", people: [{ id: "rita", name: "Rita Auditor" }] } }),
  "GET /roles": () => ({ body: { roles: [{ id: "aud", key: "auditor", name: "Auditor", description: "" }] } }),
};

describe("who does what", () => {
  it("finds steps by a term and names who does each, deputies and gaps included", async () => {
    fakeAPI({
      ...signedInAs(),
      ...organisation,
      "GET /processes": () => ({ body: { processes: [] } }),
      "GET /steps": (_, url) => ({
        body: {
          asOf: "2026-10-31",
          steps: url.includes("q=cause")
            ? [
                {
                  processId: "complaint",
                  processKey: "complaint-handling",
                  processName: "Customer complaint",
                  version: 1,
                  key: "cause",
                  name: "Find the root cause",
                  kind: "step",
                  responsibilities: [
                    { kind: "accountable", positionId: "hoq", people: people(["carol", "Carol Checker", "hoq"]) },
                    { kind: "responsible", roleId: "aud", people: people(["rita", "Rita Auditor", null], ["dora", "Dora Doer", null]) },
                    { kind: "informed", positionId: "hoq", people: [] },
                  ],
                },
                {
                  processId: "complaint",
                  processKey: "complaint-handling",
                  processName: "Customer complaint",
                  version: 1,
                  key: "more",
                  name: "Cause more trouble",
                  kind: "step",
                  responsibilities: [],
                },
              ]
            : [],
        },
      }),
    });
    const user = userEvent.setup();
    const { router } = renderApp("/processes");
    await user.click(await screen.findByRole("link", { name: "Who does what" }));
    expect(await screen.findByText("Type two letters or more.")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Step or process"), "cause");
    const found = await screen.findByRole("list", { name: "Steps found" });
    const cause = within(found).getAllByRole("listitem")[0] as HTMLElement;
    expect(within(cause).getByRole("heading", { name: "Find the root cause" })).toBeInTheDocument();
    expect(within(cause).getByRole("link", { name: "Carol Checker" })).toHaveAttribute("href", "/people/carol");
    expect(within(cause).getByText("(standing in)")).toBeInTheDocument();
    expect(await within(cause).findByText("Auditor:")).toBeInTheDocument();
    expect(within(cause).getByRole("link", { name: "Dora Doer" })).toBeInTheDocument();
    expect(within(cause).getByText("Nobody on this day, not even a deputy")).toBeInTheDocument();
    expect(within(found).getByText("Nobody is named for this step.")).toBeInTheDocument();
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "cause" }));

    await user.clear(screen.getByLabelText("Step or process"));
    await user.type(screen.getByLabelText("Step or process"), "zzz");
    expect(await screen.findByText("No step of a published process matches.")).toBeInTheDocument();
  });

  it("shows a role with the positions carrying it, who has it and what it is responsible for", async () => {
    fakeAPI({
      ...signedInAs(),
      ...organisation,
      "GET /roles/aud": () => ({
        body: {
          asOf: "2026-10-31",
          role: {
            id: "aud",
            key: "auditor",
            name: "Auditor",
            description: "Checks the books.",
            positions: ["hoq"],
            holders: [
              { personId: "rita", holdingId: "h1", share: 100, through: null },
              { personId: "rita", holdingId: "h2", share: 100, through: "hoq" },
            ],
          },
        },
      }),
      "GET /responsibilities": () => ({
        body: {
          asOf: "2026-10-31",
          responsibilities: [
            {
              processId: "inv",
              processKey: "supplier-invoice",
              processName: "Supplier invoice",
              version: 1,
              stepKey: "approve",
              stepName: "Approve the invoice",
              stepKind: "step",
              kind: "responsible",
              roleId: "aud",
              people: people(["rita", "Rita Auditor", null]),
            },
          ],
        },
      }),
    });
    renderApp("/roles/aud");
    expect(await screen.findByRole("heading", { level: 1, name: "Auditor" })).toBeInTheDocument();
    expect(screen.getByText("Checks the books.")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Carried by" })).getByRole("link", { name: "Head of Quality" })).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Held by" })).getByText(/directly/i)).toBeInTheDocument();
    const duties = await screen.findByRole("region", { name: "Responsible for" });
    expect(within(duties).getByText("Responsible")).toBeInTheDocument();
    expect(within(duties).getByRole("link", { name: "Approve the invoice" })).toHaveAttribute("href", "/processes/inv");
  });

  it("says when a role cannot be read", async () => {
    fakeAPI({ ...signedInAs() });
    renderApp("/roles/nowhere");
    expect(await screen.findByText(/This role could not be read/)).toBeInTheDocument();
  });
});
