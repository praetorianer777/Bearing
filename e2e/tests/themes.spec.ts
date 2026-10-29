import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

const canvas = (page: Page) => page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--color-canvas").trim());

test("an administrator imports a theme, and switching themes and light or dark changes the look", async ({ page }) => {
  const name = `Tide ${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
  const file = {
    format: "armature-theme/1",
    name,
    spec: {
      colors: { light: { canvas: "#f4f7fb" }, dark: { canvas: "#0b1220" } },
      fonts: {},
      shape: { radiusControl: 2 },
      shadows: {},
      cursors: {},
      icons: {},
      css: "",
    },
    assets: [],
  };
  await signIn(page);
  await page.goto("/settings/themes");
  await expect(page.getByRole("heading", { level: 1, name: "Themes" })).toBeVisible();
  await page
    .getByLabel("Theme file")
    .setInputFiles({ name: "tide.armature-theme.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(file)) });
  await expect(page.getByRole("status")).toHaveText(`${name} is imported.`);
  expect((await new AxeBuilder({ page }).analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  const menu = page.getByRole("button", { name: "Account and settings" });
  await menu.click();
  await page.getByRole("radio", { name: "Light" }).check();
  const builtIn = await canvas(page);
  await page.getByLabel("Theme", { exact: true }).selectOption({ label: name });
  await expect.poll(() => canvas(page)).toBe("#f4f7fb");
  await page.getByRole("radio", { name: "Dark" }).check();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect.poll(() => canvas(page)).toBe("#0b1220");

  await page.reload();
  await expect.poll(() => canvas(page)).toBe("#0b1220");
  await menu.click();
  await page.getByLabel("Theme", { exact: true }).selectOption({ label: "Bearing's own" });
  await page.getByRole("radio", { name: "Light" }).check();
  await expect.poll(() => canvas(page)).toBe(builtIn);

  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: `Remove: ${name}` }).click();
  await expect(page.getByRole("button", { name: `Remove: ${name}` })).toHaveCount(0);
});
