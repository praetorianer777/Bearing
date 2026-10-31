import { Link } from "@tanstack/react-router";
import { type ReactNode, useId } from "react";
import { atLeast, useMe } from "@/api/auth";
import { type Link as Reached, usePersonAudit, usePersonPage } from "@/api/people";
import { t, useLanguage } from "@/i18n";
import { type Held, personView, type Seat } from "./view";

export type PersonSearch = { asOf?: string };

// Links among other words are underlined, so they do not rest on colour alone.
const inlineLink = "text-ink underline decoration-border-strong underline-offset-2 hover:decoration-ink";

const today = () => new Date().toISOString().slice(0, 10);

/** A part of the page under its heading; with nothing to show, it says so. */
function Section({ title, empty, children }: { title: string; empty?: string; children?: ReactNode }) {
  const id = useId();
  return (
    <section aria-labelledby={id} className="rounded-xl border border-border bg-surface p-5">
      <h2 id={id} className="text-lg font-semibold">
        {title}
      </h2>
      <div className="mt-2 text-sm">{children || <p className="text-ink-muted">{empty}</p>}</div>
    </section>
  );
}

function PersonName({ id, name }: { id: string; name: string }) {
  return (
    <Link to="/people/$personId" params={{ personId: id }} className={inlineLink}>
      {name}
    </Link>
  );
}

function HeldList({ items, dated }: { items: Held[]; dated?: boolean }) {
  return (
    <ul className="space-y-1.5">
      {items.map((h) => (
        <li key={h.holdingId} className="flex flex-wrap items-baseline justify-between gap-2">
          <span>
            <span className="font-medium text-ink">{h.title}</span>
            {h.unitName && h.unitId && (
              <>
                {" · "}
                <Link to="/organisation" search={{ focus: `unit:${h.unitId}` }} className={inlineLink}>
                  {h.unitName}
                </Link>
              </>
            )}
            {h.kind === "role" && <span className="text-ink-subtle"> · {t.person.directRole}</span>}
          </span>
          <span className="text-ink-subtle">
            {h.share < 100 && `${t.chart.share(h.share)} · `}
            {dated ? t.person.span(h.from, h.to) : t.person.since(h.from)}
          </span>
        </li>
      ))}
    </ul>
  );
}

function Positions({ items }: { items: Reached[] }) {
  return (
    <ul className="space-y-1.5">
      {items.map((l) => (
        <li key={l.positionId} className="flex flex-wrap items-baseline justify-between gap-2">
          <span className="font-medium text-ink">
            {l.title}
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

function Seats({ items }: { items: Seat[] }) {
  return (
    <ul className="space-y-1">
      {items.map((s) => (
        <li key={s.positionId}>
          <span className="font-medium text-ink">{s.title}</span>
          <span className="text-ink-muted"> · {s.holders.length ? s.holders.join(", ") : t.chart.vacant}</span>
        </li>
      ))}
    </ul>
  );
}

function Changes({ personId }: { personId: string }) {
  const events = usePersonAudit(personId, true);
  if (!events.data) return null;
  return (
    <Section title={t.person.changes} empty={t.person.noChanges}>
      {events.data.length > 0 && (
        <ul className="space-y-1">
          {events.data.map((e) => (
            <li key={e.id} className="flex flex-wrap justify-between gap-2">
              <span className="text-ink">{t.person.change(e.table, e.operation)}</span>
              <span className="text-ink-subtle">
                {e.actorName ?? t.person.bearing} · {new Date(e.at).toLocaleString()}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}

/** Everything about a person in one place, as it stands on a day. */
export function PersonPage({ personId, search, onSearch }: { personId: string; search: PersonSearch; onSearch: (next: PersonSearch) => void }) {
  useLanguage();
  const ids = useId();
  const asOf = search.asOf ?? today();
  const page = usePersonPage(personId, asOf);
  const role = useMe().data?.role;
  if (page.isError) return <p className="px-6 py-8 text-sm text-danger">{t.person.unreachable}</p>;
  if (!page.data) return null;
  const { person, chain, reports } = page.data;
  const view = personView(page.data);
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-start gap-4">
        {person.hasPhoto && <img src={`/api/v1/people/${person.id}/photo`} alt="" className="size-16 rounded-full object-cover" />}
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">{person.name}</h1>
          <p className="mt-1 text-sm text-ink-muted">
            {[t.person.kinds[person.kind], person.email, person.phone, person.location].filter(Boolean).join(" · ")}
          </p>
          {person.leftOn && <p className="mt-1 text-sm text-warning">{t.person.leftOn(person.leftOn)}</p>}
        </div>
        <div className="flex flex-col gap-1 text-xs text-ink-muted">
          <label htmlFor={`${ids}-as-of`}>{t.chart.asOf}</label>
          <input
            id={`${ids}-as-of`}
            type="date"
            value={asOf}
            onChange={(event) => event.target.value && onSearch({ asOf: event.target.value })}
            className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
          />
        </div>
      </header>
      <Link to="/organisation" search={{ focus: `person:${person.id}`, asOf: search.asOf }} className="self-start text-sm text-accent hover:underline">
        {t.person.onChart}
      </Link>
      <div className="grid gap-5 md:grid-cols-2">
        <Section title={t.person.holds} empty={t.person.holdsNothing}>
          {view.current.length > 0 && <HeldList items={view.current} />}
        </Section>
        <Section title={t.person.roles} empty={t.person.noRoles}>
          {view.roles.length > 0 && (
            <ul className="space-y-1">
              {view.roles.map((r) => (
                <li key={`${r.roleId}-${r.through ?? ""}`}>
                  <span className="font-medium text-ink">{r.name}</span>
                  <span className="text-ink-subtle"> · {r.through ? t.person.through(r.through) : t.person.directRole}</span>
                </li>
              ))}
            </ul>
          )}
        </Section>
        <Section title={t.person.chain} empty={t.person.noChain}>
          {chain.length > 0 && <Positions items={chain} />}
        </Section>
        <Section title={t.person.reports} empty={t.person.noReports}>
          {reports.length > 0 && <Positions items={reports} />}
        </Section>
        <Section title={t.person.deputies} empty={t.person.noDeputies}>
          {view.deputies.length > 0 &&
            view.deputies.map((d) => (
              <div key={d.for} className="mb-2 last:mb-0">
                <p className="text-ink-subtle">{t.person.deputiesFor(d.for)}</p>
                <Seats items={d.by} />
              </div>
            ))}
        </Section>
        <Section title={t.person.standsInFor} empty={t.person.standsInForNobody}>
          {view.standsInFor.length > 0 && <Seats items={view.standsInFor} />}
        </Section>
      </div>
      <Section title={t.person.history} empty={t.person.noHistory}>
        {view.history.length > 0 && <HeldList items={view.history} dated />}
      </Section>
      {role && atLeast(role, "editor") && <Changes personId={person.id} />}
    </div>
  );
}
