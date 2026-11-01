import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { fakeAPI, renderApp, signedInAs } from "@/test/render";

afterEach(() => {
  cleanup();
  applyLanguage("");
  vi.restoreAllMocks();
});

describe("access tokens", () => {
  it("makes a token, shows its secret once, and revokes it", async () => {
    const made: unknown[] = [];
    let tokens: object[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /tokens": () => ({ body: { tokens } }),
      "POST /tokens": (body) => {
        made.push(body);
        const token = { id: "t1", name: "Reporting", write: false, createdAt: "2026-11-01T09:00:00Z", expiresAt: "2027-01-30T09:00:00Z" };
        tokens = [token];
        return { status: 201, body: { token, secret: "bearing_pat_secret" } };
      },
      "DELETE /tokens/t1": () => {
        tokens = [];
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/tokens");
    expect(await screen.findByText("You have no tokens.")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Name"), "Reporting");
    await user.selectOptions(screen.getByLabelText("Ends after"), "90");
    await user.click(screen.getByRole("button", { name: "Make the token" }));
    expect(await screen.findByRole("status")).toHaveTextContent("bearing_pat_secret");
    expect(made).toEqual([{ name: "Reporting", write: false, days: 90 }]);
    const yours = await screen.findByRole("list", { name: "Your tokens" });
    expect(within(yours).getByText(/^Reads · until/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Revoke Reporting" }));
    expect(await screen.findByText("You have no tokens.")).toBeInTheDocument();
  });

  it("says what is wrong with a token that cannot be made", async () => {
    fakeAPI({
      ...signedInAs(),
      "GET /tokens": () => ({ body: { tokens: [] } }),
      "POST /tokens": () => ({ status: 422, body: { error: { code: "invalid", message: "Check the fields.", fields: { name: "Name the token." } } } }),
    });
    const user = userEvent.setup();
    renderApp("/settings/tokens");
    await user.click(await screen.findByRole("button", { name: "Make the token" }));
    expect(await screen.findByText("Name the token.")).toBeInTheDocument();
  });
});
