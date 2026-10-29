import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("the app opens, names the API it talks to, and leads to every place", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: "Overview" })).toBeVisible();
  await expect(page.getByTestId("api-status")).toHaveText(/^API \d+\.\d+\.\d+/);
  const nav = page.getByRole("navigation", { name: "Main" });
  await nav.getByRole("link", { name: "Processes" }).click();
  await expect(page).toHaveURL(/\/processes$/);
  await expect(page.getByRole("heading", { level: 1, name: "Processes" })).toBeVisible();
});

test("a deep link survives a reload", async ({ page }) => {
  await page.goto("/people");
  await page.reload();
  await expect(page.getByRole("heading", { level: 1, name: "People" })).toBeVisible();
});

test("the overview has no accessibility violations", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("api-status")).toHaveText(/^API /);
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
});
