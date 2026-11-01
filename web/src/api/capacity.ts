import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type Target = components["schemas"]["TargetResponse"];
export type CapacityPeriod = components["schemas"]["CapacityPeriodResponse"];
export type Gap = components["schemas"]["GapResponse"];

/** Whose capacity or targets: a unit (with everything inside it), a position or a role. */
export type TargetSubject = { unitId: string } | { positionId: string } | { roleId: string };

const subjectKey = (subject: TargetSubject) => Object.entries(subject).flat().join(":");

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
