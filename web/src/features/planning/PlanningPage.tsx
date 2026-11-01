import { useId, useState } from "react";
import { atLeast, useMe } from "@/api/auth";
import type { CapacityBy } from "@/api/capacity";
import { useGaps } from "@/api/capacity";
import { type Position, useOrganisation } from "@/api/org";
import { Link } from "@tanstack/react-router";
import { inlineLink, today } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { DemandSection } from "./DemandSection";
import { daysAfter, Gaps } from "./GapList";
import { NobodyThere } from "./NobodyThere";
import { PlanForm } from "./PlanForm";
import { PlanningGrid } from "./PlanningGrid";

export type PlanningSearch = { from?: string; to?: string; by?: CapacityBy; unit?: string };

function DayField({ label, value, onChange }: { label: string; value: string; onChange: (day: string) => void }) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1 text-xs text-ink-muted">
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        type="date"
        value={value}
        onChange={(event) => event.target.value && onChange(event.target.value)}
        className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
      />
    </div>
  );
}

/** Capacity against target over weeks or months, unit by unit, with the gaps below and hires planned where they show. */
export function PlanningPage({ search, onSearch }: { search: PlanningSearch; onSearch: (next: PlanningSearch) => void }) {
  useLanguage();
  const id = useId();
  const by = search.by ?? "week";
  const from = search.from ?? today();
  const to = search.to ?? daysAfter(from, by === "week" ? 90 : 364);
  const role = useMe().data?.role;
  const editor = !!role && atLeast(role, "editor");
  const org = useOrganisation(from);
  const gaps = useGaps({ from, to, unitId: search.unit });
  const [planning, setPlanning] = useState<Position | null>(null);
  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-end gap-4">
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">{t.planning.title}</h1>
          <p className="mt-1 text-sm text-ink-muted">{t.planning.intro}</p>
        </div>
        <DayField label={t.planning.from} value={from} onChange={(day) => onSearch({ ...search, from: day })} />
        <DayField label={t.planning.to} value={to} onChange={(day) => onSearch({ ...search, to: day })} />
        <div className="flex flex-col gap-1 text-xs text-ink-muted">
          <label htmlFor={`${id}-by`}>{t.planning.by}</label>
          <select
            id={`${id}-by`}
            value={by}
            onChange={(e) => onSearch({ ...search, by: e.target.value as CapacityBy, to: undefined })}
            className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
          >
            <option value="week">{t.planning.weeks}</option>
            <option value="month">{t.planning.months}</option>
          </select>
        </div>
      </header>
      {org.isError ? (
        <p role="alert" className="text-sm text-danger">
          {t.planning.unreachable}
        </p>
      ) : (
        org.data && (
          <PlanningGrid
            key={search.unit ?? ""}
            rootId={search.unit}
            range={{ from, to, by }}
            units={org.data.units}
            positions={org.data.positions}
            people={org.data.people}
            plan={editor ? setPlanning : undefined}
          />
        )
      )}
      {org.data && <DemandSection unitId={search.unit ?? org.data.units.find((u) => !u.parentId)?.id} from={from} />}
      {planning && org.data && <PlanForm key={planning.id} position={planning} people={org.data.people} from={from} onDone={() => setPlanning(null)} />}
      <section aria-labelledby={`${id}-gaps`} className="rounded-xl border border-border bg-surface p-5 text-sm">
        <h2 id={`${id}-gaps`} className="mb-2 text-lg font-semibold">
          {t.gaps.title}
        </h2>
        {search.unit && (
          <p className="mb-2">
            <Link to="/planning" search={{ ...search, unit: undefined }} className={inlineLink}>
              {t.planning.wholeOrganisation}
            </Link>
          </p>
        )}
        {gaps.isError ? (
          <p role="alert" className="text-danger">
            {t.gaps.unreachable}
          </p>
        ) : (
          gaps.data && <Gaps gaps={gaps.data} />
        )}
        <div className="mt-4">
          <NobodyThere subject={{ unitId: search.unit }} />
        </div>
      </section>
    </div>
  );
}
