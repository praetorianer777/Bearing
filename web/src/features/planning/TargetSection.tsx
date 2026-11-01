import { type FormEvent, useState } from "react";
import { atLeast, useMe } from "@/api/auth";
import { type TargetSubject, useCapacityOn, useRemoveTarget, useSetTarget, useTargets } from "@/api/capacity";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { Section } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";

/** Full-time equivalents as people read them: at most two decimals, in the reader's language. */
export const fte = (value: number) => t.target.fte(value.toLocaleString(undefined, { maximumFractionDigits: 2 }));

function SetTarget({ subject, asOf }: { subject: TargetSubject; asOf: string }) {
  const set = useSetTarget(subject);
  const [value, setValue] = useState("");
  const [from, setFrom] = useState(asOf);
  const [to, setTo] = useState("");
  const submit = (event: FormEvent) => {
    event.preventDefault();
    set.mutate({ fte: Number(value.replace(",", ".")), from, ...(to ? { to } : {}) }, { onSuccess: () => setValue("") });
  };
  return (
    <form onSubmit={submit} className="mt-4 flex flex-col gap-3 border-t border-border pt-4">
      <p className="text-sm font-medium">{t.target.set}</p>
      <div className="grid gap-3 sm:grid-cols-3">
        <Field
          label={t.target.value}
          inputMode="decimal"
          required
          value={value}
          onChange={(e) => setValue(e.target.value)}
          error={fieldError(set.error, "fte")}
        />
        <Field label={t.target.from} type="date" required value={from} onChange={(e) => setFrom(e.target.value)} error={fieldError(set.error, "from")} />
        <Field label={t.target.to} type="date" value={to} onChange={(e) => setTo(e.target.value)} error={fieldError(set.error, "to")} />
      </div>
      <p className="text-xs text-ink-muted">{t.target.replaces}</p>
      <FormError error={set.error} />
      <Button type="submit" disabled={set.isPending || !value} className="self-start">
        {t.target.submit}
      </Button>
    </form>
  );
}

/** What a unit, a position or a role should have, against what it has on the day, and its targets over time. */
export function TargetSection({ subject, asOf }: { subject: TargetSubject; asOf: string }) {
  useLanguage();
  const role = useMe().data?.role;
  const editor = !!role && atLeast(role, "editor");
  const targets = useTargets(subject);
  const capacity = useCapacityOn(subject, asOf);
  const remove = useRemoveTarget();
  const now = capacity.data;
  return (
    <Section title={t.target.title}>
      {now && (
        <p className="text-base">
          {now.target == null ? (
            <span className="text-ink-muted">{t.target.none}</span>
          ) : (
            <>
              {t.target.should(fte(now.target))}
              <span className={now.fte < now.target ? "font-medium text-ink" : "text-ink-muted"}>
                {" · "}
                {now.fte < now.target ? t.target.short(fte(now.target - now.fte)) : t.target.met}
              </span>
            </>
          )}
          <span className="block text-sm text-ink-muted">{t.target.has(fte(now.fte))}</span>
        </p>
      )}
      {!!targets.data?.length && (
        <ul aria-label={t.target.history} className="mt-3 space-y-1">
          {[...targets.data].reverse().map((target) => (
            <li key={target.id} className="flex flex-wrap items-center justify-between gap-2">
              <span>{fte(target.fte)}</span>
              <span className="flex items-center gap-3 text-ink-subtle">
                {t.person.span(target.from, target.to ?? null)}
                {editor && (
                  <Button
                    variant="danger"
                    className="h-7"
                    disabled={remove.isPending}
                    onClick={() => remove.mutate(target.id)}
                    aria-label={`${t.target.remove}: ${fte(target.fte)} ${t.person.span(target.from, target.to ?? null)}`}
                  >
                    {t.target.remove}
                  </Button>
                )}
              </span>
            </li>
          ))}
        </ul>
      )}
      <FormError error={remove.error} />
      {editor && <SetTarget key={asOf} subject={subject} asOf={asOf} />}
    </Section>
  );
}
