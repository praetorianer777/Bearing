import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { ApiError, api, unwrap } from "@/api/client";
import { type CapacityBy, type CapacityPeriod, type PlanningSubject, useCapacityRange } from "@/api/capacity";
import type { Holding } from "@/api/people";
import type { Person, Position, Unit } from "@/api/org";
import { PersonName, PositionName, UnitName } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { fte } from "./TargetSection";
import { type CsvRow, type Period, periodLabel, periodsOf, short, toCsv } from "./periods";

export type Range = { from: string; to: string; by: CapacityBy };

type Row =
  | { kind: "unit"; key: string; depth: number; unit: Unit; open: boolean; subject: PlanningSubject; name: string }
  | { kind: "position"; key: string; depth: number; position: Position; open: boolean; subject: PlanningSubject; name: string }
  | { kind: "person"; key: string; depth: number; personId: string; subject: PlanningSubject; name: string };

const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name);

/** The rows shown: units from the root down as far as they are open, each open unit's positions after its sub-units, and an open position's holders over the range. */
function rowsOf(
  units: Unit[],
  positions: Position[],
  people: Map<string, Person>,
  open: Set<string>,
  holdings: Map<string, Holding[]>,
  range: Range,
  rootId?: string,
): Row[] {
  const out: Row[] = [];
  const unitRows = (parentId: string | null, depth: number, only?: string) => {
    for (const unit of units.filter((u) => (only ? u.id === only : u.parentId === parentId)).sort(byName)) {
      const key = `unit:${unit.id}`;
      out.push({ kind: "unit", key, depth, unit, open: open.has(key), subject: { unitId: unit.id }, name: unit.name });
      if (!open.has(key)) continue;
      unitRows(unit.id, depth + 1);
      for (const position of positions.filter((p) => p.unitId === unit.id).sort((a, b) => a.title.localeCompare(b.title))) {
        const pkey = `position:${position.id}`;
        out.push({ kind: "position", key: pkey, depth: depth + 1, position, open: open.has(pkey), subject: { positionId: position.id }, name: position.title });
        if (!open.has(pkey)) continue;
        const seen = new Set<string>();
        for (const h of holdings.get(position.id) ?? []) {
          if (h.from > range.to || (h.to != null && h.to <= range.from) || seen.has(h.personId)) continue;
          seen.add(h.personId);
          out.push({
            kind: "person",
            key: `${pkey}/person:${h.personId}`,
            depth: depth + 2,
            personId: h.personId,
            subject: { personId: h.personId },
            name: people.get(h.personId)?.name ?? "",
          });
        }
      }
    }
  };
  unitRows(null, 0, rootId);
  return out;
}

function Cell({ period, row }: { period: CapacityPeriod | undefined; row: Row }) {
  if (!period) return <td className="border-b border-border px-2 py-1.5" />;
  const missing = row.kind !== "person" && short(period);
  const gap = period.target != null && period.fte < period.target ? t.target.short(fte(period.target - period.fte)) : "";
  const vacant = (period.vacant ?? 0) > 0 ? t.gaps.vacant(t.gaps.seats(String(period.vacant))) : "";
  return (
    <td className={`border-b border-border px-2 py-1.5 text-right tabular-nums ${missing ? "bg-warning-subtle font-medium text-ink" : "text-ink"}`}>
      {period.fte.toLocaleString(undefined, { maximumFractionDigits: 2 })}
      {period.target != null && <span className="text-ink-muted"> / {period.target.toLocaleString(undefined, { maximumFractionDigits: 2 })}</span>}
      {missing && <span className="sr-only">{`: ${[gap, vacant].filter(Boolean).join(", ")}`}</span>}
    </td>
  );
}

function GridRow({
  row,
  periods,
  range,
  toggle,
  plan,
}: {
  row: Row;
  periods: Period[];
  range: Range;
  toggle: (key: string) => void;
  plan?: (p: Position) => void;
}) {
  const capacity = useCapacityRange(row.subject, range);
  const byStart = new Map((capacity.data ?? []).map((p) => [p.from, p]));
  return (
    <tr>
      <th
        scope="row"
        className="sticky left-0 z-10 border-b border-border bg-surface px-2 py-1.5 text-left font-normal"
        style={{ paddingLeft: `${0.5 + row.depth * 1.25}rem` }}
      >
        <span className="flex items-center gap-1.5 whitespace-nowrap">
          {row.kind !== "person" ? (
            <button
              type="button"
              aria-expanded={row.open}
              aria-label={(row.open ? t.planning.fold : t.planning.unfold)(row.name)}
              onClick={() => toggle(row.key)}
              className="flex size-5 items-center justify-center rounded text-ink-muted hover:bg-surface-raised"
            >
              {row.open ? "▾" : "▸"}
            </button>
          ) : (
            <span className="size-5" />
          )}
          {row.kind === "unit" && (
            <span className="font-medium">
              <UnitName id={row.unit.id} name={row.name} />
            </span>
          )}
          {row.kind === "position" && <PositionName id={row.position.id} title={row.name} />}
          {row.kind === "person" && <PersonName id={row.personId} name={row.name} />}
          {row.kind === "position" && plan && (
            <button
              type="button"
              onClick={() => plan(row.position)}
              aria-label={t.planning.planFor(row.name)}
              className="ml-1 rounded border border-border-strong px-1.5 text-xs text-ink hover:bg-surface-raised"
            >
              {t.planning.plan}
            </button>
          )}
        </span>
      </th>
      {capacity.isError ? (
        capacity.error instanceof ApiError && capacity.error.status === 403 ? (
          <td colSpan={periods.length} className="border-b border-border px-2 py-1.5 text-ink-muted">
            {t.planning.hidden}
          </td>
        ) : (
          <td colSpan={periods.length} className="border-b border-border px-2 py-1.5 text-danger">
            {t.planning.rowUnreachable}
          </td>
        )
      ) : (
        periods.map((p) => <Cell key={p.from} period={byStart.get(p.from)} row={row} />)
      )}
    </tr>
  );
}

/** Units over weeks or months, each cell what a unit, a position or a person has against its target, gaps marked. */
export function PlanningGrid({
  range,
  units,
  positions,
  people,
  plan,
  rootId,
}: {
  rootId?: string;
  range: Range;
  units: Unit[];
  positions: Position[];
  people: Person[];
  plan?: (p: Position) => void;
}) {
  useLanguage();
  const client = useQueryClient();
  const roots = units.filter((u) => (rootId ? u.id === rootId : u.parentId === null)).map((u) => `unit:${u.id}`);
  const [open, setOpen] = useState<Set<string>>(() => new Set(roots));
  const toggle = (key: string) =>
    setOpen((was) => {
      const next = new Set(was);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  const openPositions = [...open].filter((k) => k.startsWith("position:")).map((k) => k.slice("position:".length));
  const holdingQueries = useQueries({
    queries: openPositions.map((positionId) => ({
      queryKey: ["holdings", positionId],
      queryFn: async () => unwrap(await api.GET("/holdings", { params: { query: { positionId } } })).holdings,
    })),
  });
  const holdings = new Map(openPositions.map((id, i) => [id, holdingQueries[i]?.data ?? []]));
  const names = useMemo(() => new Map(people.map((p) => [p.id, p])), [people]);
  const rows = rowsOf(units, positions, names, open, holdings, range, rootId);
  const periods = periodsOf(range.from, range.to, range.by);

  const download = () => {
    const csvRows: CsvRow[] = rows.map((row) => ({
      kind: row.kind,
      name: row.name,
      periods: client.getQueryData<CapacityPeriod[]>(["capacity", Object.entries(row.subject).flat().join(":"), range.from, range.to, range.by]),
    }));
    const url = URL.createObjectURL(new Blob([toCsv(csvRows)], { type: "text/csv" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = `planning-${range.from}-${range.to}.csv`;
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="flex flex-col gap-2">
      <div className="flex justify-end">
        <button
          type="button"
          onClick={download}
          className="rounded-md border border-border-strong bg-surface px-3 py-1.5 text-sm text-ink hover:bg-surface-raised"
        >
          {t.planning.export}
        </button>
      </div>
      <div className="overflow-x-auto rounded-xl border border-border bg-surface">
        <table className="w-full border-collapse text-sm">
          <caption className="sr-only">{t.planning.caption}</caption>
          <thead>
            <tr>
              <th scope="col" className="sticky left-0 z-10 border-b border-border bg-surface px-2 py-2 text-left font-medium text-ink-muted">
                {t.planning.unit}
              </th>
              {periods.map((p) => (
                <th key={p.from} scope="col" className="whitespace-nowrap border-b border-border px-2 py-2 text-right font-medium text-ink-muted">
                  {periodLabel(p, range.by)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <GridRow key={row.key} row={row} periods={periods} range={range} toggle={toggle} plan={plan} />
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-ink-muted">{t.planning.legend}</p>
    </div>
  );
}
