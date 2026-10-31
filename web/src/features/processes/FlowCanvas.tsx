import {
  Background,
  type Connection,
  Controls,
  type Edge,
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

/** What is picked on the canvas: one node or one arrow, by key. */
export type Selection = { kind: "node" | "arrow"; key: string } | null;

type NodeData = { node: FlowNode; flagged: boolean; subprocess?: { id: string; name: string } };
type FlowNodeView = Node<NodeData>;

const tone = (flagged: boolean, selected: boolean) =>
  selected ? "border-accent ring-2 ring-accent/40" : flagged ? "border-warning border-dashed" : "border-border-strong";

function Ends({ kind }: { kind: NodeKind }) {
  return (
    <>
      {kind !== "start" && <Handle type="target" position={Side.Top} className="size-2.5! bg-ink-muted!" />}
      {kind !== "end" && <Handle type="source" position={Side.Bottom} className="size-2.5! bg-ink-muted!" />}
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
  const { node, flagged, subprocess } = data;
  return (
    <article
      aria-label={`${t.process.kinds[node.kind]}: ${node.name}`}
      className={`flex flex-col items-center justify-center rounded-md bg-surface px-3 text-center text-sm shadow-sm ${node.kind === "subprocess" ? "border-4 border-double" : "border"} ${tone(flagged, selected)}`}
      style={NODE_SIZE[node.kind]}
    >
      <Ends kind={node.kind} />
      <Label node={node} />
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

const nodeTypes = { start: TerminalNode, end: TerminalNode, step: StepNode, subprocess: StepNode, decision: DecisionNode };

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
}) {
  const view = useReactFlow();
  // React Flow measures each node once drawn and hides it until then; the sizes are kept apart from the flow.
  const [measured, setMeasured] = useState<Record<string, { width: number; height: number }>>({});

  const nodes: FlowNodeView[] = useMemo(
    () =>
      flow.nodes.map((n) => ({
        id: n.key,
        type: n.kind,
        position: { x: n.x, y: n.y },
        selected: selection?.kind === "node" && selection.key === n.key,
        measured: measured[n.key],
        data: {
          node: n,
          flagged: flagged.has(n.key),
          subprocess: n.subprocessId ? { id: n.subprocessId, name: subprocesses.get(n.subprocessId) ?? "" } : undefined,
        },
      })),
    [flow.nodes, selection, measured, flagged, subprocesses],
  );

  const nameOf = new Map(flow.nodes.map((n) => [n.key, n.name || t.process.unnamed]));
  const edges: Edge[] = flow.arrows.map((a) => {
    const picked = selection?.kind === "arrow" && selection.key === a.key;
    const colour = picked ? "var(--color-accent)" : flagged.has(a.key) ? "var(--color-warning)" : "var(--color-ink-muted)";
    return {
      id: a.key,
      source: a.from,
      target: a.to,
      type: "smoothstep",
      label: a.label || undefined,
      selected: picked,
      style: { stroke: colour, strokeWidth: picked ? 2.5 : 1.5, strokeDasharray: flagged.has(a.key) ? "5 3" : undefined },
      markerEnd: { type: MarkerType.ArrowClosed, color: colour },
      labelBgPadding: [6, 3],
      labelBgBorderRadius: 4,
      labelStyle: { fill: "var(--color-ink)", fontSize: 12 },
      labelBgStyle: { fill: "var(--color-surface)" },
      ariaLabel: t.process.arrowFromTo(nameOf.get(a.from) ?? a.from, nameOf.get(a.to) ?? a.to),
    };
  });

  const fitted = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (fitted.current === fitKey || flow.nodes.length === 0) return;
    fitted.current = fitKey;
    requestAnimationFrame(() => view.fitView({ padding: 0.2, maxZoom: 1.2 }));
  }, [fitKey, flow.nodes.length, view]);

  const onNodesChange = (changes: NodeChange<FlowNodeView>[]) => {
    const sizes: Record<string, { width: number; height: number }> = {};
    for (const c of changes) {
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
      onNodesChange={onNodesChange}
      onConnect={editable ? connect : undefined}
      isValidConnection={(c) => mayConnect(flow, c.source, c.target)}
      onNodeClick={(_, n) => onSelect({ kind: "node", key: n.id })}
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
