import type { NodeKind } from "@/api/processes";
import { NODE_SIZE } from "./arrange";
import type { Flow } from "./editor";

/** Where an arrow leaves or reaches a node: the bottom and top as drawn by hand, the sides when the canvas chooses. */
export type SourceSide = "bottom" | "left" | "right";
export type TargetSide = "top" | "left-in" | "right-in";

const centre = (n: { kind: NodeKind; x: number; y: number }) => ({
  x: n.x + NODE_SIZE[n.kind].width / 2,
  y: n.y + NODE_SIZE[n.kind].height / 2,
});

/**
 * The sides each arrow without a route takes. One leading back up goes out and in on the same side, round the nodes
 * between rather than through them; one leading down and well aside leaves from the side facing its target; any
 * other leaves from the bottom for the top.
 */
export function sides(flow: Flow): Record<string, { source: SourceSide; target: TargetSide }> {
  const byKey = new Map(flow.nodes.map((n) => [n.key, n]));
  const out: Record<string, { source: SourceSide; target: TargetSide }> = {};
  for (const a of flow.arrows) {
    const from = byKey.get(a.from);
    const to = byKey.get(a.to);
    if (!from || !to) continue;
    const s = centre(from);
    const t = centre(to);
    const half = NODE_SIZE[from.kind].width / 2;
    const dx = t.x - s.x;
    if (to.y < from.y + NODE_SIZE[from.kind].height) {
      const side = dx < -half ? "left" : "right";
      out[a.key] = { source: side, target: `${side}-in` };
    } else if (Math.abs(dx) > half + 20) {
      out[a.key] = { source: dx > 0 ? "right" : "left", target: "top" };
    } else {
      out[a.key] = { source: "bottom", target: "top" };
    }
  }
  return out;
}

const RADIUS = 8;

/** A routed arrow as an SVG path, its corners rounded, and where its label goes: the middle of a longest stretch. */
export function routePath(route: number[]): { path: string; labelX: number; labelY: number } {
  const points: { x: number; y: number }[] = [];
  for (let i = 0; i + 1 < route.length; i += 2) points.push({ x: route[i] as number, y: route[i + 1] as number });
  const first = points[0] ?? { x: 0, y: 0 };
  let path = `M ${first.x} ${first.y}`;
  for (let i = 1; i < points.length; i++) {
    const p = points[i] as { x: number; y: number };
    const prev = points[i - 1] as { x: number; y: number };
    const next = points[i + 1];
    if (!next) {
      path += ` L ${p.x} ${p.y}`;
      continue;
    }
    const r = Math.min(RADIUS, Math.hypot(p.x - prev.x, p.y - prev.y) / 2, Math.hypot(next.x - p.x, next.y - p.y) / 2);
    const inX = p.x - Math.sign(p.x - prev.x) * r;
    const inY = p.y - Math.sign(p.y - prev.y) * r;
    const outX = p.x + Math.sign(next.x - p.x) * r;
    const outY = p.y + Math.sign(next.y - p.y) * r;
    path += ` L ${inX} ${inY} Q ${p.x} ${p.y} ${outX} ${outY}`;
  }
  // Of the longest stretches, the one nearest the middle of the arrow: two arrows leaving one decision then carry their
  // conditions apart rather than side by side where they leave.
  const lengths = points.slice(1).map((b, i) => Math.hypot(b.x - (points[i] as { x: number }).x, b.y - (points[i] as { y: number }).y));
  const longest = Math.max(...lengths);
  const middle = (lengths.length - 1) / 2;
  const longestOnes = lengths.flatMap((length, i) => (length >= longest - 1 ? [i] : []));
  const chosen = longestOnes.reduce((best, i) => (Math.abs(i - middle) < Math.abs(best - middle) ? i : best), longestOnes[0] ?? 0);
  const a = points[chosen] as { x: number; y: number };
  const b = points[chosen + 1] ?? a;
  return { path, labelX: (a.x + b.x) / 2, labelY: (a.y + b.y) / 2 };
}
