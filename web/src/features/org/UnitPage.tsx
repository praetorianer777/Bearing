import { Link } from "@tanstack/react-router";
import { type Member, useUnitPage } from "@/api/org";
import { UNIT_ICONS, type UnitIconName } from "@/components/icons";
import { t, useLanguage } from "@/i18n";
import { AsOfPicker, Changes, PersonName, PositionName, Section, today, UnitName } from "./common";

export type DaySearch = { asOf?: string };

function Holders({ members }: { members: Member[] }) {
  if (members.length === 0) return <span className="text-ink-subtle">{t.chart.vacant}</span>;
  return (
    <>
      {members.map((m, i) => (
        <span key={m.holdingId}>
          {i > 0 && ", "}
          <PersonName id={m.personId} name={m.name} />
          {m.share < 100 && <span className="text-ink-subtle"> ({t.chart.share(m.share)})</span>}
        </span>
      ))}
    </>
  );
}

/** A unit as it stands on a day: its type and place in the tree, its head, positions, members and sub-units. */
export function UnitPage({ unitId, search, onSearch }: { unitId: string; search: DaySearch; onSearch: (next: DaySearch) => void }) {
  useLanguage();
  const asOf = search.asOf ?? today();
  const page = useUnitPage(unitId, asOf);
  if (page.isError) return <p className="px-6 py-8 text-sm text-danger">{t.unit.unreachable}</p>;
  if (!page.data) return null;
  const { unit, units, types, positions, members } = page.data;
  const names = new Map(units.map((u) => [u.id, u.name]));
  const type = types.find((ty) => ty.id === unit.typeId);
  const Icon = UNIT_ICONS[(type?.icon ?? "unit") as UnitIconName] ?? UNIT_ICONS.unit;
  const subUnits = units.filter((u) => u.parentId === unit.id).sort((a, b) => a.name.localeCompare(b.name));
  const own = members.filter((m) => m.unitId === unit.id);
  const held = (positionId: string) => own.filter((m) => m.positionId === positionId);
  const head = positions.find((p) => p.headOfUnit);
  const exists = unit.parentId !== null || units.some((u) => u.id === unit.id);
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-start gap-4">
        <span
          className="flex size-12 shrink-0 items-center justify-center rounded-lg"
          style={{ color: type?.colour, backgroundColor: type ? `${type.colour}1f` : undefined }}
        >
          <Icon />
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">
            {unit.name}
            {unit.shortName && <span className="ml-2 text-base font-normal text-ink-subtle">{unit.shortName}</span>}
          </h1>
          <p className="mt-1 text-sm text-ink-muted">
            {type?.name}
            {unit.parentId && (
              <>
                {" · "}
                {t.unit.partOf} <UnitName id={unit.parentId} name={names.get(unit.parentId) ?? ""} />
              </>
            )}
            {!unit.parentId && exists && ` · ${t.unit.root}`}
          </p>
          {unit.archivedOn && <p className="mt-1 text-sm text-warning">{t.unit.archivedOn(unit.archivedOn)}</p>}
          {!exists && !unit.archivedOn && <p className="mt-1 text-sm text-warning">{t.unit.notThere}</p>}
          {unit.description && <p className="mt-2 text-sm text-ink">{unit.description}</p>}
        </div>
        <AsOfPicker value={asOf} onChange={(day) => onSearch({ asOf: day })} />
      </header>
      <Link to="/organisation" search={{ focus: `unit:${unit.id}`, asOf: search.asOf }} className="self-start text-sm text-accent hover:underline">
        {t.unit.onChart}
      </Link>
      <div className="grid gap-5 md:grid-cols-2">
        <Section title={t.unit.head} empty={t.unit.noHead}>
          {head && (
            <p>
              <span className="font-medium">
                <PositionName id={head.id} title={head.title} />
              </span>
              {" · "}
              <Holders members={held(head.id)} />
            </p>
          )}
        </Section>
        <Section title={t.unit.subUnits} empty={t.unit.noSubUnits}>
          {subUnits.length > 0 && (
            <ul className="space-y-1">
              {subUnits.map((u) => (
                <li key={u.id}>
                  <UnitName id={u.id} name={u.name} />
                </li>
              ))}
            </ul>
          )}
        </Section>
      </div>
      <Section title={t.unit.positions} empty={t.unit.noPositions}>
        {positions.length > 0 && (
          <ul className="space-y-1.5">
            {positions.map((p) => (
              <li key={p.id} className="flex flex-wrap items-baseline justify-between gap-2">
                <span className="font-medium">
                  <PositionName id={p.id} title={p.title} />
                  {p.headOfUnit && <span className="font-normal text-ink-subtle"> · {t.unit.headMark}</span>}
                </span>
                <span>
                  <Holders members={held(p.id)} />
                </span>
              </li>
            ))}
          </ul>
        )}
      </Section>
      <Section title={t.unit.members(members.length)} empty={t.unit.noMembers}>
        {members.length > 0 && (
          <ul className="grid gap-x-6 gap-y-1 sm:grid-cols-2">
            {members.map((m) => (
              <li key={m.holdingId} className="min-w-0 truncate">
                <PersonName id={m.personId} name={m.name} />
                <span className="text-ink-subtle">
                  {" · "}
                  {m.title}
                  {m.unitId !== unit.id && ` · ${names.get(m.unitId) ?? ""}`}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Section>
      <Section title={t.unit.placements} empty={t.unit.noPlacements}>
        {!!unit.placements?.length && (
          <ul className="space-y-1">
            {[...unit.placements].reverse().map((p) => (
              <li key={p.from} className="flex flex-wrap justify-between gap-2">
                <span>{p.parentId ? t.unit.inside(names.get(p.parentId) ?? t.unit.elsewhere) : t.unit.atRoot}</span>
                <span className="text-ink-subtle">{t.person.span(p.from, p.to)}</span>
              </li>
            ))}
          </ul>
        )}
      </Section>
      <Changes subject={{ kind: "units", id: unit.id }} />
    </div>
  );
}
