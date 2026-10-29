import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, unwrap } from "./client";
import type { components } from "./schema";

export type Account = components["schemas"]["AccountResponse"];
export type Role = Account["role"];
export type Invitation = components["schemas"]["InvitationResponse"];
export type InstallationInfo = components["schemas"]["InstallationResponse"];

/** Every role, from the most it may do to the least. */
export const ROLES: Role[] = ["administrator", "editor", "member", "reader"];

/** Whether role may do everything need may. */
export const atLeast = (role: Role, need: Role) => ROLES.indexOf(role) <= ROLES.indexOf(need);

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

export function useUpdateMe() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string }) => unwrap(await api.PATCH("/auth/me", { body })).account,
    onSuccess: (account) => client.setQueryData(meQuery.queryKey, account),
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

export const useSetAccountRole = () =>
  useAccountsMutation(async ({ accountID, role }: { accountID: string; role: Role }) =>
    unwrap(await api.PUT("/accounts/{accountID}/role", { params: { path: { accountID } }, body: { role } })),
  );

export const useSetDisabled = () =>
  useAccountsMutation(async ({ accountID, disabled }: { accountID: string; disabled: boolean }) =>
    unwrap(await api.POST(disabled ? "/accounts/{accountID}/disable" : "/accounts/{accountID}/enable", { params: { path: { accountID } } })),
  );

export type JoinRequest = components["schemas"]["JoinRequestResponse"];
export type GroupGrant = components["schemas"]["GroupGrant"];

/** Where the browser goes to sign in through the identity provider, coming back to next. */
export const ssoStartURL = (next = "/") => `/api/v1/auth/oidc/start?next=${encodeURIComponent(next)}`;

export const useJoinRequests = () => useQuery({ queryKey: ["join-requests"], queryFn: async () => unwrap(await api.GET("/join-requests")).joinRequests });

export const useGroupGrants = () => useQuery({ queryKey: ["group-grants"], queryFn: async () => unwrap(await api.GET("/group-grants")).grants });

export function useAdmit() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ requestID, role }: { requestID: string; role: Role }) =>
      unwrap(await api.POST("/join-requests/{requestID}/admit", { params: { path: { requestID } }, body: { role } })).account,
    onSuccess: () => Promise.all([client.invalidateQueries({ queryKey: ["join-requests"] }), client.invalidateQueries({ queryKey: ["accounts"] })]),
  });
}

export function useDecline() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (requestID: string) => unwrap(await api.DELETE("/join-requests/{requestID}", { params: { path: { requestID } } })),
    onSuccess: () => client.invalidateQueries({ queryKey: ["join-requests"] }),
  });
}

export function useSetGroupGrants() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (grants: GroupGrant[]) => unwrap(await api.PUT("/group-grants", { body: { grants } })).grants,
    onSuccess: (grants) => client.setQueryData(["group-grants"], grants),
  });
}
