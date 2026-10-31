import { describe, expect, it } from "vitest";
import type { FlowNode } from "@/api/processes";
import { arrange } from "./arrange";
import { edit, type Flow, start } from "./editor";
import { routePath, sides } from "./routing";

const node = (key: string, kind: FlowNode["kind"], x: number, y: number): FlowNode => ({
  key,
  kind,
  name: key,
  description: "",
  x,
  y,
  subprocessId: null,
  responsibilities: [],
});
const arrow = (key: string, from: string, to: string, route: number[] = []) => ({ key, from, to, label: "", route });

const flow: Flow = {
  nodes: [node("s", "start", 0, 0), node("check", "step", 0, 100), node("big", "decision", 0, 200), node("fix", "step", 400, 340), node("e", "end", 0, 340)],
  arrows: [arrow("a1", "s", "check"), arrow("a2", "check", "big"), arrow("yes", "big", "fix"), arrow("no", "big", "e"), arrow("back", "fix", "check")],
};

describe("arrows without a route", () => {
  it("lead down from bottom to top, out of the side facing a target well aside, and round the side when leading back up", () => {
    const chosen = sides(flow);
    expect(chosen.a1).toEqual({ source: "bottom", target: "top" });
    expect(chosen.no).toEqual({ source: "bottom", target: "top" });
    expect(chosen.yes).toEqual({ source: "right", target: "top" });
    expect(chosen.back).toEqual({ source: "left", target: "left-in" });
    const leftward = sides({ nodes: [node("a", "step", 400, 0), node("b", "step", 0, 200)], arrows: [arrow("x", "a", "b")] });
    expect(leftward.x).toEqual({ source: "left", target: "top" });
    const loop = sides({ nodes: [node("a", "step", 0, 0), node("b", "step", 0, 200)], arrows: [arrow("x", "b", "a"), arrow("y", "b", "gone")] });
    expect(loop.x).toEqual({ source: "right", target: "right-in" });
    expect(loop.y).toBeUndefined();
  });
});

describe("routed arrows", () => {
  it("round their corners and carry their label in the middle of their longest stretch", () => {
    const { path, labelX, labelY } = routePath([0, 0, 0, 100, 200, 100, 200, 120]);
    expect(path.startsWith("M 0 0 L 0 92 Q 0 100 8 100")).toBe(true);
    expect(path.endsWith("L 200 120")).toBe(true);
    expect([labelX, labelY]).toEqual([100, 100]);
    expect(routePath([5, 5, 5, 50]).path).toBe("M 5 5 L 5 50");
    const aside = routePath([0, 0, 0, 30, 90, 30, 90, 120, 180, 120, 180, 150]);
    expect([aside.labelX, aside.labelY]).toEqual([90, 75]);
  });

  it("keep their routes until a node of theirs moves, and get them from arranging", async () => {
    const laid = await arrange(flow);
    expect(Object.keys(laid.routes).sort()).toEqual(["a1", "a2", "back", "no", "yes"]);
    expect(laid.routes.a1?.length).toBeGreaterThanOrEqual(4);
    let h = edit(start(flow), { type: "arrange", ...laid });
    expect(h.present.arrows.every((a) => a.route.length >= 4)).toBe(true);
    h = edit(h, { type: "move", key: "fix", to: { x: 500, y: 340 } });
    const routed = Object.fromEntries(h.present.arrows.map((a) => [a.key, a.route.length > 0]));
    expect(routed).toEqual({ a1: true, a2: true, yes: false, no: true, back: false });
    h = edit(h, { type: "connect", key: "new", from: "s", to: "fix" });
    expect(h.present.arrows.at(-1)?.route).toEqual([]);
  });
});
