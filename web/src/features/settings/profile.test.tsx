import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { atLeast } from "@/api/auth";
import { applyLanguage } from "@/i18n";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

describe("roles", () => {
  it("order from the most an account may do to the least", () => {
    expect(atLeast("administrator", "editor")).toBe(true);
    expect(atLeast("editor", "editor")).toBe(true);
    expect(atLeast("member", "editor")).toBe(false);
    expect(atLeast("reader", "member")).toBe(false);
  });
});

describe("the profile page", () => {
  const MIA = { ...ADMIN, name: "Mia", email: "mia@acme.test", role: "member", ownRole: "member" };

  it("lets a member change their name", async () => {
    const sent: unknown[] = [];
    fakeAPI({
      ...signedInAs(MIA),
      "PATCH /auth/me": (body) => {
        sent.push(body);
        const name = (body as { name: string }).name;
        if (!name.trim()) return { status: 422, body: { error: { code: "validation_failed", message: "x", fields: { name: "Give your name." } } } };
        return { body: { account: { ...MIA, name } } };
      },
    });
    const user = userEvent.setup();
    renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Account and settings" }));
    await user.click(screen.getByRole("link", { name: "Your profile" }));
    const name = await screen.findByLabelText("Name");
    expect(screen.getByText("Your role: Member.")).toBeInTheDocument();
    await user.clear(name);
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Give your name.")).toBeInTheDocument();
    await user.type(name, "Mia Meyer");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Saved.");
    expect(sent.at(-1)).toEqual({ name: "Mia Meyer" });
  });

  it("shows a reader their profile without letting them change it, and the group their role comes through", async () => {
    fakeAPI(signedInAs({ ...MIA, role: "reader", ownRole: "reader" }));
    renderApp("/settings/profile");
    expect(await screen.findByLabelText("Name")).toHaveAttribute("readonly");
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(screen.getByText(/Readers keep the name/)).toBeInTheDocument();
    cleanup();

    fakeAPI(signedInAs({ ...MIA, role: "editor", ownRole: "reader" }));
    renderApp("/settings/profile");
    expect(await screen.findByText("Your role: Editor, through a group at the identity provider. Your own role is Reader.")).toBeInTheDocument();
  });
});
