import { describe, expect, it } from "vitest";
import type { FlowArrow, FlowNode, Responsibility } from "@/api/processes";
import { NODE_SIZE } from "./arrange";
import type { Flow } from "./editor";
import { LANE_HEADER, laneOf, NOBODY, rowOf, swimlanes } from "./lanes";

const node = (key: string, kind: FlowNode["kind"], responsibilities: Responsibility[] = [], x = 0): FlowNode => ({
  key,
  kind,
  name: key,
  description: "",
  x,
  y: 0,
  subprocessId: null,
  responsibilities,
});
const arrow = (key: string, from: string, to: string): FlowArrow => ({ key, from, to, label: "", route: [] });

const clerk: Responsibility = { kind: "responsible", roleId: "clerk" };
const head: Responsibility = { kind: "accountable", positionId: "head" };

// An invoice comes in, the clerk checks it, a decision sends it back to be checked again or on to the head who
// approves it, a step nobody has taken on books it, and a step only the head is accountable for closes it.
const invoice: Flow = {
  nodes: [
    node("in", "start"),
    node("check", "step", [{ kind: "informed", unitId: "fin" }, clerk]),
    node("ok", "decision"),
    node("approve", "step", [{ kind: "responsible", positionId: "head" }]),
    node("book", "step"),
    node("close", "step", [head, { kind: "consulted", roleId: "clerk" }]),
    node("out", "end"),
  ],
  arrows: [
    arrow("1", "in", "check"),
    arrow("2", "check", "ok"),
    arrow("3", "ok", "check"),
    arrow("4", "ok", "approve"),
    arrow("5", "approve", "book"),
    arrow("6", "book", "close"),
    arrow("7", "close", "out"),
    arrow("8", "ok", "out"),
  ],
};

describe("lanes of a process", () => {
  it("puts a step in its responsible party's lane, or its accountable one's, or nobody's", () => {
    const lane = laneOf(invoice);
    expect(lane.check).toBe("role:clerk");
    expect(lane.approve).toBe("position:head");
    expect(lane.close).toBe("position:head");
    expect(lane.book).toBe(NOBODY);
  });

  it("lets a start, an end and a decision of nobody's follow the flow", () => {
    const lane = laneOf(invoice);
    expect(lane.in).toBe("role:clerk");
    expect(lane.ok).toBe("role:clerk");
    expect(lane.out).toBe("position:head");
    const owned = { ...invoice, nodes: invoice.nodes.map((n) => (n.key === "ok" ? { ...n, responsibilities: [head] } : n)) };
    expect(laneOf(owned).ok).toBe("position:head");
    expect(laneOf({ nodes: [node("s", "start"), node("e", "end")], arrows: [arrow("1", "s", "e")] })).toEqual({ s: NOBODY, e: NOBODY });
  });

  it("runs the rows down the flow, a loop back up not pushing anything further down", () => {
    expect(rowOf(invoice)).toEqual({ in: 0, check: 1, ok: 2, approve: 3, book: 4, close: 5, out: 6 });
  });

  it("orders the lanes as the flow first reaches them, nobody's last, and puts nodes of one row and lane side by side", () => {
    const { lanes, flow } = swimlanes(invoice);
    expect(lanes.map((l) => l.key)).toEqual(["role:clerk", "position:head", NOBODY]);
    for (const [a, b] of lanes.slice(1).map((l, i) => [lanes[i], l] as const)) expect(b.x).toBe((a?.x ?? 0) + (a?.width ?? 0));
    const at = (key: string) => flow.nodes.find((n) => n.key === key) as FlowNode;
    const within = (key: string, laneKey: string) => {
      const l = lanes.find((x) => x.key === laneKey);
      const n = at(key);
      return !!l && n.x >= l.x && n.x + NODE_SIZE[n.kind].width <= l.x + l.width;
    };
    expect(within("check", "role:clerk") && within("approve", "position:head") && within("book", NOBODY)).toBe(true);
    expect(at("check").y).toBeGreaterThan(at("in").y);
    expect(at("in").y).toBeGreaterThanOrEqual(LANE_HEADER);

    const twins: Flow = {
      nodes: [node("s", "start"), node("d", "decision", [clerk]), node("b", "step", [clerk], 300), node("a", "step", [clerk], 100), node("e", "end")],
      arrows: [arrow("1", "s", "d"), arrow("2", "d", "a"), arrow("3", "d", "b"), arrow("4", "a", "e"), arrow("5", "b", "e")],
    };
    const laid = swimlanes(twins);
    const a = laid.flow.nodes.find((n) => n.key === "a") as FlowNode;
    const b = laid.flow.nodes.find((n) => n.key === "b") as FlowNode;
    expect(a.y).toBe(b.y);
    expect(b.x - a.x).toBeGreaterThanOrEqual(NODE_SIZE.step.width);
    expect(laid.lanes).toHaveLength(1);
  });

  it("routes every arrow from its node's bottom to its target's top, square, and through no other node", () => {
    const { flow } = swimlanes(invoice);
    const byKey = new Map(flow.nodes.map((n) => [n.key, n]));
    for (const a of flow.arrows) {
      const from = byKey.get(a.from) as FlowNode;
      const to = byKey.get(a.to) as FlowNode;
      const points = a.route.flatMap((v, i) => (i % 2 === 0 ? [{ x: v, y: a.route[i + 1] as number }] : []));
      expect(points[0]).toEqual({ x: from.x + NODE_SIZE[from.kind].width / 2, y: from.y + NODE_SIZE[from.kind].height });
      expect(points.at(-1)).toEqual({ x: to.x + NODE_SIZE[to.kind].width / 2, y: to.y });
      for (const [p, q] of points.slice(1).map((q, i) => [points[i] as { x: number; y: number }, q] as const)) {
        expect(p.x === q.x || p.y === q.y).toBe(true);
        for (const n of flow.nodes) {
          const { width, height } = NODE_SIZE[n.kind];
          const inside = Math.max(p.x, q.x) > n.x && Math.min(p.x, q.x) < n.x + width && Math.max(p.y, q.y) > n.y && Math.min(p.y, q.y) < n.y + height;
          expect(inside, `${a.key} crosses ${n.key}`).toBe(false);
        }
      }
    }
  });
});
