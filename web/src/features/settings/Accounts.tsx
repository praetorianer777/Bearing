import { type FormEvent, useId, useState } from "react";
import {
  type Account,
  ROLES,
  type Role,
  useAccounts,
  useCreateAccount,
  useInvitations,
  useInvite,
  useMe,
  useRevokeInvitation,
  useSetAccountPassword,
  useSetAccountRole,
  useSetDisabled,
} from "@/api/auth";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { locale, t, useLanguage } from "@/i18n";
import { GroupGrants, JoinRequests } from "./Grants";

function RoleSelect({ name, defaultValue = "member" }: { name: string; defaultValue?: Role }) {
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-ink">
        {t.forms.role}
      </label>
      <select id={id} name={name} defaultValue={defaultValue} className="h-9 rounded-md border border-border-strong bg-surface px-2 text-base text-ink">
        {ROLES.map((role) => (
          <option key={role} value={role}>
            {t.roles[role]}
          </option>
        ))}
      </select>
    </div>
  );
}

function values(event: FormEvent<HTMLFormElement>): Record<string, string> {
  event.preventDefault();
  return Object.fromEntries([...new FormData(event.currentTarget)].map(([k, v]) => [k, String(v)]));
}

function PasswordForm({ account, onDone }: { account: Account; onDone: (message: string) => void }) {
  const setPassword = useSetAccountPassword();
  const submit = (event: FormEvent<HTMLFormElement>) => {
    const password = values(event).password ?? "";
    setPassword.mutate({ accountID: account.id, password }, { onSuccess: () => onDone(t.accounts.passwordSet(account.name)) });
  };
  return (
    <form onSubmit={submit} className="mt-2 flex items-end gap-2" noValidate>
      <div className="flex-1">
        <Field
          label={t.forms.newPassword}
          name="password"
          type="password"
          autoComplete="new-password"
          placeholder={t.forms.passwordHint}
          error={fieldError(setPassword.error, "password")}
        />
      </div>
      <Button type="submit" disabled={setPassword.isPending}>
        {t.accounts.savePassword}
      </Button>
      <Button variant="secondary" onClick={() => onDone("")}>
        {t.accounts.cancel}
      </Button>
    </form>
  );
}

function AccountRow({ account, self, onMessage }: { account: Account; self: boolean; onMessage: (message: string) => void }) {
  const [settingPassword, setSettingPassword] = useState(false);
  const setDisabled = useSetDisabled();
  const setRole = useSetAccountRole();
  return (
    <li className="border-b border-border py-3 last:border-b-0">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <div className="min-w-0 flex-1">
          <p className="truncate text-base font-medium text-ink">
            {account.name}
            {self && <span className="ml-1.5 text-xs font-normal text-ink-subtle">({t.accounts.you})</span>}
          </p>
          <p className="truncate text-sm text-ink-muted">{account.email}</p>
          {account.groups.length > 0 && (
            <p className="truncate text-xs text-ink-subtle">
              {t.accounts.groups}: {account.groups.join(", ")}
            </p>
          )}
        </div>
        <div className="flex flex-col items-end gap-0.5">
          {self ? (
            <span className="text-sm text-ink-muted">{t.roles[account.ownRole]}</span>
          ) : (
            <select
              aria-label={t.accounts.ownRole(account.name)}
              value={account.ownRole}
              disabled={setRole.isPending}
              onChange={(event) => setRole.mutate({ accountID: account.id, role: event.target.value as Role })}
              className="h-8 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
            >
              {ROLES.map((role) => (
                <option key={role} value={role}>
                  {t.roles[role]}
                </option>
              ))}
            </select>
          )}
          {account.role !== account.ownRole && <span className="text-xs text-ink-subtle">{t.accounts.throughGroup(t.roles[account.role])}</span>}
        </div>
        <span className={`text-sm ${account.disabled ? "text-danger" : "text-ink-muted"}`}>
          {account.disabled ? t.accounts.disabled : account.hasPassword ? t.accounts.active : t.accounts.ssoOnly}
        </span>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => setSettingPassword(true)} aria-label={`${t.accounts.setPassword}: ${account.name}`}>
            {t.accounts.setPassword}
          </Button>
          {!self && (
            <Button
              variant={account.disabled ? "secondary" : "danger"}
              disabled={setDisabled.isPending}
              onClick={() => setDisabled.mutate({ accountID: account.id, disabled: !account.disabled })}
              aria-label={`${account.disabled ? t.accounts.enable : t.accounts.disable}: ${account.name}`}
            >
              {account.disabled ? t.accounts.enable : t.accounts.disable}
            </Button>
          )}
        </div>
      </div>
      {settingPassword && (
        <PasswordForm
          account={account}
          onDone={(message) => {
            setSettingPassword(false);
            if (message) onMessage(message);
          }}
        />
      )}
    </li>
  );
}

function InviteForm() {
  const invite = useInvite();
  const [result, setResult] = useState<{ email: string; link: string } | null>(null);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    const form = event.currentTarget;
    const v = values(event);
    invite.mutate(
      { email: v.email ?? "", role: (v.role ?? "member") as Role },
      {
        onSuccess: (answer) => {
          setResult({ email: answer.invitation.email, link: answer.link });
          form.reset();
        },
      },
    );
  };
  return (
    <section aria-labelledby="invite-heading" className="rounded-xl border border-border bg-surface p-5">
      <h2 id="invite-heading" className="text-lg font-semibold">
        {t.accounts.invite}
      </h2>
      <form onSubmit={submit} className="mt-3 grid gap-3 sm:grid-cols-[1fr_12rem_auto] sm:items-end" noValidate>
        <Field label={t.forms.email} name="email" type="email" error={fieldError(invite.error, "email")} />
        <RoleSelect name="role" />
        <Button type="submit" disabled={invite.isPending}>
          {t.accounts.inviteSubmit}
        </Button>
      </form>
      <div className="mt-2">
        <FormError error={invite.error} />
      </div>
      {result && (
        <div role="status" className="mt-3 text-sm text-ink">
          <p>{t.accounts.invited(result.email)}</p>
          <input
            readOnly
            value={result.link}
            aria-label={t.accounts.invited(result.email)}
            className="mt-1 w-full rounded-md bg-surface-sunken px-2 py-1.5 font-mono text-xs"
          />
        </div>
      )}
    </section>
  );
}

function CreateForm() {
  const create = useCreateAccount();
  const [created, setCreated] = useState("");
  const submit = (event: FormEvent<HTMLFormElement>) => {
    const form = event.currentTarget;
    const v = values(event);
    create.mutate(
      { name: v.name ?? "", email: v.email ?? "", role: (v.role ?? "member") as Role, password: v.password ?? "" },
      {
        onSuccess: (account) => {
          setCreated(account.email);
          form.reset();
        },
      },
    );
  };
  return (
    <section aria-labelledby="create-heading" className="rounded-xl border border-border bg-surface p-5">
      <h2 id="create-heading" className="text-lg font-semibold">
        {t.accounts.create}
      </h2>
      <form onSubmit={submit} className="mt-3 grid gap-3 sm:grid-cols-2" noValidate>
        <Field label={t.forms.name} name="name" error={fieldError(create.error, "name")} />
        <Field label={t.forms.email} name="email" type="email" error={fieldError(create.error, "email")} />
        <RoleSelect name="role" />
        <Field
          label={t.forms.password}
          name="password"
          type="password"
          autoComplete="new-password"
          placeholder={t.forms.passwordHint}
          error={fieldError(create.error, "password")}
        />
        <div className="sm:col-span-2">
          <Button type="submit" disabled={create.isPending}>
            {t.accounts.createSubmit}
          </Button>
        </div>
      </form>
      <div className="mt-2">
        <FormError error={create.error} />
      </div>
      {created && (
        <p role="status" className="mt-3 text-sm text-ink">
          {t.accounts.created(created)}
        </p>
      )}
    </section>
  );
}

function OpenInvitations() {
  const invitations = useInvitations();
  const revoke = useRevokeInvitation();
  const dates = new Intl.DateTimeFormat(locale(), { dateStyle: "medium" });
  return (
    <section aria-labelledby="open-heading">
      <h2 id="open-heading" className="text-lg font-semibold">
        {t.accounts.open}
      </h2>
      {invitations.data?.length === 0 && <p className="mt-2 text-sm text-ink-muted">{t.accounts.none}</p>}
      <ul className="mt-2">
        {invitations.data?.map((invitation) => (
          <li key={invitation.id} className="flex items-center gap-3 border-b border-border py-2 last:border-b-0">
            <span className="min-w-0 flex-1 truncate text-sm text-ink">{invitation.email}</span>
            <span className="text-sm text-ink-muted">{t.roles[invitation.role]}</span>
            <span className="text-sm text-ink-subtle">{t.accounts.expires(dates.format(new Date(invitation.expiresAt)))}</span>
            <Button variant="secondary" onClick={() => revoke.mutate(invitation.id)} aria-label={`${t.accounts.revoke}: ${invitation.email}`}>
              {t.accounts.revoke}
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function AccountsPage() {
  useLanguage();
  const accounts = useAccounts();
  const me = useMe();
  const [message, setMessage] = useState("");
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold">{t.accounts.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.accounts.intro}</p>
      </div>
      {message && (
        <p role="status" className="rounded-md bg-accent-subtle px-3 py-2 text-sm text-ink">
          {message}
        </p>
      )}
      <ul aria-label={t.accounts.title} className="rounded-xl border border-border bg-surface px-5">
        {accounts.data?.map((account) => (
          <AccountRow key={account.id} account={account} self={account.id === me.data?.id} onMessage={setMessage} />
        ))}
      </ul>
      <JoinRequests />
      <InviteForm />
      <OpenInvitations />
      <CreateForm />
      <GroupGrants />
    </div>
  );
}
