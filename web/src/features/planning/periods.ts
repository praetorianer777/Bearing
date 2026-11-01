import type { CapacityBy, CapacityPeriod } from "@/api/capacity";
import { locale, t } from "@/i18n";

export type Period = { from: string; to: string };

const utc = (day: string) => new Date(`${day}T00:00:00Z`);
const written = (d: Date) => d.toISOString().slice(0, 10);

/** The periods of a range as the API counts them: weeks from Monday or months, the first and last cut by the range. */
export function periodsOf(from: string, to: string, by: CapacityBy): Period[] {
  const out: Period[] = [];
  const end = utc(to);
  for (let start = utc(from); start <= end; ) {
    const next = new Date(start);
    if (by === "week") next.setUTCDate(next.getUTCDate() + 7 - ((next.getUTCDay() + 6) % 7));
    else next.setUTCMonth(next.getUTCMonth() + 1, 1);
    const last = new Date(Math.min(next.getTime() - 86_400_000, end.getTime()));
    out.push({ from: written(start), to: written(last) });
    start = next;
  }
  return out;
}

/** How a period's column is headed: the week's first day, or the month. */
export function periodLabel(period: Period, by: CapacityBy): string {
  const d = utc(period.from);
  return by === "week"
    ? t.planning.week(d.toLocaleDateString(locale(), { day: "numeric", month: "short", timeZone: "UTC" }))
    : d.toLocaleDateString(locale(), { month: "short", year: "numeric", timeZone: "UTC" });
}

/** Whether a period misses people: under its target, or with seats nobody fills. */
export const short = (p: CapacityPeriod) => (p.target != null && p.fte < p.target) || (p.vacant ?? 0) > 0;

const csvField = (value: string) => (/[",\n]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value);

export type CsvRow = { kind: string; name: string; periods: CapacityPeriod[] | undefined };

/** The grid as a CSV file, one line per row and period, so a spreadsheet can sum and chart it. */
export function toCsv(rows: CsvRow[]): string {
  const lines = [["kind", "name", "from", "to", "fte", "target", "seats", "vacant", "people"].join(",")];
  for (const row of rows) {
    for (const p of row.periods ?? []) {
      lines.push(
        [
          row.kind,
          csvField(row.name),
          p.from,
          p.to,
          String(p.fte),
          p.target == null ? "" : String(p.target),
          p.seats == null ? "" : String(p.seats),
          p.vacant == null ? "" : String(p.vacant),
          String(p.people),
        ].join(","),
      );
    }
  }
  return `${lines.join("\n")}\n`;
}
