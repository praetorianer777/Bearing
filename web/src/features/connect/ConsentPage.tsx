import { useRouterState } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useConsent, useDecide } from "@/api/oauth";
import { Button, FormError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";

/** Where the browser goes back to the app: the address the API gave, which is always one the app registered. */
export const goTo = { assign: (to: string) => window.location.assign(to) };

/** The consent page: an app asks to act as the person signed in, and they connect it or cancel. */
export function ConsentPage() {
  useLanguage();
  const query = useRouterState({ select: (s) => s.location.searchStr });
  const consent = useConsent(query);
  const decide = useDecide();
  const [write, setWrite] = useState(true);
  const back = consent.data?.redirect;
  useEffect(() => {
    if (back) goTo.assign(back);
  }, [back]);

  if (consent.isError) {
    return (
      <div className="mx-auto max-w-xl px-6 py-10">
        <h1 className="text-2xl font-semibold">{t.connect.titleUnknown}</h1>
        <FormError error={consent.error} />
      </div>
    );
  }
  const c = consent.data?.consent;
  if (!c) return null;
  const asksWrite = c.scopes.includes("write");
  const answer = (allow: boolean) =>
    decide.mutate(
      { query: query.replace(/^\?/, ""), allow, scope: allow && asksWrite && !write ? "read" : undefined },
      { onSuccess: (out) => goTo.assign(out.redirect) },
    );
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-5 px-6 py-10">
      <div>
        <h1 className="text-2xl font-semibold">{t.connect.title(c.client.name)}</h1>
        <p className="mt-2 text-base text-ink-muted">{t.connect.intro(c.client.name, c.person)}</p>
        {c.client.clientUri && <p className="mt-1 text-sm text-ink-subtle">{c.client.clientUri}</p>}
      </div>
      <section aria-labelledby="connect-may" className="rounded-xl border border-border bg-surface p-5">
        <h2 id="connect-may" className="text-base font-semibold">
          {t.connect.mayTitle}
        </h2>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-ink">
          <li>{t.connect.read}</li>
          {asksWrite && write && <li>{t.connect.write}</li>}
        </ul>
        {asksWrite && c.mayAllow && (
          <label className="mt-3 flex items-center gap-2 text-sm text-ink">
            <input type="checkbox" checked={write} onChange={(e) => setWrite(e.target.checked)} />
            {t.connect.allowWrite}
          </label>
        )}
        <p className="mt-3 text-xs text-ink-subtle">{t.connect.mcpOnly}</p>
      </section>
      {c.allowsApp && <p className="rounded-md border border-warning bg-warning-subtle px-3 py-2 text-sm text-ink">{t.connect.allowsApp(c.client.name)}</p>}
      {c.refusal && (
        <p role="alert" className="rounded-md bg-danger-subtle px-3 py-2 text-sm text-danger">
          {c.refusal}
        </p>
      )}
      {c.mayAllow && (
        <div className="flex flex-col gap-2">
          <div className="flex gap-3">
            <Button onClick={() => answer(true)} disabled={decide.isPending}>
              {t.connect.allow}
            </Button>
            <Button variant="secondary" onClick={() => answer(false)} disabled={decide.isPending}>
              {t.connect.cancel}
            </Button>
          </div>
          <p className="text-xs text-ink-subtle">{t.connect.goesBack(c.redirectHost)}</p>
          <FormError error={decide.error} />
        </div>
      )}
    </div>
  );
}
