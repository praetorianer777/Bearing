import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// The demo organisation, Kestrel Works, leaves the Head of Quality vacant, which both lists of gaps show.
test("the planning page and a unit's page show where people are missing", async ({ page }) => {
  await signIn(page);
  await page.goto("/planning");
  await expect(page.getByRole("heading", { level: 1, name: "Planning" })).toBeVisible();
  const gaps = page.getByRole("list", { name: "Gaps" });
  await expect(gaps.getByRole("listitem").filter({ hasText: "Head of Quality" }).first()).toContainText("1 seat vacant");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await page.goto("/search?q=quality");
  await page
    .getByRole("region", { name: /^Units/ })
    .getByRole("link", { name: "Quality", exact: true })
    .click();
  const unitGaps = page.getByRole("region", { name: "Gaps", exact: true });
  await expect(unitGaps.getByRole("listitem").filter({ hasText: "Head of Quality" }).first()).toContainText("vacant");
  await unitGaps.getByRole("link", { name: "Head of Quality" }).first().click();
  await expect(page.getByRole("heading", { level: 1, name: "Head of Quality" })).toBeVisible();
});
