import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// The stack's seed loads the demo organisation, Kestrel Works, and its example processes, which this finds.
test("the search box finds a person as it is typed, and every kind on the search page", async ({ page }) => {
  await signIn(page);
  await page.keyboard.press("/");
  const box = page.getByRole("combobox", { name: /Search/ });
  await expect(box).toBeFocused();
  await box.pressSequentially("carol chec");
  const suggestions = page.getByRole("listbox", { name: "Suggestions" });
  await expect(suggestions.getByRole("option", { name: /Carol Checker/ })).toBeVisible();
  expect((await new AxeBuilder({ page }).include("header").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  await box.press("ArrowDown");
  await box.press("Enter");
  await expect(page.getByRole("heading", { level: 1, name: "Carol Checker" })).toBeVisible();

  await box.fill("invoice");
  await expect(suggestions.getByRole("option", { name: /Supplier invoice/ }).first()).toBeVisible();
  await box.press("Enter");
  await expect(page).toHaveURL(/\/search\?q=invoice/);
  const group = (name: string) => page.getByRole("region", { name: new RegExp(`^${name}`) });
  await expect(group("Processes").getByRole("link", { name: "Supplier invoice" })).toBeVisible();
  await expect(group("Roles").getByRole("link", { name: "Invoice approver" })).toBeVisible();
  await expect(group("Steps").getByRole("link", { name: "Approve the invoice" })).toBeVisible();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await group("Steps").getByRole("link", { name: "Approve the invoice" }).click();
  await expect(page.getByRole("heading", { level: 1, name: /Supplier invoice/ })).toBeVisible();
  await expect(page).toHaveURL(/version=\d+/);
});
