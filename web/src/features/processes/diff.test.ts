import { describe, expect, it } from "vitest";
import type { FlowArrow, FlowNode } from "@/api/processes";
import { diff } from "./diff";
import type { Flow } from "./editor";

const node = (key: string, kind: FlowNode["kind"], extra: Partial<FlowNode> = {}): FlowNode => ({
  key,
  kind,
  name: key,
  description: "",
  x: 0,
  y: 0,
  subprocessId: null,
  responsibilities: [],
  ...extra,
});
const arrow = (key: string, from: string, to: string, label = ""): FlowArrow => ({ key, from, to, label, route: [] });

const before: Flow = {
  nodes: [
    node("s", "start"),
    node("check", "step", {
      responsibilities: [
        { kind: "responsible", roleId: "clerk" },
        { kind: "informed", unitId: "fin" },
      ],
    }),
    node("fax", "step", { x: 400, y: 80 }),
    node("e", "end"),
  ],
  arrows: [arrow("1", "s", "check"), arrow("2", "check", "fax"), arrow("3", "fax", "e"), arrow("4", "check", "e", "ok")],
};

describe("comparing two versions of a process", () => {
  it("finds nothing where only the drawing moved", () => {
    const moved = { nodes: before.nodes.map((n) => ({ ...n, x: n.x + 50 })), arrows: before.arrows.map((a) => ({ ...a, route: [0, 0, 0, 10] })) };
    const found = diff(before, moved);
    expect(found.nodes).toEqual([]);
    expect(found.arrows).toEqual([]);
    expect(found.marks.size).toBe(0);
  });

  it("names nodes and arrows added, removed and changed, with what changed of each, in the order the flow runs", () => {
    const after: Flow = {
      nodes: [
        node("s", "start"),
        node("check", "step", {
          name: "Check it",
          description: "Against the order",
          responsibilities: [
            { kind: "responsible", roleId: "clerk" },
            { kind: "accountable", positionId: "head" },
          ],
        }),
        node("ok", "decision", { name: "Fine?" }),
        node("e", "end"),
      ],
      arrows: [arrow("1", "s", "check"), arrow("5", "check", "ok"), arrow("4", "ok", "e", "yes"), arrow("6", "ok", "check", "no")],
    };
    const found = diff(before, after);
    expect(found.nodes.map((n) => [n.key, n.mark])).toEqual([
      ["check", "changed"],
      ["ok", "added"],
      ["fax", "removed"],
    ]);
    const check = found.nodes[0];
    expect(check?.fields).toEqual(["name", "description"]);
    expect(check?.responsibilities).toEqual([
      { kind: "accountable", party: "position:head", mark: "added" },
      { kind: "informed", party: "unit:fin", mark: "removed" },
    ]);
    expect(found.arrows.map((a) => [a.key, a.mark, a.fields])).toEqual([
      ["5", "added", []],
      ["2", "removed", []],
      ["6", "added", []],
      ["4", "changed", ["ends", "label"]],
      ["3", "removed", []],
    ]);
    expect(found.merged.nodes.find((n) => n.key === "fax")).toMatchObject({ x: 400, y: 80 });
    expect(found.merged.arrows.map((a) => a.key)).toEqual(["1", "5", "4", "6", "2", "3"]);
    expect(found.merged.arrows.find((a) => a.key === "2")?.route).toEqual([]);
    expect(found.marks.get("fax")).toBe("removed");
    expect(found.marks.get("1")).toBeUndefined();
  });

  it("sees a subprocess chosen and a node of another kind", () => {
    const after: Flow = {
      ...before,
      nodes: before.nodes.map((n) => (n.key === "fax" ? { ...n, kind: "subprocess" as const, subprocessId: "p2" } : n)),
    };
    expect(diff(before, after).nodes).toEqual([
      { key: "fax", name: "fax", kind: "subprocess", mark: "changed", fields: ["kind", "subprocess"], responsibilities: [] },
    ]);
  });
});
