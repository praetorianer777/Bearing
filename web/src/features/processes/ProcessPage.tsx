import { ReactFlowProvider } from "@xyflow/react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useEffect, useId, useMemo, useReducer, useRef, useState } from "react";
import { atLeast, useMe } from "@/api/auth";
import { useOrganisation } from "@/api/org";
import {
  type FlowNode,
  type Problem,
  type Process,
  type StepPeople,
  useDeleteProcess,
  useProcess,
  useProcesses,
  useProcessPeople,
  useProcessVersion,
  usePublish,
  useSaveDraft,
  useUpdateProcess,
} from "@/api/processes";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { AsOfPicker, PositionName, today } from "@/features/org/common";
import { t, useLanguage } from "@/i18n";
import { arrange } from "./arrange";
import { type Edit, edit, type Flow, type Point, start } from "./editor";
import { FlowCanvas, Palette, type Selection } from "./FlowCanvas";
import { type Party, useParties } from "./raci";
import { NodeResponsibilities, RaciMatrix } from "./Responsibilities";

export type ProcessSearch = { version?: number; view?: "matrix"; asOf?: string };

const inputClass = "w-full rounded-md border border-border-strong bg-surface px-2 py-1.5 text-sm text-ink";

function ProblemList({ problems, flow, onPick }: { problems: Problem[]; flow: Flow; onPick: (s: Selection) => void }) {
  const name = (key?: string) => flow.nodes.find((n) => n.key === key)?.name || t.process.unnamed;
  const where = (p: Problem) => {
    if (p.node) return name(p.node);
    const arrow = flow.arrows.find((a) => a.key === p.arrow);
    return arrow ? `${name(arrow.from)} → ${name(arrow.to)}` : undefined;
  };
  return (
    <ul className="space-y-1 text-sm">
      {problems.map((p) => {
        const at: Selection = p.node ? { kind: "node", key: p.node } : p.arrow ? { kind: "arrow", key: p.arrow } : null;
        const text = t.process.problems[p.code as keyof typeof t.process.problems] ?? p.message;
        return (
          <li key={`${p.code}-${p.node ?? ""}-${p.arrow ?? ""}`}>
            {at ? (
              <button type="button" onClick={() => onPick(at)} className="text-left text-warning hover:underline">
                {where(p) ? `${where(p)}: ${text}` : text}
              </button>
            ) : (
              <span className="text-warning">{text}</span>
            )}
          </li>
        );
      })}
    </ul>
  );
}

/** What keeps the draft from being published, and apart from it what is only worth a look. */
function Problems({ problems, flow, onPick }: { problems: Problem[]; flow: Flow; onPick: (s: Selection) => void }) {
  const blocking = problems.filter((p) => !p.warning);
  const warnings = problems.filter((p) => p.warning);
  return (
    <div className="flex flex-col gap-3">
      {blocking.length === 0 ? (
        <p className="text-sm text-success">{t.process.publishable}</p>
      ) : (
        <ProblemList problems={blocking} flow={flow} onPick={onPick} />
      )}
      {warnings.length > 0 && (
        <div>
          <h3 className="mb-1 text-xs text-ink-muted">{t.process.warnings}</h3>
          <ProblemList problems={warnings} flow={flow} onPick={onPick} />
        </div>
      )}
    </div>
  );
}

/** What is picked: a node's name, description, subprocess and responsibilities, or an arrow's condition; read only outside the draft. */
function Inspector({
  flow,
  selection,
  editable,
  others,
  parties,
  nameOf,
  people,
  onEdit,
  onRemove,
}: {
  flow: Flow;
  selection: Selection;
  editable: boolean;
  others: Process[];
  parties: Party[];
  nameOf: (key: string) => string;
  people: StepPeople[];
  onEdit: (e: Edit) => void;
  onRemove: () => void;
}) {
  const id = useId();
  const node = selection?.kind === "node" ? flow.nodes.find((n) => n.key === selection.key) : undefined;
  const arrow = selection?.kind === "arrow" ? flow.arrows.find((a) => a.key === selection.key) : undefined;
  if (!node && !arrow) return <p className="text-sm text-ink-muted">{editable ? t.process.pickHint : t.process.pickToRead}</p>;
  const settle = () => onEdit({ type: "settle" });
  if (arrow) {
    const from = flow.nodes.find((n) => n.key === arrow.from);
    const to = flow.nodes.find((n) => n.key === arrow.to);
    return (
      <div className="flex flex-col gap-3 text-sm">
        <p className="text-ink-muted">{t.process.arrowBetween(from?.name || t.process.unnamed, to?.name || t.process.unnamed)}</p>
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-label`} className="text-xs text-ink-muted">
            {t.process.condition}
          </label>
          <input
            id={`${id}-label`}
            value={arrow.label}
            readOnly={!editable}
            onChange={(e) => onEdit({ type: "label", key: arrow.key, label: e.target.value })}
            onBlur={settle}
            className={inputClass}
          />
        </div>
        {editable && (
          <Button variant="secondary" onClick={onRemove} className="self-start">
            {t.process.removeArrow}
          </Button>
        )}
      </div>
    );
  }
  const n = node as FlowNode;
  const change = (c: Partial<FlowNode>) => onEdit({ type: "node", key: n.key, change: c });
  return (
    <div className="flex flex-col gap-3 text-sm">
      <p className="text-xs uppercase tracking-wide text-ink-subtle">{t.process.kinds[n.kind]}</p>
      <div className="flex flex-col gap-1">
        <label htmlFor={`${id}-name`} className="text-xs text-ink-muted">
          {t.forms.name}
        </label>
        <input
          id={`${id}-name`}
          value={n.name}
          readOnly={!editable}
          onChange={(e) => change({ name: e.target.value })}
          onBlur={settle}
          className={inputClass}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor={`${id}-description`} className="text-xs text-ink-muted">
          {t.process.description}
        </label>
        <textarea
          id={`${id}-description`}
          rows={4}
          value={n.description}
          readOnly={!editable}
          onChange={(e) => change({ description: e.target.value })}
          onBlur={settle}
          className={inputClass}
        />
      </div>
      {n.kind === "subprocess" && (
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-sub`} className="text-xs text-ink-muted">
            {t.process.standsFor}
          </label>
          <select
            id={`${id}-sub`}
            value={n.subprocessId ?? ""}
            disabled={!editable}
            onChange={(e) => {
              change({ subprocessId: e.target.value || null });
              settle();
            }}
            className={inputClass}
          >
            <option value="">{t.process.noSubprocess}</option>
            {others.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
      )}
      {n.kind !== "start" && n.kind !== "end" && (
        <NodeResponsibilities node={n} editable={editable} parties={parties} nameOf={nameOf} people={people.find((p) => p.key === n.key)} onEdit={onEdit} />
      )}
      {editable && (
        <Button variant="secondary" onClick={onRemove} className="self-start">
          {t.process.removeNode}
        </Button>
      )}
    </div>
  );
}

function Details({ process, onDone }: { process: Process; onDone: () => void }) {
  const update = useUpdateProcess(process.id);
  const remove = useDeleteProcess(process.id);
  const navigate = useNavigate();
  const organisation = useOrganisation(today());
  const id = useId();
  const positions = [...(organisation.data?.positions ?? [])].sort((a, b) => a.title.localeCompare(b.title));
  const submit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    update.mutate(
      {
        name: String(form.get("name")),
        key: String(form.get("key")),
        area: String(form.get("area")),
        description: String(form.get("description")),
        ownerId: String(form.get("ownerId")),
      },
      { onSuccess: onDone },
    );
  };
  return (
    <form onSubmit={submit} noValidate className="grid gap-3 border-b border-border bg-surface px-6 py-4 sm:grid-cols-2">
      <Field label={t.forms.name} name="name" defaultValue={process.name} error={fieldError(update.error, "name")} />
      <Field label={t.processes.key} name="key" defaultValue={process.key} error={fieldError(update.error, "key")} />
      <Field label={t.processes.area} name="area" defaultValue={process.area} error={fieldError(update.error, "area")} />
      <div className="flex flex-col gap-1">
        <label htmlFor={`${id}-owner`} className="text-sm font-medium text-ink">
          {t.process.owner}
        </label>
        <select
          id={`${id}-owner`}
          name="ownerId"
          defaultValue={process.ownerId ?? ""}
          className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm"
        >
          <option value="">{t.process.noOwner}</option>
          {positions.map((p) => (
            <option key={p.id} value={p.id}>
              {p.title}
            </option>
          ))}
        </select>
      </div>
      <div className="flex flex-col gap-1 sm:col-span-2">
        <label htmlFor={`${id}-description`} className="text-sm font-medium text-ink">
          {t.process.description}
        </label>
        <textarea id={`${id}-description`} name="description" rows={2} defaultValue={process.description} className={inputClass} />
      </div>
      <div className="flex flex-wrap items-center gap-3 sm:col-span-2">
        <Button type="submit" disabled={update.isPending}>
          {t.process.saveDetails}
        </Button>
        <Button variant="secondary" onClick={onDone}>
          {t.process.cancel}
        </Button>
        {process.latest === null && (
          <Button variant="secondary" className="ml-auto" onClick={() => remove.mutate(undefined, { onSuccess: () => navigate({ to: "/processes" }) })}>
            {t.process.remove}
          </Button>
        )}
        <FormError error={update.error ?? remove.error} />
      </div>
    </form>
  );
}

function Editor({ process, search, onSearch }: { process: Process; search: ProcessSearch; onSearch: (next: ProcessSearch) => void }) {
  const role = useMe().data?.role;
  const mayEdit = !!role && atLeast(role, "editor");
  const viewing = search.version;
  const version = useProcessVersion(process.id, viewing);
  const processes = useProcesses();
  const organisation = useOrganisation(today());
  const draft = process.draft as NonNullable<Process["draft"]>;
  const [history, dispatch] = useReducer(edit, { nodes: draft.nodes, arrows: draft.arrows }, start);
  const [saved, setSaved] = useState<Flow>(history.present);
  const [problems, setProblems] = useState<Problem[]>(draft.problems);
  // The revision the next save is made from: the one read, then each save's own, never one a refetch brings in.
  const revision = useRef(draft.revision);
  const [selection, setSelection] = useState<Selection>(null);
  const [note, setNote] = useState("");
  const [editingDetails, setEditingDetails] = useState(false);
  // Each arrangement fits the view anew, so the whole flow is in sight where the layout put it.
  const [arranged, setArranged] = useState(0);
  const parties = useParties();
  const asOf = search.asOf ?? today();
  // The saved draft's people while the draft is shown; unsaved changes come down to people once saved.
  const people = useProcessPeople(process.id, viewing ?? "draft", asOf);
  const save = useSaveDraft(process.id);
  const publish = usePublish(process.id);

  const editable = mayEdit && viewing === undefined;
  const shown: Flow = viewing === undefined ? history.present : { nodes: version.data?.nodes ?? [], arrows: version.data?.arrows ?? [] };
  // A view nobody may change can still be laid out for reading; the layout goes with the version it was made for.
  const [tidy, setTidy] = useState<{ for: string; places: Record<string, Point>; routes: Record<string, number[]> } | undefined>();
  const tidyFor = `${viewing ?? "draft"}`;
  const tidied = tidy && tidy.for === tidyFor ? tidy : undefined;
  const flow: Flow = tidied
    ? {
        nodes: shown.nodes.map((n) => ({ ...n, ...(tidied.places[n.key] ?? {}) })),
        arrows: shown.arrows.map((a) => ({ ...a, route: tidied.routes[a.key] ?? [] })),
      }
    : shown;
  const onTidy = () => {
    if (tidied) {
      setTidy(undefined);
      return;
    }
    arrange(shown).then((laid) => setTidy({ for: tidyFor, ...laid }));
  };
  const dirty = history.present !== saved;
  const flagged = useMemo(
    () => new Set(viewing === undefined ? problems.flatMap((p) => [p.node, p.arrow].filter((k): k is string => !!k)) : []),
    [problems, viewing],
  );
  const names = useMemo(() => new Map((processes.data ?? []).map((p) => [p.id, p.name])), [processes.data]);
  const owner = organisation.data?.positions.find((p) => p.id === process.ownerId);

  useEffect(() => {
    if (!editable) return;
    const onKey = (event: KeyboardEvent) => {
      // Fields keep their own undo.
      if (event.target instanceof Element && event.target.closest("input, textarea, select")) return;
      if (!(event.ctrlKey || event.metaKey)) return;
      const key = event.key.toLowerCase();
      if (key === "z" || key === "y") {
        event.preventDefault();
        dispatch({ type: key === "y" || event.shiftKey ? "redo" : "undo" });
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [editable]);

  const onSave = () => {
    const sent = history.present;
    save.mutate(
      { revision: revision.current, nodes: sent.nodes, arrows: sent.arrows },
      {
        onSuccess: (changed) => {
          revision.current = changed.draft?.revision ?? revision.current;
          setProblems(changed.draft?.problems ?? []);
          setSaved(sent);
        },
      },
    );
  };
  const onPublish = () =>
    publish.mutate(note, {
      onSuccess: () => {
        setNote("");
        revision.current += 1;
      },
    });
  const onArrange = () =>
    arrange(history.present).then(({ places, routes }) => {
      dispatch({ type: "arrange", places, routes });
      setArranged((n) => n + 1);
    });
  const removePicked = () => {
    if (!selection) return;
    dispatch({ type: "remove", nodes: selection.kind === "node" ? [selection.key] : [], arrows: selection.kind === "arrow" ? [selection.key] : [] });
    setSelection(null);
  };
  const versions = process.versions ?? [];

  return (
    <div className="flex h-full flex-col">
      <header className="flex flex-wrap items-end gap-3 border-b border-border bg-surface px-6 py-3">
        <div className="mr-auto min-w-0">
          <p className="text-xs text-ink-subtle">
            <Link to="/processes" className="hover:underline">
              {t.processes.title}
            </Link>
            {process.area && ` · ${process.area}`}
          </p>
          <h1 className="truncate text-xl font-semibold">{process.name}</h1>
          <p className="text-sm text-ink-muted">
            {owner ? (
              <>
                {t.process.ownedBy} <PositionName id={owner.id} title={owner.title} />
              </>
            ) : (
              t.process.noOwner
            )}
            {process.description && ` · ${process.description}`}
          </p>
        </div>
        <div className="flex flex-col gap-1 text-xs text-ink-muted">
          <label htmlFor={`${process.id}-version`}>{t.process.showing}</label>
          <select
            id={`${process.id}-version`}
            value={viewing ?? ""}
            onChange={(e) => {
              setSelection(null);
              onSearch({ ...search, version: e.target.value ? Number(e.target.value) : undefined });
            }}
            className="h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink"
          >
            <option value="">{t.process.draft}</option>
            {versions.map((v) => (
              <option key={v.number} value={v.number}>
                {t.process.version(v.number)}
              </option>
            ))}
          </select>
        </div>
        <AsOfPicker value={asOf} onChange={(day) => onSearch({ ...search, asOf: day })} />
        <fieldset className="flex h-9 overflow-hidden rounded-md border border-border-strong text-sm">
          <legend className="sr-only">{t.process.viewAs}</legend>
          {(["canvas", "matrix"] as const).map((v) => (
            <button
              key={v}
              type="button"
              aria-pressed={(search.view ?? "canvas") === v}
              onClick={() => onSearch({ ...search, view: v === "matrix" ? "matrix" : undefined })}
              className={`px-3 ${(search.view ?? "canvas") === v ? "bg-primary text-on-primary" : "bg-surface text-ink hover:bg-surface-sunken"}`}
            >
              {t.process.views[v]}
            </button>
          ))}
        </fieldset>
        {!editable && search.view !== "matrix" && (
          <Button variant="secondary" onClick={onTidy} aria-pressed={!!tidied} disabled={shown.nodes.length === 0}>
            {tidied ? t.process.asDrawn : t.process.tidy}
          </Button>
        )}
        {editable && (
          <>
            <Button variant="secondary" onClick={() => dispatch({ type: "undo" })} disabled={history.past.length === 0}>
              {t.process.undo}
            </Button>
            <Button variant="secondary" onClick={() => dispatch({ type: "redo" })} disabled={history.future.length === 0}>
              {t.process.redo}
            </Button>
            <Button variant="secondary" onClick={onArrange} disabled={history.present.nodes.length === 0}>
              {t.process.arrange}
            </Button>
            <Button onClick={onSave} disabled={!dirty || save.isPending}>
              {dirty ? t.process.save : t.process.saved}
            </Button>
          </>
        )}
        {mayEdit && viewing === undefined && (
          <Button variant="secondary" onClick={() => setEditingDetails((was) => !was)} aria-expanded={editingDetails}>
            {t.process.details}
          </Button>
        )}
      </header>
      {editingDetails && <Details process={process} onDone={() => setEditingDetails(false)} />}
      {editable && <Palette flow={history.present} onEdit={dispatch} />}
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <div className="relative min-h-[60vh] flex-1 md:min-h-0">
          {search.view === "matrix" ? (
            <RaciMatrix
              flow={flow}
              nameOf={parties.nameOf}
              onPick={(key) => {
                setSelection({ kind: "node", key });
                onSearch({ ...search, view: undefined });
              }}
            />
          ) : (
            <FlowCanvas
              flow={flow}
              editable={editable}
              selection={selection}
              onSelect={setSelection}
              onEdit={dispatch}
              flagged={flagged}
              subprocesses={names}
              fitKey={`${process.id}|${viewing ?? "draft"}|${flow.nodes.length > 0}|${arranged}|${tidied ? "tidy" : "drawn"}`}
              parties={parties.names}
            />
          )}
        </div>
        <aside className="flex w-full flex-col gap-5 overflow-y-auto border-t border-border bg-surface p-5 md:w-80 md:border-t-0 md:border-l">
          <section aria-labelledby={`${process.id}-picked`}>
            <h2 id={`${process.id}-picked`} className="mb-2 text-sm font-semibold">
              {selection?.kind === "arrow" ? t.process.arrow : t.process.node}
            </h2>
            <Inspector
              flow={flow}
              selection={selection}
              editable={editable}
              others={(processes.data ?? []).filter((p) => p.id !== process.id)}
              parties={parties.list}
              nameOf={parties.nameOf}
              people={people.data ?? []}
              onEdit={dispatch}
              onRemove={removePicked}
            />
          </section>
          {viewing === undefined && (
            <section aria-labelledby={`${process.id}-problems`}>
              <h2 id={`${process.id}-problems`} className="mb-2 text-sm font-semibold">
                {t.process.problemsTitle}
              </h2>
              {dirty && <p className="mb-1 text-xs text-ink-subtle">{t.process.unsaved}</p>}
              <Problems problems={problems} flow={history.present} onPick={setSelection} />
              <FormError error={save.error} />
            </section>
          )}
          {editable && (
            <section aria-labelledby={`${process.id}-publish`} className="flex flex-col gap-2">
              <h2 id={`${process.id}-publish`} className="text-sm font-semibold">
                {t.process.publishTitle}
              </h2>
              <label htmlFor={`${process.id}-note`} className="text-xs text-ink-muted">
                {t.process.note}
              </label>
              <input id={`${process.id}-note`} value={note} onChange={(e) => setNote(e.target.value)} className={inputClass} />
              <Button onClick={onPublish} disabled={dirty || problems.some((p) => !p.warning) || publish.isPending} className="self-start">
                {t.process.publish}
              </Button>
              {dirty && <p className="text-xs text-ink-subtle">{t.process.saveFirst}</p>}
              <FormError error={publish.error} />
            </section>
          )}
          <section aria-labelledby={`${process.id}-versions`}>
            <h2 id={`${process.id}-versions`} className="mb-2 text-sm font-semibold">
              {t.process.versions}
            </h2>
            {versions.length === 0 ? (
              <p className="text-sm text-ink-muted">{t.process.neverPublished}</p>
            ) : (
              <ol className="space-y-2 text-sm">
                {versions.map((v) => (
                  <li key={v.number}>
                    <button type="button" onClick={() => onSearch({ ...search, version: v.number })} className="font-medium text-ink hover:underline">
                      {t.process.version(v.number)}
                    </button>
                    <span className="text-ink-subtle">
                      {" · "}
                      {new Date(v.publishedAt).toLocaleDateString()}
                      {v.publisherName && ` · ${v.publisherName}`}
                    </span>
                    {v.note && <p className="text-ink-muted">{v.note}</p>}
                  </li>
                ))}
              </ol>
            )}
          </section>
        </aside>
      </div>
    </div>
  );
}

/** A process drawn on a canvas: its draft to edit, for editors, and every version published from it to read. */
export function ProcessPage({ processId, search, onSearch }: { processId: string; search: ProcessSearch; onSearch: (next: ProcessSearch) => void }) {
  useLanguage();
  const process = useProcess(processId);
  if (process.isError) return <p className="px-6 py-8 text-sm text-danger">{t.process.unreachable}</p>;
  if (!process.data?.draft) return null;
  return (
    <ReactFlowProvider>
      <Editor key={process.data.id} process={process.data} search={search} onSearch={onSearch} />
    </ReactFlowProvider>
  );
}
