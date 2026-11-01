import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { API_BASE } from "@/config";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type OrgImport = components["schemas"]["ImportResponse"];
export type ImportFiles = components["schemas"]["ImportRequest"];

export const EXPORT_KINDS = ["units", "positions", "people", "holdings"] as const;
export type ExportKind = (typeof EXPORT_KINDS)[number];

/** Where one file of the organisation downloads as CSV, as it stands on a day. */
export const exportURL = (kind: ExportKind, asOf: string) => `${API_BASE}/export/${kind}?asOf=${asOf}`;

/** Previews an import: nothing changes until it is run. */
export const usePreviewImport = () => useMutation({ mutationFn: async (body: ImportFiles) => unwrap(await api.POST("/imports", { body })).import });

/** An import, read again every second while the worker has it, and the organisation read again once it is done. */
export const useImport = (importID: string | undefined) => {
  const client = useQueryClient();
  return useQuery({
    queryKey: ["import", importID],
    enabled: !!importID,
    queryFn: async () => {
      const found = unwrap(await api.GET("/imports/{importID}", { params: { path: { importID: importID ?? "" } } })).import;
      if (found.status === "done") await client.invalidateQueries({ predicate: (q) => q.queryKey[0] !== "import" });
      return found;
    },
    refetchInterval: (query) => (["queued", "running"].includes(query.state.data?.status ?? "") ? 1000 : false),
  });
};

/** Hands a previewed import to the worker. */
export const useRunImport = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (importID: string) => unwrap(await api.POST("/imports/{importID}/run", { params: { path: { importID } } })).import,
    onSuccess: (found) => client.setQueryData(["import", found.id], found),
  });
};
