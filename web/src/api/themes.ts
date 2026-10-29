import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { API_BASE } from "@/config";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type Theme = components["schemas"]["ThemeResponse"];
export type ThemePackage = components["schemas"]["ThemePackage"];

const themesKey = ["themes"];

export const useThemes = (enabled = true) =>
  useQuery({ queryKey: themesKey, queryFn: async () => unwrap(await api.GET("/themes")).themes, enabled, staleTime: 5 * 60_000 });

/** Where a theme downloads as one armature-theme/1 file. */
export const themeExportURL = (themeID: string) => `${API_BASE}/themes/${themeID}/export`;

function useThemesMutation<T, R>(fn: (input: T) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: fn, onSuccess: () => client.invalidateQueries({ queryKey: themesKey }) });
}

export const useImportTheme = () => useThemesMutation(async (body: ThemePackage) => unwrap(await api.POST("/themes/import", { body })).theme);

export const useDeleteTheme = () =>
  useThemesMutation(async (themeID: string) => unwrap(await api.DELETE("/themes/{themeID}", { params: { path: { themeID } } })));

export const useSetDefaultTheme = () => useThemesMutation(async (themeId: string | null) => unwrap(await api.PUT("/themes/default", { body: { themeId } })));
