import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkExtendedEdge } from "elkjs/lib/elk-api";
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

/** Where ELK puts each node of a flow, top to bottom with the start first, and the way it routes each arrow round them. */
export async function arrange(flow: Flow): Promise<{ places: Record<string, Point>; routes: Record<string, number[]> }> {
  const laid = await elk.layout({
    id: "flow",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.layered.spacing.nodeNodeBetweenLayers": "64",
      "elk.layered.spacing.edgeNodeBetweenLayers": "24",
      "elk.spacing.nodeNode": "56",
      "elk.spacing.edgeEdge": "16",
      "elk.spacing.edgeNode": "24",
      "elk.spacing.edgeLabel": "4",
      "elk.layered.nodePlacement.strategy": "BALANCED",
      "elk.layered.cycleBreaking.strategy": "DEPTH_FIRST",
      "elk.layered.crossingMinimization.semiInteractive": "true",
    },
    children: flow.nodes.map((n) => ({ id: n.key, ...NODE_SIZE[n.kind] })),
    edges: flow.arrows.map((a) => ({
      id: a.key,
      sources: [a.from],
      targets: [a.to],
      // A condition takes room on its arrow, so the layout keeps other lines off it.
      labels: a.label ? [{ text: a.label, width: a.label.length * 7 + 16, height: 20 }] : [],
    })),
  });
  const places: Record<string, Point> = {};
  for (const node of laid.children ?? []) places[node.id] = { x: Math.round(node.x ?? 0), y: Math.round(node.y ?? 0) };
  const routes: Record<string, number[]> = {};
  for (const edge of (laid.edges ?? []) as ElkExtendedEdge[]) {
    const section = edge.sections?.[0];
    if (!section) continue;
    const points = [section.startPoint, ...(section.bendPoints ?? []), section.endPoint];
    routes[edge.id] = points.flatMap((p) => [Math.round(p.x), Math.round(p.y)]);
  }
  return { places, routes };
}
