import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import type { components } from "./schema";

export type SearchHit = components["schemas"]["SearchHitResponse"];
export type SearchResults = components["schemas"]["SearchResponse"];

/** Whether a search has enough to go on: two letters or digits, as the API asks. */
export const searchable = (q: string) => (q.match(/[\p{L}\p{N}]/gu)?.length ?? 0) >= 2;

/** Everything a search finds on a day, grouped by kind. */
export const useSearch = (q: string, asOf: string) =>
  useQuery({
    queryKey: ["search", q, asOf],
    queryFn: async () => unwrap(await api.GET("/search", { params: { query: { q, asOf } } })),
    enabled: searchable(q),
  });

/** The best few hits for a search box being typed in; the last answer stays while the next is on its way. */
export const useQuickSearch = (q: string) =>
  useQuery({
    queryKey: ["search-quick", q],
    queryFn: async () => unwrap(await api.GET("/search/quick", { params: { query: { q } } })).hits,
    enabled: searchable(q),
    placeholderData: keepPreviousData,
  });
