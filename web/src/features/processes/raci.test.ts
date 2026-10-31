import { describe, expect, it } from "vitest";
import type { FlowNode } from "@/api/processes";
import { inFlowOrder, matrix, partyOf, responsibility } from "./raci";

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

describe("responsibilities in a matrix", () => {
  it("names each party by its kind and id and back", () => {
    expect(partyOf(responsibility("accountable", "role:r1"))).toBe("role:r1");
    expect(responsibility("informed", "position:p1")).toEqual({ kind: "informed", positionId: "p1" });
    expect(responsibility("consulted", "unit:u1")).toEqual({ kind: "consulted", unitId: "u1" });
  });

  it("reads the nodes in the order the flow runs, those reached by none after as drawn", () => {
    const flow = {
      nodes: [node("late", "step", { y: 300 }), node("e", "end"), node("b", "step"), node("s", "start"), node("a", "step"), node("lost", "step", { y: 100 })],
      arrows: [
        { key: "1", from: "s", to: "a", label: "" },
        { key: "2", from: "a", to: "b", label: "" },
        { key: "3", from: "b", to: "a", label: "" },
        { key: "4", from: "b", to: "e", label: "" },
      ],
    };
    expect(inFlowOrder(flow)).toEqual(["s", "a", "b", "e", "lost", "late"]);
  });

  it("puts every step down, decisions with parties too, and each party across in the order it first shows", () => {
    const flow = {
      nodes: [
        node("s", "start"),
        node("check", "step", { responsibilities: [responsibility("accountable", "position:head"), responsibility("responsible", "role:clerk")] }),
        node("big", "decision"),
        node("ask", "decision", { responsibilities: [responsibility("consulted", "unit:legal")] }),
        node("pay", "step", { responsibilities: [responsibility("informed", "position:head"), responsibility("responsible", "position:head")] }),
        node("empty", "step"),
        node("e", "end"),
      ],
      arrows: [
        { key: "1", from: "s", to: "check", label: "" },
        { key: "2", from: "check", to: "big", label: "" },
        { key: "3", from: "big", to: "ask", label: "" },
        { key: "4", from: "ask", to: "pay", label: "" },
        { key: "5", from: "pay", to: "empty", label: "" },
        { key: "6", from: "empty", to: "e", label: "" },
      ],
    };
    const m = matrix(flow);
    expect(m.rows.map((r) => r.key)).toEqual(["check", "ask", "pay", "empty"]);
    expect(m.columns).toEqual(["position:head", "role:clerk", "unit:legal"]);
    expect(m.cells.get("pay|position:head")).toEqual(["responsible", "informed"]);
    expect(m.cells.get("empty|position:head")).toBeUndefined();
  });
});
