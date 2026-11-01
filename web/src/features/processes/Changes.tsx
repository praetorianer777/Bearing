import { t } from "@/i18n";
import type { Diff, Mark } from "./diff";
import type { Selection } from "./FlowCanvas";

const markClass: Record<Mark, string> = { added: "text-success", removed: "text-danger", changed: "text-warning" };

function MarkLabel({ mark }: { mark: Mark }) {
  return <span className={`mr-1.5 text-xs font-semibold ${markClass[mark]}`}>{t.compare.marks[mark]}</span>;
}

/** What changed between two versions, node by node in the order the flow runs and then arrow by arrow; each leads to it on the canvas. */
export function Changes({ diff, nameOf, onPick }: { diff: Diff; nameOf: (key: string) => string; onPick: (s: Selection) => void }) {
  if (diff.nodes.length === 0 && diff.arrows.length === 0) return <p className="text-sm text-ink-muted">{t.compare.same}</p>;
  const names = new Map(diff.merged.nodes.map((n) => [n.key, n.name || t.process.unnamed]));
  return (
    <div className="flex flex-col gap-3 text-sm">
      {diff.nodes.length > 0 && (
        <ul aria-label={t.compare.nodes} className="space-y-1.5">
          {diff.nodes.map((n) => (
            <li key={n.key}>
              <MarkLabel mark={n.mark} />
              <button type="button" onClick={() => onPick({ kind: "node", key: n.key })} className="text-left text-ink hover:underline">
                {t.process.kinds[n.kind]}: {n.name || t.process.unnamed}
              </button>
              {n.fields.length > 0 && <p className="pl-1 text-xs text-ink-muted">{n.fields.map((f) => t.compare.fields[f]).join(", ")}</p>}
              {n.responsibilities.length > 0 && (
                <ul className="pl-1 text-xs text-ink-muted">
                  {n.responsibilities.map((r) => (
                    <li key={`${r.mark}-${r.kind}-${r.party}`}>
                      <span className={markClass[r.mark]}>{r.mark === "added" ? "+" : "−"}</span> {t.raci.kinds[r.kind]}: {nameOf(r.party)}
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ul>
      )}
      {diff.arrows.length > 0 && (
        <ul aria-label={t.compare.arrows} className="space-y-1.5">
          {diff.arrows.map((a) => (
            <li key={a.key}>
              <MarkLabel mark={a.mark} />
              <button type="button" onClick={() => onPick({ kind: "arrow", key: a.key })} className="text-left text-ink hover:underline">
                {t.process.arrowBetween(names.get(a.from) ?? a.from, names.get(a.to) ?? a.to)}
                {a.label && ` (${a.label})`}
              </button>
              {a.fields.length > 0 && <p className="pl-1 text-xs text-ink-muted">{a.fields.map((f) => t.compare.fields[f]).join(", ")}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
