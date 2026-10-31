import { getNodesBounds, getViewportForBounds, type Node } from "@xyflow/react";
import { toPng, toSvg } from "html-to-image";

const PADDING = 32;

/** Draws every card and line, not only what the viewport shows, into a PNG or an SVG and hands it to the browser. */
export async function exportChart(nodes: Node[], format: "png" | "svg", fileName: string): Promise<void> {
  const viewport = document.querySelector<HTMLElement>(".react-flow__viewport");
  if (!viewport || nodes.length === 0) return;
  const bounds = getNodesBounds(nodes);
  const width = Math.ceil(bounds.width + 2 * PADDING);
  const height = Math.ceil(bounds.height + 2 * PADDING);
  const { x, y, zoom } = getViewportForBounds(bounds, width, height, 1, 1, PADDING);
  const background = getComputedStyle(document.documentElement).getPropertyValue("--color-canvas").trim() || "#ffffff";
  const options = {
    backgroundColor: background,
    width,
    height,
    style: { width: `${width}px`, height: `${height}px`, transform: `translate(${x}px, ${y}px) scale(${zoom})` },
  };
  const url = format === "png" ? await toPng(viewport, options) : await toSvg(viewport, options);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${fileName}.${format}`;
  link.click();
}
