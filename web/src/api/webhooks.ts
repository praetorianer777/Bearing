import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { WEBHOOK_DELIVERIES_PAGE_SIZE } from "@/config";
import { api, unwrap } from "./client";
import type { components } from "./schema";

type Wire = components["schemas"];

/** Where changes to the organisation are posted; its secret is there only in the answer that made or rotated it. */
export type Webhook = Wire["Webhook"];
export type WebhookInput = Wire["WebhookInput"];
/** One attempt at one event. */
export type WebhookDelivery = Wire["WebhookDelivery"];
export type DeliveryState = WebhookDelivery["state"];

/** What an endpoint may take, as webhooks@1 names it in Bearing; * is every one. */
export const WEBHOOK_TOPICS = [
  "position.holder_changed",
  "position.vacated",
  "reporting_line.changed",
  "unit.changed",
  "responsibility.changed",
  "holding.planned",
  "holding.started",
  "holding.ended",
  "person.left",
  "capacity.gap_changed",
] as const;
export type WebhookTopic = (typeof WEBHOOK_TOPICS)[number] | "*";

const webhooksKey = ["webhooks"] as const;

export const useWebhooks = () => useQuery({ queryKey: webhooksKey, queryFn: async () => unwrap(await api.GET("/webhooks")).webhooks });

/** A webhook's log, newest first; nothing is asked while id is null. */
export const useWebhookDeliveries = (id: string | null) =>
  useQuery({
    queryKey: [...webhooksKey, "deliveries", id],
    queryFn: async () =>
      unwrap(
        await api.GET("/webhooks/{webhookID}/deliveries", {
          params: { path: { webhookID: id ?? "" }, query: { limit: String(WEBHOOK_DELIVERIES_PAGE_SIZE) } },
        }),
      ).deliveries,
    enabled: id !== null,
  });

function useWebhookMutation<TInput, TOut>(fn: (input: TInput) => Promise<TOut>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: fn, onSettled: () => client.invalidateQueries({ queryKey: webhooksKey }) });
}

export const useCreateWebhook = () => useWebhookMutation(async (body: WebhookInput) => unwrap(await api.POST("/webhooks", { body })).webhook);

export const useUpdateWebhook = () =>
  useWebhookMutation(
    async ({ id, ...body }: WebhookInput & { id: string }) =>
      unwrap(await api.PATCH("/webhooks/{webhookID}", { params: { path: { webhookID: id } }, body })).webhook,
  );

export const useDeleteWebhook = () =>
  useWebhookMutation(async (id: string) => unwrap(await api.DELETE("/webhooks/{webhookID}", { params: { path: { webhookID: id } } })));

export const useRotateWebhookSecret = () =>
  useWebhookMutation(async (id: string) => unwrap(await api.POST("/webhooks/{webhookID}/rotate-secret", { params: { path: { webhookID: id } } })).webhook);

export const useTestWebhook = () =>
  useWebhookMutation(async (id: string) => unwrap(await api.POST("/webhooks/{webhookID}/test", { params: { path: { webhookID: id } } })).delivery);

export const useRedeliverWebhook = () =>
  useWebhookMutation(
    async ({ webhookId, deliveryId }: { webhookId: string; deliveryId: string }) =>
      unwrap(await api.POST("/webhooks/{webhookID}/deliveries/{deliveryID}/redeliver", { params: { path: { webhookID: webhookId, deliveryID: deliveryId } } }))
        .delivery,
  );
