import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";
import { goTo } from "./ConsentPage";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

const query = "response_type=code&client_id=c1&redirect_uri=http%3A%2F%2F127.0.0.1%3A9%2Fcb&state=s&code_challenge=x&code_challenge_method=S256";

const consent = (extra: object = {}) => ({
  consent: {
    client: { id: "c1", name: "Assistant", clientUri: "https://assistant.example" },
    redirectHost: "127.0.0.1:9",
    person: "Ada Admin",
    scopes: ["read", "write"],
    pending: false,
    allowsApp: false,
    mayAllow: true,
    ...extra,
  },
  redirect: "",
});

describe("the consent page", () => {
  it("passes the app's request on as it came, and connects it for reading only when the person says so", async () => {
    const asked: string[] = [];
    const decided: unknown[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /oauth/consent": (_: unknown, url: string) => {
        asked.push(new URL(url).search);
        return { body: consent() };
      },
      "POST /oauth/consent": (body) => {
        decided.push(body);
        return { body: { redirect: "http://127.0.0.1:9/cb?code=abc&state=s" } };
      },
    });
    const went = vi.spyOn(goTo, "assign").mockImplementation(() => {});
    const user = userEvent.setup();
    renderApp(`/connect?${query}`);
    expect(await screen.findByRole("heading", { name: "Connect Assistant" })).toBeInTheDocument();
    expect(asked[0]).toBe(`?${query}`);
    const may = screen.getByRole("region", { name: "It may" });
    expect(within(may).getByText("change them, where your role allows")).toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: "Let it change things too" }));
    expect(within(may).queryByText("change them, where your role allows")).not.toBeInTheDocument();
    expect(screen.getByText("Your browser then goes back to 127.0.0.1:9.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(went).toHaveBeenCalledWith("http://127.0.0.1:9/cb?code=abc&state=s"));
    expect(decided).toEqual([{ query, allow: true, scope: "read" }]);
  });

  it("tells an administrator that connecting allows the app, and somebody else that they cannot", async () => {
    fakeAPI({ ...signedInAs(), "GET /oauth/consent": () => ({ body: consent({ pending: true, allowsApp: true }) }) });
    renderApp(`/connect?${query}`);
    expect(await screen.findByText(/Connecting it allows it for everybody here/)).toBeInTheDocument();
    cleanup();

    fakeAPI({
      ...signedInAs({ ...ADMIN, role: "member" }),
      "GET /oauth/consent": () => ({ body: consent({ pending: true, mayAllow: false, refusal: "A Bearing administrator must allow Assistant first." }) }),
    });
    renderApp(`/connect?${query}`);
    expect(await screen.findByRole("alert")).toHaveTextContent("must allow Assistant first");
    expect(screen.queryByRole("button", { name: "Connect" })).not.toBeInTheDocument();
  });

  it("goes straight back to the app with what was wrong, and names an app nobody knows", async () => {
    fakeAPI({ ...signedInAs(), "GET /oauth/consent": () => ({ body: { consent: null, redirect: "http://127.0.0.1:9/cb?error=invalid_request" } }) });
    const went = vi.spyOn(goTo, "assign").mockImplementation(() => {});
    renderApp(`/connect?${query}`);
    await waitFor(() => expect(went).toHaveBeenCalledWith("http://127.0.0.1:9/cb?error=invalid_request"));
    cleanup();

    fakeAPI({
      ...signedInAs(),
      "GET /oauth/consent": () => ({ status: 400, body: { error: { code: "unknown_app", message: "Bearing does not know this app." } } }),
    });
    renderApp(`/connect?${query}`);
    expect(await screen.findByRole("heading", { name: "This app cannot connect" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Bearing does not know this app.");
  });
});

describe("connected apps", () => {
  const grant = { id: "g1", clientId: "c1", app: "Assistant", scopes: ["read"], connectedAt: "2026-11-01T09:00:00Z", until: "2027-01-30T09:00:00Z" };
  const app = (status: string) => ({
    id: "c1",
    name: "Assistant",
    kind: "registered",
    keycloakClientId: "",
    status,
    scopes: ["read", "write"],
    clientUri: "",
    redirectUris: ["http://127.0.0.1:9/cb"],
    confidential: false,
    people: 1,
    createdAt: "2026-11-01T09:00:00Z",
  });

  it("lists what the person connected and disconnects it, and administrators remove an app for everybody", async () => {
    let grants = [grant];
    let apps = [app("allowed"), { ...app("pending"), id: "c2", name: "Stranger", people: 0 }];
    fakeAPI({
      ...signedInAs(),
      "GET /connected-apps": () => ({ body: { grants } }),
      "DELETE /connected-apps/g1": () => {
        grants = [];
        return { status: 204 };
      },
      "GET /oauth/clients": () => ({ body: { clients: apps } }),
      "DELETE /oauth/clients/c2": () => {
        apps = apps.filter((a) => a.id !== "c2");
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/connected-apps");
    const yours = await screen.findByRole("list", { name: "Yours" });
    expect(within(yours).getByText("Assistant")).toBeInTheDocument();
    expect(within(yours).getByText(/^Reads · connected on/)).toBeInTheDocument();
    const every = await screen.findByRole("list", { name: "Every app" });
    expect(within(every).getByText("waiting for an administrator")).toBeInTheDocument();
    expect(within(every).getByText(/1 person connected/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Disconnect Assistant" }));
    expect(await screen.findByText("You have not connected any app.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Remove Stranger" }));
    await waitFor(() => expect(within(screen.getByRole("list", { name: "Every app" })).queryByText("Stranger")).not.toBeInTheDocument());
  });

  it("registers a partner app and shows its secret once", async () => {
    const registered: unknown[] = [];
    let apps: object[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /connected-apps": () => ({ body: { grants: [] } }),
      "GET /oauth/clients": () => ({ body: { clients: apps } }),
      "POST /oauth/clients": (body) => {
        registered.push(body);
        const made = { ...app("allowed"), id: "p1", name: "Rotor", kind: "partner", keycloakClientId: "rotor", redirectUris: [], people: 0 };
        apps = [made];
        return { status: 201, body: { client: made, secret: "bearing_ocs_shown_once" } };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/connected-apps");
    const form = await screen.findByRole("region", { name: "Register a partner app" });
    await user.type(within(form).getByLabelText("Name"), "Rotor");
    await user.type(within(form).getByLabelText("Realm client"), "rotor");
    await user.click(within(form).getByRole("checkbox", { name: /It may change/ }));
    await user.click(within(form).getByRole("button", { name: "Register" }));
    expect(await within(form).findByRole("status")).toHaveTextContent("bearing_ocs_shown_once");
    expect(registered).toEqual([{ name: "Rotor", keycloakClientId: "rotor", write: true }]);
    expect(await screen.findByText(/partner, through the realm client rotor/)).toBeInTheDocument();
  });

  it("shows somebody who is no administrator their own apps only", async () => {
    fakeAPI({ ...signedInAs({ ...ADMIN, role: "member" }), "GET /connected-apps": () => ({ body: { grants: [] } }) });
    renderApp("/settings/connected-apps");
    expect(await screen.findByText("You have not connected any app.")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Every app" })).not.toBeInTheDocument();
  });
});
