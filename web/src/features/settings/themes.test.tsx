import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { THEME_STYLE_ID } from "@/config";
import { applyLanguage } from "@/i18n";
import { applyAppearance, applyThemeCSS } from "@/lib/appearance";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";

const harbourFile = readFileSync(resolve(__dirname, "../../../../backend/internal/theme/testdata/harbour.armature-theme.json"), "utf8");
const harbour = JSON.parse(harbourFile);

const MEMBER = { ...ADMIN, id: "0190a000-0000-7000-8000-000000000002", name: "Max Member", role: "member", ownRole: "member" };

type Row = {
  id: string;
  name: string;
  ownerId: string;
  ownerName: string;
  shared: boolean;
  default: boolean;
  active: boolean;
  inUse: number;
  spec: unknown;
  assets: unknown[];
  updatedAt: string;
};

const HARBOUR: Row = {
  id: "t1",
  name: "Harbour",
  ownerId: ADMIN.id,
  ownerName: ADMIN.name,
  shared: true,
  default: true,
  active: false,
  inUse: 3,
  spec: harbour.spec,
  assets: [{ ...harbour.assets[0], size: 300 }],
  updatedAt: "2026-10-30T10:00:00Z",
};
const PLAIN: Row = {
  id: "t2",
  name: "Plain",
  ownerId: MEMBER.id,
  ownerName: MEMBER.name,
  shared: false,
  default: false,
  active: false,
  inUse: 0,
  spec: { colors: { light: { canvas: "#fafafa" }, dark: {} }, fonts: {}, shape: {}, shadows: {}, cursors: {}, icons: {}, css: "" },
  assets: [],
  updatedAt: "2026-10-30T10:00:00Z",
};

afterEach(() => {
  cleanup();
  applyLanguage("");
  applyAppearance({ mode: "system" });
  applyThemeCSS(null);
  localStorage.clear();
  vi.restoreAllMocks();
});

const themeStyle = () => document.getElementById(THEME_STYLE_ID)?.textContent ?? null;

/** A server that keeps the themes and the reader's choice, answering the active theme from both. */
function server(account: object, start: Row[]) {
  let themes = start.map((each) => ({ ...each }));
  let choice: string | null | undefined;
  const sent: { method: string; body: unknown }[] = [];
  const marked = () => themes.map((each) => ({ ...each, active: each.id === choice }));
  const active = () => {
    if (choice === undefined) {
      const everybodys = themes.find((each) => each.default);
      return { theme: everybodys ?? null, source: everybodys ? "everyone" : "" };
    }
    const chosen = marked().find((each) => each.id === choice);
    return { theme: chosen ?? null, source: chosen ? "chosen" : "" };
  };
  fakeAPI({
    ...signedInAs(account),
    "GET /themes": () => ({ body: { themes: marked() } }),
    "GET /themes/active": () => ({ body: active() }),
    "PUT /themes/active": (body) => {
      sent.push({ method: "choose", body });
      const { themeId, builtIn } = body as { themeId: string | null; builtIn?: boolean };
      choice = themeId ?? (builtIn ? null : undefined);
      return { body: { theme: marked().find((each) => each.id === themeId) ?? null } };
    },
    "PUT /themes/default": (body) => {
      sent.push({ method: "default", body });
      const { themeId } = body as { themeId: string | null };
      themes = themes.map((each) => ({ ...each, default: each.id === themeId }));
      return { status: 204 };
    },
    "PATCH /themes/t2": (body) => {
      sent.push({ method: "update", body });
      themes = themes.map((each) => (each.id === "t2" ? { ...each, ...(body as object) } : each));
      return { body: { theme: themes.find((each) => each.id === "t2") } };
    },
    "DELETE /themes/t2": () => {
      themes = themes.filter((each) => each.id !== "t2");
      return { status: 204 };
    },
    "POST /themes/import": (body) => {
      sent.push({ method: "import", body });
      if ((body as { name: string }).name !== harbour.name)
        return { status: 422, body: { error: { code: "invalid_theme", message: "The extra CSS loads elsewhere." } } };
      const made = { ...HARBOUR, id: "t3", ownerId: (account as { id: string }).id, shared: false, default: false, inUse: 0 };
      themes = [...themes, made];
      return { status: 201, body: { theme: made } };
    },
  });
  return sent;
}

describe("choosing how Bearing looks", () => {
  it("keeps the theme on the server, so it follows the person, and the mode in the browser", async () => {
    const sent = server(MEMBER, [HARBOUR, PLAIN]);
    const user = userEvent.setup();
    renderApp("/");
    await waitFor(() => expect(themeStyle()).toContain("--color-accent: #0e7490;"));

    await user.click(await screen.findByRole("button", { name: "Account and settings" }));
    await user.click(screen.getByRole("radio", { name: "Dark" }));
    expect(document.documentElement.dataset.theme).toBe("dark");
    await user.click(screen.getByRole("radio", { name: "As the system" }));
    expect(document.documentElement.dataset.theme).toBeUndefined();

    const theme = screen.getByLabelText("Theme");
    await waitFor(() => expect(theme).toHaveValue(""));
    expect(await within(theme).findByRole("option", { name: "Plain" })).toBeInTheDocument();
    await user.selectOptions(theme, "Plain");
    await waitFor(() => expect(themeStyle()).toContain("--color-canvas: #fafafa;"));
    expect(sent.at(-1)).toEqual({ method: "choose", body: { themeId: "t2" } });
    await waitFor(() => expect(theme).toHaveValue("t2"));

    await user.selectOptions(theme, "Bearing's own");
    await waitFor(() => expect(themeStyle()).toBeNull());
    expect(sent.at(-1)).toEqual({ method: "choose", body: { themeId: null, builtIn: true } });
    await waitFor(() => expect(theme).toHaveValue("built-in"));
    await user.selectOptions(theme, "What everybody sees");
    await waitFor(() => expect(themeStyle()).toContain("#0e7490"));
    expect(sent.at(-1)).toEqual({ method: "choose", body: { themeId: null } });

    await user.click(screen.getByRole("link", { name: "Themes" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Themes" })).toBeInTheDocument();
  });

  it("steps through the modes from the sidebar", async () => {
    server(MEMBER, []);
    const user = userEvent.setup();
    renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Light or dark: As the system. Click for the next." }));
    expect(document.documentElement.dataset.theme).toBe("light");
    await user.click(screen.getByRole("button", { name: "Light or dark: Light. Click for the next." }));
    expect(document.documentElement.dataset.theme).toBe("dark");
    await user.click(screen.getByRole("button", { name: "Light or dark: Dark. Click for the next." }));
    expect(document.documentElement.dataset.theme).toBeUndefined();
  });
});

describe("the themes page", () => {
  it("shows a member their own themes and the shared ones, and what they may do to each", async () => {
    const sent = server(MEMBER, [HARBOUR, PLAIN]);
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const user = userEvent.setup();
    const { router } = renderApp("/settings/themes");
    expect(await screen.findByText("You are using Harbour, which everybody sees until they choose.")).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Settings" })).toBeNull();

    const mine = await screen.findByRole("table", { name: "Mine" });
    expect(within(mine).getByRole("link", { name: "Plain" })).toBeInTheDocument();
    expect(within(mine).getByText("you")).toBeInTheDocument();
    expect(within(mine).queryByText("Harbour")).toBeNull();

    await user.click(screen.getByRole("radio", { name: "Shared with me" }));
    const shared = await screen.findByRole("table", { name: "Shared with me" });
    expect(within(shared).getByText("Harbour")).toBeInTheDocument();
    expect(within(shared).queryByRole("link", { name: "Harbour" })).toBeNull();
    expect(within(shared).getByText("Everybody sees it")).toBeInTheDocument();
    expect(within(shared).getByText("3 people")).toBeInTheDocument();
    await user.click(within(shared).getByRole("button", { name: "Actions for Harbour" }));
    const menu = screen.getByRole("menu", { name: "Actions for Harbour" });
    expect(within(menu).getByRole("menuitem", { name: "Edit" })).toBeDisabled();
    expect(within(menu).getByRole("menuitem", { name: "Delete" })).toBeDisabled();
    expect(within(menu).queryByRole("menuitem", { name: "Show it to everybody" })).toBeNull();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Use Bearing's own look" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Back to Bearing's own look.");
    expect(await screen.findByText("You are using Bearing's own look, over Harbour, which everybody else sees.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Use what everybody sees" }));
    expect(await screen.findByText("You are using Harbour, which everybody sees until they choose.")).toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Mine" }));
    await user.click(await screen.findByRole("button", { name: "Actions for Plain" }));
    await user.click(screen.getByRole("menuitem", { name: "Use this theme" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Now using Plain.");
    expect(await screen.findByText("Yours now")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Actions for Plain" }));
    await user.click(screen.getByRole("menuitem", { name: "Stop using" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Back to what everybody sees.");

    await user.click(screen.getByRole("button", { name: "Actions for Plain" }));
    await user.click(screen.getByRole("menuitem", { name: "Share with everybody" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Plain is shared with everybody.");
    expect(sent.at(-1)).toEqual({ method: "update", body: { shared: true } });
    await user.click(await screen.findByRole("button", { name: "Actions for Plain" }));
    await user.click(screen.getByRole("menuitem", { name: "Stop sharing" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Plain is yours alone again.");

    await user.click(screen.getByRole("button", { name: "Actions for Plain" }));
    await user.click(screen.getByRole("menuitem", { name: "Delete" }));
    expect(confirm).toHaveBeenCalledWith("Delete Plain and its files for good? Nobody is using it.");
    expect(await screen.findByText("No themes of your own yet.")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Deleted Plain.");

    await user.click(screen.getByRole("link", { name: "New theme" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings/themes/new"));
  });

  it("lets an administrator show a shared theme to everybody, and stop", async () => {
    const sent = server(ADMIN, [{ ...HARBOUR, default: false }]);
    const user = userEvent.setup();
    renderApp("/settings/themes");
    expect(await screen.findByRole("navigation", { name: "Settings" })).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Actions for Harbour" }));
    await user.click(screen.getByRole("menuitem", { name: "Show it to everybody" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Harbour is what everybody sees until they choose.");
    expect(sent.at(-1)).toEqual({ method: "default", body: { themeId: "t1" } });
    await user.click(await screen.findByRole("button", { name: "Actions for Harbour" }));
    await user.click(screen.getByRole("menuitem", { name: "Stop showing it to everybody" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Everybody sees Bearing's own look again.");
    expect(sent.at(-1)).toEqual({ method: "default", body: { themeId: null } });
    await user.click(await screen.findByRole("button", { name: "Actions for Harbour" }));
    await user.click(screen.getByRole("menuitem", { name: "Edit" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Theme" })).toBeInTheDocument();
  });

  it("imports a sibling's theme file as one of the reader's own", async () => {
    const sent = server(MEMBER, []);
    const user = userEvent.setup();
    renderApp("/settings/themes");
    expect(await screen.findByText("No themes of your own yet.")).toBeInTheDocument();
    const file = screen.getByLabelText("Theme file, exported from Armature, Exciter, Stator or Bearing");
    await user.upload(file, new File(["not json"], "broken.json", { type: "application/json" }));
    expect(await screen.findByText("That file is not JSON, so it cannot be a theme file.")).toBeInTheDocument();
    await user.upload(file, new File([JSON.stringify({ ...harbour, name: "Elsewhere" })], "elsewhere.json", { type: "application/json" }));
    expect(await screen.findByText("The extra CSS loads elsewhere.")).toBeInTheDocument();
    await user.upload(file, new File([harbourFile], "harbour.armature-theme.json", { type: "application/json" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Imported Harbour.");
    expect(sent.at(-1)?.body).toMatchObject({ format: "armature-theme/1" });
    expect(await screen.findByRole("link", { name: "Harbour" })).toHaveAttribute("href", "/settings/themes/t3");
  });

  it("says when nobody has shared a theme", async () => {
    server(MEMBER, []);
    const user = userEvent.setup();
    renderApp("/settings/themes");
    await user.click(await screen.findByRole("radio", { name: "Shared with me" }));
    expect(await screen.findByText("Nobody has shared a theme yet.")).toBeInTheDocument();
    expect(await screen.findByText("You are using Bearing's own look.")).toBeInTheDocument();
  });
});
