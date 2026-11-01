import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type Target = components["schemas"]["TargetResponse"];
export type CapacityPeriod = components["schemas"]["CapacityPeriodResponse"];
export type Gap = components["schemas"]["GapResponse"];

/** Whose capacity or targets: a unit (with everything inside it), a position or a role. */
export type TargetSubject = { unitId: string } | { positionId: string } | { roleId: string };

const subjectKey = (subject: Record<string, string>) => Object.entries(subject).flat().join(":");

/** A subject's targets, oldest first. */
export const useTargets = (subject: TargetSubject) =>
  useQuery({
    queryKey: ["targets", subjectKey(subject)],
    queryFn: async () => unwrap(await api.GET("/targets", { params: { query: subject } })).targets,
  });

/** A subject's capacity on one day; a unit's with everything inside it, as its target is meant. */
export const useCapacityOn = (subject: TargetSubject, day: string) =>
  useQuery({
    queryKey: ["capacity", subjectKey(subject), day],
    queryFn: async () => {
      const query = { from: day, to: day, by: "day" };
      const answer =
        "unitId" in subject
          ? await api.GET("/units/{unitID}/capacity", { params: { path: { unitID: subject.unitId }, query: { ...query, subUnits: "true" } } })
          : "positionId" in subject
            ? await api.GET("/positions/{positionID}/capacity", { params: { path: { positionID: subject.positionId }, query } })
            : await api.GET("/roles/{roleID}/capacity", { params: { path: { roleID: subject.roleId }, query } });
      return unwrap(answer).periods[0] as CapacityPeriod | undefined;
    },
  });

function useTargetMutation<T, R>(fn: (input: T) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () =>
      Promise.all([
        client.invalidateQueries({ queryKey: ["targets"] }),
        client.invalidateQueries({ queryKey: ["capacity"] }),
        client.invalidateQueries({ queryKey: ["gaps"] }),
        client.invalidateQueries({ queryKey: ["audit"] }),
      ]),
  });
}

export const useSetTarget = (subject: TargetSubject) =>
  useTargetMutation(async (input: { fte: number; from: string; to?: string }) => unwrap(await api.POST("/targets", { body: { ...subject, ...input } })).target);

export const useRemoveTarget = () =>
  useTargetMutation(async (targetID: string) => unwrap(await api.DELETE("/targets/{targetID}", { params: { path: { targetID } } })));

/** Where people are missing, or too many, over a range: in a unit (with its sub-units) or the whole organisation. */
export const useGaps = (range: { from: string; to: string; unitId?: string }) =>
  useQuery({
    queryKey: ["gaps", range.unitId ?? "", range.from, range.to],
    queryFn: async () =>
      unwrap(
        await api.GET("/gaps", { params: { query: { from: range.from, to: range.to, ...(range.unitId ? { unitId: range.unitId, subUnits: "true" } : {}) } } }),
      ).gaps,
  });

export type CapacityBy = "week" | "month";

/** Who a row of the planning grid counts: a unit with everything inside it, a position or a person. */
export type PlanningSubject = { unitId: string } | { positionId: string } | { personId: string };

/** A subject's capacity period by period over a range. */
export const useCapacityRange = (subject: PlanningSubject, range: { from: string; to: string; by: CapacityBy }) =>
  useQuery({
    queryKey: ["capacity", subjectKey(subject), range.from, range.to, range.by],
    queryFn: async () => {
      const query = { from: range.from, to: range.to, by: range.by };
      const answer =
        "unitId" in subject
          ? await api.GET("/units/{unitID}/capacity", { params: { path: { unitID: subject.unitId }, query: { ...query, subUnits: "true" } } })
          : "positionId" in subject
            ? await api.GET("/positions/{positionID}/capacity", { params: { path: { positionID: subject.positionId }, query } })
            : await api.GET("/people/{personID}/capacity", { params: { path: { personID: subject.personId }, query } });
      return unwrap(answer).periods;
    },
  });

export type PlannedHolding = { positionId: string; personId?: string; name?: string; from: string; to?: string; share: number };

/** Plans a holding into a position: of somebody in the organisation, or of somebody new added as external first. */
export const usePlanHolding = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ positionId, personId, name, from, to, share }: PlannedHolding) => {
      const holder = personId ?? unwrap(await api.POST("/people", { body: { name: name ?? "" } })).person.id;
      return unwrap(await api.POST("/holdings", { body: { personId: holder, positionId, from, share, ...(to ? { to } : {}) } })).holding;
    },
    onSuccess: () => Promise.all(["capacity", "gaps", "holdings", "organisation", "people"].map((key) => client.invalidateQueries({ queryKey: [key] }))),
  });
};

export type Demand = components["schemas"]["DemandResponse"];

/** A unit's capacity against what Armature says the work asks of its people, week by week; partner says why not. */
export const useDemand = (unitId: string | undefined, range: { from: string; to: string }) =>
  useQuery({
    queryKey: ["demand", unitId ?? "", range.from, range.to],
    queryFn: async () =>
      unwrap(
        await api.GET("/units/{unitID}/demand", {
          params: { path: { unitID: unitId ?? "" }, query: { from: range.from, to: range.to, subUnits: "true" } },
        }),
      ),
    enabled: !!unitId,
  });
