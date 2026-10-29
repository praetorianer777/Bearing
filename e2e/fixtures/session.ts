import { expect, type Page } from "@playwright/test";
import { ADMIN } from "./stack";

/** Signs in on the sign-in page, as a person does, and waits for the overview. */
export async function signIn(page: Page, email = ADMIN.email, password = ADMIN.password): Promise<void> {
  await page.goto("/sign-in");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Overview" })).toBeVisible();
}

/** An address nobody else in the run uses. */
export function uniqueEmail(name: string): string {
  return `${name}-${Date.now()}-${Math.floor(Math.random() * 1e6)}@bearing.test`;
}
