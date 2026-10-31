import { afterEach, describe, expect, it, vi } from "vitest";
import type { Person, Position, Unit } from "@/api/org";
import { CHART_PLACES_STORAGE_KEY } from "@/config";
import { CARD_WIDTH, type ChartInput, chartGraph, elkGraph } from "./graph";
import { layout, readPlaces, writePlaces } from "./layout";
import { parseFocus } from "./OrgChart";

const unit = (id: string, name: string, parentId: string | null): Unit => ({
  id,
  name,
  parentId,
  typeId: "t",
  shortName: null,
  description: "",
  archivedOn: null,
});

const position = (id: string, unitId: string, title: string, extra: Partial<Position> = {}): Position => ({
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

const held = (personId: string, positionId: string, share = 100) => ({
  id: `h-${personId}`,
  personId,
  positionId,
  roleId: null,
  share,
  from: "2025-01-01",
  to: null,
});

const person = (id: string, name: string): Person => ({
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

// A company with a division and a team inside it: the CEO, the head of the division and a planner in it, the team's
// lead and a vacant developer chair; the planner has a dotted line to the CEO, and the head stands in for the CEO.
const input: ChartInput = {
  units: [unit("co", "Company", null), unit("div", "Division", "co"), unit("team", "Team", "div")],
  positions: [
    position("ceo", "co", "CEO", { headOfUnit: true, holders: [held("cleo", "ceo")], deputies: ["head"] }),
    position("head", "div", "Head of Division", { headOfUnit: true, reportsToId: "ceo", holders: [held("hal", "head")] }),
    position("planner", "div", "Planner", { reportsToId: "head", dottedTo: ["ceo"], holders: [held("pia", "planner", 50)] }),
    position("lead", "team", "Team lead", { headOfUnit: true, reportsToId: "head", holders: [held("lee", "lead")] }),
    position("dev", "team", "Developer", { reportsToId: "lead", seats: 2 }),
  ],
  people: [person("cleo", "Cleo"), person("hal", "Hal"), person("pia", "Pia"), person("lee", "Lee")],
};

const none = new Set<string>();
const ids = (cards: { id: string }[]) => cards.map((c) => c.id).sort();
const lines = (ls: { id: string }[]) => ls.map((l) => l.id).sort();

describe("the chart's input", () => {
  it("draws every position as a card with its holders, and tells solid lines from dotted ones", () => {
    const { cards, lines: drawn } = chartGraph(input, { collapsed: none, deputies: false });
    expect(ids(cards)).toEqual(["ceo", "dev", "head", "lead", "planner"]);
    const planner = cards.find((c) => c.id === "planner");
    expect(planner?.holders).toEqual([{ personId: "pia", name: "Pia", share: 50 }]);
    expect(planner?.unitName).toBe("Division");
    expect(cards.find((c) => c.id === "dev")).toMatchObject({ vacant: true, seats: 2 });
    expect(lines(drawn)).toEqual(["dotted:ceo:planner", "solid:ceo:head", "solid:head:lead", "solid:head:planner", "solid:lead:dev"]);
  });

  it("folds a unit into its head, counting what it hides", () => {
    const { cards, lines: drawn } = chartGraph(input, { collapsed: new Set(["div"]), deputies: false });
    expect(ids(cards)).toEqual(["ceo", "head"]);
    expect(cards.find((c) => c.id === "head")).toMatchObject({ collapsible: true, collapsed: true, hidden: 3 });
    expect(lines(drawn)).toEqual(["solid:ceo:head"]);
    const open = chartGraph(input, { collapsed: none, deputies: false }).cards;
    expect(open.find((c) => c.id === "lead")).toMatchObject({ collapsible: true, collapsed: false, hidden: 0 });
    expect(open.find((c) => c.id === "planner")?.collapsible).toBe(false);
  });

  it("focuses on a unit with what is inside it", () => {
    expect(ids(chartGraph(input, { collapsed: none, deputies: false, focus: { kind: "unit", id: "team" } }).cards)).toEqual(["dev", "lead"]);
  });

  it("focuses on a person with the chain above them and those reporting to them", () => {
    const lee = chartGraph(input, { collapsed: none, deputies: false, focus: { kind: "person", id: "lee" } });
    expect(ids(lee.cards)).toEqual(["ceo", "dev", "head", "lead"]);
    const cleo = chartGraph(input, { collapsed: none, deputies: false, focus: { kind: "person", id: "cleo" } });
    expect(ids(cleo.cards)).toEqual(["ceo", "head", "planner"]);
  });

  it("draws deputies only when asked", () => {
    expect(lines(chartGraph(input, { collapsed: none, deputies: true }).lines)).toContain("deputy:ceo:head");
    expect(lines(chartGraph(input, { collapsed: none, deputies: false }).lines)).not.toContain("deputy:ceo:head");
  });

  it("lays out the tree of solid lines alone, each card as tall as its holders need", () => {
    const { cards, lines: drawn } = chartGraph(input, { collapsed: none, deputies: true });
    const graph = elkGraph(cards, drawn);
    expect(graph.layoutOptions?.["elk.direction"]).toBe("DOWN");
    expect(graph.edges?.map((e) => e.id).every((id) => id.startsWith("solid:"))).toBe(true);
    expect(graph.edges).toHaveLength(4);
    const dev = graph.children?.find((c) => c.id === "dev");
    expect(dev).toMatchObject({ width: CARD_WIDTH });
    expect(graph.children?.find((c) => c.id === "ceo")?.height).toBe(dev?.height);
  });

  it("keeps cards where a person moved them, and lays the rest out", async () => {
    const { cards, lines: drawn } = chartGraph(input, { collapsed: none, deputies: false });
    const places = await layout(cards, drawn, { dev: { x: 999, y: 7 } });
    expect(places.dev).toEqual({ x: 999, y: 7 });
    const y = (id: string) => places[id]?.y ?? Number.NaN;
    expect(y("ceo")).toBeLessThan(y("head"));
    expect(y("head")).toBeLessThan(y("lead"));
  });

  it("reads a focus from the address", () => {
    expect(parseFocus("unit:u1")).toEqual({ kind: "unit", id: "u1" });
    expect(parseFocus("person:p1")).toEqual({ kind: "person", id: "p1" });
    expect(parseFocus("role:r1")).toBeUndefined();
    expect(parseFocus("unit:")).toBeUndefined();
    expect(parseFocus(undefined)).toBeUndefined();
  });
});

describe("the places cards were moved to", () => {
  afterEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("are remembered in the browser and forgotten when reset", () => {
    writePlaces({ a: { x: 1, y: 2 } });
    expect(readPlaces()).toEqual({ a: { x: 1, y: 2 } });
    writePlaces({});
    expect(localStorage.getItem(CHART_PLACES_STORAGE_KEY)).toBeNull();
    localStorage.setItem(CHART_PLACES_STORAGE_KEY, "not json");
    expect(readPlaces()).toEqual({});
    localStorage.setItem(CHART_PLACES_STORAGE_KEY, "3");
    expect(readPlaces()).toEqual({});
  });

  it("are none in a browser that keeps nothing", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(readPlaces()).toEqual({});
    expect(() => writePlaces({ a: { x: 1, y: 2 } })).not.toThrow();
  });
});
