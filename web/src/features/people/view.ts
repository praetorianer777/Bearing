import type { Person, Position, Unit } from "@/api/org";
import type { Holding, OrgRole } from "@/api/people";

/** A holding as the page names it: of a position in a unit, or of a role held directly. */
export type Held = {
  holdingId: string;
  kind: "position" | "role";
  title: string;
  positionId?: string;
  roleId?: string;
  unitId?: string;
  unitName?: string;
  share: number;
  from: string;
  to: string | null;
};

/** A role somebody has on the day, held directly or through a position that carries it. */
export type HadRole = { roleId: string; name: string; through?: string };

/** A position as the page lists it with whoever holds it on the day. */
export type Seat = { positionId: string; title: string; holders: string[] };

export type PersonPageData = {
  person: Person;
  current: Holding[];
  history: Holding[];
  units: Unit[];
  positions: Position[];
  people: Person[];
  roles: OrgRole[];
};

export type PersonView = {
  current: Held[];
  history: Held[];
  roles: HadRole[];
  /** For each position the person holds, the positions standing in for it, first first. */
  deputies: { for: string; by: Seat[] }[];
  /** The positions the person's positions stand in for. */
  standsInFor: Seat[];
};

/** What the person page shows, from what the API answered. */
export function personView(data: PersonPageData): PersonView {
  const positions = new Map(data.positions.map((p) => [p.id, p]));
  const units = new Map(data.units.map((u) => [u.id, u.name]));
  const roles = new Map(data.roles.map((r) => [r.id, r.name]));
  const names = new Map(data.people.map((p) => [p.id, p.name]));

  const named = (h: Holding): Held => {
    const base = { holdingId: h.id, share: h.share, from: h.from, to: h.to };
    if (h.positionId) {
      const p = positions.get(h.positionId);
      return { ...base, kind: "position", positionId: h.positionId, title: p?.title ?? "", unitId: p?.unitId, unitName: p ? units.get(p.unitId) : undefined };
    }
    return { ...base, kind: "role", roleId: h.roleId ?? undefined, title: roles.get(h.roleId ?? "") ?? "" };
  };
  const seat = (positionId: string): Seat => {
    const p = positions.get(positionId);
    return { positionId, title: p?.title ?? "", holders: (p?.holders ?? []).map((h) => names.get(h.personId) ?? "") };
  };

  const current = data.current.map(named);
  const held = current.flatMap((h) => (h.positionId ? [h.positionId] : []));
  const had: HadRole[] = [
    ...current.flatMap((h) => (h.kind === "role" && h.roleId ? [{ roleId: h.roleId, name: h.title }] : [])),
    ...held.flatMap((id) => (positions.get(id)?.roles ?? []).map((roleId) => ({ roleId, name: roles.get(roleId) ?? "", through: positions.get(id)?.title }))),
  ];

  return {
    current,
    history: [...data.history].sort((a, b) => b.from.localeCompare(a.from)).map(named),
    roles: had.sort((a, b) => a.name.localeCompare(b.name)),
    deputies: held.map((id) => ({ for: positions.get(id)?.title ?? "", by: (positions.get(id)?.deputies ?? []).map(seat) })).filter((d) => d.by.length > 0),
    standsInFor: data.positions.filter((p) => p.deputies.some((d) => held.includes(d))).map((p) => seat(p.id)),
  };
}
