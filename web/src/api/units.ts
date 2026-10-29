import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type UnitType = components["schemas"]["UnitTypeResponse"];
export type UnitTypeInput = components["schemas"]["UnitTypeRequest"];

const unitTypesKey = ["unit-types"];

export const useUnitTypes = () => useQuery({ queryKey: unitTypesKey, queryFn: async () => unwrap(await api.GET("/unit-types")).unitTypes });

function useUnitTypesMutation<T, R>(fn: (input: T) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: fn, onSuccess: () => client.invalidateQueries({ queryKey: unitTypesKey }) });
}

export const useCreateUnitType = () => useUnitTypesMutation(async (body: UnitTypeInput) => unwrap(await api.POST("/unit-types", { body })).unitType);

export const useUpdateUnitType = () =>
  useUnitTypesMutation(
    async ({ unitTypeID, ...body }: UnitTypeInput & { unitTypeID: string }) =>
      unwrap(await api.PATCH("/unit-types/{unitTypeID}", { params: { path: { unitTypeID } }, body })).unitType,
  );

export const useDeleteUnitType = () =>
  useUnitTypesMutation(async (unitTypeID: string) => unwrap(await api.DELETE("/unit-types/{unitTypeID}", { params: { path: { unitTypeID } } })));
