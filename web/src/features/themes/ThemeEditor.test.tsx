import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { THEME_STYLE_ID } from "@/config";
import { applyThemeCSS } from "@/lib/appearance";
import { emptySpec } from "@/lib/theme";
import { ADMIN, fakeAPI, renderApp, signedInAs } from "@/test/render";

const PICTURE = { id: "a1", name: "back.svg", contentType: "image/svg+xml", size: 2048 };
const FONT = { id: "a2", name: "face.woff2", contentType: "font/woff2", size: 40_000 };

const NIGHT = {
  id: "t9",
  name: "Night",
  ownerId: ADMIN.id,
  ownerName: ADMIN.name,
  shared: false,
  default: false,
  active: false,
  inUse: 0,
  spec: emptySpec(),
  assets: [PICTURE, FONT],
  updatedAt: "2026-10-30T10:00:00Z",
};

const EXAMPLES = [
  { key: "deep-tech", name: "Deep-Tech", description: "Slate and glass.", spec: { ...emptySpec(), colors: { light: { accent: "#1f5fc5" }, dark: {} } } },
  {
    key: "constellation",
    name: "Constellation",
    description: "A network drawn live.",
    spec: { ...emptySpec(), colors: { light: { accent: "#00699e" }, dark: { accent: "#4fd1c5" } }, effect: "constellation" },
  },
];

afterEach(() => {
  cleanup();
  applyThemeCSS(null);
  vi.restoreAllMocks();
});

const themeStyle = () => document.getElementById(THEME_STYLE_ID)?.textContent ?? null;

/** A server holding Night for the reader to edit, keeping what was saved and chosen. */
function editingNight() {
  const out: { saved?: Record<string, unknown>; chosen?: unknown } = {};
  let theme = { ...NIGHT };
  fakeAPI({
    ...signedInAs(),
    "GET /themes/t9": () => ({ body: { theme } }),
    "PATCH /themes/t9": (body) => {
      out.saved = body as Record<string, unknown>;
      theme = { ...theme, ...(body as object) };
      return { body: { theme } };
    },
    "PUT /themes/active": (body) => {
      out.chosen = body;
      theme = { ...theme, active: true };
      return { body: { theme } };
    },
  });
  return out;
}

describe("the theme editor", () => {
  it("starts a new theme blank or from an example, and saves it as the reader's", async () => {
    const sent: unknown[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /themes/examples": { body: { examples: EXAMPLES } },
      "POST /themes": (body) => {
        sent.push(body);
        if ((body as { name: string }).name === "Taken")
          return { status: 409, body: { error: { code: "duplicate_name", message: "You already have a theme with that name." } } };
        return { status: 201, body: { theme: { ...NIGHT, ...(body as object), id: "t10", assets: [] } } };
      },
      "GET /themes/t10": () => ({ body: { theme: { ...NIGHT, ...(sent.at(-1) as object), id: "t10", assets: [] } } }),
    });
    const user = userEvent.setup();
    const { router } = renderApp("/settings/themes/new");
    expect(await screen.findByRole("heading", { level: 1, name: "New theme" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save theme" })).toBeDisabled();

    await user.click(await screen.findByRole("radio", { name: "Constellation" }));
    expect(screen.getByLabelText("Theme name")).toHaveValue("Constellation");
    expect(screen.getByText("1 of 44 colours changed.")).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "Blank" }));
    expect(screen.getByText("Nothing changed yet: every colour is the stylesheet's.")).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "Constellation" }));
    expect(screen.getByLabelText("Theme name")).toHaveValue("Constellation");

    await user.click(screen.getByRole("tab", { name: "Files" }));
    expect(screen.getByText("Save the theme first; then pictures and fonts can be added to it here.")).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Theme name"));
    await user.type(screen.getByLabelText("Theme name"), "Taken");
    await user.click(screen.getByRole("button", { name: "Save theme" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("You already have a theme with that name.");

    await user.clear(screen.getByLabelText("Theme name"));
    await user.type(screen.getByLabelText("Theme name"), "Stars");
    await user.click(screen.getByLabelText("Shared with everybody"));
    await user.click(screen.getByRole("button", { name: "Save theme" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings/themes/t10"));
    expect(sent.at(-1)).toMatchObject({ name: "Stars", shared: true, spec: { effect: "constellation", colors: { dark: { accent: "#4fd1c5" } } } });
    expect(await screen.findByRole("heading", { level: 1, name: "Stars" })).toBeInTheDocument();
  });

  it("changes the colours in both palettes and previews the draft on the page before anything is saved", async () => {
    const editing = editingNight();
    const user = userEvent.setup();
    renderApp("/settings/themes/t9");
    expect(await screen.findByRole("heading", { level: 1, name: "Night" })).toBeInTheDocument();
    expect(screen.queryByRole("radio", { name: "Blank" })).toBeNull();

    await user.click(screen.getByLabelText("Links, the current item"));
    await user.paste("#ff0066");
    expect(screen.getByText("1 of 44 colours changed.")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Pick Page background"), { target: { value: "#eeeeee" } });
    expect(screen.getByText("2 of 44 colours changed.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Reset Page background" }));
    await user.click(screen.getByRole("radio", { name: "Dark" }));
    expect(screen.getByText("Nothing changed yet: every colour is the stylesheet's.")).toBeInTheDocument();
    await user.click(screen.getByRole("textbox", { name: "Text" }));
    await user.paste("#f0f0f0");
    await user.click(screen.getByRole("radio", { name: "Light" }));

    await user.click(screen.getByLabelText("Preview on this page"));
    await waitFor(() => expect(themeStyle()).toContain("--color-accent: #ff0066;"));
    expect(editing.saved).toBeUndefined();
    await user.click(screen.getByLabelText("Preview on this page"));
    await waitFor(() => expect(themeStyle()).toBeNull());

    await user.click(screen.getByRole("button", { name: "Save theme" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Saved Night.");
    expect(editing.saved).toMatchObject({ name: "Night", shared: false, spec: { colors: { light: { accent: "#ff0066" }, dark: { ink: "#f0f0f0" } } } });

    await user.click(screen.getByRole("button", { name: "Use this theme" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Now using Night.");
    expect(editing.chosen).toEqual({ themeId: "t9" });
  });

  it("changes the type, the shape and the cursors", async () => {
    const editing = editingNight();
    const user = userEvent.setup();
    renderApp("/settings/themes/t9");
    // The tabs move with the arrow keys as well as with clicks.
    (await screen.findByRole("tab", { name: "Colours" })).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Type" })).toHaveAttribute("aria-selected", "true");
    await user.type(screen.getByLabelText("Text family"), "Inter");
    await user.selectOptions(screen.getByLabelText("Text family font file"), "face.woff2");
    await user.type(screen.getByLabelText("Code family"), "Mono");
    await user.click(screen.getByRole("button", { name: "Reset: Code family" }));
    expect(screen.queryByLabelText("Code family font file")).toBeNull();

    await user.click(screen.getByRole("tab", { name: "Shape" }));
    await user.type(screen.getByLabelText("Control radius"), "99");
    expect(screen.getByLabelText("Control radius")).toHaveValue(32);
    await user.type(screen.getByLabelText("Overlay radius"), "4");
    await user.clear(screen.getByLabelText("Overlay radius"));
    await user.type(screen.getByLabelText("Raised"), "0 1px 2px black");

    await user.click(screen.getByRole("tab", { name: "Cursors" }));
    await user.selectOptions(screen.getByLabelText("Pointer cursor picture"), "back.svg");
    expect(within(screen.getByLabelText("Pointer cursor picture")).queryByRole("option", { name: "face.woff2" })).toBeNull();
    await user.type(screen.getByLabelText("Point x"), "3");
    await user.type(screen.getByLabelText("Point y"), "4");
    await user.selectOptions(screen.getByLabelText("Wait cursor picture"), "back.svg");
    await user.selectOptions(screen.getByLabelText("Wait cursor picture"), "");

    await user.click(screen.getByRole("button", { name: "Save theme" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Saved Night.");
    expect(editing.saved).toMatchObject({
      spec: {
        fonts: { sans: { family: "Inter", assetId: "a2" } },
        shape: { radiusControl: 32 },
        shadows: { "1": "0 1px 2px black" },
        cursors: { pointer: { assetId: "a1", hotspotX: 3, hotspotY: 4 } },
      },
    });
    expect((editing.saved?.spec as { fonts: object }).fonts).not.toHaveProperty("mono");
    expect((editing.saved?.spec as { cursors: object }).cursors).not.toHaveProperty("wait");
  });

  it("changes the icons, the backdrop and its moving picture, and the extra CSS", async () => {
    const editing = editingNight();
    const user = userEvent.setup();
    renderApp("/settings/themes/t9");
    await user.click(await screen.findByRole("tab", { name: "Icons" }));
    await user.type(screen.getByLabelText("Find an icon"), "sea");
    expect(screen.queryByLabelText("home path data")).toBeNull();
    await user.type(screen.getByLabelText("search path data"), "M2 8h12");
    await user.selectOptions(screen.getByLabelText("search icon picture"), "back.svg");
    await user.selectOptions(screen.getByLabelText("search icon picture"), "");
    await user.clear(screen.getByLabelText("Find an icon"));
    await user.selectOptions(screen.getByLabelText("home icon picture"), "back.svg");
    await user.click(screen.getByRole("button", { name: "Reset: home" }));
    await user.selectOptions(screen.getByLabelText("unit icon picture"), "back.svg");
    await user.type(screen.getByLabelText("unit path data"), "M1 1h2");
    await user.clear(screen.getByLabelText("unit path data"));
    await user.selectOptions(screen.getByLabelText("unit icon picture"), "");

    await user.click(screen.getByRole("tab", { name: "Backdrop" }));
    await user.selectOptions(screen.getByLabelText("Picture"), "back.svg");
    await user.click(screen.getByRole("radio", { name: "Tile" }));
    await user.click(screen.getByLabelText("Preview on this page"));
    await user.click(screen.getByRole("radio", { name: "Confetti" }));
    await waitFor(() => expect(document.querySelector('[data-backdrop-effect="confetti"]')).not.toBeNull());
    await user.click(screen.getByRole("radio", { name: "None" }));
    await user.click(screen.getByRole("radio", { name: "Constellation" }));

    await user.click(screen.getByRole("tab", { name: "Advanced" }));
    await user.type(screen.getByLabelText("Extra CSS"), ".x {{}");
    expect(screen.getByText(/0 of 32 KB/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save theme" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Saved Night.");
    expect(editing.saved?.spec).toMatchObject({
      icons: { search: { paths: ["M2 8h12"] } },
      backdrop: { assetId: "a1", fit: "tile" },
      effect: "constellation",
      css: ".x {}",
    });
    expect(Object.keys((editing.saved?.spec as { icons: object }).icons)).toEqual(["search"]);
  });

  it("adds files, refuses one too large, and removes only what the theme does not use", async () => {
    let theme = { ...NIGHT, assets: [PICTURE], spec: { ...emptySpec(), backdrop: { assetId: "a1", fit: "cover" } } };
    const added: unknown[] = [];
    fakeAPI({
      ...signedInAs(),
      "GET /themes/t9": () => ({ body: { theme } }),
      "POST /themes/t9/assets": (body) => {
        added.push(body);
        const made = { id: "a3", name: "cursor.svg", contentType: "image/svg+xml", size: 120 };
        theme = { ...theme, assets: [...theme.assets, made] };
        return { status: 201, body: { asset: made } };
      },
      "DELETE /themes/t9/assets/a3": () => {
        theme = { ...theme, assets: theme.assets.filter((a) => a.id !== "a3") };
        return { status: 204 };
      },
    });
    const user = userEvent.setup();
    renderApp("/settings/themes/t9");
    await user.click(await screen.findByRole("tab", { name: "Files" }));
    const files = screen.getByRole("list", { name: "Files" });
    expect(within(files).getByText("back.svg")).toBeInTheDocument();
    expect(within(files).getByText("2 KB")).toBeInTheDocument();
    expect(within(files).getByText("In use")).toBeInTheDocument();
    expect(within(files).getByRole("button", { name: "Remove back.svg" })).toBeDisabled();

    const input = screen.getByLabelText("Add a file");
    const huge = new File(["x"], "huge.png", { type: "image/png" });
    Object.defineProperty(huge, "size", { value: 5 * 1024 * 1024 });
    await user.upload(input, huge);
    expect(await screen.findByRole("alert")).toHaveTextContent("That file is over 4 MB.");

    await user.upload(input, new File(['<svg xmlns="http://www.w3.org/2000/svg"/>'], "cursor.svg", { type: "image/svg+xml" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Added cursor.svg.");
    expect(added[0]).toEqual({ name: "cursor.svg", data: btoa('<svg xmlns="http://www.w3.org/2000/svg"/>') });
    expect(screen.queryByRole("alert")).toBeNull();
    await user.click(await screen.findByRole("button", { name: "Remove cursor.svg" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Removed cursor.svg.");
    await waitFor(() => expect(screen.queryByText("cursor.svg")).toBeNull());
  });

  it("says when the theme is not there to edit", async () => {
    fakeAPI({ ...signedInAs() });
    renderApp("/settings/themes/gone");
    expect(await screen.findByRole("alert")).toHaveTextContent("Nothing here.");
    expect(screen.getByRole("link", { name: "All themes" })).toHaveAttribute("href", "/settings/themes");
  });
});
