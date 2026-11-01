import { Link } from "@tanstack/react-router";
import { useEffect, useId, useState } from "react";
import { type SearchHit, type SearchResults, searchable, useSearch } from "@/api/search";
import { AsOfPicker, today } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { hitKey, hitTarget } from "./hits";

export type SearchPageSearch = { q?: string; asOf?: string };

const GROUPS = ["people", "positions", "units", "roles", "processes", "steps"] as const;

function Group({ name, hits, asOf }: { name: (typeof GROUPS)[number]; hits: SearchHit[]; asOf?: string }) {
  const id = useId();
  return (
    <section aria-labelledby={id} className="rounded-xl border border-border bg-surface px-5 py-4">
      <h2 id={id} className="text-sm font-semibold text-ink-muted">
        {t.search.groups[name]} <span className="font-normal text-ink-subtle">{hits.length}</span>
      </h2>
      <ul className="mt-2 flex flex-col">
        {hits.map((hit) => (
          <li key={hitKey(hit)} className="flex flex-wrap items-baseline gap-x-2 border-b border-border py-2 last:border-b-0">
            <Link {...hitTarget(hit, asOf)} className="font-medium text-ink underline decoration-border-strong underline-offset-2 hover:decoration-ink">
              {hit.name}
            </Link>
            {hit.detail && <span className="text-sm text-ink-muted">{hit.detail}</span>}
          </li>
        ))}
      </ul>
    </section>
  );
}

function Results({ results, asOf }: { results: SearchResults; asOf?: string }) {
  const found = GROUPS.filter((name) => results[name].length > 0);
  if (found.length === 0) return <p className="text-sm text-ink-muted">{t.search.none}</p>;
  return (
    <div className="flex flex-col gap-4">
      {found.map((name) => (
        <Group key={name} name={name} hits={results[name]} asOf={asOf} />
      ))}
    </div>
  );
}

/** Everything a search finds on a day, grouped by kind, best first in each. */
export function SearchPage({ search, onSearch }: { search: SearchPageSearch; onSearch: (next: SearchPageSearch) => void }) {
  useLanguage();
  const id = useId();
  const asOf = search.asOf ?? today();
  const asked = search.q ?? "";
  const [term, setTerm] = useState(asked);
  const results = useSearch(asked, asOf);
  // The box along the top may ask anew while this page is open.
  useEffect(() => setTerm(asked), [asked]);
  useEffect(() => {
    const timer = setTimeout(() => {
      if (term !== asked) onSearch({ ...search, q: term || undefined });
    }, 300);
    return () => clearTimeout(timer);
  }, [term, asked, search, onSearch]);
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-end gap-4">
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">{t.search.title}</h1>
          <p className="mt-1 text-sm text-ink-muted">{t.search.intro}</p>
        </div>
        <AsOfPicker value={asOf} onChange={(day) => onSearch({ ...search, asOf: day })} />
      </header>
      <div className="flex flex-col gap-1">
        <label htmlFor={`${id}-q`} className="text-sm font-medium">
          {t.search.field}
        </label>
        <input
          id={`${id}-q`}
          type="search"
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          className="h-10 rounded-md border border-border-strong bg-surface px-3 text-sm text-ink"
        />
      </div>
      {!searchable(asked) ? (
        <p className="text-sm text-ink-muted">{t.search.hint}</p>
      ) : results.isError ? (
        <p role="alert" className="text-sm text-danger">
          {t.search.unreachable}
        </p>
      ) : results.data ? (
        <Results results={results.data} asOf={search.asOf} />
      ) : null}
    </div>
  );
}
