import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

type Schemas = components["schemas"];

// An edit to the organisation reaches the chart, every page, capacity and gaps, so all of them are read again.
function useEdit<T, R>(fn: (input: T) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: fn, onSuccess: () => client.invalidateQueries() });
}

const body = <T extends object>(input: T) => Object.fromEntries(Object.entries(input).filter(([, v]) => v !== "" && v !== undefined)) as T;

export const useCreateUnit = () =>
  useEdit(
    async (input: { typeId: string; name: string; shortName?: string; parentId?: string; from?: string }) =>
      unwrap(await api.POST("/units", { body: body(input) })).unit,
  );

export const useUpdateUnit = (unitID: string) =>
  useEdit(async (input: Schemas["UnitRequest"]) => unwrap(await api.PATCH("/units/{unitID}", { params: { path: { unitID } }, body: input })).unit);

export const useMoveUnit = () =>
  useEdit(
    async ({ unitID, parentId, from }: { unitID: string; parentId: string; from: string }) =>
      unwrap(await api.POST("/units/{unitID}/move", { params: { path: { unitID } }, body: { parentId, from } })).unit,
  );

export const useArchiveUnit = (unitID: string) =>
  useEdit(async (on: string) => unwrap(await api.POST("/units/{unitID}/archive", { params: { path: { unitID } }, body: { on } })).unit);

export const useRestoreUnit = (unitID: string) => useEdit(async () => unwrap(await api.POST("/units/{unitID}/restore", { params: { path: { unitID } } })).unit);

export const useCreatePosition = () =>
  useEdit(
    async (input: { unitId: string; title: string; seats?: number; headOfUnit?: boolean; from?: string }) =>
      unwrap(await api.POST("/positions", { body: body(input) })).position,
  );

export const useUpdatePosition = (positionID: string) =>
  useEdit(
    async (input: Schemas["PositionRequest"]) => unwrap(await api.PATCH("/positions/{positionID}", { params: { path: { positionID } }, body: input })).position,
  );

export const useSetReportsTo = () =>
  useEdit(
    async ({ positionID, reportsToId, from }: { positionID: string; reportsToId?: string; from: string }) =>
      unwrap(await api.PUT("/positions/{positionID}/reports-to", { params: { path: { positionID } }, body: body({ reportsToId, from }) })).position,
  );

export const useArchivePosition = (positionID: string) =>
  useEdit(async (on: string) => unwrap(await api.POST("/positions/{positionID}/archive", { params: { path: { positionID } }, body: { on } })).position);

export const useGiveHolding = () =>
  useEdit(
    async (input: { personId: string; positionId: string; share?: number; from: string; to?: string }) =>
      unwrap(await api.POST("/holdings", { body: body(input) })).holding,
  );

export const useEndHolding = () =>
  useEdit(
    async ({ holdingID, on }: { holdingID: string; on: string }) =>
      unwrap(await api.POST("/holdings/{holdingID}/end", { params: { path: { holdingID } }, body: { on } })).holding,
  );

export const useRemoveHolding = () =>
  useEdit(async (holdingID: string) => unwrap(await api.DELETE("/holdings/{holdingID}", { params: { path: { holdingID } } })));

export const useCreatePerson = () => useEdit(async (input: Schemas["PersonRequest"]) => unwrap(await api.POST("/people", { body: body(input) })).person);

export const useUpdatePerson = (personID: string) =>
  useEdit(async (input: Schemas["PersonRequest"]) => unwrap(await api.PATCH("/people/{personID}", { params: { path: { personID } }, body: input })).person);

export const useLeave = (personID: string) =>
  useEdit(async (on: string) => unwrap(await api.POST("/people/{personID}/leave", { params: { path: { personID } }, body: { on } })).person);

export const useReturn = (personID: string) =>
  useEdit(async () => unwrap(await api.POST("/people/{personID}/return", { params: { path: { personID } } })).person);

export const useCreateRole = () => useEdit(async (input: Schemas["OrgRoleRequest"]) => unwrap(await api.POST("/roles", { body: body(input) })).role);

export const useUpdateRole = (roleID: string) =>
  useEdit(async (input: Schemas["OrgRoleRequest"]) => unwrap(await api.PATCH("/roles/{roleID}", { params: { path: { roleID } }, body: input })).role);

export const useDeleteRole = (roleID: string) => useEdit(async () => unwrap(await api.DELETE("/roles/{roleID}", { params: { path: { roleID } } })));
