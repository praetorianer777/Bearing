import type { Person, Position, Unit } from "@/api/org";
import type { components } from "@/api/schema";
import { t } from "@/i18n";

export type Duty = components["schemas"]["DutyResponse"];
type RoleSummary = { id: string; name: string };

/** The kinds of relation the explorer follows, each switched on or off. */
export const RELATIONS = ["reporting", "holdings", "roles", "deputies", "units", "responsibilities"] as const;
export type Relation = (typeof RELATIONS)[number];

export type NodeKind = "unit" | "position" | "person" | "role" | "step";

export type ExploreNode = { id: string; kind: NodeKind; label: string; detail: string; depth: number; expanded: boolean };
/** A relation drawn from source to target, read "source label target", such as "Analyst reports to Head of Quality". */
export type ExploreEdge = { id: string; source: string; target: string; relation: Relation; label: string };

export type Organisation = { units: Unit[]; positions: Position[]; people: Person[]; roles: RoleSummary[] };

/** A node's id, as the address and the graph keep it: its kind and its id, a step's process and key. */
export const nodeId = (kind: NodeKind, id: string) => `${kind}:${id}`;

export function parseNode(raw?: string): { kind: NodeKind; id: string } | undefined {
  const at = (raw ?? "").indexOf(":");
  const kind = (raw ?? "").slice(0, at) as NodeKind;
  const id = (raw ?? "").slice(at + 1);
  return at > 0 && id && ["unit", "position", "person", "role", "step"].includes(kind) ? { kind, id } : undefined;
}

type Neighbour = { id: string; source: string; target: string; relation: Relation; label: string };

/**
 * The neighbourhood of a start node: the start, and around every node expanded the nodes it is related to by the
 * relations switched on, nearest first. Responsibilities are those read for the expanded nodes, by node id.
 */
export function explore(
  org: Organisation,
  start: string,
  expanded: ReadonlySet<string>,
  relations: ReadonlySet<Relation>,
  duties: ReadonlyMap<string, Duty[]>,
): { nodes: ExploreNode[]; edges: ExploreEdge[] } {
  const units = new Map(org.units.map((u) => [u.id, u]));
  const positions = new Map(org.positions.map((p) => [p.id, p]));
  const people = new Map(org.people.map((p) => [p.id, p]));
  const roles = new Map(org.roles.map((r) => [r.id, r]));
  const steps = new Map<string, Duty>();
  for (const list of duties.values()) for (const d of list) steps.set(`${d.processId}:${d.stepKey}`, d);

  const describe = (id: string): Omit<ExploreNode, "depth" | "expanded"> | undefined => {
    const ref = parseNode(id);
    if (!ref) return undefined;
    switch (ref.kind) {
      case "unit": {
        const u = units.get(ref.id);
        return u && { id, kind: "unit", label: u.name, detail: t.explore.kinds.unit };
      }
      case "position": {
        const p = positions.get(ref.id);
        return p && { id, kind: "position", label: p.title, detail: units.get(p.unitId)?.name ?? "" };
      }
      case "person": {
        const p = people.get(ref.id);
        return p && { id, kind: "person", label: p.name, detail: t.explore.kinds.person };
      }
      case "role": {
        const r = roles.get(ref.id);
        return r && { id, kind: "role", label: r.name, detail: t.explore.kinds.role };
      }
      default: {
        const d = steps.get(ref.id);
        return d && { id, kind: "step", label: d.stepName || t.process.unnamed, detail: d.processName };
      }
    }
  };

  const neighbours = (id: string): Neighbour[] => {
    const ref = parseNode(id);
    if (!ref) return [];
    const out: Neighbour[] = [];
    const add = (relation: Relation, source: string, target: string, label: string) => {
      if (relations.has(relation)) out.push({ id: `${source}|${relation}|${label}|${target}`, source, target, relation, label });
    };
    const self = id;
    if (ref.kind === "position") {
      const p = positions.get(ref.id);
      if (p) {
        if (p.reportsToId) add("reporting", self, nodeId("position", p.reportsToId), t.explore.lines.reportsTo);
        for (const d of p.dottedTo) add("reporting", self, nodeId("position", d), t.explore.lines.dottedTo);
        for (const h of p.holders) add("holdings", nodeId("person", h.personId), self, t.explore.lines.holds);
        for (const r of p.roles) add("roles", self, nodeId("role", r), t.explore.lines.carries);
        for (const d of p.deputies) add("deputies", nodeId("position", d), self, t.explore.lines.standsInFor);
        add("units", self, nodeId("unit", p.unitId), t.explore.lines.in);
      }
      for (const other of org.positions) {
        if (other.reportsToId === ref.id) add("reporting", nodeId("position", other.id), self, t.explore.lines.reportsTo);
        if (other.dottedTo.includes(ref.id)) add("reporting", nodeId("position", other.id), self, t.explore.lines.dottedTo);
        if (other.deputies.includes(ref.id)) add("deputies", self, nodeId("position", other.id), t.explore.lines.standsInFor);
      }
    }
    if (ref.kind === "person") {
      for (const p of org.positions) if (p.holders.some((h) => h.personId === ref.id)) add("holdings", self, nodeId("position", p.id), t.explore.lines.holds);
    }
    if (ref.kind === "unit") {
      const u = units.get(ref.id);
      if (u?.parentId) add("units", self, nodeId("unit", u.parentId), t.explore.lines.partOf);
      for (const child of org.units) if (child.parentId === ref.id) add("units", nodeId("unit", child.id), self, t.explore.lines.partOf);
      for (const p of org.positions) if (p.unitId === ref.id) add("units", nodeId("position", p.id), self, t.explore.lines.in);
    }
    if (ref.kind === "role") {
      for (const p of org.positions) if (p.roles.includes(ref.id)) add("roles", nodeId("position", p.id), self, t.explore.lines.carries);
    }
    for (const d of duties.get(id) ?? []) add("responsibilities", self, nodeId("step", `${d.processId}:${d.stepKey}`), t.explore.raci[d.kind]);
    return out;
  };

  const nodes = new Map<string, ExploreNode>();
  const edges = new Map<string, ExploreEdge>();
  const first = describe(start);
  if (!first) return { nodes: [], edges: [] };
  nodes.set(start, { ...first, depth: 0, expanded: expanded.has(start) });
  for (let queue = [start]; queue.length > 0; ) {
    const id = queue.shift() as string;
    if (!expanded.has(id)) continue;
    const depth = nodes.get(id)?.depth ?? 0;
    for (const n of neighbours(id)) {
      const other = n.source === id ? n.target : n.source;
      if (!nodes.has(other)) {
        const known = describe(other);
        if (!known) continue;
        nodes.set(other, { ...known, depth: depth + 1, expanded: expanded.has(other) });
        queue.push(other);
      }
      edges.set(n.id, n);
    }
  }
  return { nodes: [...nodes.values()], edges: [...edges.values()] };
}

/** Columns by distance from the start, each column's nodes one under the other, centred on the start. */
export function place(nodes: ExploreNode[]): Record<string, { x: number; y: number }> {
  const columns = new Map<number, ExploreNode[]>();
  for (const n of nodes) columns.set(n.depth, [...(columns.get(n.depth) ?? []), n]);
  const out: Record<string, { x: number; y: number }> = {};
  for (const [depth, column] of columns) {
    column.forEach((n, i) => {
      out[n.id] = { x: depth * 300, y: (i - (column.length - 1) / 2) * 96 };
    });
  }
  return out;
}
