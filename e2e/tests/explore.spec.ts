import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// The demo organisation, Kestrel Works: the Test lab lead, Carol Checker, stands in for the Head of Quality.
test("the explorer opens around a position and goes further from a person", async ({ page }) => {
  await signIn(page);
  await page.goto("/search?q=test+lab+lead");
  await page
    .getByRole("region", { name: /^Positions/ })
    .getByRole("link", { name: "Test lab lead" })
    .click();
  await page.getByRole("link", { name: "Explore what is around it" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Explore" })).toBeVisible();

  await expect(page.getByRole("article", { name: "Test lab lead" })).toBeVisible();
  const carol = page.getByRole("article", { name: "Carol Checker" });
  await expect(carol).toBeVisible();
  await expect(page.getByRole("article", { name: "Head of Quality" }).first()).toBeVisible();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await carol.getByRole("button", { name: "Show what is around Carol Checker" }).click();
  await page.getByText(/relations as a list/).click();
  const relations = page.getByRole("list", { name: "Relations" });
  await expect(relations).toContainText("Carol Checker holds Test lab lead");
  await expect(relations).toContainText("Test lab lead stands in for Head of Quality");

  await page.getByLabel("Deputies").uncheck();
  await expect(relations).not.toContainText("stands in for");
});
