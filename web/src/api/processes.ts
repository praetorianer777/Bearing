import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type Process = components["schemas"]["ProcessResponse"];
export type ProcessInput = components["schemas"]["ProcessRequest"];
export type FlowNode = components["schemas"]["NodeBody"];
export type FlowArrow = components["schemas"]["ArrowBody"];
export type NodeKind = FlowNode["kind"];
export type Problem = components["schemas"]["ProblemResponse"];
export type Version = components["schemas"]["VersionResponse"];

const processesKey = ["processes"];
const processKey = (processID: string) => ["processes", processID];

export const useProcesses = () => useQuery({ queryKey: processesKey, queryFn: async () => unwrap(await api.GET("/processes")).processes });

/** One process with its draft, the draft's problems and the versions published from it. */
export const useProcess = (processID: string) =>
  useQuery({
    queryKey: processKey(processID),
    queryFn: async () => unwrap(await api.GET("/processes/{processID}", { params: { path: { processID } } })).process,
  });

/** A published version with its graph; published versions never change, so it is read once. */
export const useProcessVersion = (processID: string, number?: number) =>
  useQuery({
    queryKey: ["processes", processID, "versions", number],
    enabled: number !== undefined,
    staleTime: Number.POSITIVE_INFINITY,
    queryFn: async () => unwrap(await api.GET("/processes/{processID}/versions/{number}", { params: { path: { processID, number: String(number) } } })).version,
  });

export const useCreateProcess = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: ProcessInput) => unwrap(await api.POST("/processes", { body })).process,
    onSuccess: (made) => {
      client.setQueryData(processKey(made.id), made);
      return client.invalidateQueries({ queryKey: processesKey, exact: true });
    },
  });
};

/** A change to the process itself or to its draft; the answer is the process as it now is. */
function useProcessChange<T>(processID: string, fn: (input: T) => Promise<Process>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (changed) => {
      client.setQueryData(processKey(processID), changed);
      void client.invalidateQueries({ queryKey: [...processKey(processID), "people"] });
      return client.invalidateQueries({ queryKey: processesKey, exact: true });
    },
  });
}

export const useUpdateProcess = (processID: string) =>
  useProcessChange(
    processID,
    async (body: ProcessInput) => unwrap(await api.PATCH("/processes/{processID}", { params: { path: { processID } }, body })).process,
  );

export const useSaveDraft = (processID: string) =>
  useProcessChange(
    processID,
    async (body: { revision: number; nodes: FlowNode[]; arrows: FlowArrow[] }) =>
      unwrap(await api.PUT("/processes/{processID}/draft", { params: { path: { processID } }, body })).process,
  );

export const usePublish = (processID: string) => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (note: string) => unwrap(await api.POST("/processes/{processID}/publish", { params: { path: { processID } }, body: { note } })).version,
    onSuccess: () => client.invalidateQueries({ queryKey: processesKey }),
  });
};

export const useDeleteProcess = (processID: string) => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.DELETE("/processes/{processID}", { params: { path: { processID } } })),
    onSuccess: () => {
      client.removeQueries({ queryKey: processKey(processID) });
      return client.invalidateQueries({ queryKey: processesKey, exact: true });
    },
  });
};

export type Responsibility = components["schemas"]["ResponsibilityBody"];
export type RACI = Responsibility["kind"];

export type StepPeople = components["schemas"]["StepPeopleResponse"];

/** Who each node of a version comes down to on a day; version is a number, the draft, or the newest published when left out. */
export const useProcessPeople = (processID: string, version: number | "draft" | undefined, asOf: string) =>
  useQuery({
    queryKey: ["processes", processID, "people", version ?? "newest", asOf],
    queryFn: async () =>
      unwrap(
        await api.GET("/processes/{processID}/people", {
          params: { path: { processID }, query: { asOf, version: version === undefined ? undefined : String(version) } },
        }),
      ).steps,
  });
