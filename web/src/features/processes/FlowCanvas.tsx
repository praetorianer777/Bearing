import {
  Background,
  BaseEdge,
  type Connection,
  Controls,
  type Edge,
  EdgeLabelRenderer,
  type EdgeProps,
  getSmoothStepPath,
  Handle,
  MarkerType,
  type Node,
  type NodeChange,
  type NodeProps,
  Position as Side,
  ReactFlow,
  useReactFlow,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Link } from "@tanstack/react-router";
import { useEffect, useMemo, useRef, useState } from "react";
import type { FlowNode, NodeKind } from "@/api/processes";
import { t } from "@/i18n";
import { NODE_SIZE } from "./arrange";
import { type Edit, type Flow, freeSpot, freshKey, mayConnect } from "./editor";
import { partyOf } from "./raci";
import { routePath, sides } from "./routing";

/** What is picked on the canvas: one node or one arrow, by key. */
export type Selection = { kind: "node" | "arrow"; key: string } | null;

type NodeData = { node: FlowNode; flagged: boolean; subprocess?: { id: string; name: string }; accountable?: string };
type Tone = "plain" | "picked" | "lit" | "dim" | "flagged";
type ArrowData = { route: number[]; label: string; tone: Tone };
type FlowEdgeView = Edge<ArrowData, "flow">;
type FlowNodeView = Node<NodeData>;
type LaneData = { name: string; group?: string; even: boolean };
type LaneView = Node<LaneData, "lane">;

/** A lane drawn behind the nodes: whose it is, where it runs and its title. */
export type LaneShown = { key: string; name: string; group?: string; x: number; width: number; height: number };

const tone = (flagged: boolean, selected: boolean) =>
  selected ? "border-accent ring-2 ring-accent/40" : flagged ? "border-warning border-dashed" : "border-border-strong";

// The sides are for the canvas to route arrows by; people draw from the bottom to the top.
const sideHandle = "opacity-0! pointer-events-none";

function Ends({ kind }: { kind: NodeKind }) {
  return (
    <>
      {kind !== "start" && (
        <>
          <Handle id="top" type="target" position={Side.Top} className="size-2.5! bg-ink-muted!" />
          <Handle id="left-in" type="target" position={Side.Left} isConnectable={false} className={sideHandle} />
          <Handle id="right-in" type="target" position={Side.Right} isConnectable={false} className={sideHandle} />
        </>
      )}
      {kind !== "end" && (
        <>
          <Handle id="bottom" type="source" position={Side.Bottom} className="size-2.5! bg-ink-muted!" />
          <Handle id="left" type="source" position={Side.Left} isConnectable={false} className={sideHandle} />
          <Handle id="right" type="source" position={Side.Right} isConnectable={false} className={sideHandle} />
        </>
      )}
    </>
  );
}

function Label({ node }: { node: FlowNode }) {
  return <span className={node.name ? "" : "italic text-ink-subtle"}>{node.name || t.process.unnamed}</span>;
}

function TerminalNode({ data, selected }: NodeProps<FlowNodeView>) {
  const { node, flagged } = data;
  return (
    <article
      aria-label={`${t.process.kinds[node.kind]}: ${node.name}`}
      className={`flex items-center justify-center rounded-full border-2 px-4 text-sm font-medium ${tone(flagged, selected)} ${node.kind === "start" ? "bg-accent-subtle text-ink" : "bg-surface-sunken text-ink"}`}
      style={NODE_SIZE[node.kind]}
    >
      <Ends kind={node.kind} />
      <Label node={node} />
    </article>
  );
}

function StepNode({ data, selected }: NodeProps<FlowNodeView>) {
  const { node, flagged, subprocess, accountable } = data;
  return (
    <article
      aria-label={`${t.process.kinds[node.kind]}: ${node.name}`}
      className={`flex flex-col items-center justify-center rounded-md bg-surface px-3 text-center text-sm shadow-sm ${node.kind === "subprocess" ? "border-4 border-double" : "border"} ${tone(flagged, selected)}`}
      style={NODE_SIZE[node.kind]}
    >
      <Ends kind={node.kind} />
      <Label node={node} />
      {accountable && (
        <span className="max-w-full truncate text-xs text-ink-subtle">
          {t.raci.letters.accountable} · {accountable}
        </span>
      )}
      {subprocess && (
        <Link
          to="/processes/$processId"
          params={{ processId: subprocess.id }}
          className="nodrag mt-0.5 max-w-full truncate text-xs text-accent hover:underline"
        >
          {subprocess.name}
        </Link>
      )}
    </article>
  );
}

function DecisionNode({ data, selected }: NodeProps<FlowNodeView>) {
  const { node, flagged } = data;
  const { width, height } = NODE_SIZE.decision;
  const stroke = selected ? "var(--color-accent)" : flagged ? "var(--color-warning)" : "var(--color-border-strong)";
  return (
    <article
      aria-label={`${t.process.kinds.decision}: ${node.name}`}
      className="relative flex items-center justify-center text-center text-sm"
      style={{ width, height }}
    >
      <svg width={width} height={height} className="pointer-events-none absolute inset-0" aria-hidden="true">
        <polygon
          points={`${width / 2},1 ${width - 1},${height / 2} ${width / 2},${height - 1} 1,${height / 2}`}
          fill="var(--color-surface)"
          stroke={stroke}
          strokeWidth={selected ? 2.5 : 1.5}
          strokeDasharray={flagged && !selected ? "5 3" : undefined}
        />
      </svg>
      <Ends kind="decision" />
      <span className="relative max-w-[60%] leading-tight">
        <Label node={node} />
      </span>
    </article>
  );
}

function LaneNode({ data, width, height }: NodeProps<LaneView>) {
  return (
    <section
      aria-label={t.process.lane(data.name)}
      className={`border-r border-border ${data.even ? "bg-surface-sunken/60" : "bg-canvas/60"}`}
      style={{ width, height }}
    >
      <p className="truncate px-6 pt-3 text-sm font-semibold text-ink">
        {data.group && <span className="block text-xs font-normal text-ink-subtle">{data.group}</span>}
        {data.name}
      </p>
    </section>
  );
}

const nodeTypes = { start: TerminalNode, end: TerminalNode, step: StepNode, subprocess: StepNode, decision: DecisionNode, lane: LaneNode };
const LANE = "lane|";

const toneColour: Record<Tone, string> = {
  plain: "var(--color-ink-muted)",
  picked: "var(--color-accent)",
  lit: "var(--color-accent)",
  dim: "var(--color-border-strong)",
  flagged: "var(--color-warning)",
};

/** An arrow along the route the layout gave it, or without one, a stepped line between the sides the canvas chose. */
function FlowEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data, markerEnd, style }: EdgeProps<FlowEdgeView>) {
  const routed = data && data.route.length >= 4 ? routePath(data.route) : undefined;
  const [stepped, stepX, stepY] = getSmoothStepPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, borderRadius: 8, offset: 24 });
  const path = routed?.path ?? stepped;
  const labelX = routed?.labelX ?? stepX;
  const labelY = routed?.labelY ?? stepY;
  const tone = data?.tone ?? "plain";
  return (
    <>
      <BaseEdge id={id} path={path} markerEnd={markerEnd} style={style} interactionWidth={16} />
      {data?.label && (
        <EdgeLabelRenderer>
          <div
            className={`nodrag nopan pointer-events-none absolute rounded border bg-surface px-1.5 py-0.5 text-xs ${tone === "lit" || tone === "picked" ? "border-accent font-medium text-ink" : "border-border text-ink"} ${tone === "dim" ? "opacity-30" : ""}`}
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
          >
            {data.label}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}

const edgeTypes = { flow: FlowEdge };

/** The kinds an editor adds from the palette; a draft begins with its start. */
const PALETTE: NodeKind[] = ["step", "decision", "subprocess", "end", "start"];

/** The kinds of node an editor adds, each put in the middle of the view where it covers no other. */
export function Palette({ flow, onEdit }: { flow: Flow; onEdit: (e: Edit) => void }) {
  const view = useReactFlow();
  const add = (kind: NodeKind) => {
    const pane = document.querySelector(".react-flow")?.getBoundingClientRect();
    const centre = pane ? view.screenToFlowPosition({ x: pane.left + pane.width / 2, y: pane.top + pane.height / 3 }) : { x: 0, y: 0 };
    const size = NODE_SIZE[kind];
    const near = { x: Math.round(centre.x - size.width / 2), y: Math.round(centre.y - size.height / 2) };
    onEdit({ type: "add", kind, key: freshKey(kind, flow), name: t.process.newName[kind], at: freeSpot(flow, kind, near, (k) => NODE_SIZE[k]) });
  };
  return (
    <div role="toolbar" aria-label={t.process.add} className="flex flex-wrap items-center gap-1 border-b border-border bg-surface px-6 py-1.5">
      <span className="mr-1 text-xs text-ink-subtle">{t.process.add}</span>
      {PALETTE.map((kind) => (
        <button key={kind} type="button" onClick={() => add(kind)} className="rounded px-2 py-1 text-sm text-ink hover:bg-surface-sunken">
          {t.process.kinds[kind]}
        </button>
      ))}
    </div>
  );
}

/**
 * A process's graph on a canvas. Editable, nodes are dragged, arrows drawn from a node's lower handle to another's
 * upper one, and what is picked is removed with Delete; every change goes out as an edit.
 */
export function FlowCanvas({
  flow,
  editable,
  selection,
  onSelect,
  onEdit,
  flagged,
  subprocesses,
  fitKey,
  parties,
  lanes = [],
}: {
  flow: Flow;
  editable: boolean;
  selection: Selection;
  onSelect: (s: Selection) => void;
  onEdit: (e: Edit) => void;
  flagged: Set<string>;
  subprocesses: Map<string, string>;
  /** The view is fitted anew whenever this changes: another process or version. */
  fitKey: string;
  /** Names of the roles, positions and units responsibilities name, by party key. */
  parties: Map<string, string>;
  /** Lanes to draw behind the nodes, read-only. */
  lanes?: LaneShown[];
}) {
  const view = useReactFlow();
  // React Flow measures each node once drawn and hides it until then; the sizes are kept apart from the flow.
  const [measured, setMeasured] = useState<Record<string, { width: number; height: number }>>({});
  const [hovered, setHovered] = useState<string | undefined>();

  // A node pointed at or picked brings its arrows and neighbours forward; a picked arrow, the two nodes it joins.
  const focusNode = hovered ?? (selection?.kind === "node" ? selection.key : undefined);
  const focusArrow = !hovered && selection?.kind === "arrow" ? selection.key : undefined;
  const { litArrows, litNodes } = useMemo(() => {
    const arrows = new Set<string>();
    const nodes = new Set<string>();
    for (const a of flow.arrows) {
      if ((focusNode && (a.from === focusNode || a.to === focusNode)) || a.key === focusArrow) {
        arrows.add(a.key);
        nodes.add(a.from).add(a.to);
      }
    }
    if (focusNode) nodes.add(focusNode);
    return { litArrows: arrows, litNodes: nodes };
  }, [flow.arrows, focusNode, focusArrow]);
  const focused = litNodes.size > 0;
  const chosen = useMemo(() => sides(flow), [flow]);

  const nodes: (FlowNodeView | LaneView)[] = useMemo(
    () => [
      ...lanes.map(
        (l, i): LaneView => ({
          id: `${LANE}${l.key}`,
          type: "lane",
          position: { x: l.x, y: 0 },
          width: l.width,
          height: l.height,
          measured: { width: l.width, height: l.height },
          selectable: false,
          draggable: false,
          focusable: false,
          zIndex: -1,
          data: { name: l.name, group: l.group, even: i % 2 === 0 },
        }),
      ),
      ...flow.nodes.map(
        (n): FlowNodeView => ({
          id: n.key,
          type: n.kind,
          position: { x: n.x, y: n.y },
          selected: selection?.kind === "node" && selection.key === n.key,
          measured: measured[n.key],
          style: focused && !litNodes.has(n.key) ? { opacity: 0.35 } : undefined,
          data: {
            node: n,
            flagged: flagged.has(n.key),
            subprocess: n.subprocessId ? { id: n.subprocessId, name: subprocesses.get(n.subprocessId) ?? "" } : undefined,
            accountable:
              n.responsibilities
                .filter((r) => r.kind === "accountable")
                .map((r) => parties.get(partyOf(r)))
                .filter(Boolean)
                .join(", ") || undefined,
          },
        }),
      ),
    ],
    [lanes, flow.nodes, selection, measured, flagged, subprocesses, parties, focused, litNodes],
  );

  const nameOf = new Map(flow.nodes.map((n) => [n.key, n.name || t.process.unnamed]));
  const edges: FlowEdgeView[] = flow.arrows.map((a) => {
    const picked = selection?.kind === "arrow" && selection.key === a.key;
    const tone: Tone = picked ? "picked" : litArrows.has(a.key) ? "lit" : focused ? "dim" : flagged.has(a.key) ? "flagged" : "plain";
    const colour = toneColour[tone];
    const ends = chosen[a.key];
    return {
      id: a.key,
      source: a.from,
      target: a.to,
      sourceHandle: ends?.source ?? "bottom",
      targetHandle: ends?.target ?? "top",
      type: "flow",
      selected: picked,
      zIndex: tone === "lit" || tone === "picked" ? 1 : 0,
      data: { route: a.route, label: a.label, tone },
      style: {
        stroke: colour,
        strokeWidth: tone === "lit" || tone === "picked" ? 2.5 : 1.5,
        strokeDasharray: flagged.has(a.key) ? "5 3" : undefined,
      },
      markerEnd: { type: MarkerType.ArrowClosed, color: colour },
      ariaLabel: t.process.arrowFromTo(nameOf.get(a.from) ?? a.from, nameOf.get(a.to) ?? a.to),
    };
  });

  const fitted = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (fitted.current === fitKey || flow.nodes.length === 0) return;
    fitted.current = fitKey;
    requestAnimationFrame(() => view.fitView({ padding: 0.2, maxZoom: 1.2 }));
  }, [fitKey, flow.nodes.length, view]);

  const onNodesChange = (changes: NodeChange<FlowNodeView | LaneView>[]) => {
    const sizes: Record<string, { width: number; height: number }> = {};
    for (const c of changes) {
      if ("id" in c && c.id.startsWith(LANE)) continue;
      if (c.type === "dimensions" && c.dimensions) sizes[c.id] = c.dimensions;
      if (c.type === "position" && editable) {
        if (c.position) onEdit({ type: "move", key: c.id, to: { x: Math.round(c.position.x), y: Math.round(c.position.y) } });
        if (c.dragging === false) onEdit({ type: "settle" });
      }
    }
    if (Object.keys(sizes).length > 0) setMeasured((was) => ({ ...was, ...sizes }));
  };

  const connect = (c: Connection) => onEdit({ type: "connect", key: freshKey("a", flow), from: c.source, to: c.target });

  return (
    <ReactFlow
      aria-label={t.process.canvas}
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      onNodesChange={onNodesChange}
      onNodeMouseEnter={(_, n) => !n.id.startsWith(LANE) && setHovered(n.id)}
      onNodeMouseLeave={() => setHovered(undefined)}
      onConnect={editable ? connect : undefined}
      isValidConnection={(c) => mayConnect(flow, c.source, c.target)}
      onNodeClick={(_, n) => onSelect(n.id.startsWith(LANE) ? null : { kind: "node", key: n.id })}
      onEdgeClick={(_, e) => onSelect({ kind: "arrow", key: e.id })}
      onPaneClick={() => onSelect(null)}
      onDelete={({ nodes: gone, edges: cut }) => {
        onEdit({ type: "remove", nodes: gone.map((n) => n.id), arrows: cut.map((e) => e.id) });
        onSelect(null);
      }}
      nodesDraggable={editable}
      nodesConnectable={editable}
      deleteKeyCode={editable ? ["Backspace", "Delete"] : null}
      minZoom={0.2}
      proOptions={{ hideAttribution: true }}
    >
      <Background />
      <Controls showInteractive={false} />
    </ReactFlow>
  );
}
