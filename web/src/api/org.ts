import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type Unit = components["schemas"]["UnitResponse"];
export type Position = components["schemas"]["PositionResponse"];
export type Person = components["schemas"]["PersonResponse"];

/** The units, positions and people of the organisation as they stand on a day, which the chart is drawn from. */
export const useOrganisation = (asOf: string) =>
  useQuery({
    queryKey: ["organisation", asOf],
    queryFn: async () => {
      const query = { params: { query: { asOf } } };
      const [units, positions, people] = await Promise.all([api.GET("/units", query), api.GET("/positions", query), api.GET("/people", query)]);
      return { units: unwrap(units).units, positions: unwrap(positions).positions, people: unwrap(people).people };
    },
  });

export type UnitType = components["schemas"]["UnitTypeResponse"];
export type Member = components["schemas"]["MemberResponse"];

/** Positions the organisation of a day does not name, read as they stood on their own day: archived ones a line or a holding still points to. */
async function missingPositions(ids: string[], known: Map<string, Position>, onDay: (id: string) => string): Promise<Position[]> {
  const missing = [...new Set(ids)].filter((id) => !known.has(id));
  return Promise.all(
    missing.map(
      async (positionID) => unwrap(await api.GET("/positions/{positionID}", { params: { path: { positionID }, query: { asOf: onDay(positionID) } } })).position,
    ),
  );
}

/** Everything the unit page shows, as of a day. */
export const useUnitPage = (unitID: string, asOf: string) =>
  useQuery({
    queryKey: ["unit-page", unitID, asOf],
    queryFn: async () => {
      const day = { params: { query: { asOf } } };
      const [unit, units, types, positions, members] = await Promise.all([
        api.GET("/units/{unitID}", { params: { path: { unitID }, query: { asOf } } }),
        api.GET("/units", day),
        api.GET("/unit-types"),
        api.GET("/positions", { params: { query: { asOf, unitId: unitID } } }),
        api.GET("/units/{unitID}/members", { params: { path: { unitID }, query: { asOf, subUnits: "true" } } }),
      ]);
      return {
        unit: unwrap(unit).unit,
        units: unwrap(units).units,
        types: unwrap(types).unitTypes,
        positions: unwrap(positions).positions,
        members: unwrap(members).members,
      };
    },
  });

/** Everything the position page shows, as of a day. */
export const usePositionPage = (positionID: string, asOf: string) =>
  useQuery({
    queryKey: ["position-page", positionID, asOf],
    queryFn: async () => {
      const day = { params: { query: { asOf } } };
      const path = { params: { path: { positionID }, query: { asOf } } };
      const [position, history, chain, reports, units, positions, present, left, roles] = await Promise.all([
        api.GET("/positions/{positionID}", path),
        api.GET("/holdings", { params: { query: { positionId: positionID } } }),
        api.GET("/positions/{positionID}/chain", path),
        api.GET("/positions/{positionID}/reports", path),
        api.GET("/units", day),
        api.GET("/positions", day),
        api.GET("/people", day),
        api.GET("/people", { params: { query: { asOf, left: "true" } } }),
        api.GET("/roles"),
      ]);
      const self = unwrap(position).position;
      const known = new Map(unwrap(positions).positions.map((p) => [p.id, p]));
      const lines = self.lines ?? [];
      const earlier = await missingPositions(
        lines.map((l) => l.reportsToId),
        known,
        (id) => lines.find((l) => l.reportsToId === id)?.from ?? asOf,
      );
      return {
        position: self,
        history: unwrap(history).holdings,
        chain: unwrap(chain).positions,
        reports: unwrap(reports).positions,
        units: unwrap(units).units,
        positions: [...known.values(), ...earlier],
        people: [...unwrap(present).people, ...unwrap(left).people],
        roles: unwrap(roles).roles,
      };
    },
  });
