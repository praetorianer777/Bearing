import { useEffect, useState } from "react";
import { type GroupAdmission, ROLES, type Role, useAdmit, useDecline, useGroupAdmissions, useJoinRequests, useSetGroupAdmissions } from "@/api/auth";
import { Button, FormError, fieldError } from "@/components/ui";
import { locale, t, useLanguage } from "@/i18n";

function RolePicker({ value, onChange, label }: { value: Role; onChange: (role: Role) => void; label: string }) {
  return (
    <select
      aria-label={label}
      value={value}
      onChange={(event) => onChange(event.target.value as Role)}
      className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
    >
      {ROLES.map((role) => (
        <option key={role} value={role}>
          {t.roles[role]}
        </option>
      ))}
    </select>
  );
}

function JoinRequestRow({ id, email, name, groups, requestedAt }: { id: string; email: string; name: string; groups: string[]; requestedAt: string }) {
  const [role, setRole] = useState<Role>("member");
  const admit = useAdmit();
  const decline = useDecline();
  const dates = new Intl.DateTimeFormat(locale(), { dateStyle: "medium" });
  return (
    <li className="flex flex-wrap items-center gap-3 border-b border-border py-3 last:border-b-0">
      <div className="min-w-0 flex-1">
        <p className="truncate text-base font-medium text-ink">{name}</p>
        <p className="truncate text-sm text-ink-muted">
          {email} · {dates.format(new Date(requestedAt))}
        </p>
        {groups.length > 0 && <p className="truncate text-xs text-ink-subtle">{groups.join(", ")}</p>}
      </div>
      <RolePicker value={role} onChange={setRole} label={`${t.forms.role}: ${name}`} />
      <Button onClick={() => admit.mutate({ requestID: id, role })} disabled={admit.isPending} aria-label={`${t.accounts.admit}: ${name}`}>
        {t.accounts.admit}
      </Button>
      <Button variant="secondary" onClick={() => decline.mutate(id)} disabled={decline.isPending} aria-label={`${t.accounts.decline}: ${name}`}>
        {t.accounts.decline}
      </Button>
      <div className="basis-full">
        <FormError error={admit.error ?? decline.error} />
      </div>
    </li>
  );
}

export function JoinRequests() {
  useLanguage();
  const requests = useJoinRequests();
  if (!requests.data?.length) return null;
  return (
    <section aria-labelledby="requests-heading" className="rounded-xl border border-border bg-surface p-5">
      <h2 id="requests-heading" className="text-lg font-semibold">
        {t.accounts.requests}
      </h2>
      <p className="mt-1 text-sm text-ink-muted">{t.accounts.requestsIntro}</p>
      <ul className="mt-2">
        {requests.data.map((request) => (
          <JoinRequestRow key={request.id} {...request} />
        ))}
      </ul>
    </section>
  );
}

export function GroupAdmissions() {
  useLanguage();
  const saved = useGroupAdmissions();
  const save = useSetGroupAdmissions();
  const [rows, setRows] = useState<GroupAdmission[]>([]);
  useEffect(() => {
    if (saved.data) setRows(saved.data);
  }, [saved.data]);
  const change = (index: number, patch: Partial<GroupAdmission>) => setRows((all) => all.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  return (
    <section aria-labelledby="admissions-heading" className="rounded-xl border border-border bg-surface p-5">
      <h2 id="admissions-heading" className="text-lg font-semibold">
        {t.accounts.admissions}
      </h2>
      <p className="mt-1 text-sm text-ink-muted">{t.accounts.admissionsIntro}</p>
      <ul className="mt-3 flex flex-col gap-2">
        {rows.map((row, index) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: rows have no identity until saved, and are edited in place
          <li key={index} className="flex flex-wrap items-start gap-2">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <input
                aria-label={`${t.accounts.group} ${index + 1}`}
                value={row.group}
                onChange={(event) => change(index, { group: event.target.value })}
                aria-invalid={fieldError(save.error, `admissions[${index}].group`) ? true : undefined}
                className="h-9 rounded-md border border-border-strong bg-surface px-2.5 text-sm text-ink aria-invalid:border-danger"
              />
              {fieldError(save.error, `admissions[${index}].group`) && (
                <span className="text-xs text-danger">{fieldError(save.error, `admissions[${index}].group`)}</span>
              )}
            </div>
            <RolePicker value={row.role} onChange={(role) => change(index, { role })} label={`${t.forms.role} ${index + 1}`} />
            <Button variant="secondary" onClick={() => setRows((all) => all.filter((_, i) => i !== index))}>
              {t.accounts.removeGroup}
            </Button>
          </li>
        ))}
      </ul>
      <div className="mt-3 flex items-center gap-2">
        <Button variant="secondary" onClick={() => setRows((all) => [...all, { group: "", role: "member" }])}>
          {t.accounts.addGroup}
        </Button>
        <Button onClick={() => save.mutate(rows)} disabled={save.isPending}>
          {t.accounts.saveAdmissions}
        </Button>
        {save.isSuccess && (
          <span role="status" className="text-sm text-ink-muted">
            {t.accounts.admissionsSaved}
          </span>
        )}
      </div>
    </section>
  );
}
