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

describe("the unit types page", () => {
  it("adds, renames, recolours and removes unit types, and keeps those in use", async () => {
    let types = [{ id: "u1", name: "Division", icon: "division", colour: "#2f5fd0", rank: 0, units: 3 }];
    const patches: unknown[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /unit-types": () => ({ body: { unitTypes: types } }),
      "POST /unit-types": (body) => {
        const b = body as { name: string; icon: string; colour: string };
        if (!b.name.trim()) return { status: 422, body: { error: { code: "validation_failed", message: "x", fields: { name: "Give a name." } } } };
        types = [...types, { id: "u2", rank: 0, units: 0, ...b }];
        return { status: 201, body: { unitType: types[1] } };
      },
      "PATCH /unit-types/u2": (body) => {
        patches.push(body);
        if ((body as { name?: string }).name === "Division") {
          return {
            status: 409,
            body: { error: { code: "name_taken", message: "Another unit type has that name.", fields: { name: "Another unit type has that name." } } },
          };
        }
        types = types.map((t) => (t.id === "u2" ? { ...t, ...(body as object) } : t));
        return { body: { unitType: types[1] } };
      },
      "DELETE /unit-types/u2": () => {
        types = types.filter((t) => t.id !== "u2");
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/unit-types");
    const list = await screen.findByRole("list", { name: "Unit types" });
    expect(within(list).getByText("3 units of this type")).toBeInTheDocument();
    expect(within(list).getByRole("button", { name: "Remove: Division" })).toBeDisabled();

    const add = screen.getByRole("region", { name: "Add a unit type" });
    await user.click(within(add).getByRole("button", { name: "Add" }));
    expect(await within(add).findByText("Give a name.")).toBeInTheDocument();
    await user.type(within(add).getByLabelText("Name"), "Team");
    await user.selectOptions(within(add).getByLabelText("Icon"), "team");
    await user.click(within(add).getByRole("button", { name: "Add" }));
    expect(await within(list).findByLabelText("Name: Team")).toHaveValue("Team");

    const name = within(list).getByLabelText("Name: Team");
    await user.clear(name);
    await user.type(name, "Division{Enter}");
    expect(await within(list).findByText("Another unit type has that name.")).toBeInTheDocument();
    await user.clear(name);
    await user.type(name, "Squad");
    await user.tab();
    await waitFor(() => expect(patches.at(-1)).toEqual({ name: "Squad" }));
    await user.selectOptions(within(list).getByLabelText("Icon: Squad"), "project");
    await waitFor(() => expect(patches.at(-1)).toEqual({ icon: "project" }));

    await user.click(within(list).getByRole("button", { name: "Remove: Squad" }));
    await waitFor(() => expect(within(list).queryByDisplayValue("Squad")).not.toBeInTheDocument());
  });

  it("says when there are none yet", async () => {
    fakeAPI({ ...signedInAs(), "GET /unit-types": { body: { unitTypes: [] } } });
    renderApp("/settings/unit-types");
    expect(await screen.findByText("No unit types yet. Add one before the first unit.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Unit types" })).toHaveAttribute("aria-current", "page");
  });
});

describe("the demo organisation", () => {
  it("loads into an empty installation once, and says when it cannot", async () => {
    let state = { loaded: false, empty: true };
    let loads = 0;
    fakeAPI({
      ...signedInAs(),
      "GET /unit-types": () => ({ body: { unitTypes: [] } }),
      "GET /demo": () => ({ body: state }),
      "POST /demo": () => {
        loads++;
        state = { loaded: true, empty: false };
        return { status: 201, body: state };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/unit-types");
    const panel = await screen.findByRole("region", { name: "Demo organisation" });
    await user.click(await within(panel).findByRole("button", { name: "Load the demo organisation" }));
    expect(await within(panel).findByText("The demo organisation is loaded.")).toBeInTheDocument();
    expect(loads).toBe(1);
  });

  it("stays out of an installation with an organisation of its own", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /unit-types": () => ({ body: { unitTypes: [] } }),
      "GET /demo": () => ({ body: { loaded: false, empty: false } }),
    });
    renderApp("/settings/unit-types");
    const panel = await screen.findByRole("region", { name: "Demo organisation" });
    expect(within(panel).getByText(/has an organisation of its own/)).toBeInTheDocument();
    expect(within(panel).queryByRole("button")).not.toBeInTheDocument();
  });
});
