import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { type ReactNode, useId } from "react";
import { atLeast, useMe } from "@/api/auth";
import { api, unwrap } from "@/api/client";
import { t } from "@/i18n";

// Links among other words are underlined, so they do not rest on colour alone.
export const inlineLink = "text-ink underline decoration-border-strong underline-offset-2 hover:decoration-ink";

export const today = () => new Date().toISOString().slice(0, 10);

/** A part of a page under its heading; with nothing to show, it says so. */
export function Section({ title, empty, children }: { title: string; empty?: string; children?: ReactNode }) {
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

export function PersonName({ id, name }: { id: string; name: string }) {
  return (
    <Link to="/people/$personId" params={{ personId: id }} className={inlineLink}>
      {name}
    </Link>
  );
}

export function PositionName({ id, title }: { id: string; title: string }) {
  return (
    <Link to="/positions/$positionId" params={{ positionId: id }} className={inlineLink}>
      {title}
    </Link>
  );
}

export function UnitName({ id, name }: { id: string; name: string }) {
  return (
    <Link to="/units/$unitId" params={{ unitId: id }} className={inlineLink}>
      {name}
    </Link>
  );
}

/** The day a page reads the organisation on, kept in the address. */
export function AsOfPicker({ value, onChange }: { value: string; onChange: (day: string) => void }) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1 text-xs text-ink-muted">
      <label htmlFor={id}>{t.chart.asOf}</label>
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

type Subject = { kind: "people"; id: string } | { kind: "units"; id: string } | { kind: "positions"; id: string };

async function audit(subject: Subject) {
  const query = { limit: "20" };
  switch (subject.kind) {
    case "people":
      return unwrap(await api.GET("/people/{personID}/audit", { params: { path: { personID: subject.id }, query } })).events;
    case "units":
      return unwrap(await api.GET("/units/{unitID}/audit", { params: { path: { unitID: subject.id }, query } })).events;
    case "positions":
      return unwrap(await api.GET("/positions/{positionID}/audit", { params: { path: { positionID: subject.id }, query } })).events;
  }
}

function ChangeList({ subject }: { subject: Subject }) {
  const events = useQuery({ queryKey: ["audit", subject.kind, subject.id], queryFn: () => audit(subject) });
  if (!events.data) return null;
  return (
    <Section title={t.audit.title} empty={t.audit.none}>
      {events.data.length > 0 && (
        <ul className="space-y-1">
          {events.data.map((e) => (
            <li key={e.id} className="flex flex-wrap justify-between gap-2">
              <span className="text-ink">{t.audit.change(e.table, e.operation)}</span>
              <span className="text-ink-subtle">
                {e.actorName ?? t.audit.bearing} · {new Date(e.at).toLocaleString()}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}

/** The latest changes to a unit, a position or a person and to what names it; only editors and administrators read them. */
export function Changes({ subject }: { subject: Subject }) {
  const role = useMe().data?.role;
  return role && atLeast(role, "editor") ? <ChangeList subject={subject} /> : null;
}
