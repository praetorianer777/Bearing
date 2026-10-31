import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

const node = (page: Page, label: string) => page.getByRole("article", { name: label });

/** Draws an arrow as a person does: from the lower handle of one node to the upper handle of another. */
async function connect(page: Page, from: string, to: string) {
  await node(page, from).locator(".react-flow__handle-bottom").dragTo(node(page, to).locator(".react-flow__handle-top"));
}

async function rename(page: Page, label: string, name: string) {
  await node(page, label).click();
  const field = page.getByRole("region", { name: "Node" }).getByLabel("Name");
  await field.fill(name);
  await field.blur();
}

// A straight arrow is a line with no width, which Playwright counts as hidden; the click goes to it directly.
const pickArrow = (page: Page, arrow: string) => page.getByLabel(arrow, { exact: true }).dispatchEvent("click");

async function condition(page: Page, arrow: string, label: string) {
  await pickArrow(page, arrow);
  await page.getByRole("region", { name: "Arrow" }).getByLabel("Condition").fill(label);
}

test("an editor draws a process with a decision and publishes it", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  const name = `Approval ${Date.now()}`;
  const form = page.getByRole("region", { name: "Add a process" });
  await form.getByLabel("Name").fill(name);
  await form.getByLabel("Area").fill("Browser tests");
  await form.getByRole("button", { name: "Add" }).click();
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
  await expect(node(page, "Start: Start")).toBeVisible();

  await pickArrow(page, "Arrow from Start to End");
  await page.getByRole("button", { name: "Remove the arrow" }).click();
  await page.getByRole("button", { name: "Decision", exact: true }).click();
  await rename(page, "Decision: Decide?", "Over the limit?");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await rename(page, "Step: New step", "Approve");
  await page.getByRole("button", { name: "Arrange" }).click();

  await connect(page, "Start: Start", "Decision: Over the limit?");
  await connect(page, "Decision: Over the limit?", "Step: Approve");
  await connect(page, "Decision: Over the limit?", "End: End");
  await connect(page, "Step: Approve", "End: End");
  await page.getByRole("button", { name: "Arrange" }).click();
  await condition(page, "Arrow from Over the limit? to Approve", "Yes");
  await condition(page, "Arrow from Over the limit? to End", "No");

  await page.getByRole("button", { name: "Save draft" }).click();
  await expect(page.getByText("Nothing keeps the draft from being published.")).toBeVisible();
  await page.getByLabel("What changed").fill("First version.");
  await page.getByRole("button", { name: "Publish as the next version" }).click();
  const versions = page.getByRole("region", { name: "Versions" });
  await expect(versions.getByRole("button", { name: "Version 1" })).toBeVisible();
  await expect(versions).toContainText("First version.");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await page.reload();
  await expect(node(page, "Step: Approve")).toBeVisible();
  await expect(page.getByLabel("Arrow from Over the limit? to Approve", { exact: true })).toBeVisible();
  await page.goto("/processes");
  await expect(page.getByRole("region", { name: "Browser tests" }).getByText("Version 1").first()).toBeVisible();
});

// The stack's seed loads the demo's processes, one of them in two versions and with a subprocess.
test("the demo's processes read as they were published", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  await page.getByRole("link", { name: "Order to delivery" }).click();
  await expect(node(page, "Decision: Within the credit limit?")).toBeVisible();
  await page.getByRole("region", { name: "Versions" }).getByRole("button", { name: "Version 1" }).click();
  await expect(node(page, "Decision: In stock?")).toBeVisible();
  await expect(node(page, "Decision: Within the credit limit?")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Save draft" })).toHaveCount(0);
  await page.getByRole("link", { name: "Production run" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Production run" })).toBeVisible();
});
