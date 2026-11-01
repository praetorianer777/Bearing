import type { FlowArrow, FlowNode, RACI } from "@/api/processes";
import type { Flow } from "./editor";
import { inFlowOrder, type PartyKey, partyOf, RACIS } from "./raci";

export type Mark = "added" | "removed" | "changed";

export type NodeField = "name" | "kind" | "description" | "subprocess";
export type ArrowField = "ends" | "label";
export type ResponsibilityChange = { kind: RACI; party: PartyKey; mark: "added" | "removed" };

export type NodeDiff = { key: string; name: string; kind: FlowNode["kind"]; mark: Mark; fields: NodeField[]; responsibilities: ResponsibilityChange[] };
export type ArrowDiff = { key: string; from: string; to: string; label: string; mark: Mark; fields: ArrowField[] };

/**
 * What changed from one version of a flow to another. Nodes and arrows are the same across versions by key, as each
 * version is copied from the one before; places and routes are left out, as they change how it is drawn, not what it says.
 */
export type Diff = {
  nodes: NodeDiff[];
  arrows: ArrowDiff[];
  /** The flow after, with what was removed put back where it stood before, to show both on one canvas. */
  merged: Flow;
  marks: Map<string, Mark>;
};

const responsibilityKey = (kind: RACI, party: PartyKey) => `${kind}|${party}`;

function nodeChanges(before: FlowNode, after: FlowNode): Pick<NodeDiff, "fields" | "responsibilities"> {
  const fields: NodeField[] = [];
  if (before.name !== after.name) fields.push("name");
  if (before.kind !== after.kind) fields.push("kind");
  if (before.description !== after.description) fields.push("description");
  if ((before.subprocessId ?? null) !== (after.subprocessId ?? null)) fields.push("subprocess");
  const had = new Set(before.responsibilities.map((r) => responsibilityKey(r.kind, partyOf(r))));
  const has = new Set(after.responsibilities.map((r) => responsibilityKey(r.kind, partyOf(r))));
  const responsibilities: ResponsibilityChange[] = [
    ...after.responsibilities
      .filter((r) => !had.has(responsibilityKey(r.kind, partyOf(r))))
      .map((r) => ({ kind: r.kind, party: partyOf(r), mark: "added" as const })),
    ...before.responsibilities
      .filter((r) => !has.has(responsibilityKey(r.kind, partyOf(r))))
      .map((r) => ({ kind: r.kind, party: partyOf(r), mark: "removed" as const })),
  ].sort((a, b) => RACIS.indexOf(a.kind) - RACIS.indexOf(b.kind) || a.mark.localeCompare(b.mark));
  return { fields, responsibilities };
}

function arrowFields(before: FlowArrow, after: FlowArrow): ArrowField[] {
  const fields: ArrowField[] = [];
  if (before.from !== after.from || before.to !== after.to) fields.push("ends");
  if (before.label !== after.label) fields.push("label");
  return fields;
}

/** What changed from before to after: nodes in the order the flow runs after, those removed where they ran before. */
export function diff(before: Flow, after: Flow): Diff {
  const was = new Map(before.nodes.map((n) => [n.key, n]));
  const is = new Map(after.nodes.map((n) => [n.key, n]));
  const merged: Flow = {
    nodes: [...after.nodes, ...before.nodes.filter((n) => !is.has(n.key))],
    // A removed arrow's route was laid out round the nodes as they stood before, so it is drawn anew between its ends.
    arrows: [...after.arrows, ...before.arrows.filter((a) => !after.arrows.some((b) => b.key === a.key)).map((a) => ({ ...a, route: [] }))],
  };
  const order = inFlowOrder(merged);
  const rank = new Map(order.map((key, i) => [key, i]));
  const byFlow = (a: { key: string }, b: { key: string }) => (rank.get(a.key) ?? 0) - (rank.get(b.key) ?? 0);

  const nodes: NodeDiff[] = [];
  for (const n of merged.nodes) {
    const old = was.get(n.key);
    const now = is.get(n.key);
    if (!old) nodes.push({ key: n.key, name: n.name, kind: n.kind, mark: "added", fields: [], responsibilities: [] });
    else if (!now) nodes.push({ key: n.key, name: n.name, kind: n.kind, mark: "removed", fields: [], responsibilities: [] });
    else {
      const changes = nodeChanges(old, now);
      if (changes.fields.length > 0 || changes.responsibilities.length > 0)
        nodes.push({ key: n.key, name: now.name, kind: now.kind, mark: "changed", ...changes });
    }
  }
  nodes.sort(byFlow);

  const wasArrow = new Map(before.arrows.map((a) => [a.key, a]));
  const isArrow = new Map(after.arrows.map((a) => [a.key, a]));
  const arrows: ArrowDiff[] = [];
  for (const a of merged.arrows) {
    const old = wasArrow.get(a.key);
    const now = isArrow.get(a.key);
    const at = { key: a.key, from: a.from, to: a.to, label: a.label };
    if (!old) arrows.push({ ...at, mark: "added", fields: [] });
    else if (!now) arrows.push({ ...at, mark: "removed", fields: [] });
    else {
      const fields = arrowFields(old, now);
      if (fields.length > 0) arrows.push({ ...at, mark: "changed", fields });
    }
  }
  arrows.sort((a, b) => (rank.get(a.from) ?? 0) - (rank.get(b.from) ?? 0) || (rank.get(a.to) ?? 0) - (rank.get(b.to) ?? 0));

  const marks = new Map<string, Mark>([...nodes, ...arrows].map((d) => [d.key, d.mark]));
  return { nodes, arrows, merged, marks };
}
