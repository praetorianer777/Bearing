import ELK from "elkjs/lib/elk.bundled.js";
import { CHART_PLACES_STORAGE_KEY } from "@/config";
import { type Card, elkGraph, type Line } from "./graph";

export type Point = { x: number; y: number };

const elk = new ELK();

/** Where ELK puts each card, with the cards a person moved kept where they put them. */
export async function layout(cards: Card[], lines: Line[], kept: Record<string, Point>): Promise<Record<string, Point>> {
  const laid = await elk.layout(elkGraph(cards, lines));
  const out: Record<string, Point> = {};
  for (const node of laid.children ?? []) out[node.id] = kept[node.id] ?? { x: node.x ?? 0, y: node.y ?? 0 };
  return out;
}

/** The cards this browser remembers moving; a browser that keeps nothing remembers none. */
export function readPlaces(): Record<string, Point> {
  try {
    const raw = localStorage.getItem(CHART_PLACES_STORAGE_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : {};
    return parsed && typeof parsed === "object" ? (parsed as Record<string, Point>) : {};
  } catch {
    return {};
  }
}

export function writePlaces(places: Record<string, Point>): void {
  try {
    if (Object.keys(places).length === 0) localStorage.removeItem(CHART_PLACES_STORAGE_KEY);
    else localStorage.setItem(CHART_PLACES_STORAGE_KEY, JSON.stringify(places));
  } catch {
    // Nothing to keep them in; the cards stay where they are for this page.
  }
}
