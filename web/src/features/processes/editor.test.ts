import { describe, expect, it } from "vitest";
import type { FlowNode } from "@/api/processes";
import { edit, type Flow, freeSpot, freshKey, type History, mayConnect, start } from "./editor";

const node = (key: string, kind: FlowNode["kind"], name = key): FlowNode => ({
  key,
  kind,
  name,
  description: "",
  x: 0,
  y: 0,
  subprocessId: null,
  responsibilities: [],
});
const flow: Flow = {
  nodes: [node("s", "start"), node("check", "step"), node("e", "end")],
  arrows: [{ key: "a1", from: "s", to: "check", label: "", route: [] }],
};

const run = (h: History, ...edits: Parameters<typeof edit>[1][]) => edits.reduce(edit, h);

describe("editing a draft", () => {
  it("adds, connects, renames and removes, and undoes each step", () => {
    let h = run(
      start(flow),
      { type: "add", kind: "decision", key: "big", name: "Big?", at: { x: 10, y: 20 } },
      { type: "connect", key: "a2", from: "check", to: "big" },
      { type: "connect", key: "a3", from: "big", to: "e" },
      { type: "label", key: "a3", label: "No" },
    );
    expect(h.present.nodes.find((n) => n.key === "big")).toMatchObject({ kind: "decision", x: 10, y: 20 });
    expect(h.present.arrows.map((a) => a.key)).toEqual(["a1", "a2", "a3"]);
    expect(h.present.arrows[2]?.label).toBe("No");

    h = edit(h, { type: "remove", nodes: ["big"], arrows: [] });
    expect(h.present.arrows.map((a) => a.key)).toEqual(["a1"]);
    h = edit(h, { type: "undo" });
    expect(h.present.arrows).toHaveLength(3);
    h = run(h, { type: "undo" }, { type: "undo" });
    expect(h.present.arrows.map((a) => a.key)).toEqual(["a1", "a2"]);
    h = edit(h, { type: "redo" });
    expect(h.present.arrows).toHaveLength(3);
    h = edit(h, { type: "move", key: "s", to: { x: 5, y: 5 } });
    expect(h.future).toEqual([]);
  });

  it("takes a run of typing or one drag back as one step", () => {
    let h = run(
      start(flow),
      { type: "node", key: "check", change: { name: "C" } },
      { type: "node", key: "check", change: { name: "Ch" } },
      { type: "node", key: "check", change: { name: "Check it" } },
      { type: "settle" },
      { type: "move", key: "check", to: { x: 1, y: 1 } },
      { type: "move", key: "check", to: { x: 2, y: 2 } },
      { type: "settle" },
      { type: "move", key: "check", to: { x: 3, y: 3 } },
    );
    expect(h.past).toHaveLength(3);
    h = edit(h, { type: "undo" });
    expect(h.present.nodes[1]).toMatchObject({ name: "Check it", x: 2, y: 2 });
    h = edit(h, { type: "undo" });
    expect(h.present.nodes[1]).toMatchObject({ name: "Check it", x: 0 });
    h = edit(h, { type: "undo" });
    expect(h.present.nodes[1]?.name).toBe("check");
    expect(edit(h, { type: "undo" })).toBe(h);
    expect(edit(start(flow), { type: "redo" }).present).toBe(flow);
  });

  it("draws no arrow into a start, out of an end, twice or back into its node", () => {
    expect(mayConnect(flow, "check", "e")).toBe(true);
    expect(mayConnect(flow, "check", "s")).toBe(false);
    expect(mayConnect(flow, "e", "check")).toBe(false);
    expect(mayConnect(flow, "s", "check")).toBe(false);
    expect(mayConnect(flow, "check", "check")).toBe(false);
    expect(mayConnect(flow, "check", "nowhere")).toBe(false);
    const h = start(flow);
    expect(edit(h, { type: "connect", key: "x", from: "e", to: "check" })).toBe(h);
    expect(edit(h, { type: "remove", nodes: [], arrows: [] })).toBe(h);
  });

  it("arranges nodes where the layout puts them and picks keys nothing has", () => {
    const h = edit(start(flow), { type: "arrange", places: { s: { x: 100, y: 0 }, e: { x: 100, y: 200 } }, routes: { a1: [0, 0, 0, 10] } });
    expect(h.present.nodes.map((n) => [n.key, n.x, n.y])).toEqual([
      ["s", 100, 0],
      ["check", 0, 0],
      ["e", 100, 200],
    ]);
    const keys = ["a1", "a1", "fresh"];
    expect(freshKey("a", { nodes: [], arrows: [{ key: "a-a1", from: "x", to: "y", label: "", route: [] }] }, () => keys.shift() as string)).toBe("a-fresh");
  });
});

describe("placing a new node", () => {
  const size = () => ({ width: 100, height: 40 });
  it("goes where it is asked when nothing is there, and below what is", () => {
    expect(freeSpot({ nodes: [], arrows: [] }, "step", { x: 0, y: 0 }, size)).toEqual({ x: 0, y: 0 });
    const taken: Flow = { nodes: [node("a", "step"), { ...node("b", "step"), y: 64 }], arrows: [] };
    expect(freeSpot(taken, "step", { x: 20, y: 10 }, size)).toEqual({ x: 20, y: 138 });
    expect(freeSpot(taken, "step", { x: 300, y: 0 }, size)).toEqual({ x: 300, y: 0 });
  });
});
