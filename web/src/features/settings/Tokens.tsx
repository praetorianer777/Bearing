import { type FormEvent, useId, useState } from "react";
import { useMakeToken, usePersonalTokens, useRevokeToken } from "@/api/tokens";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";

const LIFETIMES = [7, 30, 90, 365] as const;

const day = (iso: string) => new Date(iso).toLocaleDateString();

/** The person's own access tokens for scripts and assistants: made here, shown once, revoked here. */
export function TokensPage() {
  useLanguage();
  const id = useId();
  const tokens = usePersonalTokens();
  const make = useMakeToken();
  const revoke = useRevokeToken();
  const [name, setName] = useState("");
  const [write, setWrite] = useState(false);
  const [days, setDays] = useState(30);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    make.mutate(
      { name, write, days },
      {
        onSuccess: () => {
          setName("");
          setWrite(false);
        },
      },
    );
  };
  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold">{t.tokens.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.tokens.intro}</p>
      </div>
      <section aria-labelledby={`${id}-make`} className="flex flex-col gap-3 rounded-xl border border-border bg-surface p-5">
        <h2 id={`${id}-make`} className="text-base font-semibold">
          {t.tokens.make}
        </h2>
        <form onSubmit={submit} className="flex flex-col gap-3" noValidate>
          <Field label={t.tokens.name} name="name" value={name} onChange={(e) => setName(e.target.value)} error={fieldError(make.error, "name")} />
          <div className="flex flex-col gap-1">
            <label htmlFor={`${id}-days`} className="text-sm font-medium text-ink">
              {t.tokens.lifetime}
            </label>
            <select
              id={`${id}-days`}
              value={days}
              onChange={(e) => setDays(Number(e.target.value))}
              className="h-9 self-start rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
            >
              {LIFETIMES.map((d) => (
                <option key={d} value={d}>
                  {t.tokens.days(d)}
                </option>
              ))}
            </select>
          </div>
          <label className="flex items-center gap-2 text-sm text-ink">
            <input type="checkbox" checked={write} onChange={(e) => setWrite(e.target.checked)} />
            {t.tokens.write}
          </label>
          <Button type="submit" disabled={make.isPending} className="self-start">
            {t.tokens.makeSubmit}
          </Button>
          <FormError error={make.error} />
        </form>
        {make.data && (
          <div role="status" className="rounded-md border border-border bg-surface-sunken p-3 text-sm">
            <p>{t.tokens.shownOnce(make.data.token.name)}</p>
            <p className="mt-2 break-all font-mono">{make.data.secret}</p>
          </div>
        )}
      </section>
      <section aria-labelledby={`${id}-yours`} className="flex flex-col gap-2">
        <h2 id={`${id}-yours`} className="text-base font-semibold">
          {t.tokens.yours}
        </h2>
        {tokens.data?.length === 0 && <p className="text-sm text-ink-muted">{t.tokens.none}</p>}
        {(tokens.data?.length ?? 0) > 0 && (
          <ul aria-label={t.tokens.yours} className="divide-y divide-border rounded-xl border border-border bg-surface px-5">
            {tokens.data?.map((token) => (
              <li key={token.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                <div>
                  <p className="font-medium">{token.name}</p>
                  <p className="text-sm text-ink-muted">
                    {token.write ? t.tokens.readsAndWrites : t.tokens.reads} · {t.tokens.until(day(token.expiresAt))}
                  </p>
                </div>
                <Button variant="secondary" onClick={() => revoke.mutate(token.id)} disabled={revoke.isPending} aria-label={t.tokens.revokeNamed(token.name)}>
                  {t.tokens.revoke}
                </Button>
              </li>
            ))}
          </ul>
        )}
        <FormError error={revoke.error} />
      </section>
    </div>
  );
}
