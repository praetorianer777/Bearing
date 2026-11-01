import { Link } from "@tanstack/react-router";
import { type Gap, useGaps } from "@/api/capacity";
import { PositionName, RoleName, Section, UnitName } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { fte } from "./TargetSection";

/** The day a number of days after day, written as days are. */
export const daysAfter = (day: string, days: number) => {
  const d = new Date(`${day}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
};

const seats = (n: number) => t.gaps.seats(n.toLocaleString(undefined, { maximumFractionDigits: 2 }));

function what(gap: Gap): string {
  switch (gap.kind) {
    case "vacant":
      return t.gaps.vacant(seats(gap.size));
    case "under_target":
      return t.gaps.under(fte(gap.size));
    case "over_target":
      return t.gaps.over(fte(gap.size));
    case "unheld":
      return t.gaps.unheld;
    default:
      return t.gaps.single;
  }
}

function Subject({ gap }: { gap: Gap }) {
  switch (gap.subjectKind) {
    case "position":
      return <PositionName id={gap.subjectId} title={gap.name} />;
    case "unit":
      return <UnitName id={gap.subjectId} name={gap.name} />;
    default:
      return <RoleName id={gap.subjectId} name={gap.name} />;
  }
}

/** Gaps as a list, earliest first, each with what is missing and when. */
export function Gaps({ gaps }: { gaps: Gap[] }) {
  useLanguage();
  if (gaps.length === 0) return <p className="text-ink-muted">{t.gaps.none}</p>;
  return (
    <ul aria-label={t.gaps.title} className="space-y-1.5">
      {gaps.map((gap) => (
        <li key={`${gap.kind}-${gap.subjectId}-${gap.from}`} className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5">
          <span>
            <span className="font-medium">
              <Subject gap={gap} />
            </span>
            {" · "}
            {what(gap)}
          </span>
          <span className="text-ink-subtle">{gap.from === gap.to ? gap.from : t.gaps.span(gap.from, gap.to)}</span>
        </li>
      ))}
    </ul>
  );
}

/** A unit's gaps, with everything inside it, over the thirteen weeks from a day. */
export function UnitGaps({ unitId, asOf }: { unitId: string; asOf: string }) {
  useLanguage();
  const gaps = useGaps({ unitId, from: asOf, to: daysAfter(asOf, 90) });
  return (
    <Section title={t.gaps.title}>
      {gaps.isError ? <p className="text-danger">{t.gaps.unreachable}</p> : gaps.data && <Gaps gaps={gaps.data} />}
      <Link to="/planning" search={{ unit: unitId, from: asOf }} className="mt-3 inline-block text-accent hover:underline">
        {t.planning.open}
      </Link>
    </Section>
  );
}
