import { useMe } from "@/api/auth";
import { useApps, useDisconnect, useGrants, useRemoveApp } from "@/api/oauth";
import { Button, FormError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";

const day = (iso: string) => new Date(iso).toLocaleDateString();

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
                      {a.redirectUris.join(", ")} · {t.connectedApps.people(a.people)}
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
    </div>
  );
}
