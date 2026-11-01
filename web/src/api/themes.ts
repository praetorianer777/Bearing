import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { API_BASE } from "@/config";
import type { ThemeSpec } from "@/lib/theme";
import { api, unwrap } from "./client";
import type { components } from "./schema";

type Wire = components["schemas"];

/** A theme the reader may see, its spec in the compiler's shape. */
export type Theme = Omit<Wire["ThemeResponse"], "spec"> & { spec: ThemeSpec };
export type ThemeAsset = Wire["ThemeAssetResponse"];
/** A theme Bearing ships, to start one's own from. */
export type ThemeExample = Omit<Wire["ThemeExampleResponse"], "spec"> & { spec: ThemeSpec };
export type ThemePackage = Wire["ThemePackage"];
/** Where the theme the reader sees comes from: their choice, the installation's default, or the built-in look. */
export type ThemeSource = "chosen" | "everyone" | "";

export interface ThemeInput {
  name?: string;
  shared?: boolean;
  spec?: ThemeSpec;
}

// The server leaves a backdrop out rather than sending null, and takes either; the compiler's null is dropped on
// the way out so the wire type holds.
function toWire(spec: ThemeSpec): Wire["ThemeSpec"] {
  const { backdrop, ...rest } = spec;
  return backdrop ? { ...rest, backdrop } : rest;
}

const fromWire = <T extends { spec: Wire["ThemeSpec"] }>(value: T) => value as Omit<T, "spec"> & { spec: ThemeSpec };

export const themesKey = ["themes"] as const;
const activeKey = [...themesKey, "active"] as const;

export const useThemes = (enabled = true) =>
  useQuery({ queryKey: themesKey, queryFn: async () => unwrap(await api.GET("/themes")).themes.map(fromWire), enabled });

export const useTheme = (themeID: string | undefined) =>
  useQuery({
    queryKey: [...themesKey, "one", themeID],
    queryFn: async () => fromWire(unwrap(await api.GET("/themes/{themeID}", { params: { path: { themeID: themeID ?? "" } } })).theme),
    enabled: Boolean(themeID),
  });

export const useThemeExamples = () =>
  useQuery({
    queryKey: [...themesKey, "examples"],
    queryFn: async () => unwrap(await api.GET("/themes/examples")).examples.map(fromWire),
    staleTime: Number.POSITIVE_INFINITY,
  });

/** The theme the reader sees on every device; null is the built-in look. */
export const useActiveTheme = (enabled = true) =>
  useQuery({
    queryKey: activeKey,
    queryFn: async () => {
      const got = unwrap(await api.GET("/themes/active"));
      return { theme: got.theme ? fromWire(got.theme) : null, source: got.source as ThemeSource };
    },
    enabled,
  });

/** Where a theme downloads as one armature-theme/1 file. */
export const themeExportURL = (themeID: string) => `${API_BASE}/themes/${themeID}/export`;

function useThemesMutation<T, R>(fn: (input: T) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: fn, onSuccess: () => client.invalidateQueries({ queryKey: themesKey }) });
}

export const useCreateTheme = () =>
  useThemesMutation(async ({ spec, ...input }: ThemeInput) =>
    fromWire(unwrap(await api.POST("/themes", { body: { ...input, spec: spec && toWire(spec) } })).theme),
  );

export const useUpdateTheme = () =>
  useThemesMutation(async ({ id, spec, ...input }: ThemeInput & { id: string }) =>
    fromWire(unwrap(await api.PATCH("/themes/{themeID}", { params: { path: { themeID: id } }, body: { ...input, spec: spec && toWire(spec) } })).theme),
  );

export const useDeleteTheme = () =>
  useThemesMutation(async (themeID: string) => unwrap(await api.DELETE("/themes/{themeID}", { params: { path: { themeID } } })));

/** A choice: a theme, null for whatever the installation shows, or the built-in look over it. */
export type ThemeChoice = string | null | { builtIn: true };

export const useChooseTheme = () =>
  useThemesMutation(async (choice: ThemeChoice) => {
    const body = choice !== null && typeof choice === "object" ? { themeId: null, builtIn: true } : { themeId: choice };
    return unwrap(await api.PUT("/themes/active", { body })).theme;
  });

/** Names the shared theme everybody sees until they choose, or null for the built-in look. */
export const useSetDefaultTheme = () => useThemesMutation(async (themeId: string | null) => unwrap(await api.PUT("/themes/default", { body: { themeId } })));

export const useImportTheme = () => useThemesMutation(async (body: ThemePackage) => fromWire(unwrap(await api.POST("/themes/import", { body })).theme));

// FileReader rather than file.arrayBuffer(), which the test DOM does not have; the data URL's tail is the base64
// that JSON carries bytes as.
function readBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).replace(/^data:[^,]*,/, ""));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

export const useAddThemeAsset = () =>
  useThemesMutation(
    async ({ id, file }: { id: string; file: File }) =>
      unwrap(await api.POST("/themes/{themeID}/assets", { params: { path: { themeID: id } }, body: { name: file.name, data: await readBase64(file) } })).asset,
  );

export const useRemoveThemeAsset = () =>
  useThemesMutation(async ({ id, assetId }: { id: string; assetId: string }) =>
    unwrap(await api.DELETE("/themes/{themeID}/assets/{assetID}", { params: { path: { themeID: id, assetID: assetId } } })),
  );
