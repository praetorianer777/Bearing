import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// The stack's seed loads the demo organisation, Kestrel Works, which this reads.
test("the org chart opens, folds a unit and focuses on a person", async ({ page }) => {
  await signIn(page);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Organisation" }).click();
  const chart = page.getByLabel("Organisation", { exact: true });
  await expect(chart.getByRole("article", { name: "Chief executive" })).toBeVisible();
  await expect(chart.getByRole("article", { name: "Head of Quality" })).toContainText("Vacant");
  await expect(chart.getByRole("article", { name: "Assembly technician" })).toBeAttached();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  // The whole organisation starts at a zoom one can read, wider than the window; fitting it brings every card in.
  await page.getByRole("button", { name: /fit view/i }).click();
  await chart.getByRole("button", { name: "Fold Operations" }).click();
  await expect(chart.getByRole("article", { name: "Assembly technician" })).toHaveCount(0);
  await expect(chart.getByRole("button", { name: "Unfold Operations" })).toHaveText(/positions folded away/);
  await chart.getByRole("button", { name: "Unfold Operations" }).click();
  await expect(chart.getByRole("article", { name: "Assembly technician" })).toBeAttached();

  await page.getByLabel("Focus on", { exact: true }).selectOption({ label: "Jonas Weber" });
  await expect(page).toHaveURL(/focus=person/);
  await expect(chart.getByRole("article", { name: "Backend lead" })).toBeVisible();
  await expect(chart.getByRole("article", { name: "Head of Platform" })).toBeAttached();
  await expect(chart.getByRole("article", { name: "Backend developer" })).toBeAttached();
  await expect(chart.getByRole("article", { name: "Assembly technician" })).toHaveCount(0);

  await page.reload();
  await expect(chart.getByRole("article", { name: "Backend lead" })).toBeVisible();
  await expect(chart.getByRole("article", { name: "Assembly lead" })).toHaveCount(0);
});

test("the chart reads the organisation as it was on a day", async ({ page }) => {
  await signIn(page);
  await page.goto("/organisation?asOf=2026-03-31");
  const chart = page.getByLabel("Organisation", { exact: true });
  await expect(chart.getByRole("article", { name: "Head of Product" })).toContainText("Nadia Petrova");
  await page.getByLabel("As of").fill("2026-04-01");
  await expect(chart.getByRole("article", { name: "Head of Product" })).toContainText("Elif Kaya");
});
