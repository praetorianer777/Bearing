import {
  applyNodeChanges,
  Background,
  Controls,
  type Edge,
  getNodesBounds,
  Handle,
  MarkerType,
  type Node,
  type NodeChange,
  type NodeProps,
  Position as Side,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Link } from "@tanstack/react-router";
import { createContext, useContext, useEffect, useId, useMemo, useRef, useState } from "react";
import { useOrganisation } from "@/api/org";
import { Button } from "@/components/ui";
import { LoadExampleData } from "@/features/settings/ExampleData";
import { t, useLanguage } from "@/i18n";
import { exportChart } from "./exportImage";
import { CARD_WIDTH, type Card, type Focus, chartGraph, type LineKind } from "./graph";
import { layout, type Point, readPlaces, writePlaces } from "./layout";

type CardNode = Node<Card, "position">;

/** What a card can ask of the chart around it: to fold its unit, or to focus on somebody holding it; and the day its links read. */
const ChartActions = createContext<{ toggle: (unitId: string) => void; focusOn: (personId: string) => void; asOf?: string }>({
  toggle: () => {},
  focusOn: () => {},
});

function PositionCard({ data }: NodeProps<CardNode>) {
  const { toggle, focusOn, asOf } = useContext(ChartActions);
  const open = data.seats - data.holders.length;
  return (
    <article
      aria-label={data.title}
      className={`rounded-lg border bg-surface px-3 py-2 text-left shadow-sm ${data.vacant ? "border-dashed border-warning" : "border-border-strong"}`}
      style={{ width: CARD_WIDTH }}
    >
      <Handle type="target" position={Side.Top} className="opacity-0" />
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <Link
            to="/positions/$positionId"
            params={{ positionId: data.id }}
            search={{ asOf }}
            className="nodrag block truncate text-sm font-semibold text-ink hover:underline"
          >
            {data.title}
          </Link>
          <p className="truncate text-xs text-ink-subtle">{data.unitName}</p>
        </div>
        {data.vacant && <span className="shrink-0 rounded bg-warning-subtle px-1.5 py-0.5 text-xs font-medium text-ink">{t.chart.vacant}</span>}
      </div>
      <ul className="mt-1.5 space-y-0.5">
        {data.holders.map((h) => (
          <li key={h.personId} className="flex items-baseline justify-between gap-2 text-sm">
            <button
              type="button"
              onClick={() => focusOn(h.personId)}
              className="nodrag truncate text-ink-muted hover:text-ink hover:underline"
              aria-label={t.chart.focusOn(h.name)}
            >
              {h.name}
            </button>
            {h.share < 100 && <span className="shrink-0 text-xs text-ink-subtle">{t.chart.share(h.share)}</span>}
          </li>
        ))}
        {!data.vacant && open > 0 && <li className="text-xs text-ink-subtle">{t.chart.openSeats(open)}</li>}
      </ul>
      {data.collapsible && (
        <button
          type="button"
          onClick={() => toggle(data.unitId)}
          aria-expanded={!data.collapsed}
          aria-label={data.collapsed ? t.chart.expand(data.unitName) : t.chart.collapse(data.unitName)}
          className="nodrag mt-1.5 text-xs text-accent hover:underline"
        >
          {data.collapsed ? t.chart.folded(data.hidden) : t.chart.fold}
        </button>
      )}
      <Handle type="source" position={Side.Bottom} className="opacity-0" />
    </article>
  );
}

const nodeTypes = { position: PositionCard };

const lineStyle: Record<LineKind, Edge["style"]> = {
  solid: { stroke: "var(--color-ink-muted)", strokeWidth: 1.5 },
  dotted: { stroke: "var(--color-accent)", strokeWidth: 1.5, strokeDasharray: "6 4" },
  deputy: { stroke: "var(--color-warning)", strokeWidth: 1.5, strokeDasharray: "2 4" },
};

// A whole organisation fitted into the window would be too small to read; it starts at a zoom one can read, and the
// rest is a pan or a zoom away.
const MIN_FIT_ZOOM = 0.45;
const TOP_MARGIN = 24;

const today = () => new Date().toISOString().slice(0, 10);

export type ChartSearch = { asOf?: string; focus?: string };

/** Reads "unit:<id>" or "person:<id>", as the address keeps a focus. */
export function parseFocus(raw?: string): Focus | undefined {
  const [kind, id] = (raw ?? "").split(":");
  return (kind === "unit" || kind === "person") && id ? { kind, id } : undefined;
}

function Chart({ search, onSearch }: { search: ChartSearch; onSearch: (next: ChartSearch) => void }) {
  const asOf = search.asOf ?? today();
  const organisation = useOrganisation(asOf);
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [deputies, setDeputies] = useState(false);
  const [kept, setKept] = useState<Record<string, Point>>(readPlaces);
  // The layout reads where cards were moved to when it runs, but moving one does not lay the chart out anew.
  const keptRef = useRef(kept);
  keptRef.current = kept;
  const [nodes, setNodes] = useState<CardNode[]>([]);
  const flow = useReactFlow();
  const ids = useId();

  const graph = useMemo(
    () => (organisation.data ? chartGraph(organisation.data, { collapsed, focus: parseFocus(search.focus), deputies }) : { cards: [], lines: [] }),
    [organisation.data, collapsed, search.focus, deputies],
  );

  // The view is fitted to a new day or focus; folding a unit or showing deputies leaves it where the person has it,
  // so what they clicked stays under their hand.
  const view = `${asOf}|${search.focus ?? ""}`;
  const fittedFor = useRef<string | undefined>(undefined);
  useEffect(() => {
    let current = true;
    layout(graph.cards, graph.lines, keptRef.current).then((places) => {
      if (!current) return;
      setNodes(graph.cards.map((card) => ({ id: card.id, type: "position", position: places[card.id] ?? { x: 0, y: 0 }, data: card })));
      if (fittedFor.current === view || graph.cards.length === 0) return;
      fittedFor.current = view;
      requestAnimationFrame(async () => {
        await flow.fitView({ padding: 0.1, minZoom: MIN_FIT_ZOOM });
        // An org chart reads from the top down, so it starts at its top, not centred in the window.
        const { x, zoom } = flow.getViewport();
        flow.setViewport({ x, y: TOP_MARGIN - getNodesBounds(flow.getNodes()).y * zoom, zoom });
      });
    });
    return () => {
      current = false;
    };
  }, [graph, flow, view]);

  const edges: Edge[] = graph.lines.map((line) => ({
    id: line.id,
    source: line.source,
    target: line.target,
    type: line.kind === "solid" ? "smoothstep" : "default",
    style: lineStyle[line.kind],
    markerEnd: line.kind === "deputy" ? { type: MarkerType.ArrowClosed, color: "var(--color-warning)" } : undefined,
    zIndex: line.kind === "solid" ? 0 : 1,
  }));

  const actions = useMemo(
    () => ({
      toggle: (unitId: string) =>
        setCollapsed((was) => {
          const next = new Set(was);
          if (!next.delete(unitId)) next.add(unitId);
          return next;
        }),
      focusOn: (personId: string) => onSearch({ ...search, focus: `person:${personId}` }),
      asOf: search.asOf,
    }),
    [onSearch, search],
  );

  const onNodesChange = (changes: NodeChange<CardNode>[]) => setNodes((was) => applyNodeChanges(changes, was));
  const keep = (_: unknown, node: CardNode) => {
    const next = { ...kept, [node.id]: node.position };
    setKept(next);
    writePlaces(next);
  };
  const resetLayout = () => {
    setKept({});
    writePlaces({});
    layout(graph.cards, graph.lines, {}).then((places) => setNodes((was) => was.map((n) => ({ ...n, position: places[n.id] ?? n.position }))));
  };

  const data = organisation.data;
  const units = [...(data?.units ?? [])].sort((a, b) => a.name.localeCompare(b.name));
  const people = [...(data?.people ?? [])].sort((a, b) => a.name.localeCompare(b.name));
  const fileName = `organisation-${asOf}`;

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-end gap-3 border-b border-border bg-surface px-6 py-3">
        <h1 className="mr-auto text-xl font-semibold">{t.chart.title}</h1>
        <div className="flex flex-col gap-1 text-xs text-ink-muted">
          <label htmlFor={`${ids}-as-of`}>{t.chart.asOf}</label>
          <input
            id={`${ids}-as-of`}
            type="date"
            value={asOf}
            onChange={(event) => event.target.value && onSearch({ ...search, asOf: event.target.value })}
            className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
          />
        </div>
        <div className="flex flex-col gap-1 text-xs text-ink-muted">
          <label htmlFor={`${ids}-focus`}>{t.chart.focus}</label>
          <select
            id={`${ids}-focus`}
            value={search.focus ?? ""}
            onChange={(event) => onSearch({ ...search, focus: event.target.value || undefined })}
            className="h-9 max-w-64 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
          >
            <option value="">{t.chart.everything}</option>
            <optgroup label={t.chart.units}>
              {units.map((u) => (
                <option key={u.id} value={`unit:${u.id}`}>
                  {u.name}
                </option>
              ))}
            </optgroup>
            <optgroup label={t.chart.people}>
              {people.map((p) => (
                <option key={p.id} value={`person:${p.id}`}>
                  {p.name}
                </option>
              ))}
            </optgroup>
          </select>
        </div>
        <label className="flex h-9 items-center gap-2 text-sm text-ink">
          <input type="checkbox" checked={deputies} onChange={(event) => setDeputies(event.target.checked)} />
          {t.chart.deputies}
        </label>
        <Button variant="secondary" onClick={() => exportChart(flow.getNodes(), "png", fileName)} disabled={nodes.length === 0}>
          {t.chart.exportPng}
        </Button>
        <Button variant="secondary" onClick={() => exportChart(flow.getNodes(), "svg", fileName)} disabled={nodes.length === 0}>
          {t.chart.exportSvg}
        </Button>
        <Button variant="secondary" onClick={resetLayout} disabled={Object.keys(kept).length === 0}>
          {t.chart.resetLayout}
        </Button>
      </div>
      <ul aria-label={t.chart.legend} className="flex flex-wrap gap-4 px-6 py-2 text-xs text-ink-muted">
        {(["solid", "dotted", "deputy"] as const).map((kind) => (
          <li key={kind} className="flex items-center gap-1.5">
            <svg width="28" height="6" aria-hidden="true">
              <line x1="0" y1="3" x2="28" y2="3" style={lineStyle[kind]} />
            </svg>
            {t.chart.lines[kind]}
          </li>
        ))}
      </ul>
      <div className="relative min-h-0 flex-1">
        {organisation.isError && <p className="p-6 text-sm text-danger">{t.chart.unreachable}</p>}
        {data && data.units.length === 0 && (
          <div className="relative z-10 flex flex-col items-start gap-3 p-6">
            <p className="text-sm text-ink-muted">{t.chart.empty}</p>
            <LoadExampleData />
          </div>
        )}
        <ChartActions.Provider value={actions}>
          <ReactFlow
            aria-label={t.chart.title}
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onNodeDragStop={keep}
            nodesConnectable={false}
            minZoom={0.1}
            proOptions={{ hideAttribution: true }}
          >
            <Background />
            <Controls showInteractive={false} />
          </ReactFlow>
        </ChartActions.Provider>
      </div>
    </div>
  );
}

/** The organisation drawn as a chart of positions and their holders, as it stands on a day. */
export function OrgChartPage({ search, onSearch }: { search: ChartSearch; onSearch: (next: ChartSearch) => void }) {
  useLanguage();
  return (
    <ReactFlowProvider>
      <Chart search={search} onSearch={onSearch} />
    </ReactFlowProvider>
  );
}
