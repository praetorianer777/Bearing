import type { SearchHit } from "@/api/search";

/** Where a hit leads: its page, and for a step the version of its process it stands in. */
export function hitTarget(hit: SearchHit, asOf?: string) {
  const day = asOf ? { asOf } : {};
  switch (hit.kind) {
    case "person":
      return { to: "/people/$personId", params: { personId: hit.id }, search: day } as const;
    case "position":
      return { to: "/positions/$positionId", params: { positionId: hit.id }, search: day } as const;
    case "unit":
      return { to: "/units/$unitId", params: { unitId: hit.id }, search: day } as const;
    case "role":
      return { to: "/roles/$roleId", params: { roleId: hit.id }, search: day } as const;
    default:
      return { to: "/processes/$processId", params: { processId: hit.id }, search: { ...day, ...(hit.version ? { version: hit.version } : {}) } } as const;
  }
}

/** A key telling hits apart: steps share their process's id. */
export const hitKey = (hit: SearchHit) => `${hit.kind}-${hit.id}-${hit.stepKey ?? ""}`;
