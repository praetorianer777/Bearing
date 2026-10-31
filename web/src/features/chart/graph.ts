import type { ElkNode } from "elkjs/lib/elk-api";
import type { Person, Position, Unit } from "@/api/org";

export type Holder = { personId: string; name: string; share: number };

/** A position as the chart draws it: one card. */
export type Card = {
  id: string;
  title: string;
  unitId: string;
  unitName: string;
  head: boolean;
  seats: number;
  holders: Holder[];
  vacant: boolean;
  /** Whether the card is its unit's head with anything below it to fold away. */
  collapsible: boolean;
  collapsed: boolean;
  /** How many positions folding the unit hides. */
  hidden: number;
};

export type LineKind = "solid" | "dotted" | "deputy";

/** A line from the manager's card down to the report's, or from a position to the one standing in for it. */
export type Line = { id: string; source: string; target: string; kind: LineKind };

export type Focus = { kind: "unit" | "person"; id: string };

export type ChartOptions = {
  collapsed: ReadonlySet<string>;
  focus?: Focus;
  deputies: boolean;
};

export type ChartInput = { units: Unit[]; positions: Position[]; people: Person[] };

export const CARD_WIDTH = 240;
const CARD_BASE_HEIGHT = 64;
const HOLDER_HEIGHT = 20;

export const cardHeight = (card: Card) => CARD_BASE_HEIGHT + HOLDER_HEIGHT * Math.max(1, card.holders.length);

/** The units inside a unit on the day, the unit itself included. */
function subtree(units: Unit[], root: string): Set<string> {
  const children = new Map<string, string[]>();
  for (const u of units) {
    if (u.parentId) children.set(u.parentId, [...(children.get(u.parentId) ?? []), u.id]);
  }
  const out = new Set<string>();
  const walk = (id: string) => {
    if (out.has(id)) return;
    out.add(id);
    for (const child of children.get(id) ?? []) walk(child);
  };
  walk(root);
  return out;
}

/** The positions a focus keeps: a unit's with its sub-units', or a person's with their chain up and their reports. */
function focused(input: ChartInput, focus: Focus): Set<string> {
  if (focus.kind === "unit") {
    const inside = subtree(input.units, focus.id);
    return new Set(input.positions.filter((p) => inside.has(p.unitId)).map((p) => p.id));
  }
  const byID = new Map(input.positions.map((p) => [p.id, p]));
  const held = input.positions.filter((p) => p.holders.some((h) => h.personId === focus.id)).map((p) => p.id);
  const out = new Set(held);
  for (const start of held) {
    let up = byID.get(start)?.reportsToId;
    while (up && !out.has(up)) {
      out.add(up);
      up = byID.get(up)?.reportsToId;
    }
  }
  for (const p of input.positions) {
    if ((p.reportsToId && held.includes(p.reportsToId)) || p.dottedTo.some((d) => held.includes(d))) out.add(p.id);
  }
  return out;
}

/** What the chart shows: the cards and the lines between those it keeps, given what is folded and focused. */
export function chartGraph(input: ChartInput, options: ChartOptions): { cards: Card[]; lines: Line[] } {
  const names = new Map(input.people.map((p) => [p.id, p.name]));
  const unitNames = new Map(input.units.map((u) => [u.id, u.name]));
  let shown = options.focus ? focused(input, options.focus) : new Set(input.positions.map((p) => p.id));

  const hiddenBy = new Map<string, number>();
  for (const unit of options.collapsed) {
    const inside = subtree(input.units, unit);
    const heads = new Set(input.positions.filter((p) => p.unitId === unit && p.headOfUnit).map((p) => p.id));
    const fold = input.positions.filter((p) => inside.has(p.unitId) && !heads.has(p.id) && shown.has(p.id)).map((p) => p.id);
    hiddenBy.set(unit, fold.length);
    shown = new Set([...shown].filter((id) => !fold.includes(id)));
  }

  const below = new Map<string, number>();
  for (const p of input.positions) {
    if (!p.headOfUnit) continue;
    const inside = subtree(input.units, p.unitId);
    below.set(p.id, input.positions.filter((o) => o.id !== p.id && inside.has(o.unitId)).length);
  }

  const cards: Card[] = input.positions
    .filter((p) => shown.has(p.id))
    .map((p) => ({
      id: p.id,
      title: p.title,
      unitId: p.unitId,
      unitName: unitNames.get(p.unitId) ?? "",
      head: p.headOfUnit,
      seats: p.seats,
      holders: p.holders.map((h) => ({ personId: h.personId, name: names.get(h.personId) ?? "", share: h.share })),
      vacant: p.holders.length === 0,
      collapsible: p.headOfUnit && (below.get(p.id) ?? 0) > 0,
      collapsed: p.headOfUnit && options.collapsed.has(p.unitId),
      hidden: p.headOfUnit ? (hiddenBy.get(p.unitId) ?? 0) : 0,
    }));

  const lines: Line[] = [];
  for (const p of input.positions) {
    if (!shown.has(p.id)) continue;
    if (p.reportsToId && shown.has(p.reportsToId)) {
      lines.push({ id: `solid:${p.reportsToId}:${p.id}`, source: p.reportsToId, target: p.id, kind: "solid" });
    }
    for (const d of p.dottedTo) {
      if (shown.has(d)) lines.push({ id: `dotted:${d}:${p.id}`, source: d, target: p.id, kind: "dotted" });
    }
    if (options.deputies) {
      for (const d of p.deputies) {
        if (shown.has(d)) lines.push({ id: `deputy:${p.id}:${d}`, source: p.id, target: d, kind: "deputy" });
      }
    }
  }
  return { cards, lines };
}

/**
 * What ELK lays out: every card at its size, and the solid lines alone, so the tree stays a tree; dotted lines and
 * deputies are drawn across it afterwards.
 */
export function elkGraph(cards: Card[], lines: Line[]): ElkNode {
  return {
    id: "chart",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.layered.spacing.nodeNodeBetweenLayers": "56",
      "elk.spacing.nodeNode": "32",
      "elk.layered.nodePlacement.strategy": "BRANDES_KOEPF",
      // Balanced, a manager sits over the middle of their reports rather than at one end of them.
      "elk.layered.nodePlacement.bk.fixedAlignment": "BALANCED",
    },
    children: cards.map((c) => ({ id: c.id, width: CARD_WIDTH, height: cardHeight(c) })),
    edges: lines.filter((l) => l.kind === "solid").map((l) => ({ id: l.id, sources: [l.source], targets: [l.target] })),
  };
}
