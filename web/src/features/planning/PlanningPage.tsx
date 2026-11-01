import { useId } from "react";
import { useGaps } from "@/api/capacity";
import { today } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { daysAfter, Gaps } from "./GapList";

export type PlanningSearch = { from?: string; to?: string };

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

/** Where the whole organisation misses people, or has too many, over a range: thirteen weeks from today at first. */
export function PlanningPage({ search, onSearch }: { search: PlanningSearch; onSearch: (next: PlanningSearch) => void }) {
  useLanguage();
  const from = search.from ?? today();
  const to = search.to ?? daysAfter(from, 90);
  const gaps = useGaps({ from, to });
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-end gap-4">
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">{t.planning.title}</h1>
          <p className="mt-1 text-sm text-ink-muted">{t.planning.intro}</p>
        </div>
        <DayField label={t.planning.from} value={from} onChange={(day) => onSearch({ ...search, from: day })} />
        <DayField label={t.planning.to} value={to} onChange={(day) => onSearch({ ...search, to: day })} />
      </header>
      <section aria-labelledby="gaps-heading" className="rounded-xl border border-border bg-surface p-5 text-sm">
        <h2 id="gaps-heading" className="mb-2 text-lg font-semibold">
          {t.gaps.title}
        </h2>
        {gaps.isError ? (
          <p role="alert" className="text-danger">
            {t.gaps.unreachable}
          </p>
        ) : (
          gaps.data && <Gaps gaps={gaps.data} />
        )}
      </section>
    </div>
  );
}
