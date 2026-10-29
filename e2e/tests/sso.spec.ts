import { expect, test } from "@playwright/test";

test("somebody let in ahead signs in through the identity provider and lands where they were headed", async ({ page }) => {
  await page.goto("/people");
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  await page.getByRole("link", { name: "Continue with single sign-on" }).click();

  // Keycloak's own login page, in the suite's realm.
  await page.locator("#username").fill("bob");
  await page.locator("#password").fill("bob password");
  await page.locator("#kc-login").click();

  await expect(page.getByRole("heading", { level: 1, name: "People" })).toBeVisible();
  await page.getByRole("button", { name: "Account and settings" }).click();
  await expect(page.getByText("bob@suite.test")).toBeVisible();
  await expect(page.getByText("Member", { exact: true })).toBeVisible();
});
