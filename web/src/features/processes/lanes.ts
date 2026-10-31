import type { FlowNode } from "@/api/processes";
import { NODE_SIZE } from "./arrange";
import type { Flow, Point } from "./editor";
import { inFlowOrder, type PartyKey, partyOf } from "./raci";

/** The lane of the steps nobody is responsible or accountable for. */
export const NOBODY = "nobody";
export type LaneKey = PartyKey | typeof NOBODY;

/** A lane as laid out: whose it is, where it runs across the canvas and how far down. */
export type Lane = { key: LaneKey; x: number; width: number; height: number };

const CELL = 180;
const GUTTER = 56;
const PAD = 24;
export const LANE_HEADER = 72;
const ROW = 160;
const TALL = 90;
const SPREAD = 10;

/** The party a node of its own stands in the lane of: the first responsible, or where none is, the first accountable. */
function ownLane(n: FlowNode): PartyKey | undefined {
  const r = n.responsibilities.find((x) => x.kind === "responsible") ?? n.responsibilities.find((x) => x.kind === "accountable");
  return r && partyOf(r);
}

/**
 * The lane each node goes in. Steps and subprocesses go in their own party's lane, or nobody's; a decision with a party
 * goes in its lane too. A start, an end or a decision without one follows the node before it in the flow, or failing
 * that the node after, so the flow does not leap across the canvas for them.
 */
export function laneOf(flow: Flow): Record<string, LaneKey> {
  const order = inFlowOrder(flow);
  const byKey = new Map(flow.nodes.map((n) => [n.key, n]));
  const out: Record<string, LaneKey> = {};
  for (const key of order) {
    const n = byKey.get(key) as FlowNode;
    const own = ownLane(n);
    if (own && n.kind !== "start" && n.kind !== "end") out[key] = own;
    else if (n.kind === "step" || n.kind === "subprocess") out[key] = NOBODY;
  }
  const follow = (keys: string[], near: (key: string) => string[]) => {
    for (const key of keys) {
      if (out[key]) continue;
      const found = near(key).find((k) => out[k]);
      if (found) out[key] = out[found] as LaneKey;
    }
  };
  follow(order, (key) => flow.arrows.filter((a) => a.to === key).map((a) => a.from));
  follow([...order].reverse(), (key) => flow.arrows.filter((a) => a.from === key).map((a) => a.to));
  for (const key of order) out[key] ??= NOBODY;
  return out;
}

/** The row of each node: the longest way to it from a start, arrows leading back up left out so loops do not push it down. */
export function rowOf(flow: Flow): Record<string, number> {
  const next = new Map<string, { key: string; to: string }[]>();
  for (const a of flow.arrows) next.set(a.from, [...(next.get(a.from) ?? []), { key: a.key, to: a.to }]);
  const back = new Set<string>();
  const state = new Map<string, "open" | "done">();
  const visit = (key: string) => {
    state.set(key, "open");
    for (const a of next.get(key) ?? []) {
      const s = state.get(a.to);
      if (s === "open") back.add(a.key);
      else if (!s) visit(a.to);
    }
    state.set(key, "done");
  };
  for (const key of inFlowOrder(flow)) if (!state.has(key)) visit(key);

  const forward = flow.arrows.filter((a) => !back.has(a.key));
  const into = new Map<string, number>(flow.nodes.map((n) => [n.key, 0]));
  for (const a of forward) into.set(a.to, (into.get(a.to) ?? 0) + 1);
  const row: Record<string, number> = {};
  const ready = inFlowOrder(flow).filter((key) => into.get(key) === 0);
  for (const key of ready) row[key] = 0;
  while (ready.length > 0) {
    const key = ready.shift() as string;
    for (const a of forward) {
      if (a.from !== key) continue;
      row[a.to] = Math.max(row[a.to] ?? 0, (row[key] as number) + 1);
      into.set(a.to, (into.get(a.to) as number) - 1);
      if (into.get(a.to) === 0) ready.push(a.to);
    }
  }
  return row;
}

/** Offsets for the arrows sharing a channel, spread round its middle; past its edges they run together at the edge. */
function spread(users: Map<string, string[]>, half: number): Map<string, number> {
  const out = new Map<string, number>();
  const room = Math.max(1, Math.floor(half / SPREAD));
  for (const [channel, arrows] of users) {
    arrows.forEach((arrow, i) => {
      const step = i - (arrows.length - 1) / 2;
      out.set(`${channel}|${arrow}`, Math.round(Math.max(-room, Math.min(room, step)) * SPREAD));
    });
  }
  return out;
}

/**
 * A flow laid out in lanes, one per party in the order the flow first reaches it and nobody's last, the nodes in rows
 * down the flow. Every arrow drops into the gap below its node and reaches its target from the gap above, through the
 * gutter beside the target's column when the target is not the row just below: gaps and gutters hold no node, so no
 * arrow crosses one.
 */
export function swimlanes(flow: Flow): { flow: Flow; lanes: Lane[] } {
  const lane = laneOf(flow);
  const row = rowOf(flow);
  const order = inFlowOrder(flow);
  const keys: LaneKey[] = [];
  for (const key of order) {
    const l = lane[key] as LaneKey;
    if (l !== NOBODY && !keys.includes(l)) keys.push(l);
  }
  if (order.some((key) => lane[key] === NOBODY)) keys.push(NOBODY);

  const byKey = new Map(flow.nodes.map((n) => [n.key, n]));
  const column: Record<string, number> = {};
  const columns = new Map<LaneKey, number>();
  for (const l of keys) {
    const cells = new Map<number, FlowNode[]>();
    for (const n of flow.nodes) if (lane[n.key] === l) cells.set(row[n.key] ?? 0, [...(cells.get(row[n.key] ?? 0) ?? []), n]);
    let most = 1;
    for (const cell of cells.values()) {
      cell.sort((a, b) => a.x - b.x || a.key.localeCompare(b.key));
      for (const [i, n] of cell.entries()) column[n.key] = i;
      most = Math.max(most, cell.length);
    }
    columns.set(l, most);
  }
  const rows = Math.max(1, ...Object.values(row).map((r) => r + 1));
  const height = LANE_HEADER + rows * ROW;
  const lanes: Lane[] = [];
  let x = 0;
  for (const key of keys) {
    const width = PAD + (columns.get(key) as number) * (CELL + GUTTER);
    lanes.push({ key, x, width, height });
    x += width;
  }
  const laneX = new Map(lanes.map((l) => [l.key, l.x]));
  const cellX = (key: string) => (laneX.get(lane[key] as LaneKey) as number) + PAD + (column[key] ?? 0) * (CELL + GUTTER);
  const places: Record<string, Point> = {};
  for (const n of flow.nodes) {
    const size = NODE_SIZE[n.kind];
    places[n.key] = { x: cellX(n.key) + (CELL - size.width) / 2, y: LANE_HEADER + (row[n.key] ?? 0) * ROW + (TALL - size.height) / 2 };
  }

  const gap = (r: number) => (r < 0 ? LANE_HEADER - 16 : LANE_HEADER + r * ROW + TALL + (ROW - TALL) / 2);
  const gutter = (key: string) => cellX(key) + CELL + GUTTER / 2;
  const gaps = new Map<string, string[]>();
  const gutters = new Map<string, string[]>();
  const take = (users: Map<string, string[]>, channel: string, arrow: string) => users.set(channel, [...(users.get(channel) ?? []), arrow]);
  const direct = (from: string, to: string) => (row[to] ?? 0) === (row[from] ?? 0) + 1;
  for (const a of flow.arrows) {
    if (!byKey.has(a.from) || !byKey.has(a.to)) continue;
    take(gaps, `${row[a.from] ?? 0}`, a.key);
    if (!direct(a.from, a.to)) {
      take(gutters, `${lane[a.to]}|${column[a.to] ?? 0}`, a.key);
      take(gaps, `${(row[a.to] ?? 0) - 1}`, a.key);
    }
  }
  const gapOffset = spread(gaps, (ROW - TALL) / 2 - 8);
  const gutterOffset = spread(gutters, GUTTER / 2 - 8);

  const arrows = flow.arrows.map((a) => {
    const from = byKey.get(a.from);
    const to = byKey.get(a.to);
    if (!from || !to) return { ...a, route: [] };
    const s = places[a.from] as Point;
    const t = places[a.to] as Point;
    const sx = s.x + NODE_SIZE[from.kind].width / 2;
    const sy = s.y + NODE_SIZE[from.kind].height;
    const tx = t.x + NODE_SIZE[to.kind].width / 2;
    const ty = t.y;
    const below = gap(row[a.from] ?? 0) + (gapOffset.get(`${row[a.from] ?? 0}|${a.key}`) ?? 0);
    if (direct(a.from, a.to)) {
      const route = sx === tx ? [sx, sy, tx, ty] : [sx, sy, sx, below, tx, below, tx, ty];
      return { ...a, route };
    }
    const g = gutter(a.to) + (gutterOffset.get(`${lane[a.to]}|${column[a.to] ?? 0}|${a.key}`) ?? 0);
    const above = gap((row[a.to] ?? 0) - 1) + (gapOffset.get(`${(row[a.to] ?? 0) - 1}|${a.key}`) ?? 0);
    return { ...a, route: [sx, sy, sx, below, g, below, g, above, tx, above, tx, ty] };
  });
  return { flow: { nodes: flow.nodes.map((n) => ({ ...n, ...(places[n.key] as Point) })), arrows }, lanes };
}
