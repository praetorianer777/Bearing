import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// The stack's seed loads the demo organisation, Kestrel Works, whose units and positions this reads.
test("a unit's page leads to its positions, and a position's page to its people", async ({ page }) => {
  await signIn(page);
  await page.goto("/people?q=carol");
  await page.getByRole("link", { name: "Carol Checker" }).click();
  await page.getByRole("region", { name: "Holds", exact: true }).getByRole("link", { name: "Test lab", exact: true }).click();

  await expect(page.getByRole("heading", { level: 1, name: /Test lab/ })).toBeVisible();
  const section = (name: string) => page.getByRole("region", { name, exact: true });
  await expect(section("Head")).toContainText("Carol Checker");
  await expect(section("Positions")).toContainText("Quality engineer");
  await page.getByText("part of").getByRole("link", { name: "Quality", exact: true }).click();

  await expect(page.getByRole("heading", { level: 1, name: /Quality/ })).toBeVisible();
  await expect(section("Head")).toContainText("Head of Quality");
  await expect(section("Head")).toContainText("Vacant");
  await expect(section("Sub-units")).toContainText("Test lab");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  await section("Head").getByRole("link", { name: "Head of Quality" }).click();

  await expect(page.getByRole("heading", { level: 1, name: "Head of Quality" })).toBeVisible();
  await expect(section("Held by")).toContainText("Vacant on this day.");
  await expect(section("Deputies")).toContainText("Test lab lead");
  await expect(section("Reports to")).toContainText("Head of Operations");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
});

test("a position's page keeps who held it before", async ({ page }) => {
  await signIn(page);
  await page.goto("/organisation?asOf=2026-03-31");
  await page.getByLabel("Organisation", { exact: true }).getByRole("link", { name: "Head of Product" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Head of Product" })).toBeVisible();
  await expect(page).toHaveURL(/asOf=2026-03-31/);
  await expect(page.getByRole("region", { name: "Held by" })).toContainText("Nadia Petrova");
  const history = page.getByRole("region", { name: "Holders over time" });
  await expect(history).toContainText("Elif Kaya");
  await expect(history).toContainText("2025-01-01 until 2026-04-01");
});
