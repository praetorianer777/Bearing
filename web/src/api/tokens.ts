import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type PersonalToken = components["schemas"]["PersonalTokenResponse"];

const tokensKey = ["tokens"];

export const usePersonalTokens = () => useQuery({ queryKey: tokensKey, queryFn: async () => unwrap(await api.GET("/tokens")).tokens });

export const useMakeToken = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string; write: boolean; days: number }) => unwrap(await api.POST("/tokens", { body })),
    onSuccess: () => client.invalidateQueries({ queryKey: tokensKey }),
  });
};

export const useRevokeToken = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (tokenID: string) => unwrap(await api.DELETE("/tokens/{tokenID}", { params: { path: { tokenID } } })),
    onSuccess: () => client.invalidateQueries({ queryKey: tokensKey }),
  });
};
