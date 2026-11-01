import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Background,
  Controls,
  type Edge,
  Handle,
  MarkerType,
  type Node,
  type NodeProps,
  Position as Side,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { createContext, useContext, useEffect, useId, useState } from "react";
import { api, unwrap } from "@/api/client";
import { useOrganisation } from "@/api/org";
import { AsOfPicker, today } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { type Duty, type ExploreNode, explore, nodeId, parseNode, place, RELATIONS, type Relation } from "./graph";

export type ExploreSearch = { node?: string; asOf?: string };

type CardNode = Node<ExploreNode, "explore">;

const Toggle = createContext<(id: string) => void>(() => {});

function NodeLink({ node }: { node: ExploreNode }) {
  const ref = parseNode(node.id);
  const className = "nodrag block truncate text-sm font-semibold text-ink hover:underline";
  switch (ref?.kind) {
    case "unit":
      return (
        <Link to="/units/$unitId" params={{ unitId: ref.id }} className={className}>
          {node.label}
        </Link>
      );
    case "position":
      return (
        <Link to="/positions/$positionId" params={{ positionId: ref.id }} className={className}>
          {node.label}
        </Link>
      );
    case "person":
      return (
        <Link to="/people/$personId" params={{ personId: ref.id }} className={className}>
          {node.label}
        </Link>
      );
    case "role":
      return (
        <Link to="/roles/$roleId" params={{ roleId: ref.id }} className={className}>
          {node.label}
        </Link>
      );
    default:
      return (
        <Link to="/processes/$processId" params={{ processId: ref?.id.split(":")[0] ?? "" }} className={className}>
          {node.label}
        </Link>
      );
  }
}

function Card({ data }: NodeProps<CardNode>) {
  const toggle = useContext(Toggle);
  return (
    <article aria-label={data.label} className="w-60 rounded-lg border border-border-strong bg-surface px-3 py-2 shadow-sm">
      <Handle type="target" position={Side.Left} className="opacity-0" />
      <p className="text-xs text-ink-subtle">{t.explore.kinds[data.kind]}</p>
      <NodeLink node={data} />
      <p className="truncate text-xs text-ink-muted">{data.detail}</p>
      {data.kind !== "step" && (
        <button
          type="button"
          onClick={() => toggle(data.id)}
          aria-expanded={data.expanded}
          aria-label={(data.expanded ? t.explore.fold : t.explore.unfold)(data.label)}
          className="nodrag mt-1 text-xs text-accent hover:underline"
        >
          {data.expanded ? t.explore.foldShort : t.explore.unfoldShort}
        </button>
      )}
      <Handle type="source" position={Side.Right} className="opacity-0" />
    </article>
  );
}

const nodeTypes = { explore: Card };

const edgeColour: Record<Relation, string> = {
  reporting: "var(--color-ink-muted)",
  holdings: "var(--color-accent)",
  roles: "var(--color-chart-3)",
  deputies: "var(--color-warning)",
  units: "var(--color-border-strong)",
  responsibilities: "var(--color-chart-5)",
};

function dutyQuery(id: string, asOf: string) {
  const ref = parseNode(id);
  const param = { unit: "unitId", position: "positionId", person: "personId", role: "roleId" }[ref?.kind as string];
  return {
    queryKey: ["explore-duties", id, asOf],
    enabled: !!param,
    queryFn: async (): Promise<Duty[]> =>
      unwrap(await api.GET("/responsibilities", { params: { query: { [param as string]: ref?.id, asOf } } })).responsibilities,
  };
}

function Graph({ search, onSearch }: { search: ExploreSearch; onSearch: (next: ExploreSearch) => void }) {
  const ids = useId();
  const asOf = search.asOf ?? today();
  const organisation = useOrganisation(asOf);
  const roles = useQuery({ queryKey: ["roles"], queryFn: async () => unwrap(await api.GET("/roles")).roles });
  const start = search.node ?? "";
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set([start]));
  const [relations, setRelations] = useState<ReadonlySet<Relation>>(() => new Set(RELATIONS));
  const toggle = (id: string) =>
    setExpanded((was) => {
      const next = new Set(was);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  const asked = relations.has("responsibilities") ? [...expanded] : [];
  const dutyAnswers = useQueries({ queries: asked.map((id) => dutyQuery(id, asOf)) });
  const duties = new Map(asked.map((id, i) => [id, dutyAnswers[i]?.data ?? []]));
  const graph =
    organisation.data && roles.data ? explore({ ...organisation.data, roles: roles.data }, start, expanded, relations, duties) : { nodes: [], edges: [] };
  const places = place(graph.nodes);
  const nodes: CardNode[] = graph.nodes.map((n) => ({ id: n.id, type: "explore", position: places[n.id] ?? { x: 0, y: 0 }, data: n }));
  const depth = new Map(graph.nodes.map((n) => [n.id, n.depth]));
  // Lines run from the nearer node to the farther, so none loops back; the arrow points the way the relation reads.
  const edges: Edge[] = graph.edges.map((e) => {
    const backwards = (depth.get(e.source) ?? 0) > (depth.get(e.target) ?? 0);
    const arrow = { type: MarkerType.ArrowClosed, color: edgeColour[e.relation] };
    return {
      id: e.id,
      source: backwards ? e.target : e.source,
      target: backwards ? e.source : e.target,
      label: e.label,
      markerEnd: backwards ? undefined : arrow,
      markerStart: backwards ? arrow : undefined,
      style: {
        stroke: edgeColour[e.relation],
        strokeWidth: 1.5,
        strokeDasharray: e.relation === "deputies" || e.label === t.explore.lines.dottedTo ? "6 4" : undefined,
      },
      labelStyle: { fontSize: 11, fill: "var(--color-ink-muted)" },
      labelBgStyle: { fill: "var(--color-surface)" },
    };
  });
  const flow = useReactFlow();
  const shownKey = graph.nodes.map((n) => n.id).join(",");
  // The view follows what is shown: a node opened or a relation switched brings the whole neighbourhood into view.
  useEffect(() => {
    if (!shownKey) return;
    const frame = requestAnimationFrame(() => flow.fitView({ padding: 0.15, maxZoom: 1 }));
    return () => cancelAnimationFrame(frame);
  }, [shownKey, flow]);
  const names = new Map(graph.nodes.map((n) => [n.id, n.label]));
  const data = organisation.data;
  const options = data
    ? [
        ...[...data.units]
          .sort((a, b) => a.name.localeCompare(b.name))
          .map((u) => ({ group: t.explore.kinds.unit, value: nodeId("unit", u.id), label: u.name })),
        ...[...data.positions]
          .sort((a, b) => a.title.localeCompare(b.title))
          .map((p) => ({ group: t.explore.kinds.position, value: nodeId("position", p.id), label: p.title })),
        ...[...data.people]
          .sort((a, b) => a.name.localeCompare(b.name))
          .map((p) => ({ group: t.explore.kinds.person, value: nodeId("person", p.id), label: p.name })),
        ...[...(roles.data ?? [])]
          .sort((a, b) => a.name.localeCompare(b.name))
          .map((r) => ({ group: t.explore.kinds.role, value: nodeId("role", r.id), label: r.name })),
      ]
    : [];
  const groups = [t.explore.kinds.unit, t.explore.kinds.position, t.explore.kinds.person, t.explore.kinds.role];
  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-end gap-3 border-b border-border bg-surface px-6 py-3">
        <div className="mr-auto">
          <h1 className="text-xl font-semibold">{t.explore.title}</h1>
          <p className="text-sm text-ink-muted">{t.explore.intro}</p>
        </div>
        <div className="flex flex-col gap-1 text-xs text-ink-muted">
          <label htmlFor={`${ids}-start`}>{t.explore.start}</label>
          <select
            id={`${ids}-start`}
            value={start}
            onChange={(e) => {
              setExpanded(new Set([e.target.value]));
              onSearch({ ...search, node: e.target.value || undefined });
            }}
            className="h-9 max-w-64 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
          >
            <option value="">{t.explore.choose}</option>
            {groups.map((group) => (
              <optgroup key={group} label={group}>
                {options
                  .filter((o) => o.group === group)
                  .map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
              </optgroup>
            ))}
          </select>
        </div>
        <AsOfPicker value={asOf} onChange={(day) => onSearch({ ...search, asOf: day })} />
      </div>
      <fieldset className="flex flex-wrap gap-4 px-6 py-2 text-sm">
        <legend className="sr-only">{t.explore.relations}</legend>
        {RELATIONS.map((r) => (
          <label key={r} className="flex items-center gap-1.5">
            <input
              type="checkbox"
              checked={relations.has(r)}
              onChange={(e) =>
                setRelations((was) => {
                  const next = new Set(was);
                  if (e.target.checked) next.add(r);
                  else next.delete(r);
                  return next;
                })
              }
            />
            <span className="inline-block h-0.5 w-4" style={{ backgroundColor: edgeColour[r] }} aria-hidden="true" />
            {t.explore.relation[r]}
          </label>
        ))}
      </fieldset>
      {organisation.isError && <p className="px-6 text-sm text-danger">{t.explore.unreachable}</p>}
      {!start && <p className="px-6 py-4 text-sm text-ink-muted">{t.explore.empty}</p>}
      <div className="relative min-h-80 flex-1">
        <Toggle.Provider value={toggle}>
          <ReactFlow
            aria-label={t.explore.title}
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            nodesConnectable={false}
            minZoom={0.2}
            proOptions={{ hideAttribution: true }}
          >
            <Background />
            <Controls showInteractive={false} />
          </ReactFlow>
        </Toggle.Provider>
      </div>
      {graph.edges.length > 0 && (
        <details className="border-t border-border px-6 py-3 text-sm">
          <summary className="cursor-pointer font-medium">{t.explore.asList(graph.edges.length)}</summary>
          <ul aria-label={t.explore.relations} className="mt-2 space-y-1">
            {graph.edges.map((e) => (
              <li key={e.id}>
                {names.get(e.source)} <span className="text-ink-muted">{e.label}</span> {names.get(e.target)}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

/** Any unit, position, person or role and what is around it, opened one relation at a time. */
export function ExplorerPage({ search, onSearch }: { search: ExploreSearch; onSearch: (next: ExploreSearch) => void }) {
  useLanguage();
  return (
    <ReactFlowProvider>
      <Graph key={search.node ?? ""} search={search} onSearch={onSearch} />
    </ReactFlowProvider>
  );
}
