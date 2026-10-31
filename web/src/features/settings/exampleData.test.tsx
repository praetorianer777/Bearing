import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";
import { partStates } from "./ExampleData";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const part = (loaded: boolean, empty: boolean) => ({ loaded, empty });

describe("where the example data stands", () => {
  it("lets the processes in only with the example organisation or after it", () => {
    expect(partStates({ organisation: part(false, true), processes: part(false, true) })).toEqual({ organisation: "ready", processes: "ready" });
    expect(partStates({ organisation: part(true, false), processes: part(false, true) })).toEqual({ organisation: "loaded", processes: "ready" });
    expect(partStates({ organisation: part(false, false), processes: part(false, true) })).toEqual({
      organisation: "taken",
      processes: "needsOrganisation",
    });
    expect(partStates({ organisation: part(true, false), processes: part(false, false) })).toEqual({ organisation: "loaded", processes: "taken" });
    expect(partStates({ organisation: part(true, false), processes: part(true, false) })).toEqual({ organisation: "loaded", processes: "loaded" });
  });
});

describe("the example data page", () => {
  it("loads what may still go in, once", async () => {
    let state = { organisation: part(true, false), processes: part(false, true) };
    let loads = 0;
    fakeAPI({
      ...signedInAs(),
      "GET /demo": () => ({ body: state }),
      "POST /demo": () => {
        loads++;
        state = { organisation: part(true, false), processes: part(true, false) };
        return { status: 201, body: state };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/example-data");
    const parts = await screen.findByRole("list", { name: "Example data" });
    expect(within(parts).getByText("Loaded")).toBeInTheDocument();
    expect(within(parts).getByText("Ready to load")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Load example data" }));
    expect(await screen.findByText("All the example data is loaded.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load example data" })).not.toBeInTheDocument();
    expect(loads).toBe(1);
    expect(screen.getByRole("link", { name: "Example data" })).toHaveAttribute("aria-current", "page");
  });

  it("stays out of an installation with an organisation of its own", async () => {
    fakeAPI({ ...signedInAs(), "GET /demo": () => ({ body: { organisation: part(false, false), processes: part(false, true) } }) });
    renderApp("/settings/example-data");
    expect(await screen.findByText("This installation has its own")).toBeInTheDocument();
    expect(screen.getByText("Needs the example organisation")).toBeInTheDocument();
    expect(screen.getByText("None of the example data can go in here.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load example data" })).not.toBeInTheDocument();
  });

  it("offers the example data where the organisation is empty, to administrators", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /units": () => ({ body: { asOf: "2026-10-31", units: [] } }),
      "GET /positions": () => ({ body: { asOf: "2026-10-31", positions: [] } }),
      "GET /people": () => ({ body: { asOf: "2026-10-31", people: [] } }),
      "GET /demo": () => ({ body: { organisation: part(false, true), processes: part(false, true) } }),
    });
    renderApp("/organisation");
    expect(await screen.findByRole("button", { name: "Load example data" })).toBeInTheDocument();
  });

  it("offers nothing to an editor", async () => {
    fakeAPI({
      ...signedInAs({ ...ADMIN, role: "editor" }),
      "GET /units": () => ({ body: { asOf: "2026-10-31", units: [] } }),
      "GET /positions": () => ({ body: { asOf: "2026-10-31", positions: [] } }),
      "GET /people": () => ({ body: { asOf: "2026-10-31", people: [] } }),
    });
    renderApp("/organisation");
    expect(await screen.findByText(/No organisation yet/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load example data" })).not.toBeInTheDocument();
  });
});
