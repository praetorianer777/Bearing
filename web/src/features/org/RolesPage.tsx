import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { t, useLanguage } from "@/i18n";
import { RoleName } from "./common";
import { AddRole, useEditor } from "./edit";

/** Every role by name, and for editors a way to add one. */
export function RolesPage() {
  useLanguage();
  const editor = useEditor();
  const roles = useQuery({ queryKey: ["roles"], queryFn: async () => unwrap(await api.GET("/roles")).roles });
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5 px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold">{t.roleList.title}</h1>
        <p className="mt-1 text-sm text-ink-muted">{t.roleList.intro}</p>
      </div>
      {roles.data &&
        (roles.data.length === 0 ? (
          <p className="text-sm text-ink-muted">{t.roleList.none}</p>
        ) : (
          <ul aria-label={t.roleList.title} className="rounded-xl border border-border bg-surface px-5 text-sm">
            {[...roles.data]
              .sort((a, b) => a.name.localeCompare(b.name))
              .map((r) => (
                <li key={r.id} className="flex flex-wrap items-baseline justify-between gap-2 border-b border-border py-2.5 last:border-b-0">
                  <span className="font-medium">
                    <RoleName id={r.id} name={r.name} />
                  </span>
                  <span className="min-w-0 truncate text-ink-muted">{r.description}</span>
                </li>
              ))}
          </ul>
        ))}
      {editor && <AddRole />}
    </div>
  );
}
