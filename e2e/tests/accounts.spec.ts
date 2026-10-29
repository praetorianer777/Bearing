import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn, uniqueEmail } from "../fixtures/session";

test("somebody signed out is sent to sign in and back where they were", async ({ page }) => {
  await page.goto("/people");
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  await page.getByLabel("Email").fill("admin@bearing.test");
  await page.getByLabel("Password").fill("bearing admin password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "People" })).toBeVisible();
});

test("an administrator invites somebody, who joins through the link and signs in", async ({ page, browser }) => {
  await signIn(page);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Settings" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Accounts" })).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  const email = uniqueEmail("ines");
  const invite = page.getByRole("region", { name: "Invite somebody" });
  await invite.getByLabel("Email").fill(email);
  await invite.getByLabel("Role").selectOption("editor");
  await invite.getByRole("button", { name: "Invite" }).click();
  const link = await invite.getByRole("textbox", { name: /is invited/ }).inputValue();
  expect(link).toMatch(/\/invitations\/[\w-]+$/);
  await expect(page.getByRole("region", { name: "Open invitations" }).getByText(email)).toBeVisible();

  const ines = await browser.newPage();
  await ines.goto(link);
  await expect(ines.getByText(`invited as ${email}, to work as Editor`)).toBeVisible();
  await ines.getByLabel("Name").fill("Ines Example");
  await ines.getByLabel("Password").fill("ines long password");
  await ines.getByRole("button", { name: "Join" }).click();
  await expect(ines.getByRole("heading", { level: 1, name: "Overview" })).toBeVisible();
  await expect(ines.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Settings" })).toHaveCount(0);

  await ines.getByRole("button", { name: "Account and settings" }).click();
  await ines.getByRole("button", { name: "Sign out" }).click();
  await expect(ines.getByRole("heading", { name: "Sign in" })).toBeVisible();
  await signIn(ines, email, "ines long password");
  await ines.close();
});
