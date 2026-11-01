import { readFileSync } from "node:fs";
import AxeBuilder from "@axe-core/playwright";
import { type Browser, expect, type Page, test } from "@playwright/test";
import { signIn, uniqueEmail } from "../fixtures/session";

const canvas = (page: Page) => page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--color-canvas").trim());

const violations = async (page: Page) => (await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`);

/** A member of the installation of their own, signed in on a browser of their own. */
async function member(admin: Page, browser: Browser): Promise<Page> {
  const email = uniqueEmail("tess");
  const made = await admin.request.post("/api/v1/accounts", { data: { name: "Tess Member", email, role: "member", password: "tess long password" } });
  expect(made.status()).toBe(201);
  const page = await (await browser.newContext()).newPage();
  await signIn(page, email, "tess long password");
  return page;
}

// The installation's default is left alone: both projects run against one stack at once, and the integration suite
// covers it.
test("a theme made in the editor is shared, used by a member on their own browser, and gone when deleted", async ({ page, browser }, info) => {
  const name = `Tide ${info.project.name} ${Date.now()}`;
  await signIn(page);
  await page.goto("/settings/themes");
  await page.getByRole("link", { name: "New theme" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "New theme" })).toBeVisible();
  await page.getByLabel("Theme name").fill(name);
  const builtIn = await canvas(page);
  await page.getByRole("textbox", { name: "Page background", exact: true }).fill("#f4f7fb");
  await page.getByLabel("Preview on this page").check();
  await expect.poll(() => canvas(page)).toBe("#f4f7fb");
  expect(await violations(page)).toEqual([]);
  await page.getByRole("button", { name: "Save theme" }).click();
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
  await expect(page).toHaveURL(/\/settings\/themes\/[0-9a-f-]{36}$/);
  // Leaving the editor ends the preview: the administrator has not chosen the theme.
  await page.getByRole("link", { name: "All themes" }).click();
  await expect.poll(() => canvas(page)).toBe(builtIn);

  await page.getByRole("button", { name: `Actions for ${name}` }).click();
  await page.getByRole("menuitem", { name: "Share with everybody" }).click();
  await expect(page.getByRole("status")).toHaveText(`${name} is shared with everybody.`);
  expect(await violations(page)).toEqual([]);

  const tess = await member(page, browser);
  // Another person's write reaches Tess's reads once the replica has it.
  await expect(async () => {
    await tess.goto("/settings/themes");
    await tess.locator("label", { hasText: "Shared with me" }).click();
    await expect(tess.getByRole("button", { name: `Actions for ${name}` })).toBeVisible({ timeout: 1000 });
  }).toPass();
  await tess.getByRole("button", { name: `Actions for ${name}` }).click();
  await tess.getByRole("menuitem", { name: "Use this theme" }).click();
  await expect(tess.getByRole("status")).toHaveText(`Now using ${name}.`);
  await expect.poll(() => canvas(tess)).toBe("#f4f7fb");

  // On a reload the theme is on the page before the server says which it is, so nothing flashes.
  let answer: () => void = () => {};
  const held = new Promise<void>((resolve) => {
    answer = resolve;
  });
  await tess.route("**/api/v1/themes/active", async (route) => {
    await held;
    // A request the page gave up on while it was held is nobody's to answer.
    await route.continue().catch(() => {});
  });
  await tess.reload();
  await expect(tess.getByRole("heading", { level: 1, name: "Themes" })).toBeVisible();
  expect(await canvas(tess)).toBe("#f4f7fb");
  answer();

  await expect(async () => {
    await page.reload();
    await expect(page.getByRole("row", { name: new RegExp(name) })).toContainText("1 person", { timeout: 1000 });
  }).toPass();
  page.once("dialog", (dialog) => {
    expect(dialog.message()).toBe(`Delete ${name} and its files for good? The one person using it goes back to what everybody sees.`);
    void dialog.accept();
  });
  await page.getByRole("button", { name: `Actions for ${name}` }).click();
  await page.getByRole("menuitem", { name: "Delete" }).click();
  await expect(page.getByRole("status")).toHaveText(`Deleted ${name}.`);

  await tess.reload();
  await expect.poll(() => canvas(tess)).toBe(builtIn);
  await tess.context().close();
});

test("a theme exported from Exciter imports, and exports again as it came", async ({ page }) => {
  const file = readFileSync(new URL("../../backend/internal/theme/testdata/exciter.armature-theme.json", import.meta.url), "utf8");
  const exciter = JSON.parse(file);
  await signIn(page);
  await page.goto("/settings/themes");
  await page
    .getByLabel("Theme file, exported from Armature, Exciter, Stator or Bearing")
    .setInputFiles({ name: "workshop.armature-theme.json", mimeType: "application/json", buffer: Buffer.from(file) });
  const status = page.getByRole("status");
  await expect(status).toHaveText(/^Imported Workshop, from Exciter( \(\d+\))?\.$/);
  const imported = (await status.textContent())?.replace(/^Imported |\.$/g, "") ?? "";

  const listed = (await (await page.request.get("/api/v1/themes")).json()) as { themes: { id: string; name: string }[] };
  const theme = listed.themes.find((each) => each.name === imported);
  if (!theme) throw new Error(`${imported} is not listed`);
  const exported = await (await page.request.get(`/api/v1/themes/${theme.id}/export`)).json();
  expect(exported.format).toBe("armature-theme/1");
  expect(exported.spec.colors).toEqual(exciter.spec.colors);
  expect(exported.spec.effect).toBe(exciter.spec.effect);
  expect(exported.assets.map((a: { name: string; data: string }) => [a.name, a.data])).toEqual(
    exciter.assets.map((a: { name: string; data: string }) => [a.name, a.data]),
  );
  expect((await page.request.delete(`/api/v1/themes/${theme.id}`)).status()).toBe(204);
});
