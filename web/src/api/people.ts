import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { Position } from "./org";
import type { components } from "./schema";

export type Holding = components["schemas"]["HoldingResponse"];
export type Link = components["schemas"]["LinkResponse"];
export type OrgRole = components["schemas"]["OrgRoleResponse"];

/** Who is in the organisation on a day, or with left, who has left by then; q narrows to names and addresses. */
export const usePeople = (asOf: string, q: string, left: boolean) =>
  useQuery({
    queryKey: ["people", asOf, q, left],
    queryFn: async () => unwrap(await api.GET("/people", { params: { query: { asOf, q: q || undefined, left: left ? "true" : undefined } } })).people,
  });

/**
 * Everything the person page shows, as of a day: the entry with what it holds then, every holding it ever had, the
 * chain above and the reports below, and the organisation of that day to name positions, units, roles and people by.
 * Positions held once and gone by the day are read as they were when the holding began.
 */
export const usePersonPage = (personID: string, asOf: string) =>
  useQuery({
    queryKey: ["person-page", personID, asOf],
    queryFn: async () => {
      const path = { params: { path: { personID }, query: { asOf } } };
      const day = { params: { query: { asOf } } };
      const [entry, history, chain, reports, units, positions, people, roles] = await Promise.all([
        api.GET("/people/{personID}", path),
        api.GET("/holdings", { params: { query: { personId: personID } } }),
        api.GET("/people/{personID}/chain", path),
        api.GET("/people/{personID}/reports", path),
        api.GET("/units", day),
        api.GET("/positions", day),
        api.GET("/people", day),
        api.GET("/roles"),
      ]);
      const known = new Map(unwrap(positions).positions.map((p) => [p.id, p]));
      const holdings = unwrap(history).holdings;
      const gone = [...new Set(holdings.flatMap((h) => (h.positionId && !known.has(h.positionId) ? [h] : [])).map((h) => h.positionId as string))];
      const earlier: Position[] = await Promise.all(
        gone.map(async (positionID) => {
          const from = holdings.find((h) => h.positionId === positionID)?.from ?? asOf;
          return unwrap(await api.GET("/positions/{positionID}", { params: { path: { positionID }, query: { asOf: from } } })).position;
        }),
      );
      const { person, holdings: current } = unwrap(entry);
      return {
        person,
        current,
        history: holdings,
        chain: unwrap(chain).positions,
        reports: unwrap(reports).positions,
        units: unwrap(units).units,
        positions: [...known.values(), ...earlier],
        people: unwrap(people).people,
        roles: unwrap(roles).roles,
      };
    },
  });
