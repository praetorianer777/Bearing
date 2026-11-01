import { type FormEvent, useId, useState } from "react";
import {
  useCreateWebhook,
  useDeleteWebhook,
  useRedeliverWebhook,
  useRotateWebhookSecret,
  useTestWebhook,
  useUpdateWebhook,
  useWebhookDeliveries,
  useWebhooks,
  WEBHOOK_TOPICS,
  type Webhook,
  type WebhookDelivery,
  type WebhookTopic,
} from "@/api/webhooks";
import { Button, Field, FormError, fieldError } from "@/components/ui";
import { locale, t, useLanguage } from "@/i18n";
import { SettingsNav } from "./SettingsNav";

// Adapted from Stator's webhooks page, which is Armature's: the secret is shown as a new token is, the log beneath
// its webhook.

const when = (iso: string) => new Intl.DateTimeFormat(locale(), { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso));

/** A topic in the reader's words; a ping and anything newer than this client in the API's. */
export function topicName(topic: string): string {
  if (topic === "*") return t.webhooks.everything;
  if (topic === "ping") return t.webhooks.ping;
  return t.webhooks.topicNames[topic] ?? topic;
}

/** What one attempt came to, in a sentence for its row. */
export function deliveryDetail(d: WebhookDelivery): string {
  if (d.state === "delivered" && d.status !== null) return t.webhooks.answered(d.status);
  if (d.state === "pending" && d.nextAttemptAt) return t.webhooks.nextTry(when(d.nextAttemptAt));
  return d.error;
}

/** The organisation's webhooks, for its administrators. */
export function WebhooksPage() {
  useLanguage();
  const id = useId();
  const hooks = useWebhooks();
  const update = useUpdateWebhook();
  const remove = useDeleteWebhook();
  const rotate = useRotateWebhookSecret();
  const test = useTestWebhook();
  const [fresh, setFresh] = useState<Webhook | null>(null);
  const [logOf, setLogOf] = useState<string | null>(null);
  const [notice, setNotice] = useState("");

  const toggle = (hook: Webhook) =>
    update.mutate(
      { id: hook.id, name: hook.name, url: hook.url, topics: hook.topics, enabled: !hook.enabled },
      { onSuccess: (saved) => setNotice(saved.enabled ? t.webhooks.turnedOn(saved.name) : t.webhooks.turnedOff(saved.name)) },
    );
  const ping = (hook: Webhook) =>
    test.mutate(hook.id, {
      onSuccess: (d) =>
        setNotice(d.state === "delivered" && d.status !== null ? t.webhooks.testDelivered(hook.name, d.status) : t.webhooks.testFailed(hook.name, d.error)),
    });
  const rotateSecret = (hook: Webhook) => {
    if (!window.confirm(t.webhooks.confirmRotate(hook.name))) return;
    rotate.mutate(hook.id, { onSuccess: setFresh });
  };
  const deleteHook = (hook: Webhook) => {
    if (!window.confirm(t.webhooks.confirmDelete(hook.name))) return;
    remove.mutate(hook.id, {
      onSuccess: () => {
        setNotice(t.webhooks.deleted(hook.name));
        if (fresh?.id === hook.id) setFresh(null);
      },
    });
  };

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-8">
      <SettingsNav />
      <div>
        <h1 className="text-2xl font-semibold">{t.webhooks.title}</h1>
        <p className="mt-1 text-base text-ink-muted">{t.webhooks.intro}</p>
      </div>
      {fresh?.secret && (
        <section aria-labelledby={`${id}-fresh`} className="flex flex-col gap-2 rounded-xl border border-accent bg-surface p-5">
          <h2 id={`${id}-fresh`} className="text-base font-semibold">
            {t.webhooks.freshTitle}
          </h2>
          <p className="text-sm text-ink-muted">{t.webhooks.freshBody(fresh.name)}</p>
          <Field label={t.webhooks.freshLabel(fresh.name)} value={fresh.secret} readOnly className="font-mono" onFocus={(e) => e.target.select()} />
          <Button variant="secondary" className="self-start" onClick={() => setFresh(null)}>
            {t.webhooks.done}
          </Button>
        </section>
      )}
      <CreateWebhook onCreated={setFresh} />
      <section aria-labelledby={`${id}-list`} className="flex flex-col gap-2">
        <h2 id={`${id}-list`} className="text-base font-semibold">
          {t.webhooks.list}
        </h2>
        <p role="status" className="text-sm text-ink-muted empty:hidden">
          {notice}
        </p>
        <FormError error={hooks.error ?? update.error ?? remove.error ?? rotate.error ?? test.error} />
        {hooks.data?.length === 0 && <p className="text-sm text-ink-muted">{t.webhooks.empty}</p>}
        {(hooks.data?.length ?? 0) > 0 && (
          <ul aria-label={t.webhooks.list} className="divide-y divide-border rounded-xl border border-border bg-surface px-5">
            {hooks.data?.map((hook) => (
              <li key={hook.id} className="flex flex-col gap-2 py-3" data-webhook={hook.name}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="font-medium">
                      {hook.name}{" "}
                      <span className={`text-sm font-normal ${hook.enabled ? "text-ink-muted" : "text-danger"}`}>
                        {hook.enabled ? t.webhooks.on : hook.disabledReason === "failing" ? t.webhooks.offFailing : t.webhooks.off}
                      </span>
                    </p>
                    <p className="truncate font-mono text-xs text-ink-muted" title={hook.url}>
                      {hook.url}
                    </p>
                    <p className="text-sm text-ink-muted">
                      {hook.topics.map(topicName).join(", ")} · {hook.owner ? t.webhooks.owner(hook.owner.name) : t.webhooks.noOwner}
                    </p>
                    {hook.disabledReason === "failing" && <p className="text-xs text-ink-muted">{t.webhooks.failingHint(hook.failures)}</p>}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {(
                      [
                        [logOf === hook.id ? t.webhooks.hideDeliveries : t.webhooks.deliveries, () => setLogOf(logOf === hook.id ? null : hook.id)],
                        [t.webhooks.test, () => ping(hook)],
                        [hook.enabled ? t.webhooks.turnOff : t.webhooks.turnOn, () => toggle(hook)],
                        [t.webhooks.rotate, () => rotateSecret(hook)],
                        [t.webhooks.delete, () => deleteHook(hook)],
                      ] as const
                    ).map(([label, act]) => (
                      <Button key={label} variant="secondary" onClick={act} aria-label={t.webhooks.actionOn(label, hook.name)}>
                        {label}
                      </Button>
                    ))}
                  </div>
                </div>
                {logOf === hook.id && <DeliveryLog hook={hook} />}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function CreateWebhook({ onCreated }: { onCreated: (hook: Webhook) => void }) {
  const id = useId();
  const create = useCreateWebhook();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [topics, setTopics] = useState<WebhookTopic[]>(["*"]);
  const choices: WebhookTopic[] = ["*", ...WEBHOOK_TOPICS];
  const submit = (event: FormEvent) => {
    event.preventDefault();
    create.mutate(
      { name: name.trim(), url: url.trim(), topics },
      {
        onSuccess: (hook) => {
          setName("");
          setUrl("");
          setTopics(["*"]);
          onCreated(hook);
        },
      },
    );
  };
  return (
    <section aria-labelledby={`${id}-new`} className="flex flex-col gap-3 rounded-xl border border-border bg-surface p-5">
      <h2 id={`${id}-new`} className="text-base font-semibold">
        {t.webhooks.newTitle}
      </h2>
      <form onSubmit={submit} noValidate className="flex flex-col gap-3">
        <Field label={t.webhooks.name} value={name} onChange={(e) => setName(e.target.value)} error={fieldError(create.error, "name")} />
        <Field
          label={t.webhooks.url}
          type="url"
          inputMode="url"
          value={url}
          placeholder="https://"
          className="font-mono"
          onChange={(e) => setUrl(e.target.value)}
          error={fieldError(create.error, "url")}
        />
        <p className="-mt-2 text-xs text-ink-subtle">{t.webhooks.urlHint}</p>
        <fieldset className="flex flex-col gap-1.5">
          <legend className="mb-1 text-sm font-medium text-ink">{t.webhooks.topics}</legend>
          {choices.map((topic) => (
            <label key={topic} className="flex items-center gap-2 text-sm text-ink">
              <input
                type="checkbox"
                checked={topics.includes(topic)}
                onChange={(e) => setTopics((now) => (e.target.checked ? [...now, topic] : now.filter((x) => x !== topic)))}
              />
              {topic === "*" ? t.webhooks.topicAny : topicName(topic)}
            </label>
          ))}
          {fieldError(create.error, "topics") && <p className="text-xs text-danger">{fieldError(create.error, "topics")}</p>}
        </fieldset>
        <p className="text-xs text-ink-subtle">{t.webhooks.ownerNote}</p>
        <Button type="submit" disabled={create.isPending} className="self-start">
          {t.webhooks.create}
        </Button>
        <FormError
          error={
            create.error && !fieldError(create.error, "name") && !fieldError(create.error, "url") && !fieldError(create.error, "topics") ? create.error : null
          }
        />
      </form>
    </section>
  );
}

function DeliveryLog({ hook }: { hook: Webhook }) {
  const deliveries = useWebhookDeliveries(hook.id);
  const redeliver = useRedeliverWebhook();
  const [said, setSaid] = useState("");
  return (
    <section aria-label={t.webhooks.logTitle(hook.name)} className="rounded-md bg-surface-sunken p-3">
      <p role="status" className="mb-2 text-sm text-ink-muted empty:hidden">
        {said}
      </p>
      <FormError error={deliveries.error ?? redeliver.error} />
      {deliveries.data?.length === 0 && <p className="text-sm text-ink-muted">{t.webhooks.logEmpty}</p>}
      <ul className="divide-y divide-border">
        {deliveries.data?.map((d) => {
          const at = when(d.attemptedAt ?? d.createdAt);
          return (
            <li key={d.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm" data-delivery-state={d.state}>
              <span className={`w-28 ${d.state === "failed" ? "text-danger" : "text-ink-muted"}`}>{t.webhooks.states[d.state]}</span>
              <span className="min-w-0 flex-1">
                <span className="block text-ink">
                  {topicName(d.topic)} <span className="text-ink-subtle">{d.manual ? t.webhooks.byHand(d.attempt) : t.webhooks.attempt(d.attempt)}</span>
                </span>
                <span className="block text-xs break-words text-ink-muted">{deliveryDetail(d)}</span>
              </span>
              <span className="text-xs text-ink-subtle">{at}</span>
              {d.state !== "pending" && (
                <Button
                  variant="secondary"
                  disabled={redeliver.isPending}
                  aria-label={t.webhooks.redeliverLabel(topicName(d.topic), at)}
                  onClick={() =>
                    redeliver.mutate(
                      { webhookId: hook.id, deliveryId: d.id },
                      { onSuccess: (again) => setSaid(t.webhooks.sentAgain(t.webhooks.states[again.state])) },
                    )
                  }
                >
                  {t.webhooks.redeliver}
                </Button>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
