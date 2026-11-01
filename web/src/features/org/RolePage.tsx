import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { TargetSection } from "@/features/planning/TargetSection";
import { Duties } from "@/features/processes/Duties";
import { t, useLanguage } from "@/i18n";
import { AsOfPicker, PersonName, PositionName, Section, today } from "./common";
import type { DaySearch } from "./UnitPage";

/** A role as it stands on a day: the positions carrying it, who has it, and what it is responsible for. */
export function RolePage({ roleId, search, onSearch }: { roleId: string; search: DaySearch; onSearch: (next: DaySearch) => void }) {
  useLanguage();
  const asOf = search.asOf ?? today();
  const page = useQuery({
    queryKey: ["role-page", roleId, asOf],
    queryFn: async () => {
      const day = { params: { query: { asOf } } };
      const [role, positions, people] = await Promise.all([
        api.GET("/roles/{roleID}", { params: { path: { roleID: roleId }, query: { asOf } } }),
        api.GET("/positions", day),
        api.GET("/people", day),
      ]);
      return { role: unwrap(role).role, positions: unwrap(positions).positions, people: unwrap(people).people };
    },
  });
  if (page.isError) return <p className="px-6 py-8 text-sm text-danger">{t.role.unreachable}</p>;
  if (!page.data) return null;
  const { role, positions, people } = page.data;
  const titles = new Map(positions.map((p) => [p.id, p.title]));
  const names = new Map(people.map((p) => [p.id, p.name]));
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-start gap-4">
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">{role.name}</h1>
          <p className="mt-1 text-sm text-ink-muted">
            {t.role.kind} · <code>{role.key}</code>
          </p>
          {role.description && <p className="mt-2 text-sm text-ink">{role.description}</p>}
        </div>
        <AsOfPicker value={asOf} onChange={(day) => onSearch({ asOf: day })} />
      </header>
      <div className="grid gap-5 md:grid-cols-2">
        <Section title={t.role.carriedBy} empty={t.role.carriedByNone}>
          {!!role.positions?.length && (
            <ul className="space-y-1">
              {role.positions.map((id) => (
                <li key={id}>
                  <PositionName id={id} title={titles.get(id) ?? ""} />
                </li>
              ))}
            </ul>
          )}
        </Section>
        <Section title={t.role.heldBy} empty={t.role.heldByNobody}>
          {!!role.holders?.length && (
            <ul className="space-y-1">
              {role.holders.map((h) => (
                <li key={h.holdingId}>
                  <PersonName id={h.personId} name={names.get(h.personId) ?? ""} />
                  <span className="text-ink-subtle"> · {h.through ? t.person.through(titles.get(h.through) ?? "") : t.person.directRole}</span>
                </li>
              ))}
            </ul>
          )}
        </Section>
      </div>
      <TargetSection subject={{ roleId: role.id }} asOf={asOf} />
      <Duties subject={{ roleId: role.id }} asOf={asOf} />
    </div>
  );
}
