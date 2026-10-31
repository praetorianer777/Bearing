import { Link } from "@tanstack/react-router";
import { useEffect, useId, useState } from "react";
import { useWhoDoes } from "@/api/processes";
import { AsOfPicker, inlineLink, PersonName, today } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { RACIS, type PartyKey, partyOf, useParties } from "./raci";

export type WhoDoesSearch = { q?: string; asOf?: string };

/** Who does it: steps found by a term, each with who is responsible, accountable, consulted and informed on a day. */
export function WhoDoesPage({ search, onSearch }: { search: WhoDoesSearch; onSearch: (next: WhoDoesSearch) => void }) {
  useLanguage();
  const id = useId();
  const asOf = search.asOf ?? today();
  const [term, setTerm] = useState(search.q ?? "");
  const steps = useWhoDoes(search.q ?? "", asOf);
  const parties = useParties();
  // The address follows the field after a pause, so each keystroke is not a request.
  useEffect(() => {
    const timer = setTimeout(() => {
      if (term !== (search.q ?? "")) onSearch({ ...search, q: term || undefined });
    }, 300);
    return () => clearTimeout(timer);
  }, [term, search, onSearch]);
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <header className="flex flex-wrap items-end gap-4">
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold">{t.whoDoes.title}</h1>
          <p className="mt-1 text-sm text-ink-muted">{t.whoDoes.intro}</p>
        </div>
        <AsOfPicker value={asOf} onChange={(day) => onSearch({ ...search, asOf: day })} />
      </header>
      <div className="flex flex-col gap-1">
        <label htmlFor={`${id}-q`} className="text-sm font-medium">
          {t.whoDoes.search}
        </label>
        <input
          id={`${id}-q`}
          type="search"
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          placeholder={t.whoDoes.placeholder}
          className="h-10 rounded-md border border-border-strong bg-surface px-3 text-sm text-ink"
        />
      </div>
      {(search.q ?? "").trim().length < 2 ? (
        <p className="text-sm text-ink-muted">{t.whoDoes.hint}</p>
      ) : !steps.data ? null : steps.data.length === 0 ? (
        <p className="text-sm text-ink-muted">{t.whoDoes.none}</p>
      ) : (
        <ul aria-label={t.whoDoes.results} className="flex flex-col gap-3">
          {steps.data.map((step) => (
            <li key={`${step.processId}-${step.key}`} className="rounded-xl border border-border bg-surface p-4">
              <p className="text-xs text-ink-subtle">
                <Link to="/processes/$processId" params={{ processId: step.processId }} search={{ asOf: search.asOf }} className={inlineLink}>
                  {step.processName}
                </Link>
                {` · ${t.process.version(step.version)}`}
              </p>
              <h2 className="font-semibold">{step.name || t.process.unnamed}</h2>
              {step.responsibilities.length === 0 ? (
                <p className="mt-1 text-sm text-warning">{t.whoDoes.nobodyNamed}</p>
              ) : (
                <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                  {RACIS.flatMap((kind) =>
                    step.responsibilities
                      .filter((r) => r.kind === kind)
                      .map((r) => (
                        <div key={`${kind}-${partyOf(r)}`} className="contents">
                          <dt className="text-ink-subtle">{t.raci.kinds[kind]}</dt>
                          <dd>
                            <span className="text-ink-muted">{parties.nameOf(partyOf(r) as PartyKey)}: </span>
                            {r.people.length === 0 ? (
                              <span className="text-warning">{t.raci.nobody}</span>
                            ) : (
                              r.people.map((p, i) => (
                                <span key={`${p.personId}-${p.positionId ?? ""}`}>
                                  {i > 0 && ", "}
                                  <PersonName id={p.personId} name={p.name} />
                                  {p.deputyFor && <span className="text-ink-subtle"> ({t.whoDoes.deputy})</span>}
                                </span>
                              ))
                            )}
                          </dd>
                        </div>
                      )),
                  )}
                </dl>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
