import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { usePeople } from "@/api/people";
import { AddPerson, useEditor } from "@/features/org/edit";
import { t, useLanguage } from "@/i18n";

export type PeopleSearch = { q?: string; left?: boolean };

const today = () => new Date().toISOString().slice(0, 10);

/** Everybody in the organisation today, or those who have left, narrowed by name or address. */
export function PeoplePage({ search, onSearch }: { search: PeopleSearch; onSearch: (next: PeopleSearch) => void }) {
  useLanguage();
  const ids = useId();
  const people = usePeople(today(), search.q ?? "", search.left ?? false);
  const editor = useEditor();
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-4 px-6 py-8">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t.people.title}</h1>
        <Link to="/roles" className="text-sm text-accent hover:underline">
          {t.roleList.all}
        </Link>
      </div>
      <div className="flex flex-wrap items-end gap-4">
        <div className="flex min-w-64 flex-1 flex-col gap-1 text-sm">
          <label htmlFor={`${ids}-q`} className="text-ink-muted">
            {t.people.search}
          </label>
          <input
            id={`${ids}-q`}
            type="search"
            value={search.q ?? ""}
            onChange={(event) => onSearch({ ...search, q: event.target.value || undefined })}
            className="h-9 rounded-md border border-border-strong bg-surface px-2.5 text-sm text-ink"
          />
        </div>
        <label className="flex h-9 items-center gap-2 text-sm text-ink">
          <input type="checkbox" checked={search.left ?? false} onChange={(event) => onSearch({ ...search, left: event.target.checked || undefined })} />
          {t.people.left}
        </label>
      </div>
      {people.isError && <p className="text-sm text-danger">{t.people.unreachable}</p>}
      {people.data?.length === 0 && <p className="text-sm text-ink-muted">{t.people.none}</p>}
      {!!people.data?.length && (
        <ul aria-label={t.people.title} className="divide-y divide-border rounded-xl border border-border bg-surface">
          {people.data.map((p) => (
            <li key={p.id} className="flex flex-wrap items-baseline justify-between gap-2 px-4 py-2.5">
              <Link to="/people/$personId" params={{ personId: p.id }} className="font-medium text-ink hover:underline">
                {p.name}
              </Link>
              <span className="text-sm text-ink-subtle">{[p.email, p.location].filter(Boolean).join(" · ")}</span>
            </li>
          ))}
        </ul>
      )}
      {editor && <AddPerson />}
    </div>
  );
}
