import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/api/client";
import { useOrganisation } from "@/api/org";
import type { RACI, Responsibility } from "@/api/processes";
import { today } from "@/features/org/common";
import { t } from "@/i18n";
import type { Flow } from "./editor";

export const RACIS: RACI[] = ["responsible", "accountable", "consulted", "informed"];

/** A party a responsibility names, as "role:<id>", "position:<id>" or "unit:<id>". */
export type PartyKey = `${"role" | "position" | "unit"}:${string}`;
export type Party = { key: PartyKey; kind: "role" | "position" | "unit"; name: string };

export function partyOf(r: Responsibility): PartyKey {
  if (r.roleId) return `role:${r.roleId}`;
  if (r.positionId) return `position:${r.positionId}`;
  return `unit:${r.unitId ?? ""}`;
}

export function responsibility(kind: RACI, party: PartyKey): Responsibility {
  const [type, id] = party.split(":") as [Party["kind"], string];
  return type === "role" ? { kind, roleId: id } : type === "position" ? { kind, positionId: id } : { kind, unitId: id };
}

/** The roles, and the positions and units of today, that a responsibility may name, each kind by name. */
export function useParties() {
  const organisation = useOrganisation(today());
  const roles = useQuery({ queryKey: ["roles"], queryFn: async () => unwrap(await api.GET("/roles")).roles });
  const byName = (a: Party, b: Party) => a.name.localeCompare(b.name);
  const list: Party[] = [
    ...(roles.data ?? []).map((r) => ({ key: `role:${r.id}` as PartyKey, kind: "role" as const, name: r.name })).sort(byName),
    ...(organisation.data?.positions ?? []).map((p) => ({ key: `position:${p.id}` as PartyKey, kind: "position" as const, name: p.title })).sort(byName),
    ...(organisation.data?.units ?? []).map((u) => ({ key: `unit:${u.id}` as PartyKey, kind: "unit" as const, name: u.name })).sort(byName),
  ];
  const names = new Map<string, string>(list.map((p) => [p.key, p.name]));
  const ready = roles.isSuccess && organisation.isSuccess;
  // Until all is read, a party not yet known is only not known yet, not gone.
  const nameOf = (key: string) => names.get(key) ?? (ready ? t.raci.unknown : "…");
  return { list, names, nameOf };
}

/** The nodes in the order the flow runs: from the start along the arrows, nodes reached by none after, as drawn. */
export function inFlowOrder(flow: Flow): string[] {
  const next = new Map<string, string[]>();
  for (const a of flow.arrows) next.set(a.from, [...(next.get(a.from) ?? []), a.to]);
  const seen = new Set<string>();
  const queue = flow.nodes.filter((n) => n.kind === "start").map((n) => n.key);
  const out: string[] = [];
  while (queue.length > 0) {
    const key = queue.shift() as string;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(key);
    queue.push(...(next.get(key) ?? []));
  }
  const rest = flow.nodes.filter((n) => !seen.has(n.key)).sort((a, b) => a.y - b.y || a.x - b.x);
  return [...out, ...rest.map((n) => n.key)];
}

export type Matrix = { rows: { key: string; name: string }[]; columns: PartyKey[]; cells: Map<string, RACI[]> };

/** The RACI matrix of a flow: its steps, decisions and subprocesses in flow order against every party named. */
export function matrix(flow: Flow): Matrix {
  const byKey = new Map(flow.nodes.map((n) => [n.key, n]));
  // Every step is a row, so one without parties shows as empty; a decision or a subprocess only when it has some.
  const rows = inFlowOrder(flow).flatMap((key) => {
    const n = byKey.get(key);
    const shown = n && (n.kind === "step" || ((n.kind === "decision" || n.kind === "subprocess") && n.responsibilities.length > 0));
    return shown ? [{ key, name: n.name }] : [];
  });
  const columns: PartyKey[] = [];
  const cells = new Map<string, RACI[]>();
  for (const row of rows) {
    for (const r of byKey.get(row.key)?.responsibilities ?? []) {
      const party = partyOf(r);
      if (!columns.includes(party)) columns.push(party);
      const cell = `${row.key}|${party}`;
      cells.set(
        cell,
        [...(cells.get(cell) ?? []), r.kind].sort((a, b) => RACIS.indexOf(a) - RACIS.indexOf(b)),
      );
    }
  }
  return { rows, columns, cells };
}
