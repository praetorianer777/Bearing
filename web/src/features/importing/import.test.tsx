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

const found = (extra: object = {}) => ({
  id: "imp",
  status: "previewed",
  from: "2026-11-02",
  counts: { create: 2, update: 1, unchanged: 5 },
  changes: [
    { file: "units", row: 2, action: "create", name: "Field lab" },
    { file: "people", row: 3, action: "update", name: "Ana Analyst" },
    { file: "people", row: 4, action: "unchanged", name: "Bo Builder" },
  ],
  errors: [],
  result: null,
  createdAt: "2026-11-01T10:00:00Z",
  finishedAt: null,
  ...extra,
});

describe("the import and export page", () => {
  it("downloads each file as of a day", async () => {
    fakeAPI(signedInAs());
    renderApp("/import");
    const exportSection = await screen.findByRole("region", { name: "Export" });
    fireEvent.change(within(exportSection).getByLabelText("As of"), { target: { value: "2026-06-30" } });
    expect(within(exportSection).getByRole("link", { name: "Holdings" })).toHaveAttribute("href", "/api/v1/export/holdings?asOf=2026-06-30");
    expect(
      within(exportSection)
        .getAllByRole("link")
        .map((a) => a.textContent),
    ).toEqual(["Units", "Positions", "People", "Holdings"]);
  });

  it("previews the files chosen, then runs the import and follows it until it is done", async () => {
    const sent: unknown[] = [];
    let status = "queued";
    fakeAPI({
      ...signedInAs(),
      "POST /imports": (body) => {
        sent.push(body);
        return { status: 201, body: { import: found() } };
      },
      "POST /imports/imp/run": { status: 202, body: { import: found({ status: "queued" }) } },
      "GET /imports/imp": () => {
        const answer = { body: { import: found({ status }) } };
        status = "done";
        return answer;
      },
    });
    renderApp("/import");
    const section = await screen.findByRole("region", { name: "Import" });
    expect(within(section).getByRole("button", { name: "Preview" })).toBeDisabled();
    await userEvent.upload(within(section).getByLabelText("Units"), new File(["ref,name,type\nlab,Field lab,Team\n"], "units.csv", { type: "text/csv" }));
    await userEvent.upload(within(section).getByLabelText("People"), new File(["ref,name\n"], "people.csv", { type: "text/csv" }));
    fireEvent.change(within(section).getByLabelText("Changes take effect from"), { target: { value: "2026-12-01" } });
    await userEvent.click(within(section).getByRole("button", { name: "Preview" }));

    const preview = await screen.findByRole("region", { name: "Preview" });
    expect(sent).toEqual([{ units: "ref,name,type\nlab,Field lab,Team\n", people: "ref,name\n", from: "2026-12-01" }]);
    expect(within(preview).getByText("2 to add, 1 to change, 5 unchanged.")).toBeVisible();
    const rows = within(within(preview).getByRole("table", { name: "What the import changes" })).getAllByRole("row");
    expect(rows.map((r) => r.textContent)).toEqual(["FileRowChangeName", "Units2AddField lab", "People3ChangeAna Analyst"]);

    await userEvent.click(within(preview).getByRole("button", { name: "Run the import" }));
    expect(await within(preview).findByRole("status")).toHaveTextContent(/running|done/);
    await waitFor(() => expect(within(preview).getByRole("status")).toHaveTextContent("The import is done."), { timeout: 3000 });
    expect(within(preview).queryByRole("button", { name: "Run the import" })).toBeNull();
  });

  it("lists the rows it cannot take and offers no run", async () => {
    fakeAPI({
      ...signedInAs(),
      "POST /imports": {
        status: 201,
        body: {
          import: found({
            counts: { create: 0, update: 0, unchanged: 0 },
            changes: [],
            errors: [
              { file: "units", row: 1, message: 'There is no column "colour".' },
              { file: "holdings", row: 3, message: 'No person has the ref or address "nobody".' },
            ],
          }),
        },
      },
    });
    renderApp("/import");
    const section = await screen.findByRole("region", { name: "Import" });
    await userEvent.upload(within(section).getByLabelText("Holdings"), new File(["x"], "h.csv", { type: "text/csv" }));
    await userEvent.click(within(section).getByRole("button", { name: "Preview" }));
    const preview = await screen.findByRole("region", { name: "Preview" });
    expect(within(preview).getByText("2 rows cannot be imported")).toBeVisible();
    expect(
      within(within(preview).getByRole("list", { name: "Rows that cannot be imported" }))
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(['Units, row 1: There is no column "colour".', 'Holdings, row 3: No person has the ref or address "nobody".']);
    expect(within(preview).queryByRole("button", { name: "Run the import" })).toBeNull();
  });

  it("says where a run was refused", async () => {
    fakeAPI({
      ...signedInAs(),
      "POST /imports": { status: 201, body: { import: found() } },
      "POST /imports/imp/run": { status: 202, body: { import: found({ status: "queued" }) } },
      "GET /imports/imp": {
        body: {
          import: found({
            status: "failed",
            result: {
              plan: { changes: [], errors: [] },
              error: { file: "holdings", row: 2, message: "More people would hold the position then than it has seats." },
            },
          }),
        },
      },
    });
    renderApp("/import");
    const section = await screen.findByRole("region", { name: "Import" });
    await userEvent.upload(within(section).getByLabelText("Units"), new File(["x"], "u.csv", { type: "text/csv" }));
    await userEvent.click(within(section).getByRole("button", { name: "Preview" }));
    await userEvent.click(await screen.findByRole("button", { name: "Run the import" }));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "The import changed nothing: Holdings, row 2: More people would hold the position then than it has seats.",
      ),
    );
  });

  it("is for editors, and speaks German", async () => {
    fakeAPI(signedInAs({ ...ADMIN, role: "member", ownRole: "member" }));
    renderApp("/import");
    expect(await screen.findByText("Only editors import and export the organisation.")).toBeVisible();
    expect(screen.queryByRole("region", { name: "Export" })).toBeNull();
    cleanup();
    applyLanguage("de");
    fakeAPI(signedInAs());
    renderApp("/import");
    expect(await screen.findByRole("heading", { level: 1, name: "Import und Export" })).toBeVisible();
    expect(screen.getByRole("link", { name: "Besetzungen" })).toBeVisible();
  });
});
