import { useId } from "react";
import { type Demand, useDemand } from "@/api/capacity";
import { t } from "@/i18n";
import { daysAfter } from "./GapList";

/** How many weeks the demand shows: thirteen, as Bearing plans by default; Armature answers at most 26. */
export const DEMAND_WEEKS = 13;

const figure = (n: number | null | undefined) => (n === null || n === undefined ? "" : String(n));

function DemandTable({ demand }: { demand: Demand }) {
  return (
    // biome-ignore lint/a11y/noNoninteractiveTabindex: a table wider than a phone scrolls, and the keyboard has to reach it.
    <section className="overflow-x-auto" aria-label={t.demand.caption} tabIndex={0}>
      <table className="w-full text-sm">
        <caption className="sr-only">{t.demand.caption}</caption>
        <thead className="text-left text-xs text-ink-muted">
          <tr>
            {[t.demand.week, t.demand.fte, t.demand.target, t.demand.allocated, t.demand.load, t.demand.nominal, t.demand.away].map((label) => (
              <th key={label} scope="col" className="px-2 py-1 font-medium">
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {demand.weeks.map((w) => (
            <tr key={w.start} className="border-t border-border">
              <th scope="row" className="px-2 py-1 text-left font-normal">
                {w.start}
              </th>
              <td className="px-2 py-1">{w.fte}</td>
              <td className="px-2 py-1">{figure(w.target)}</td>
              <td className={`px-2 py-1 ${w.allocatedFte !== null && w.allocatedFte > w.fte ? "font-semibold text-danger" : ""}`}>{figure(w.allocatedFte)}</td>
              <td className="px-2 py-1">{figure(w.loadHours)}</td>
              <td className="px-2 py-1">{figure(w.nominalHours)}</td>
              <td className="px-2 py-1">{figure(w.daysAway)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

/** A unit's capacity against Armature's demand, or why Armature is not asked. */
export function DemandSection({ unitId, from }: { unitId: string | undefined; from: string }) {
  const id = useId();
  const demand = useDemand(unitId, { from, to: daysAfter(from, DEMAND_WEEKS * 7 - 1) });
  const data = demand.data;
  return (
    <section aria-labelledby={`${id}-demand`} className="rounded-xl border border-border bg-surface p-5 text-sm">
      <h2 id={`${id}-demand`} className="mb-1 text-lg font-semibold">
        {t.demand.title}
      </h2>
      <p className="mb-3 text-ink-muted">{t.demand.intro}</p>
      {demand.isError && (
        <p role="alert" className="text-danger">
          {t.demand.unreachable}
        </p>
      )}
      {data && data.partner.status !== "ok" && (
        <p role="status" className="mb-3 rounded-md bg-surface-sunken p-3">
          {data.partner.message}
        </p>
      )}
      {data && <DemandTable demand={data} />}
      {data && data.warnings > 0 && (
        <div className="mt-3">
          <h3 className="font-medium">{t.demand.warnings(data.warnings)}</h3>
          {data.over.length > 0 ? (
            <ul className="mt-1 list-disc pl-5">
              {data.over.map((o) => (
                <li key={`${o.personId}-${o.week}`}>
                  {o.reason === "over_allocated"
                    ? t.demand.overAllocated(o.name, o.week, o.allocationPercent)
                    : t.demand.overLoaded(o.name, o.week, o.loadHours, o.nominalHours, o.share)}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-ink-muted">{t.demand.namedToEditors}</p>
          )}
        </div>
      )}
      {data && data.unmatched.length > 0 && <p className="mt-3 text-ink-muted">{t.demand.unmatched(data.unmatched.map((m) => m.name).join(", "))}</p>}
    </section>
  );
}
