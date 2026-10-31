import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// The stack's seed loads the demo organisation, Kestrel Works, whose people this reads.
test("a person's page shows what they hold, whom they report to and whom they stand in for", async ({ page }) => {
  await signIn(page);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "People" }).click();
  await page.getByLabel("Find by name or address").fill("carol");
  await page.getByRole("list", { name: "People" }).getByRole("link", { name: "Carol Checker" }).click();

  await expect(page.getByRole("heading", { level: 1, name: "Carol Checker" })).toBeVisible();
  const section = (name: string) => page.getByRole("region", { name, exact: true });
  await expect(section("Holds")).toContainText("Test lab lead");
  await expect(section("Reports to")).toContainText("Head of Quality");
  await expect(section("Reports to")).toContainText("Vacant");
  await expect(section("Reports to")).toContainText("Gabriel Santos");
  await expect(section("Direct reports")).toContainText("Quality engineer");
  await expect(section("Stands in for")).toContainText("Head of Quality");
  await expect(section("Changes")).toBeVisible();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await section("Reports to").getByRole("link", { name: "Gabriel Santos" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Gabriel Santos" })).toBeVisible();
  await expect(section("Holds")).toContainText("Head of Operations");
});

test("somebody who left keeps their past", async ({ page }) => {
  await signIn(page);
  await page.goto("/people?left=true&q=nadia");
  await page.getByRole("list", { name: "People" }).getByRole("link", { name: "Nadia Petrova" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Nadia Petrova" })).toBeVisible();
  await expect(page.getByText("Left on 2026-04-01")).toBeVisible();
  await expect(page.getByRole("region", { name: "Holds", exact: true })).toContainText("Holds nothing on this day.");
  await expect(page.getByRole("region", { name: "Positions and roles over time" })).toContainText("2025-01-01 until 2026-04-01");
  await page.getByLabel("As of").fill("2026-03-31");
  await expect(page.getByRole("region", { name: "Holds", exact: true })).toContainText("Head of Product");
});
