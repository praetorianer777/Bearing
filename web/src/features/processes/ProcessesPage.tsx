import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { atLeast, useMe } from "@/api/auth";
import { type Process, useCreateProcess, useProcesses } from "@/api/processes";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { LoadExampleData } from "@/features/settings/ExampleData";
import { t, useLanguage } from "@/i18n";

/** The key a name suggests: small letters, digits and dashes. */
export function keyFor(name: string): string {
  return name
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/ß/g, "ss")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 63);
}

function CreateForm() {
  const create = useCreateProcess();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [key, setKey] = useState<string | undefined>();
  const submit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    create.mutate(
      { name, key: key ?? keyFor(name), area: String(form.get("area")) },
      { onSuccess: (made) => navigate({ to: "/processes/$processId", params: { processId: made.id } }) },
    );
  };
  return (
    <section aria-labelledby="add-process" className="rounded-xl border border-border bg-surface p-5">
      <h2 id="add-process" className="text-lg font-semibold">
        {t.processes.add}
      </h2>
      <form onSubmit={submit} noValidate className="mt-3 flex flex-wrap items-start gap-3">
        <div className="min-w-48 flex-1">
          <Field label={t.forms.name} name="name" value={name} onChange={(e) => setName(e.target.value)} error={fieldError(create.error, "name")} />
        </div>
        <div className="min-w-40">
          <Field
            label={t.processes.key}
            name="key"
            value={key ?? keyFor(name)}
            onChange={(e) => setKey(e.target.value)}
            error={fieldError(create.error, "key")}
          />
        </div>
        <div className="min-w-40">
          <Field label={t.processes.area} name="area" error={fieldError(create.error, "area")} />
        </div>
        <Button type="submit" disabled={create.isPending} className="mt-6">
          {t.processes.addSubmit}
        </Button>
      </form>
      <div className="mt-3">
        <FormError error={create.error} />
      </div>
    </section>
  );
}

function Status({ process }: { process: Process }) {
  if (process.latest === null) return <span className="text-warning">{t.processes.draftOnly}</span>;
  return (
    <span className="text-ink-muted">
      {t.processes.latest(process.latest)}
      {process.changed && <span className="text-ink-subtle"> · {t.processes.changed}</span>}
    </span>
  );
}

/** Every process, by area. */
export function ProcessesPage() {
  useLanguage();
  const processes = useProcesses();
  const role = useMe().data?.role;
  const areas = new Map<string, Process[]>();
  for (const p of processes.data ?? []) areas.set(p.area, [...(areas.get(p.area) ?? []), p]);
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold">{t.processes.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.processes.intro}</p>
        <Link to="/who-does" className="mt-2 inline-block text-sm text-accent hover:underline">
          {t.whoDoes.title}
        </Link>
      </div>
      {processes.isError && <p className="text-sm text-danger">{t.processes.unreachable}</p>}
      {processes.data?.length === 0 && (
        <div className="flex flex-col items-start gap-3">
          <p className="text-sm text-ink-muted">{t.processes.none}</p>
          <LoadExampleData />
        </div>
      )}
      {[...areas]
        .sort(([a], [b]) => Number(a === "") - Number(b === ""))
        .map(([area, list], i) => (
          <section key={area} aria-labelledby={`area-${i}`} className="rounded-xl border border-border bg-surface px-5 py-4">
            <h2 id={`area-${i}`} className="text-sm font-semibold uppercase tracking-wide text-ink-subtle">
              {area || t.processes.noArea}
            </h2>
            <ul className="mt-2 divide-y divide-border">
              {list.map((p) => (
                <li key={p.id} className="flex flex-wrap items-baseline justify-between gap-2 py-2">
                  <div className="min-w-0">
                    <Link to="/processes/$processId" params={{ processId: p.id }} className="font-medium text-ink hover:underline">
                      {p.name}
                    </Link>
                    {p.description && <p className="truncate text-sm text-ink-muted">{p.description}</p>}
                  </div>
                  <span className="text-sm">
                    <Status process={p} />
                  </span>
                </li>
              ))}
            </ul>
          </section>
        ))}
      {role && atLeast(role, "editor") && <CreateForm />}
    </div>
  );
}
