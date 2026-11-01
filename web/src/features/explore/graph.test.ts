import { describe, expect, it } from "vitest";
import type { Person, Position, Unit } from "@/api/org";
import { type Duty, explore, nodeId, parseNode, place, RELATIONS, type Relation } from "./graph";

const unit = (id: string, name: string, parentId: string | null) =>
  ({ id, name, parentId, typeId: "t", shortName: null, description: "", archivedOn: null }) as Unit;
const position = (id: string, unitId: string, title: string, extra: Partial<Position> = {}) =>
  ({
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
  }) as Position;
const holder = (personId: string, positionId: string) => ({
  id: `h-${personId}`,
  personId,
  positionId,
  roleId: null,
  share: 100,
  from: "2025-01-01",
  to: null,
});
const person = (id: string, name: string) => ({ id, name }) as Person;

const org = {
  units: [unit("ops", "Operations", null), unit("q", "Quality", "ops"), unit("lab", "Test lab", "q")],
  positions: [
    position("hops", "ops", "Head of Operations"),
    position("hoq", "q", "Head of Quality", { reportsToId: "hops", holders: [holder("carol", "hoq")], roles: ["aud"], deputies: ["lead"] }),
    position("lead", "lab", "Test lab lead", { reportsToId: "hoq", holders: [holder("dora", "lead")] }),
    position("ana", "q", "Quality analyst", { reportsToId: "hoq", dottedTo: ["hops"], roles: ["aud"] }),
  ],
  people: [person("carol", "Carol Checker"), person("dora", "Dora Doer")],
  roles: [{ id: "aud", name: "Auditor" }],
};
const all = new Set<Relation>(RELATIONS);
const none = new Map<string, Duty[]>();
const labels = (g: ReturnType<typeof explore>) => g.nodes.map((n) => `${n.depth} ${n.label}`);
const lines = (g: ReturnType<typeof explore>) => {
  const name = new Map(g.nodes.map((n) => [n.id, n.label]));
  return g.edges.map((e) => `${name.get(e.source)} ${e.label} ${name.get(e.target)}`).sort();
};

describe("exploring around a node", () => {
  it("shows the start alone until it is expanded, then everything it is related to", () => {
    const start = nodeId("position", "hoq");
    expect(labels(explore(org, start, new Set(), all, none))).toEqual(["0 Head of Quality"]);
    const g = explore(org, start, new Set([start]), all, none);
    expect(labels(g)).toEqual([
      "0 Head of Quality",
      "1 Head of Operations",
      "1 Carol Checker",
      "1 Auditor",
      "1 Test lab lead",
      "1 Quality",
      "1 Quality analyst",
    ]);
    expect(lines(g)).toEqual([
      "Carol Checker holds Head of Quality",
      "Head of Quality carries Auditor",
      "Head of Quality is in Quality",
      "Head of Quality reports to Head of Operations",
      "Quality analyst reports to Head of Quality",
      "Test lab lead reports to Head of Quality",
      "Test lab lead stands in for Head of Quality",
    ]);
    expect(g.nodes.every((n) => n.expanded === (n.id === start))).toBe(true);
  });

  it("goes further from every node expanded, keeping each node and line once", () => {
    const start = nodeId("position", "hoq");
    const g = explore(org, start, new Set([start, nodeId("role", "aud"), nodeId("unit", "q")]), all, none);
    expect(g.nodes.filter((n) => n.label === "Quality analyst")).toHaveLength(1);
    expect(lines(g)).toContain("Quality analyst carries Auditor");
    expect(lines(g)).toContain("Quality is part of Operations");
    expect(lines(g)).toContain("Test lab is part of Quality");
    expect(lines(g).filter((l) => l === "Quality analyst reports to Head of Quality")).toHaveLength(1);
    expect(g.nodes.find((n) => n.label === "Test lab")?.depth).toBe(2);
  });

  it("follows only the relations switched on", () => {
    const start = nodeId("position", "ana");
    const g = explore(org, start, new Set([start]), new Set<Relation>(["reporting"]), none);
    expect(lines(g)).toEqual(["Quality analyst reports by a dotted line to Head of Operations", "Quality analyst reports to Head of Quality"]);
    expect(explore(org, start, new Set([start]), new Set(), none).edges).toEqual([]);
  });

  it("leads from a person to what they hold, and to the steps they answer for", () => {
    const start = nodeId("person", "carol");
    const duty = {
      processId: "inv",
      processKey: "invoice",
      processName: "Supplier invoice",
      stepKey: "approve",
      stepName: "Approve the invoice",
      stepKind: "step",
      kind: "accountable",
      version: 1,
      people: [],
    } as Duty;
    const g = explore(org, start, new Set([start]), all, new Map([[start, [duty]]]));
    expect(lines(g)).toEqual(["Carol Checker holds Head of Quality", "Carol Checker is accountable for Approve the invoice"]);
    expect(g.nodes.find((n) => n.kind === "step")).toMatchObject({ id: "step:inv:approve", detail: "Supplier invoice" });
  });

  it("knows nothing of a node the organisation does not have", () => {
    expect(explore(org, "position:nowhere", new Set(["position:nowhere"]), all, none).nodes).toEqual([]);
    expect(explore(org, "nonsense", new Set(), all, none).nodes).toEqual([]);
  });
});

describe("reading and placing nodes", () => {
  it("reads a node from the address", () => {
    expect(parseNode("step:inv:approve")).toEqual({ kind: "step", id: "inv:approve" });
    expect(parseNode("unit:q")).toEqual({ kind: "unit", id: "q" });
    expect(parseNode("planet:x")).toBeUndefined();
    expect(parseNode(undefined)).toBeUndefined();
  });

  it("puts each distance in a column, centred on the start", () => {
    const g = explore(org, "position:hoq", new Set(["position:hoq"]), new Set<Relation>(["reporting"]), none);
    const at = place(g.nodes);
    expect(at["position:hoq"]).toEqual({ x: 0, y: 0 });
    const column = g.nodes.filter((n) => n.depth === 1).map((n) => at[n.id]);
    expect(column.map((p) => p?.x)).toEqual([300, 300, 300]);
    expect(column.map((p) => p?.y)).toEqual([-96, 0, 96]);
  });
});
