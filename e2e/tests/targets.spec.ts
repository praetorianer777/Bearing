import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// Each project sets a target on a unit of its own of the demo organisation, so the two running at once do not meet.
test("an administrator sets a unit's target and sees how far it falls short", async ({ page }, info) => {
  const name = info.project.name === "mobile" ? "Test lab" : "Quality";
  await signIn(page);
  await page.goto(`/search?q=${encodeURIComponent(name)}`);
  await page
    .getByRole("region", { name: /^Units/ })
    .getByRole("link", { name, exact: true })
    .click();
  await expect(page.getByRole("heading", { level: 1, name: new RegExp(name) })).toBeVisible();

  const section = page.getByRole("region", { name: "Target", exact: true });
  await section.getByLabel("Full-time equivalents").fill("40");
  await section.getByRole("button", { name: "Set" }).click();
  await expect(section).toContainText("Should have 40 FTE");
  await expect(section).toContainText("short");
  await expect(section.getByRole("list", { name: "Targets over time" })).toContainText("40 FTE");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await expect(page.getByRole("region", { name: "Changes" })).toContainText("Target added");
  await section.getByRole("button", { name: /^Remove: 40 FTE/ }).click();
  await expect(section).toContainText("No target on this day.");
});
