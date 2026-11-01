import { useId } from "react";
import { type AbsenceSubject, useAbsences } from "@/api/capacity";
import { t } from "@/i18n";

/** The working days nobody is there for a subject, from Armature's days away; editors see it, nobody else. */
export function NobodyThere({ subject, from, to }: { subject: AbsenceSubject; from?: string; to?: string }) {
  const id = useId();
  const absences = useAbsences(subject, { from, to });
  const data = absences.data;
  if (!data || data.hidden) return null;
  return (
    <section aria-labelledby={`${id}-nobody`} className="text-sm">
      <h3 id={`${id}-nobody`} className="mb-1 font-semibold">
        {t.nobodyThere.title}
      </h3>
      {data.partner.status !== "ok" ? (
        <p className="text-ink-muted">{data.partner.message}</p>
      ) : data.gaps.length === 0 ? (
        <p className="text-ink-muted">{t.nobodyThere.none}</p>
      ) : (
        <ul aria-label={t.nobodyThere.title} className="space-y-1">
          {data.gaps.map((g) => (
            <li key={`${g.subjectId}-${g.stepKey ?? ""}-${g.kind ?? ""}-${g.from}`}>
              <span className="font-medium">{g.stepName ? t.nobodyThere.step(g.stepName, g.name, g.kind ?? "") : g.name}</span>
              <span className="text-ink-muted"> · {g.from === g.to ? g.from : t.gaps.span(g.from, g.to)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
