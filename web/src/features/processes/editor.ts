import type { FlowArrow, FlowNode, NodeKind } from "@/api/processes";

export type Flow = { nodes: FlowNode[]; arrows: FlowArrow[] };
export type Point = { x: number; y: number };

/**
 * A draft being edited: what it is now, the states undo goes back to and those redo goes forward to. A run of edits
 * to one field, or one drag, is one step back: the edits carry the same run, and settle ends it.
 */
export type History = { past: Flow[]; present: Flow; future: Flow[]; run?: string };

export type Edit =
  | { type: "add"; kind: NodeKind; key: string; name: string; at: Point }
  | { type: "move"; key: string; to: Point }
  | { type: "connect"; key: string; from: string; to: string }
  | { type: "node"; key: string; change: Partial<Pick<FlowNode, "name" | "description" | "subprocessId">> }
  | { type: "label"; key: string; label: string }
  | { type: "remove"; nodes: string[]; arrows: string[] }
  | { type: "arrange"; places: Record<string, Point> }
  | { type: "settle" }
  | { type: "undo" }
  | { type: "redo" };

// Undo reaches this far back; a longer session forgets its oldest steps.
const DEPTH = 100;

export const start = (flow: Flow): History => ({ past: [], present: flow, future: [] });

/** A key no node or arrow of the flow has yet. */
export function freshKey(prefix: string, flow: Flow, random = () => Math.random().toString(36).slice(2, 8)): string {
  const taken = new Set([...flow.nodes.map((n) => n.key), ...flow.arrows.map((a) => a.key)]);
  for (;;) {
    const key = `${prefix}-${random()}`;
    if (!taken.has(key)) return key;
  }
}

/** Where a new node of a kind goes near a point: moved down until it covers no node already there. */
export function freeSpot(flow: Flow, kind: NodeKind, near: Point, size: (k: NodeKind) => { width: number; height: number }): Point {
  const { width, height } = size(kind);
  const gap = 16;
  const at = { ...near };
  const covers = () =>
    flow.nodes.some((n) => {
      const other = size(n.kind);
      return at.x < n.x + other.width + gap && n.x < at.x + width + gap && at.y < n.y + other.height + gap && n.y < at.y + height + gap;
    });
  while (covers()) at.y += gap * 2;
  return at;
}

/** Whether an arrow may be drawn from one node to another: not into a start or out of an end, not twice, not back into itself. */
export function mayConnect(flow: Flow, from: string, to: string): boolean {
  const kind = (key: string) => flow.nodes.find((n) => n.key === key)?.kind;
  return (
    from !== to &&
    kind(from) !== undefined &&
    kind(to) !== undefined &&
    kind(from) !== "end" &&
    kind(to) !== "start" &&
    !flow.arrows.some((a) => a.from === from && a.to === to)
  );
}

function apply(flow: Flow, edit: Edit): Flow {
  switch (edit.type) {
    case "add":
      return {
        ...flow,
        nodes: [...flow.nodes, { key: edit.key, kind: edit.kind, name: edit.name, description: "", x: edit.at.x, y: edit.at.y, subprocessId: null }],
      };
    case "move":
      return { ...flow, nodes: flow.nodes.map((n) => (n.key === edit.key ? { ...n, x: edit.to.x, y: edit.to.y } : n)) };
    case "connect":
      if (!mayConnect(flow, edit.from, edit.to)) return flow;
      return { ...flow, arrows: [...flow.arrows, { key: edit.key, from: edit.from, to: edit.to, label: "" }] };
    case "node":
      return { ...flow, nodes: flow.nodes.map((n) => (n.key === edit.key ? { ...n, ...edit.change } : n)) };
    case "label":
      return { ...flow, arrows: flow.arrows.map((a) => (a.key === edit.key ? { ...a, label: edit.label } : a)) };
    case "remove": {
      const nodes = new Set(edit.nodes);
      const arrows = new Set(edit.arrows);
      if (nodes.size === 0 && arrows.size === 0) return flow;
      return {
        nodes: flow.nodes.filter((n) => !nodes.has(n.key)),
        arrows: flow.arrows.filter((a) => !arrows.has(a.key) && !nodes.has(a.from) && !nodes.has(a.to)),
      };
    }
    case "arrange":
      return { ...flow, nodes: flow.nodes.map((n) => (edit.places[n.key] ? { ...n, ...edit.places[n.key] } : n)) };
    default:
      return flow;
  }
}

/** The run an edit belongs to, if it is one of many small ones that undo as one. */
function runOf(edit: Edit): string | undefined {
  switch (edit.type) {
    case "move":
      return `move:${edit.key}`;
    case "node":
      return `node:${edit.key}:${Object.keys(edit.change).join(",")}`;
    case "label":
      return `label:${edit.key}`;
    default:
      return undefined;
  }
}

export function edit(history: History, e: Edit): History {
  switch (e.type) {
    case "undo": {
      const previous = history.past.at(-1);
      if (!previous) return history;
      return { past: history.past.slice(0, -1), present: previous, future: [history.present, ...history.future] };
    }
    case "redo": {
      const [next, ...rest] = history.future;
      if (!next) return history;
      return { past: [...history.past, history.present], present: next, future: rest };
    }
    case "settle":
      return history.run ? { ...history, run: undefined } : history;
  }
  const changed = apply(history.present, e);
  if (changed === history.present) return history;
  const run = runOf(e);
  if (run && run === history.run) return { ...history, present: changed, future: [] };
  return { past: [...history.past, history.present].slice(-DEPTH), present: changed, future: [], run };
}
