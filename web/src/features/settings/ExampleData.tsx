import { useMe } from "@/api/auth";
import { type DemoState, useDemo, useLoadDemo } from "@/api/demo";
import { Button, FormError } from "@/components/ui";
import { t, useLanguage } from "@/i18n";
import { SettingsNav } from "./SettingsNav";

type PartState = "loaded" | "ready" | "taken" | "needsOrganisation";

/** Where each part of the example data stands: the processes go in only with the example organisation or after it. */
export function partStates({ organisation, processes }: DemoState): { organisation: PartState; processes: PartState } {
  const org: PartState = organisation.loaded ? "loaded" : organisation.empty ? "ready" : "taken";
  const flows: PartState = processes.loaded ? "loaded" : !processes.empty ? "taken" : org === "loaded" || org === "ready" ? "ready" : "needsOrganisation";
  return { organisation: org, processes: flows };
}

const canLoad = (state: DemoState) => Object.values(partStates(state)).includes("ready");

function LoadButton() {
  const demo = useDemo();
  const load = useLoadDemo();
  if (!demo.data || !canLoad(demo.data)) return null;
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Button onClick={() => load.mutate()} disabled={load.isPending}>
        {t.demo.load}
      </Button>
      <FormError error={load.error} />
    </div>
  );
}

/** The button that loads the example data, for an administrator while some of it may still go in. */
export function LoadExampleData() {
  return useMe().data?.role === "administrator" ? <LoadButton /> : null;
}

export function ExampleDataPage() {
  useLanguage();
  const demo = useDemo();
  const states = demo.data && partStates(demo.data);
  const parts = [
    { key: "organisation", title: t.demo.organisation, intro: t.demo.organisationIntro },
    { key: "processes", title: t.demo.processes, intro: t.demo.processesIntro },
  ] as const;
  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-8">
      <SettingsNav />
      <div>
        <h1 className="text-2xl font-semibold">{t.demo.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.demo.intro}</p>
      </div>
      {states && (
        <ul aria-label={t.demo.title} className="divide-y divide-border rounded-xl border border-border bg-surface px-5">
          {parts.map((part) => (
            <li key={part.key} className="flex flex-wrap items-start justify-between gap-3 py-4">
              <div className="min-w-0 flex-1">
                <h2 className="text-base font-semibold">{part.title}</h2>
                <p className="mt-1 text-sm text-ink-muted">{part.intro}</p>
              </div>
              <span className={`text-sm ${states[part.key] === "ready" ? "text-accent" : "text-ink-muted"}`}>{t.demo.state[states[part.key]]}</span>
            </li>
          ))}
        </ul>
      )}
      {demo.data && !canLoad(demo.data) && (
        <p className="text-sm text-ink-muted">{states?.organisation === "loaded" && states.processes === "loaded" ? t.demo.done : t.demo.nothing}</p>
      )}
      <LoadButton />
    </div>
  );
}
