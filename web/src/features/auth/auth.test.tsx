import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ADMIN, fakeAPI, nobodySignedIn, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("coming in", () => {
  it("sends somebody signed out to sign in, and back where they were", async () => {
    let signedIn = false;
    fakeAPI({
      ...nobodySignedIn,
      "GET /auth/me": () => (signedIn ? { body: { account: ADMIN } } : { status: 401, body: { error: { code: "unauthorized", message: "x" } } }),
      "POST /auth/sign-in": (body) => {
        const { email, password } = body as { email: string; password: string };
        if (password !== "the right password")
          return { status: 401, body: { error: { code: "bad_credentials", message: "That email and password do not match an account." } } };
        signedIn = true;
        return { body: { account: { ...ADMIN, email } } };
      },
    });
    const user = userEvent.setup();
    const { router } = renderApp("/people");
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ next: "/people" });

    await user.type(screen.getByLabelText("Email"), ADMIN.email);
    await user.type(screen.getByLabelText("Password"), "wrong");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("do not match");

    await user.clear(screen.getByLabelText("Password"));
    await user.type(screen.getByLabelText("Password"), "the right password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("heading", { level: 1, name: "People" })).toBeInTheDocument();
  });

  it("sends everybody to setup until the installation is set up, and names the problems", async () => {
    fakeAPI({
      ...nobodySignedIn,
      "GET /installation": { body: { name: "", setUp: false, mail: false } },
      "POST /installation/setup": {
        status: 422,
        body: { error: { code: "validation_failed", message: "Some fields need attention.", fields: { password: "Use at least 12 characters." } } },
      },
    });
    const user = userEvent.setup();
    renderApp("/");
    expect(await screen.findByRole("heading", { name: "Set up Bearing" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Organisation"), "Acme");
    await user.type(screen.getByLabelText("Your name"), "Ada");
    await user.type(screen.getByLabelText("Email"), "ada@acme.test");
    await user.type(screen.getByLabelText("Password"), "short");
    await user.click(screen.getByRole("button", { name: "Set up" }));
    expect(await screen.findByText("Use at least 12 characters.")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toHaveAttribute("aria-invalid", "true");
  });

  it("offers a reset only when the installation sends mail, and says the same for every address", async () => {
    fakeAPI({ ...nobodySignedIn, "POST /auth/password-reset": { status: 202 } });
    const user = userEvent.setup();
    renderApp("/sign-in");
    await user.click(await screen.findByRole("link", { name: "Forgot your password?" }));
    await user.type(await screen.findByLabelText("Email"), "anybody@acme.test");
    await user.click(screen.getByRole("button", { name: "Send the link" }));
    expect(await screen.findByRole("status")).toHaveTextContent("If an account has this address");
  });

  it("hides the reset where no mail goes out", async () => {
    fakeAPI({ ...nobodySignedIn, "GET /installation": { body: { name: "Acme", setUp: true, mail: false } } });
    renderApp("/sign-in");
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Forgot your password?" })).not.toBeInTheDocument();
  });

  it("accepts an invitation with a name and a password", async () => {
    let accepted = false;
    fakeAPI({
      ...nobodySignedIn,
      "GET /auth/me": () =>
        accepted ? { body: { account: { ...ADMIN, role: "editor" } } } : { status: 401, body: { error: { code: "unauthorized", message: "x" } } },
      "GET /invitation-links/tok": { body: { email: "ines@acme.test", role: "editor" } },
      "POST /invitation-links/tok": () => {
        accepted = true;
        return { status: 201, body: { account: { ...ADMIN, email: "ines@acme.test", name: "Ines", role: "editor" } } };
      },
    });
    const user = userEvent.setup();
    renderApp("/invitations/tok");
    expect(await screen.findByText(/invited as ines@acme.test, to work as Editor/)).toBeInTheDocument();
    await user.type(screen.getByLabelText("Name"), "Ines");
    await user.type(screen.getByLabelText("Password"), "ines long password");
    await user.click(screen.getByRole("button", { name: "Join" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Overview" })).toBeInTheDocument();
  });

  it("says when an invitation is no good", async () => {
    fakeAPI({ ...nobodySignedIn, "GET /invitation-links/old": { status: 410, body: { error: { code: "link_invalid", message: "gone" } } } });
    renderApp("/invitations/old");
    expect(await screen.findByRole("alert")).toHaveTextContent("unknown, used or expired");
  });

  it("chooses a new password from a mailed link", async () => {
    fakeAPI({ ...nobodySignedIn, "POST /auth/password-reset/tok": { status: 204 } });
    const user = userEvent.setup();
    renderApp("/reset-password/tok");
    await user.type(await screen.findByLabelText("New password"), "a fresh long password");
    await user.click(screen.getByRole("button", { name: "Save the password" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Your password is changed");
  });

  it("signs out from the account menu", async () => {
    let signedIn = true;
    fakeAPI({
      ...signedInAs(),
      "GET /auth/me": () => (signedIn ? { body: { account: ADMIN } } : { status: 401, body: { error: { code: "unauthorized", message: "x" } } }),
      "POST /auth/sign-out": () => {
        signedIn = false;
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Account and settings" }));
    expect(screen.getByText(ADMIN.email)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Sign out" }));
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
  });
});

describe("the accounts page", () => {
  const BERT = { ...ADMIN, id: "0190a000-0000-7000-8000-000000000002", email: "bert@acme.test", name: "Bert", role: "member", ownRole: "member" };

  it("is for administrators only", async () => {
    fakeAPI(signedInAs(BERT));
    renderApp("/settings/accounts");
    expect(await screen.findByRole("heading", { level: 1, name: "Overview" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Settings" })).not.toBeInTheDocument();
  });

  it("lists the accounts, invites with a link to hand on, and switches an account off", async () => {
    const disabled: string[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /accounts": () => ({ body: { accounts: [ADMIN, { ...BERT, disabled: disabled.includes(BERT.id) }] } }),
      "GET /invitations": { body: { invitations: [] } },
      "POST /invitations": (body) => ({
        status: 201,
        body: {
          invitation: {
            id: "i1",
            email: (body as { email: string }).email,
            role: "reader",
            invitedBy: ADMIN.id,
            createdAt: ADMIN.createdAt,
            expiresAt: ADMIN.createdAt,
          },
          link: "http://localhost/invitations/abc",
        },
      }),
      [`POST /accounts/${BERT.id}/disable`]: () => {
        disabled.push(BERT.id);
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/accounts");
    const list = await screen.findByRole("list", { name: "Accounts" });
    expect(await within(list).findByText("Bert")).toBeInTheDocument();
    expect(within(list).queryByRole("button", { name: "Switch off: Ada Admin" })).not.toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Switch off: Bert" }));
    expect(await within(list).findByText("Switched off")).toBeInTheDocument();

    const invite = screen.getByRole("region", { name: "Invite somebody" });
    await user.type(within(invite).getByLabelText("Email"), "rex@acme.test");
    await user.selectOptions(within(invite).getByLabelText("Role"), "reader");
    await user.click(within(invite).getByRole("button", { name: "Invite" }));
    expect(await within(invite).findByDisplayValue("http://localhost/invitations/abc")).toBeInTheDocument();
  });

  it("gives an account a new password, makes an account and takes back an invitation", async () => {
    const revoked: string[] = [];
    let created = false;
    fakeAPI({
      ...signedInAs(),
      "GET /accounts": () => ({ body: { accounts: created ? [ADMIN, BERT, { ...BERT, id: "x", name: "Max", email: "max@acme.test" }] : [ADMIN, BERT] } }),
      "GET /invitations": () => ({
        body: {
          invitations: revoked.length
            ? []
            : [{ id: "i9", email: "old@acme.test", role: "member", invitedBy: null, createdAt: ADMIN.createdAt, expiresAt: "2026-11-05T09:00:00Z" }],
        },
      }),
      "DELETE /invitations/i9": () => {
        revoked.push("i9");
        return { status: 204 };
      },
      [`POST /accounts/${BERT.id}/password`]: (body) =>
        (body as { password: string }).password.length < 12
          ? { status: 422, body: { error: { code: "validation_failed", message: "x", fields: { password: "Use at least 12 characters." } } } }
          : { status: 204 },
      "POST /accounts": (body) => {
        created = true;
        return { status: 201, body: { account: { ...BERT, id: "x", name: "Max", email: (body as { email: string }).email } } };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/accounts");
    const list = await screen.findByRole("list", { name: "Accounts" });

    await user.click(await within(list).findByRole("button", { name: "New password: Bert" }));
    await user.type(within(list).getByLabelText("New password"), "short");
    await user.click(within(list).getByRole("button", { name: "Save" }));
    expect(await within(list).findByText("Use at least 12 characters.")).toBeInTheDocument();
    await user.clear(within(list).getByLabelText("New password"));
    await user.type(within(list).getByLabelText("New password"), "bert long password");
    await user.click(within(list).getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/Bert has a new password/)).toBeInTheDocument();

    expect(await screen.findByText("old@acme.test")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Take back: old@acme.test" }));
    expect(await screen.findByText("None.")).toBeInTheDocument();

    const create = screen.getByRole("region", { name: "Make an account with a password" });
    await user.type(within(create).getByLabelText("Name"), "Max");
    await user.type(within(create).getByLabelText("Email"), "max@acme.test");
    await user.type(within(create).getByLabelText("Password"), "max long password");
    await user.click(within(create).getByRole("button", { name: "Make the account" }));
    expect(await within(create).findByRole("status")).toHaveTextContent("The account for max@acme.test is made");
    expect(await within(list).findByText("Max")).toBeInTheDocument();
  });

  it("grants an account a role of its own, and says when a group grants a higher one", async () => {
    const granted: unknown[] = [];
    const DORA = { ...BERT, id: "0190a000-0000-7000-8000-000000000003", name: "Dora", email: "dora@suite.test", role: "editor", ownRole: "reader" };
    fakeAPI({
      ...signedInAs(),
      "GET /accounts": { body: { accounts: [ADMIN, BERT, DORA] } },
      "GET /invitations": { body: { invitations: [] } },
      [`PUT /accounts/${BERT.id}/role`]: (body) => {
        granted.push(body);
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/accounts");
    const list = await screen.findByRole("list", { name: "Accounts" });
    expect(await within(list).findByText("holds Editor through a group")).toBeInTheDocument();
    expect(within(list).queryByLabelText("Own role: Ada Admin")).not.toBeInTheDocument();
    expect(within(list).getByLabelText("Own role: Dora")).toHaveValue("reader");
    await user.selectOptions(within(list).getByLabelText("Own role: Bert"), "editor");
    await waitFor(() => expect(granted).toEqual([{ role: "editor" }]));
  });

  it("lets in who asked to join, declines another, and saves who comes in by group", async () => {
    let requests = [
      { id: "r1", email: "carol@suite.test", name: "Carol", groups: ["quality"], requestedAt: ADMIN.createdAt },
      { id: "r2", email: "mallory@suite.test", name: "Mallory", groups: [], requestedAt: ADMIN.createdAt },
    ];
    const admitted: unknown[] = [];
    let saved: unknown;
    fakeAPI({
      ...signedInAs(),
      "GET /accounts": { body: { accounts: [{ ...ADMIN, groups: ["bearing-administrators", "engineering"] }] } },
      "GET /invitations": { body: { invitations: [] } },
      "GET /join-requests": () => ({ body: { joinRequests: requests } }),
      "POST /join-requests/r1/admit": (body) => {
        admitted.push(body);
        requests = requests.filter((r) => r.id !== "r1");
        return { status: 201, body: { account: { ...BERT, email: "carol@suite.test", name: "Carol", role: "editor" } } };
      },
      "DELETE /join-requests/r2": () => {
        requests = requests.filter((r) => r.id !== "r2");
        return { status: 204 };
      },
      "GET /group-grants": { body: { grants: [{ group: "marketing", role: "reader" }] } },
      "PUT /group-grants": (body) => {
        const rows = (body as { grants: { group: string; role: string }[] }).grants;
        if (rows.some((r) => !r.group.trim()))
          return { status: 422, body: { error: { code: "validation_failed", message: "x", fields: { "grants[1].group": "Name the group." } } } };
        saved = body;
        return { body };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/accounts");
    expect(await screen.findByText("Groups: bearing-administrators, engineering")).toBeInTheDocument();

    const asked = await screen.findByRole("region", { name: "Asked to join" });
    expect(within(asked).getByText("quality")).toBeInTheDocument();
    await user.selectOptions(within(asked).getByLabelText("Role: Carol"), "editor");
    await user.click(within(asked).getByRole("button", { name: "Let in: Carol" }));
    await user.click(await within(asked).findByRole("button", { name: "Decline: Mallory" }));
    expect(admitted).toEqual([{ role: "editor" }]);
    await waitFor(() => expect(screen.queryByRole("region", { name: "Asked to join" })).not.toBeInTheDocument());

    const grants = screen.getByRole("region", { name: "Roles by group" });
    expect(await within(grants).findByDisplayValue("marketing")).toBeInTheDocument();
    await user.click(within(grants).getByRole("button", { name: "Add a group" }));
    await user.click(within(grants).getByRole("button", { name: "Save" }));
    expect(await within(grants).findByText("Name the group.")).toBeInTheDocument();
    await user.type(within(grants).getByLabelText("Group 2"), "quality");
    await user.selectOptions(within(grants).getByLabelText("Role 2"), "editor");
    await user.click(within(grants).getByRole("button", { name: "Save" }));
    expect(await within(grants).findByRole("status")).toHaveTextContent("Saved.");
    expect(saved).toEqual({
      grants: [
        { group: "marketing", role: "reader" },
        { group: "quality", role: "editor" },
      ],
    });
    await user.click(within(grants).getAllByRole("button", { name: "Remove" })[0] as HTMLElement);
    expect(within(grants).queryByDisplayValue("marketing")).not.toBeInTheDocument();
  });
});

describe("single sign-on", () => {
  it("offers the provider with the page to come back to, and says how a sign-in there ended", async () => {
    fakeAPI({ ...nobodySignedIn, "GET /installation": { body: { name: "Acme", setUp: true, mail: false, sso: true } } });
    renderApp("/sign-in?next=%2Fpeople&sso=pending");
    expect(await screen.findByRole("link", { name: "Continue with single sign-on" })).toHaveAttribute("href", "/api/v1/auth/oidc/start?next=%2Fpeople");
    expect(screen.getByRole("status")).toHaveTextContent("You have asked to join");
  });

  it("is not offered without a provider", async () => {
    fakeAPI(nobodySignedIn);
    renderApp("/sign-in");
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Continue with single sign-on" })).not.toBeInTheDocument();
  });
});
