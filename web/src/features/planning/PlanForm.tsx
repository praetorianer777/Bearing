import { type FormEvent, useEffect, useId, useRef, useState } from "react";
import type { Person, Position } from "@/api/org";
import { usePlanHolding } from "@/api/capacity";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";

/**
 * Plans a holding into a position from a day: somebody in the organisation, as a handover or a second seat, or
 * somebody new, as a hire. It lands as a holding planned ahead, which closes the gap from its first day.
 */
export function PlanForm({ position, people, from, onDone }: { position: Position; people: Person[]; from: string; onDone: () => void }) {
  useLanguage();
  const id = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const plan = usePlanHolding();
  const [who, setWho] = useState<"new" | "existing">("new");
  const [name, setName] = useState("");
  const [personId, setPersonId] = useState("");
  const [start, setStart] = useState(from);
  const [end, setEnd] = useState("");
  const [share, setShare] = useState("100");
  const [done, setDone] = useState("");
  useEffect(() => heading.current?.focus(), []);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const holder = who === "new" ? { name: name.trim() } : { personId };
    plan.mutate(
      { positionId: position.id, ...holder, from: start, share: Number(share), ...(end ? { to: end } : {}) },
      {
        onSuccess: () => setDone(t.planning.planned(who === "new" ? name.trim() : (people.find((p) => p.id === personId)?.name ?? ""), position.title, start)),
      },
    );
  };
  return (
    <section aria-labelledby={`${id}-heading`} className="rounded-xl border border-border bg-surface p-5 text-sm">
      <h2 id={`${id}-heading`} ref={heading} tabIndex={-1} className="text-lg font-semibold outline-none">
        {t.planning.planFor(position.title)}
      </h2>
      <p className="mt-1 text-ink-muted">{t.planning.planIntro}</p>
      {done ? (
        <div className="mt-3 flex flex-col gap-3">
          <p role="status">{done}</p>
          <Button variant="secondary" onClick={onDone} className="self-start">
            {t.planning.close}
          </Button>
        </div>
      ) : (
        <form onSubmit={submit} className="mt-3 flex flex-col gap-3">
          <fieldset className="flex flex-wrap gap-4">
            <legend className="mb-1 font-medium">{t.planning.who}</legend>
            {(["new", "existing"] as const).map((option) => (
              <label key={option} className="flex items-center gap-1.5">
                <input type="radio" name={`${id}-who`} checked={who === option} onChange={() => setWho(option)} />
                {option === "new" ? t.planning.somebodyNew : t.planning.somebodyThere}
              </label>
            ))}
          </fieldset>
          {who === "new" ? (
            <Field label={t.planning.name} required value={name} onChange={(e) => setName(e.target.value)} error={fieldError(plan.error, "name")} />
          ) : (
            <div className="flex flex-col gap-1">
              <label htmlFor={`${id}-person`} className="font-medium">
                {t.planning.person}
              </label>
              <select
                id={`${id}-person`}
                required
                value={personId}
                onChange={(e) => setPersonId(e.target.value)}
                className="h-9 rounded-md border border-border-strong bg-surface px-2 text-base text-ink"
              >
                <option value="">{t.planning.choose}</option>
                {[...people]
                  .sort((a, b) => a.name.localeCompare(b.name))
                  .map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
              </select>
            </div>
          )}
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label={t.target.from} type="date" required value={start} onChange={(e) => setStart(e.target.value)} error={fieldError(plan.error, "from")} />
            <Field label={t.target.to} type="date" value={end} onChange={(e) => setEnd(e.target.value)} error={fieldError(plan.error, "to")} />
            <Field
              label={t.planning.share}
              type="number"
              min={1}
              max={100}
              required
              value={share}
              onChange={(e) => setShare(e.target.value)}
              error={fieldError(plan.error, "share")}
            />
          </div>
          <FormError error={plan.error} />
          <div className="flex gap-2">
            <Button type="submit" disabled={plan.isPending}>
              {t.planning.submit}
            </Button>
            <Button variant="secondary" onClick={onDone}>
              {t.planning.cancel}
            </Button>
          </div>
        </form>
      )}
    </section>
  );
}
