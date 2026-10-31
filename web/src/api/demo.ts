import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";

const demoKey = ["demo"];

export const useDemo = () => useQuery({ queryKey: demoKey, queryFn: async () => unwrap(await api.GET("/demo")) });

export const useLoadDemo = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/demo")),
    onSuccess: () => client.invalidateQueries(),
  });
};
