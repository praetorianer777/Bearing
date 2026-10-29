import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyLanguage } from "@/i18n";
import { applyAppearance, INSTALLATION_THEME } from "@/lib/appearance";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";

const harbourFile = readFileSync(resolve(__dirname, "../../../../backend/internal/theme/testdata/harbour.armature-theme.json"), "utf8");
const harbour = JSON.parse(harbourFile);

const HARBOUR = { id: "t1", name: "Harbour", default: true, spec: harbour.spec, assets: [{ ...harbour.assets[0], size: 300 }] };
const PLAIN = {
  id: "t2",
  name: "Plain",
  default: false,
  spec: { colors: { light: { canvas: "#ffffff" }, dark: {} }, fonts: {}, shape: {}, shadows: {}, cursors: {}, icons: {}, css: "" },
  assets: [],
};

afterEach(() => {
  cleanup();
  applyLanguage("");
  applyAppearance({ mode: "system", theme: INSTALLATION_THEME });
  vi.restoreAllMocks();
});

const themeStyle = () => document.querySelector("style[data-theme-id]");

describe("choosing how Bearing looks", () => {
  it("switches between light, dark and the system, and among the installation's themes", async () => {
    fakeAPI({ ...signedInAs({ ...ADMIN, role: "reader", ownRole: "reader" }), "GET /themes": { body: { themes: [HARBOUR, PLAIN] } } });
    const user = userEvent.setup();
    renderApp("/");
    await waitFor(() => expect(themeStyle()?.getAttribute("data-theme-id")).toBe("t1"));
    expect(themeStyle()?.textContent).toContain("--color-accent: #0e7490;");

    await user.click(await screen.findByRole("button", { name: "Account and settings" }));
    await user.click(screen.getByRole("radio", { name: "Dark" }));
    expect(document.documentElement.dataset.theme).toBe("dark");
    await user.click(screen.getByRole("radio", { name: "As the system" }));
    expect(document.documentElement.dataset.theme).toBeUndefined();

    const theme = screen.getByLabelText("Theme");
    expect(await within(theme).findByRole("option", { name: "Plain" })).toBeInTheDocument();
    await user.selectOptions(theme, "Plain");
    await waitFor(() => expect(themeStyle()?.getAttribute("data-theme-id")).toBe("t2"));
    await user.selectOptions(theme, "Bearing's own");
    await waitFor(() => expect(themeStyle()).toBeNull());
  });
});

describe("the themes page", () => {
  it("is for administrators only", async () => {
    fakeAPI({ ...signedInAs({ ...ADMIN, role: "editor", ownRole: "editor" }), "GET /themes": { body: { themes: [] } } });
    renderApp("/settings/themes");
    expect(await screen.findByRole("heading", { level: 1, name: "Overview" })).toBeInTheDocument();
  });

  it("imports a theme file, names where everybody starts, exports and removes", async () => {
    let themes: unknown[] = [];
    const sent: unknown[] = [];
    let defaultTo: unknown = "unset";
    fakeAPI({
      ...signedInAs(),
      "GET /themes": () => ({ body: { themes } }),
      "POST /themes/import": (body) => {
        sent.push(body);
        if ((body as { name: string }).name !== harbour.name) {
          return { status: 422, body: { error: { code: "invalid_theme", message: "The extra CSS loads elsewhere." } } };
        }
        themes = [{ ...HARBOUR, default: false }];
        return { status: 201, body: { theme: themes[0] } };
      },
      "PUT /themes/default": (body) => {
        defaultTo = (body as { themeId: string | null }).themeId;
        themes = [{ ...HARBOUR, default: defaultTo === "t1" }];
        return { status: 204 };
      },
      "DELETE /themes/t1": () => {
        themes = [];
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/themes");
    expect(await screen.findByText("No themes yet. Everybody sees Bearing's own look.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Themes" })).toHaveAttribute("aria-current", "page");

    const file = screen.getByLabelText("Theme file");
    await user.upload(file, new File(["not json"], "broken.json", { type: "application/json" }));
    expect(await screen.findByText("That file is not JSON, so it cannot be a theme file.")).toBeInTheDocument();
    await user.upload(file, new File([JSON.stringify({ ...harbour, name: "Elsewhere" })], "elsewhere.json", { type: "application/json" }));
    expect(await screen.findByText("The extra CSS loads elsewhere.")).toBeInTheDocument();
    await user.upload(file, new File([harbourFile], "harbour.armature-theme.json", { type: "application/json" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Harbour is imported.");
    expect((sent.at(-1) as { format: string }).format).toBe("armature-theme/1");

    expect(await screen.findByRole("link", { name: "Export: Harbour" })).toHaveAttribute("href", "/api/v1/themes/t1/export");
    await user.click(screen.getByRole("button", { name: "Start everybody here: Harbour" }));
    expect(await screen.findByText("everybody starts here")).toBeInTheDocument();
    expect(defaultTo).toBe("t1");
    await user.click(screen.getByRole("button", { name: "Back to Bearing's own: Harbour" }));
    await waitFor(() => expect(defaultTo).toBeNull());
    await user.click(await screen.findByRole("button", { name: "Remove: Harbour" }));
    expect(await screen.findByText("No themes yet. Everybody sees Bearing's own look.")).toBeInTheDocument();
  });
});
