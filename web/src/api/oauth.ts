import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type Consent = NonNullable<components["schemas"]["ConsentResponse"]["consent"]>;
export type Grant = components["schemas"]["GrantResponse"];
export type App = components["schemas"]["ClientResponse"];

/** What the consent page asks, for the authorize request it was opened with, passed on exactly as it came. */
export const useConsent = (query: string) =>
  useQuery({
    queryKey: ["consent", query],
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/oauth/consent", { querySerializer: () => query.replace(/^\?/, "") })),
  });

export const useDecide = () =>
  useMutation({ mutationFn: async (body: { query: string; allow: boolean; scope?: string }) => unwrap(await api.POST("/oauth/consent", { body })) });

const grantsKey = ["connected-apps"];
const appsKey = ["apps"];

export const useGrants = () => useQuery({ queryKey: grantsKey, queryFn: async () => unwrap(await api.GET("/connected-apps")).grants });

export const useDisconnect = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (grantID: string) => unwrap(await api.DELETE("/connected-apps/{grantID}", { params: { path: { grantID } } })),
    onSuccess: () => client.invalidateQueries({ queryKey: grantsKey }),
  });
};

export const useApps = (enabled: boolean) => useQuery({ queryKey: appsKey, enabled, queryFn: async () => unwrap(await api.GET("/oauth/clients")).clients });

export const useRemoveApp = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (clientID: string) => unwrap(await api.DELETE("/oauth/clients/{clientID}", { params: { path: { clientID } } })),
    onSuccess: () => Promise.all([client.invalidateQueries({ queryKey: appsKey }), client.invalidateQueries({ queryKey: grantsKey })]),
  });
};
