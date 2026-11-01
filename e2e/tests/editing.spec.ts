import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

const today = () => new Date().toISOString().slice(0, 10);

// Each run makes a unit of its own under Quality, so runs and projects do not meet.
test("an editor adds a unit with positions, moves one on the chart, gives it and ends the holding", async ({ page }, info) => {
  const stamp = `${info.project.name} ${Date.now()}`;
  const unitName = `Bench ${stamp}`;
  await signIn(page);
  await page.goto("/search?q=quality");
  await page
    .getByRole("region", { name: /^Units/ })
    .getByRole("link", { name: "Quality", exact: true })
    .click();

  await page.getByText("Add a sub-unit", { exact: true }).click();
  const sub = page.locator("details", { has: page.getByText("Add a sub-unit", { exact: true }) });
  await sub.getByLabel("Name").fill(unitName);
  await sub.getByRole("button", { name: "Add" }).click();
  await expect(sub.getByRole("status")).toContainText(`${unitName} added.`);
  await page.getByRole("region", { name: "Sub-units" }).getByRole("link", { name: unitName }).click();
  await expect(page.getByRole("heading", { level: 1, name: new RegExp(unitName) })).toBeVisible();

  const addPosition = async (title: string, head: boolean) => {
    const summary = page.getByText("Add a position", { exact: true });
    const form = page.locator("details", { has: summary });
    if (!(await form.evaluate((d) => (d as HTMLDetailsElement).open))) await summary.click();
    await form.getByLabel("Title").fill(title);
    await form.getByLabel("Heads the unit").setChecked(head);
    await form.getByRole("button", { name: "Add" }).click();
    await expect(form.getByRole("status")).toContainText(`${title} added.`);
  };
  await addPosition("Bench lead", true);
  await addPosition("Bench tester", false);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await page.getByRole("link", { name: "Show on the org chart" }).click();
  const lead = page.getByRole("article", { name: "Bench lead" });
  const tester = page.getByRole("article", { name: "Bench tester" });
  await expect(tester).toBeVisible();
  const from = await tester.boundingBox();
  const to = await lead.boundingBox();
  if (!from || !to) throw new Error("the cards are not on the chart");
  await page.mouse.move(from.x + from.width / 2, from.y + 8);
  await page.mouse.down();
  await page.mouse.move(to.x + to.width / 2, to.y + 8, { steps: 12 });
  await page.mouse.up();
  const drop = page.getByRole("region", { name: "Change the organisation" });
  await expect(drop).toContainText("Bench tester reports to Bench lead");
  await drop.getByRole("button", { name: "Make the change" }).click();
  await expect(drop).toBeHidden();

  await tester.getByRole("link", { name: "Bench tester" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Bench tester" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Reports to", exact: true })).toContainText("Bench lead");

  await page.getByText("Give the position to somebody", { exact: true }).click();
  const give = page.locator("details", { has: page.getByText("Give the position to somebody", { exact: true }) });
  await give.getByLabel("Person").selectOption({ index: 1 });
  await give.getByRole("button", { name: "Give" }).click();
  await expect(give.getByRole("status")).toContainText(`holds it from ${today()}`);
  await expect(page.getByRole("region", { name: "Held by" }).getByRole("link")).toHaveCount(1);

  await page.getByText("End or remove a holding", { exact: true }).click();
  const holdings = page.locator("details", { has: page.getByText("End or remove a holding", { exact: true }) });
  const tomorrow = new Date(Date.now() + 86_400_000).toISOString().slice(0, 10);
  await holdings.getByLabel("On").fill(tomorrow);
  await holdings.getByRole("button", { name: /^End / }).click();
  await expect(holdings.getByRole("status")).toContainText(`ends on ${tomorrow}`);
  await expect(page.getByRole("region", { name: "Holders over time" })).toContainText(`until ${tomorrow}`);
});
