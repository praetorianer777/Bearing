import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";
import { deliveryDetail, topicName } from "./Webhooks";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const hook = {
  id: "w1",
  name: "Rotor",
  url: "https://rotor.example/hook",
  topics: ["position.vacated"],
  enabled: true,
  version: 1,
  disabledReason: null,
  failures: 0,
  owner: { id: "a1", name: "Ada Admin" },
  createdAt: "2026-11-01T09:00:00Z",
  updatedAt: "2026-11-01T09:00:00Z",
};

const delivery = (fields: object) => ({
  id: "d1",
  webhookId: "w1",
  eventId: "e1",
  topic: "position.vacated",
  attempt: 1,
  manual: false,
  state: "failed",
  status: 500,
  error: "The receiver answered 500. Check its log, then send it again.",
  nextAttemptAt: null,
  attemptedAt: "2026-11-01T09:05:00Z",
  deliveredAt: null,
  createdAt: "2026-11-01T09:05:00Z",
  ...fields,
});

describe("webhooks", () => {
  it("adds a webhook, shows its secret once, tests it, turns it off and sends a failed post again", async () => {
    const made: unknown[] = [];
    const changed: unknown[] = [];
    let hooks: object[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /webhooks": () => ({ body: { webhooks: hooks } }),
      "POST /webhooks": (body) => {
        made.push(body);
        hooks = [hook];
        return { status: 201, body: { webhook: { ...hook, secret: "bearing_whs_secret" } } };
      },
      "POST /webhooks/w1/test": () => ({ body: { delivery: delivery({ topic: "ping", manual: true, state: "delivered", status: 204, error: "" }) } }),
      "PATCH /webhooks/w1": (body) => {
        changed.push(body);
        hooks = [{ ...hook, enabled: false }];
        return { body: { webhook: hooks[0] } };
      },
      "GET /webhooks/w1/deliveries": () => ({ body: { deliveries: [delivery({})] } }),
      "POST /webhooks/w1/deliveries/d1/redeliver": () => ({ body: { delivery: delivery({ id: "d2", manual: true, state: "delivered", status: 200 }) } }),
    });
    const user = userEvent.setup();
    renderApp("/settings/webhooks");
    expect(await screen.findByText("No webhook yet.")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Name"), "Rotor");
    await user.type(screen.getByLabelText("Address"), "https://rotor.example/hook");
    await user.click(screen.getByLabelText("Every event, also those added later"));
    await user.click(screen.getByLabelText("A position is left empty"));
    await user.click(screen.getByRole("button", { name: "Add the webhook" }));
    expect(await screen.findByLabelText("Secret of Rotor")).toHaveValue("bearing_whs_secret");
    expect(made).toEqual([{ name: "Rotor", url: "https://rotor.example/hook", topics: ["position.vacated"] }]);

    const row = within(await screen.findByRole("list", { name: "Webhooks" }));
    expect(row.getByText(/A position is left empty · saved by Ada Admin/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Test: Rotor" }));
    expect(await screen.findByText("Rotor answered the test with 204.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Turn off: Rotor" }));
    expect(await screen.findByText("Rotor is off.")).toBeInTheDocument();
    expect(changed).toEqual([{ name: "Rotor", url: "https://rotor.example/hook", topics: ["position.vacated"], enabled: false }]);

    await user.click(screen.getByRole("button", { name: "Log: Rotor" }));
    const log = within(await screen.findByRole("region", { name: "Log of Rotor" }));
    expect(await log.findByText("The receiver answered 500. Check its log, then send it again.")).toBeInTheDocument();
    await user.click(log.getByRole("button", { name: /^Send again: A position is left empty/ }));
    expect(await log.findByText("Sent again: delivered.")).toBeInTheDocument();
  });

  it("rotates the secret and deletes a webhook once asked", async () => {
    let hooks: object[] = [{ ...hook, enabled: false, disabledReason: "failing", failures: 6, owner: null }];
    fakeAPI({
      ...signedInAs(),
      "GET /webhooks": () => ({ body: { webhooks: hooks } }),
      "POST /webhooks/w1/rotate-secret": () => ({ body: { webhook: { ...hook, secret: "bearing_whs_new" } } }),
      "POST /webhooks/w1/test": () => ({
        body: { delivery: delivery({ topic: "ping", manual: true, status: null, error: "The receiver could not be reached." }) },
      }),
      "DELETE /webhooks/w1": () => {
        hooks = [];
        return { status: 204 };
      },
    });
    const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValue(true);
    const user = userEvent.setup();
    renderApp("/settings/webhooks");
    expect(await screen.findByText("Off: failing")).toBeInTheDocument();
    expect(screen.getByText(/after 6 failed attempts/)).toBeInTheDocument();
    expect(screen.getByText(/no owner/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Test: Rotor" }));
    expect(await screen.findByText("The test of Rotor failed. The receiver could not be reached.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "New secret: Rotor" }));
    expect(screen.queryByLabelText("Secret of Rotor")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "New secret: Rotor" }));
    expect(await screen.findByLabelText("Secret of Rotor")).toHaveValue("bearing_whs_new");
    await user.click(screen.getByRole("button", { name: "Delete: Rotor" }));
    expect(await screen.findByText("Deleted Rotor.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Secret of Rotor")).not.toBeInTheDocument();
    expect(confirm).toHaveBeenCalledWith("Give Rotor a new secret? The old one stops working at once.");
  });

  it("says why a webhook cannot be added", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /webhooks": () => ({ body: { webhooks: [] } }),
      "POST /webhooks": () => ({
        status: 422,
        body: {
          error: { code: "invalid", message: "Check the fields.", fields: { url: "Bearing may not post to an address inside the server's own network." } },
        },
      }),
    });
    const user = userEvent.setup();
    renderApp("/settings/webhooks");
    await user.click(await screen.findByRole("button", { name: "Add the webhook" }));
    expect(await screen.findByText("Bearing may not post to an address inside the server's own network.")).toBeInTheDocument();
  });

  it("names topics and attempts in the reader's words", () => {
    expect(topicName("*")).toBe("Every event");
    expect(topicName("ping")).toBe("Test");
    expect(topicName("page.published")).toBe("page.published");
    expect(deliveryDetail(delivery({ state: "delivered", status: 200 }) as never)).toBe("The receiver answered 200.");
    expect(deliveryDetail(delivery({ state: "pending", nextAttemptAt: "2026-11-01T10:00:00Z" }) as never)).toMatch(/^Tried again at /);
  });
});
