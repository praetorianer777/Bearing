import { useId, useState } from "react";
import type { FlowNode, RACI } from "@/api/processes";
import { Button } from "@/components/ui";
import { t } from "@/i18n";
import type { Edit, Flow } from "./editor";
import { type Party, type PartyKey, partyOf, RACIS, matrix, responsibility } from "./raci";

const selectClass = "h-9 rounded-md border border-border-strong bg-surface px-2 text-sm text-ink";

/** A node's responsibilities as a list; editable, each is removed on its own and a new one added below. */
export function NodeResponsibilities({
  node,
  editable,
  parties,
  nameOf,
  onEdit,
}: {
  node: FlowNode;
  editable: boolean;
  parties: Party[];
  nameOf: (key: string) => string;
  onEdit: (e: Edit) => void;
}) {
  const id = useId();
  const [kind, setKind] = useState<RACI>("responsible");
  const [party, setParty] = useState<PartyKey | "">("");
  const list = [...node.responsibilities].sort((a, b) => RACIS.indexOf(a.kind) - RACIS.indexOf(b.kind));
  const taken = (p: PartyKey) => node.responsibilities.some((r) => r.kind === kind && partyOf(r) === p);
  const add = () => {
    if (!party || taken(party)) return;
    onEdit({ type: "responsibilities", key: node.key, list: [...node.responsibilities, responsibility(kind, party)] });
    setParty("");
  };
  const remove = (index: number) => {
    const gone = list[index];
    onEdit({ type: "responsibilities", key: node.key, list: node.responsibilities.filter((r) => r !== gone) });
  };
  const groups = (["role", "position", "unit"] as const).map((k) => [k, parties.filter((p) => p.kind === k)] as const);
  return (
    <section aria-labelledby={`${id}-heading`} className="flex flex-col gap-2">
      <h3 id={`${id}-heading`} className="text-xs text-ink-muted">
        {t.raci.title}
      </h3>
      {list.length === 0 ? (
        <p className="text-sm text-ink-subtle">{t.raci.none}</p>
      ) : (
        <ul className="space-y-1">
          {list.map((r, i) => (
            <li key={`${r.kind}-${partyOf(r)}`} className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate">
                <abbr title={t.raci.kinds[r.kind]} className="mr-1.5 inline-block w-4 font-semibold no-underline">
                  {t.raci.letters[r.kind]}
                </abbr>
                {nameOf(partyOf(r))}
              </span>
              {editable && (
                <button
                  type="button"
                  onClick={() => remove(i)}
                  aria-label={t.raci.remove(t.raci.kinds[r.kind], nameOf(partyOf(r)))}
                  className="shrink-0 text-xs text-ink-subtle hover:text-danger"
                >
                  {t.raci.removeShort}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {editable && (
        <div className="flex flex-col gap-2">
          <div className="flex gap-2">
            <label htmlFor={`${id}-kind`} className="sr-only">
              {t.raci.kind}
            </label>
            <select id={`${id}-kind`} value={kind} onChange={(e) => setKind(e.target.value as RACI)} className={selectClass}>
              {RACIS.map((k) => (
                <option key={k} value={k}>
                  {t.raci.kinds[k]}
                </option>
              ))}
            </select>
            <label htmlFor={`${id}-party`} className="sr-only">
              {t.raci.party}
            </label>
            <select id={`${id}-party`} value={party} onChange={(e) => setParty(e.target.value as PartyKey | "")} className={`${selectClass} min-w-0 flex-1`}>
              <option value="">{t.raci.choose}</option>
              {groups.map(([k, ps]) => (
                <optgroup key={k} label={t.raci.groups[k]}>
                  {ps.map((p) => (
                    <option key={p.key} value={p.key} disabled={taken(p.key)}>
                      {p.name}
                    </option>
                  ))}
                </optgroup>
              ))}
            </select>
          </div>
          <Button variant="secondary" onClick={add} disabled={!party} className="self-start">
            {t.raci.add}
          </Button>
        </div>
      )}
    </section>
  );
}

/** The RACI matrix: the flow's steps down, the parties across, and in each cell how the party stands to the step. */
export function RaciMatrix({ flow, nameOf, onPick }: { flow: Flow; nameOf: (key: string) => string; onPick: (key: string) => void }) {
  const { rows, columns, cells } = matrix(flow);
  if (rows.length === 0) return <p className="p-6 text-sm text-ink-muted">{t.raci.noSteps}</p>;
  return (
    <div className="h-full overflow-auto p-6">
      <table className="border-collapse text-sm">
        <caption className="mb-3 text-left text-ink-muted">{t.raci.caption}</caption>
        <thead>
          <tr>
            <th scope="col" className="sticky left-0 bg-canvas px-3 py-2 text-left font-semibold">
              {t.raci.step}
            </th>
            {columns.map((c) => (
              <th key={c} scope="col" className="max-w-36 border-b border-border px-3 py-2 text-left align-bottom font-medium">
                <span className="block text-xs font-normal text-ink-subtle">{t.raci.groups[c.split(":")[0] as Party["kind"]]}</span>
                {nameOf(c)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const kinds = columns.flatMap((c) => cells.get(`${row.key}|${c}`) ?? []);
            const accountable = kinds.filter((k) => k === "accountable").length;
            const short = accountable !== 1 || !kinds.includes("responsible");
            return (
              <tr key={row.key} className="border-b border-border">
                <th scope="row" className="sticky left-0 bg-canvas px-3 py-2 text-left font-normal">
                  <button type="button" onClick={() => onPick(row.key)} className="text-left text-ink hover:underline">
                    {row.name || t.process.unnamed}
                  </button>
                  {short && <span className="ml-2 text-xs text-warning">{t.raci.short}</span>}
                </th>
                {columns.map((c) => {
                  const here = cells.get(`${row.key}|${c}`) ?? [];
                  return (
                    <td key={c} className={`px-3 py-2 text-center font-semibold ${here.includes("accountable") ? "bg-accent-subtle" : ""}`}>
                      {here.map((k) => (
                        <abbr key={k} title={t.raci.kinds[k]} className="no-underline">
                          {t.raci.letters[k]}
                        </abbr>
                      ))}
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
