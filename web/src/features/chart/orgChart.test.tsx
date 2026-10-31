import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CHART_PLACES_STORAGE_KEY } from "@/config";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";

vi.mock("html-to-image", () => ({
  toPng: vi.fn(async () => "data:image/png;base64,AA=="),
  toSvg: vi.fn(async () => "data:image/svg+xml;charset=utf-8,%3Csvg%3E"),
}));

afterEach(() => {
  cleanup();
  applyLanguage("");
  localStorage.clear();
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
  dottedTo: [],
  holders: [] as object[],
  roles: [],
  deputies: [],
  ...extra,
});
const held = (personId: string, share = 100) => ({ id: `h-${personId}`, personId, positionId: "x", roleId: null, share, from: "2025-01-01", to: null });
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

const organisation = {
  units: [unit("co", "Company", null), unit("div", "Division", "co")],
  positions: [
    position("ceo", "co", "CEO", { headOfUnit: true, holders: [held("cleo")], deputies: ["head"] }),
    position("head", "div", "Head of Division", { headOfUnit: true, reportsToId: "ceo", holders: [held("hal")] }),
    position("planner", "div", "Planner", { reportsToId: "head", dottedTo: ["ceo"], seats: 2, holders: [held("pia", 50)] }),
    position("analyst", "div", "Analyst", { reportsToId: "head" }),
  ],
  people: [person("cleo", "Cleo"), person("hal", "Hal"), person("pia", "Pia")],
};

function serve(org = organisation) {
  fakeAPI({
    ...signedInAs(),
    "GET /units": () => ({ body: { asOf: "2026-10-31", units: org.units } }),
    "GET /positions": () => ({ body: { asOf: "2026-10-31", positions: org.positions } }),
    "GET /people": () => ({ body: { asOf: "2026-10-31", people: org.people } }),
  });
  const fetched = vi.mocked(globalThis.fetch);
  return {
    asOfs: () => fetched.mock.calls.map(([input]) => new URL(input instanceof Request ? input.url : String(input)).searchParams.get("asOf")).filter(Boolean),
  };
}

describe("the org chart", () => {
  it("draws positions with their holders, folds a unit and focuses on a person", async () => {
    serve();
    const user = userEvent.setup();
    renderApp("/organisation");
    const chart = await screen.findByLabelText("Organisation");
    expect(await within(chart).findByRole("article", { name: "Planner" })).toHaveTextContent("Pia");
    expect(within(chart).getByRole("article", { name: "Planner" })).toHaveTextContent("50 %");
    expect(within(chart).getByRole("article", { name: "Planner" })).toHaveTextContent("1 seat open");
    expect(within(chart).getByRole("article", { name: "Analyst" })).toHaveTextContent("Vacant");

    await user.click(within(chart).getByRole("button", { name: "Fold Division" }));
    await waitFor(() => expect(within(chart).queryByRole("article", { name: "Planner" })).not.toBeInTheDocument());
    expect(within(chart).getByRole("button", { name: "Unfold Division" })).toHaveTextContent("2 positions folded away");
    await user.click(within(chart).getByRole("button", { name: "Unfold Division" }));
    expect(await within(chart).findByRole("article", { name: "Planner" })).toBeInTheDocument();

    await user.click(within(chart).getByRole("button", { name: "Focus on Pia" }));
    await waitFor(() => expect(within(chart).queryByRole("article", { name: "Analyst" })).not.toBeInTheDocument());
    expect(within(chart).getByRole("article", { name: "CEO" })).toBeInTheDocument();
    expect(screen.getByLabelText("Focus on")).toHaveValue("person:pia");

    await user.selectOptions(screen.getByLabelText("Focus on"), "unit:div");
    await waitFor(() => expect(within(chart).queryByRole("article", { name: "CEO" })).not.toBeInTheDocument());
    await user.selectOptions(screen.getByLabelText("Focus on"), "");
    expect(await within(chart).findByRole("article", { name: "CEO" })).toBeInTheDocument();
  });

  it("reads the organisation as of the day chosen", async () => {
    const api = serve();
    renderApp("/organisation?asOf=2026-03-31");
    expect(await screen.findByRole("article", { name: "CEO" })).toBeInTheDocument();
    expect(screen.getByLabelText("As of")).toHaveValue("2026-03-31");
    fireEvent.change(screen.getByLabelText("As of"), { target: { value: "2025-06-01" } });
    await waitFor(() => expect(api.asOfs()).toContain("2025-06-01"));
    expect(api.asOfs()).toContain("2026-03-31");
  });

  it("shows deputies on demand, exports the chart and forgets moved cards", async () => {
    serve();
    localStorage.setItem(CHART_PLACES_STORAGE_KEY, JSON.stringify({ ceo: { x: 500, y: 0 } }));
    const user = userEvent.setup();
    renderApp("/organisation");
    expect(await screen.findByRole("article", { name: "CEO" })).toBeInTheDocument();
    await user.click(screen.getByLabelText("Show deputies"));
    expect(screen.getByLabelText("Show deputies")).toBeChecked();

    const { toPng, toSvg } = await import("html-to-image");
    const clicked = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    await user.click(screen.getByRole("button", { name: "Export PNG" }));
    await waitFor(() => expect(toPng).toHaveBeenCalled());
    await user.click(screen.getByRole("button", { name: "Export SVG" }));
    await waitFor(() => expect(toSvg).toHaveBeenCalled());
    expect(clicked).toHaveBeenCalledTimes(2);

    const reset = screen.getByRole("button", { name: "Reset the layout" });
    expect(reset).toBeEnabled();
    await user.click(reset);
    expect(reset).toBeDisabled();
    expect(localStorage.getItem(CHART_PLACES_STORAGE_KEY)).toBeNull();
  });

  it("says when there is no organisation yet", async () => {
    serve({ units: [], positions: [], people: [] });
    renderApp("/organisation");
    expect(await screen.findByText(/No organisation yet/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Export PNG" })).toBeDisabled();
  });

  it("says when the organisation cannot be read", async () => {
    fakeAPI({ ...signedInAs(), "GET /units": () => ({ status: 500, body: { error: { code: "internal", message: "x" } } }) });
    renderApp("/organisation");
    expect(await screen.findByText(/could not be read/)).toBeInTheDocument();
  });
});
