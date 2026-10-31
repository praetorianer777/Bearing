import { Link } from "@tanstack/react-router";
import { useOrganisation } from "@/api/org";
import { type Duty, type DutySubject, type RACI, useDuties } from "@/api/processes";
import { inlineLink, PositionName, Section, today } from "@/features/org/common";
import { t } from "@/i18n";
import { RACIS } from "./raci";

function Step({ duty, asOf, personId, titles }: { duty: Duty; asOf?: string; personId?: string; titles: Map<string, string> }) {
  // A person's duty can be theirs as a deputy; it says for whom they stand in.
  const standing = personId ? duty.people.find((p) => p.personId === personId && p.deputyFor) : undefined;
  return (
    <li>
      <Link to="/processes/$processId" params={{ processId: duty.processId }} search={{ asOf: asOf === today() ? undefined : asOf }} className={inlineLink}>
        {duty.stepName || t.process.unnamed}
      </Link>
      <span className="text-ink-subtle"> · {duty.processName}</span>
      {standing?.deputyFor && (
        <span className="text-ink-subtle">
          {" · "}
          {t.raci.standsInFor} <PositionName id={standing.deputyFor} title={titles.get(standing.deputyFor) ?? t.duties.vacantPosition} />
        </span>
      )}
    </li>
  );
}

/** What a person, position, role or unit is responsible for on a day, kind by kind, across the published processes. */
export function Duties({ subject, asOf }: { subject: DutySubject; asOf: string }) {
  const duties = useDuties(subject, asOf);
  const organisation = useOrganisation(asOf);
  const titles = new Map((organisation.data?.positions ?? []).map((p) => [p.id, p.title]));
  if (!duties.data) return null;
  const byKind = (kind: RACI) => duties.data.filter((d) => d.kind === kind);
  const personId = "personId" in subject ? subject.personId : undefined;
  return (
    <Section title={t.duties.title} empty={t.duties.none}>
      {duties.data.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2">
          {RACIS.filter((k) => byKind(k).length > 0).map((kind) => (
            <div key={kind}>
              <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-ink-subtle">{t.raci.kinds[kind]}</h3>
              <ul className="space-y-1">
                {byKind(kind).map((d) => (
                  <Step key={`${d.processId}-${d.stepKey}-${d.roleId ?? d.positionId ?? d.unitId}`} duty={d} asOf={asOf} personId={personId} titles={titles} />
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </Section>
  );
}
