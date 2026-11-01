import { type FormEvent, useState } from "react";
import { useMe } from "@/api/auth";
import { useApps, useDisconnect, useGrants, useRegisterPartner, useRemoveApp } from "@/api/oauth";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";

const day = (iso: string) => new Date(iso).toLocaleDateString();

/** An administrator registers a partner app for token exchange; its secret is shown once, to hand to the app. */
function RegisterPartner() {
  const register = useRegisterPartner();
  const [name, setName] = useState("");
  const [realmClient, setRealmClient] = useState("");
  const [write, setWrite] = useState(false);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    register.mutate(
      { name, keycloakClientId: realmClient, write },
      {
        onSuccess: () => {
          setName("");
          setRealmClient("");
          setWrite(false);
        },
      },
    );
  };
  const made = register.data;
  return (
    <section aria-labelledby="partner" className="flex flex-col gap-3 rounded-xl border border-border bg-surface p-5">
      <div>
        <h2 id="partner" className="text-base font-semibold">
          {t.connectedApps.partnerTitle}
        </h2>
        <p className="mt-1 text-sm text-ink-muted">{t.connectedApps.partnerIntro}</p>
      </div>
      <form onSubmit={submit} className="flex flex-col gap-3" noValidate>
        <Field
          label={t.connectedApps.partnerName}
          name="name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={fieldError(register.error, "name")}
        />
        <Field
          label={t.connectedApps.realmClient}
          name="keycloakClientId"
          value={realmClient}
          onChange={(e) => setRealmClient(e.target.value)}
          error={fieldError(register.error, "keycloakClientId")}
        />
        <label className="flex items-center gap-2 text-sm text-ink">
          <input type="checkbox" checked={write} onChange={(e) => setWrite(e.target.checked)} />
          {t.connectedApps.partnerWrite}
        </label>
        <Button type="submit" disabled={register.isPending} className="self-start">
          {t.connectedApps.register}
        </Button>
        <FormError error={register.error} />
      </form>
      {made && (
        <div role="status" className="rounded-md border border-border bg-surface-sunken p-3 text-sm">
          <p>{t.connectedApps.secretOnce(made.client.name)}</p>
          <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
            <dt className="text-ink-muted">{t.connectedApps.clientId}</dt>
            <dd className="break-all font-mono">{made.client.id}</dd>
            <dt className="text-ink-muted">{t.connectedApps.secret}</dt>
            <dd className="break-all font-mono">{made.secret}</dd>
          </dl>
        </div>
      )}
    </section>
  );
}

/** The apps the person connected, each disconnected on its own; for administrators also every app that registered. */
export function ConnectedAppsPage() {
  useLanguage();
  const administers = useMe().data?.role === "administrator";
  const grants = useGrants();
  const disconnect = useDisconnect();
  const apps = useApps(administers);
  const remove = useRemoveApp();
  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold">{t.connectedApps.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.connectedApps.intro}</p>
      </div>
      <section aria-labelledby="yours" className="flex flex-col gap-2">
        <h2 id="yours" className="text-base font-semibold">
          {t.connectedApps.yours}
        </h2>
        {grants.data?.length === 0 && <p className="text-sm text-ink-muted">{t.connectedApps.none}</p>}
        {(grants.data?.length ?? 0) > 0 && (
          <ul aria-label={t.connectedApps.yours} className="divide-y divide-border rounded-xl border border-border bg-surface px-5">
            {grants.data?.map((g) => (
              <li key={g.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                <div>
                  <p className="font-medium">{g.app}</p>
                  <p className="text-sm text-ink-muted">
                    {g.scopes.includes("write") ? t.connectedApps.readsAndWrites : t.connectedApps.reads} · {t.connectedApps.since(day(g.connectedAt))}
                  </p>
                </div>
                <Button
                  variant="secondary"
                  onClick={() => disconnect.mutate(g.id)}
                  disabled={disconnect.isPending}
                  aria-label={t.connectedApps.disconnectApp(g.app)}
                >
                  {t.connectedApps.disconnect}
                </Button>
              </li>
            ))}
          </ul>
        )}
        <FormError error={disconnect.error} />
      </section>
      {administers && (
        <section aria-labelledby="all-apps" className="flex flex-col gap-2">
          <h2 id="all-apps" className="text-base font-semibold">
            {t.connectedApps.allApps}
          </h2>
          <p className="text-sm text-ink-muted">{t.connectedApps.allAppsIntro}</p>
          {apps.data?.length === 0 && <p className="text-sm text-ink-muted">{t.connectedApps.noApps}</p>}
          {(apps.data?.length ?? 0) > 0 && (
            <ul aria-label={t.connectedApps.allApps} className="divide-y divide-border rounded-xl border border-border bg-surface px-5">
              {apps.data?.map((a) => (
                <li key={a.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                  <div className="min-w-0">
                    <p className="font-medium">
                      {a.name}{" "}
                      <span className={`text-xs ${a.status === "pending" ? "text-warning" : "text-ink-subtle"}`}>
                        {t.connectedApps.status[a.status as "pending" | "allowed"]}
                      </span>
                    </p>
                    <p className="truncate text-sm text-ink-muted">
                      {a.kind === "partner" ? t.connectedApps.partnerOf(a.keycloakClientId) : a.redirectUris.join(", ")} · {t.connectedApps.people(a.people)}
                    </p>
                  </div>
                  <Button variant="danger" onClick={() => remove.mutate(a.id)} disabled={remove.isPending} aria-label={t.connectedApps.removeApp(a.name)}>
                    {t.connectedApps.remove}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <FormError error={remove.error} />
        </section>
      )}
      {administers && <RegisterPartner />}
    </div>
  );
}
