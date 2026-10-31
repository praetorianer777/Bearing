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
