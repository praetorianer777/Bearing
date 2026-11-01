import { Link } from "@tanstack/react-router";
import { usePositionPage } from "@/api/org";
import type { Link as Reached } from "@/api/people";
import { t, useLanguage } from "@/i18n";
import { TargetSection } from "@/features/planning/TargetSection";
import { Duties } from "@/features/processes/Duties";
import { PositionChanges, useEditor } from "./edit";
import { AsOfPicker, Changes, PersonName, PositionName, RoleName, Section, today, UnitName } from "./common";
import type { DaySearch } from "./UnitPage";

function Reachable({ items }: { items: Reached[] }) {
  return (
    <ul className="space-y-1.5">
      {items.map((l) => (
        <li key={l.positionId} className="flex flex-wrap items-baseline justify-between gap-2">
          <span className="font-medium">
            <PositionName id={l.positionId} title={l.title} />
            {l.kind === "dotted" && <span className="font-normal text-ink-subtle"> · {t.chart.lines.dotted}</span>}
          </span>
          <span className="text-ink-muted">
            {l.holders.length === 0
              ? t.chart.vacant
              : l.holders.map((h, i) => (
                  <span key={h.personId}>
                    {i > 0 && ", "}
                    <PersonName id={h.personId} name={h.name} />
                  </span>
                ))}
          </span>
        </li>
      ))}
    </ul>
  );
}

/** A position as it stands on a day: who holds it and held it, its lines, deputies and roles. */
export function PositionPage({ positionId, search, onSearch }: { positionId: string; search: DaySearch; onSearch: (next: DaySearch) => void }) {
  useLanguage();
  const asOf = search.asOf ?? today();
  const page = usePositionPage(positionId, asOf);
  const editor = useEditor();
  if (page.isError) return <p className="px-6 py-8 text-sm text-danger">{t.position.unreachable}</p>;
  if (!page.data) return null;
  const { position, history, chain, reports, units, positions, people, roles } = page.data;
  const unitName = units.find((u) => u.id === position.unitId)?.name ?? "";
  const titles = new Map(positions.map((p) => [p.id, p.title]));
  const names = new Map(people.map((p) => [p.id, p.name]));
  const roleNames = new Map(roles.map((r) => [r.id, r.name]));
  const standsInFor = positions.filter((p) => p.deputies.includes(position.id));
  const open = position.seats - position.holders.length;
  const holderName = (personId: string) => names.get(personId) ?? t.position.somebody;
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-start gap-4">
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">{position.title}</h1>
          <p className="mt-1 text-sm text-ink-muted">
            <UnitName id={position.unitId} name={unitName} />
            {position.headOfUnit && ` · ${t.unit.headMark}`}
            {` · ${t.position.seats(position.seats)} · ${t.position.existsFrom(position.existsFrom)}`}
          </p>
          {position.archivedOn && <p className="mt-1 text-sm text-warning">{t.position.archivedOn(position.archivedOn)}</p>}
          {position.description && <p className="mt-2 text-sm text-ink">{position.description}</p>}
        </div>
        <AsOfPicker value={asOf} onChange={(day) => onSearch({ asOf: day })} />
      </header>
      <Link to="/organisation" search={{ focus: `unit:${position.unitId}`, asOf: search.asOf }} className="self-start text-sm text-accent hover:underline">
        {t.position.onChart}
      </Link>
      <div className="grid gap-5 md:grid-cols-2">
        <Section title={t.position.holders} empty={t.position.vacant}>
          {position.holders.length > 0 && (
            <ul className="space-y-1">
              {position.holders.map((h) => (
                <li key={h.id} className="flex flex-wrap justify-between gap-2">
                  <PersonName id={h.personId} name={holderName(h.personId)} />
                  <span className="text-ink-subtle">
                    {h.share < 100 && `${t.chart.share(h.share)} · `}
                    {t.person.since(h.from)}
                  </span>
                </li>
              ))}
              {open > 0 && <li className="text-ink-subtle">{t.chart.openSeats(open)}</li>}
            </ul>
          )}
        </Section>
        <Section title={t.person.roles} empty={t.position.noRoles}>
          {position.roles.length > 0 && (
            <ul className="space-y-1">
              {position.roles.map((id) => (
                <li key={id} className="font-medium text-ink">
                  <RoleName id={id} name={roleNames.get(id) ?? ""} />
                </li>
              ))}
            </ul>
          )}
        </Section>
        <Section title={t.person.chain} empty={t.position.noChain}>
          {chain.length > 0 && <Reachable items={chain} />}
        </Section>
        <Section title={t.person.reports} empty={t.position.noReports}>
          {reports.length > 0 && <Reachable items={reports} />}
        </Section>
        <Section title={t.person.deputies} empty={t.position.noDeputies}>
          {position.deputies.length > 0 && (
            <ol className="list-decimal space-y-1 pl-5">
              {position.deputies.map((id) => (
                <li key={id}>
                  <PositionName id={id} title={titles.get(id) ?? ""} />
                </li>
              ))}
            </ol>
          )}
        </Section>
        <Section title={t.person.standsInFor} empty={t.person.standsInForNobody}>
          {standsInFor.length > 0 && (
            <ul className="space-y-1">
              {standsInFor.map((p) => (
                <li key={p.id}>
                  <PositionName id={p.id} title={p.title} />
                </li>
              ))}
            </ul>
          )}
        </Section>
      </div>
      <Section title={t.position.holdersOverTime} empty={t.position.neverHeld}>
        {history.length > 0 && (
          <ul className="space-y-1">
            {[...history]
              .sort((a, b) => b.from.localeCompare(a.from))
              .map((h) => (
                <li key={h.id} className="flex flex-wrap justify-between gap-2">
                  <PersonName id={h.personId} name={holderName(h.personId)} />
                  <span className="text-ink-subtle">
                    {h.share < 100 && `${t.chart.share(h.share)} · `}
                    {t.person.span(h.from, h.to)}
                  </span>
                </li>
              ))}
          </ul>
        )}
      </Section>
      <Section title={t.position.lines} empty={t.position.noLines}>
        {!!position.lines?.length && (
          <ul className="space-y-1">
            {[...position.lines].reverse().map((l) => (
              <li key={`${l.kind}-${l.reportsToId}-${l.from}`} className="flex flex-wrap justify-between gap-2">
                <span>
                  {l.kind === "solid" ? t.chart.lines.solid : t.chart.lines.dotted} <PositionName id={l.reportsToId} title={titles.get(l.reportsToId) ?? ""} />
                </span>
                <span className="text-ink-subtle">{t.person.span(l.from, l.to)}</span>
              </li>
            ))}
          </ul>
        )}
      </Section>
      <TargetSection subject={{ positionId: position.id }} asOf={asOf} />
      <Duties subject={{ positionId: position.id }} asOf={asOf} />
      {editor && <PositionChanges key={`${position.id}-${asOf}`} position={position} positions={positions} people={people} holdings={history} />}
      <Changes subject={{ kind: "positions", id: position.id }} />
    </div>
  );
}
