import ELK from "elkjs/lib/elk.bundled.js";
import type { NodeKind } from "@/api/processes";
import type { Flow, Point } from "./editor";

/** How big each kind of node is drawn, so the layout leaves room for it. */
export const NODE_SIZE: Record<NodeKind, { width: number; height: number }> = {
  start: { width: 160, height: 44 },
  end: { width: 160, height: 44 },
  step: { width: 180, height: 56 },
  subprocess: { width: 180, height: 60 },
  decision: { width: 180, height: 90 },
};

const elk = new ELK();

/** Where ELK puts each node of a flow, top to bottom, with the start first. */
export async function arrange(flow: Flow): Promise<Record<string, Point>> {
  const laid = await elk.layout({
    id: "flow",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.layered.spacing.nodeNodeBetweenLayers": "56",
      "elk.spacing.nodeNode": "48",
      "elk.layered.nodePlacement.strategy": "BALANCED",
      "elk.layered.cycleBreaking.strategy": "DEPTH_FIRST",
    },
    children: flow.nodes.map((n) => ({ id: n.key, ...NODE_SIZE[n.kind] })),
    edges: flow.arrows.map((a) => ({ id: a.key, sources: [a.from], targets: [a.to] })),
  });
  const out: Record<string, Point> = {};
  for (const node of laid.children ?? []) out[node.id] = { x: Math.round(node.x ?? 0), y: Math.round(node.y ?? 0) };
  return out;
}
