import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, unwrap } from "./client";
import type { components } from "./schema";

export type Account = components["schemas"]["AccountResponse"];
export type Role = Account["role"];
export type Invitation = components["schemas"]["InvitationResponse"];
export type InstallationInfo = components["schemas"]["InstallationResponse"];

export const ROLES: Role[] = ["administrator", "editor", "member", "reader"];

export const installationQuery = {
  queryKey: ["installation"],
  queryFn: async (): Promise<InstallationInfo> => unwrap(await api.GET("/installation")),
} as const;

/** Who is signed in, or null for nobody. */
export const meQuery = {
  queryKey: ["me"],
  queryFn: async (): Promise<Account | null> => {
    try {
      return unwrap(await api.GET("/auth/me")).account;
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return null;
      throw err;
    }
  },
} as const;

export const useInstallation = () => useQuery(installationQuery);
export const useMe = () => useQuery(meQuery);

export function useSignIn() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: { email: string; password: string }) => unwrap(await api.POST("/auth/sign-in", { body })).account,
    onSuccess: (account) => client.setQueryData(meQuery.queryKey, account),
  });
}

export function useSetUp() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string; adminName: string; email: string; password: string }) =>
      unwrap(await api.POST("/installation/setup", { body })).account,
    onSuccess: async (account) => {
      client.setQueryData(meQuery.queryKey, account);
      await client.invalidateQueries({ queryKey: installationQuery.queryKey });
    },
  });
}

export function useSignOut() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/auth/sign-out")),
    onSuccess: () => {
      client.clear();
      client.setQueryData(meQuery.queryKey, null);
    },
  });
}

export function useRequestPasswordReset() {
  return useMutation({ mutationFn: async (email: string) => unwrap(await api.POST("/auth/password-reset", { body: { email } })) });
}

export function useResetPassword(token: string) {
  return useMutation({
    mutationFn: async (password: string) => unwrap(await api.POST("/auth/password-reset/{token}", { params: { path: { token } }, body: { password } })),
  });
}

export function useInvitationLink(token: string) {
  return useQuery({
    queryKey: ["invitation-link", token],
    queryFn: async () => unwrap(await api.GET("/invitation-links/{token}", { params: { path: { token } } })),
    retry: false,
  });
}

export function useAcceptInvitation(token: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string; password: string }) =>
      unwrap(await api.POST("/invitation-links/{token}", { params: { path: { token } }, body })).account,
    onSuccess: (account) => client.setQueryData(meQuery.queryKey, account),
  });
}

export const useAccounts = () => useQuery({ queryKey: ["accounts"], queryFn: async () => unwrap(await api.GET("/accounts")).accounts });
export const useInvitations = () => useQuery({ queryKey: ["invitations"], queryFn: async () => unwrap(await api.GET("/invitations")).invitations });

function useAccountsMutation<T, R>(fn: (input: T) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => Promise.all([client.invalidateQueries({ queryKey: ["accounts"] }), client.invalidateQueries({ queryKey: ["invitations"] })]),
  });
}

export const useCreateAccount = () =>
  useAccountsMutation(async (body: { name: string; email: string; role: Role; password: string }) => unwrap(await api.POST("/accounts", { body })).account);

export const useInvite = () => useAccountsMutation(async (body: { email: string; role: Role }) => unwrap(await api.POST("/invitations", { body })));

export const useRevokeInvitation = () =>
  useAccountsMutation(async (invitationID: string) => unwrap(await api.DELETE("/invitations/{invitationID}", { params: { path: { invitationID } } })));

export const useSetAccountPassword = () =>
  useAccountsMutation(async ({ accountID, password }: { accountID: string; password: string }) =>
    unwrap(await api.POST("/accounts/{accountID}/password", { params: { path: { accountID } }, body: { password } })),
  );

export const useSetDisabled = () =>
  useAccountsMutation(async ({ accountID, disabled }: { accountID: string; disabled: boolean }) =>
    unwrap(await api.POST(disabled ? "/accounts/{accountID}/disable" : "/accounts/{accountID}/enable", { params: { path: { accountID } } })),
  );
