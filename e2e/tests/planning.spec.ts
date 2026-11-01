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
  await expect(page.getByRole("table", { name: /^Full-time equivalents/ })).toContainText("Kestrel Works");
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

// Each project plans into a vacant position of its own, so the two running at once do not take each other's seat.
test("an editor plans a holding into a gap and sees it close in the month the holding starts", async ({ page }, info) => {
  const [unit, title] = info.project.name === "mobile" ? ["Infrastructure", "Security engineer"] : ["Quality", "Head of Quality"];
  const now = new Date();
  const nextMonth = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 1)).toISOString().slice(0, 10);
  await signIn(page);
  await page.goto(`/search?q=${encodeURIComponent(unit)}`);
  await page
    .getByRole("region", { name: /^Units/ })
    .getByRole("link", { name: unit, exact: true })
    .click();
  await page.getByRole("link", { name: "Open in planning" }).click();
  await page.getByLabel("Count by").selectOption("month");

  const row = page.getByRole("row").filter({ has: page.getByRole("rowheader", { name: new RegExp(title) }) });
  const cells = row.getByRole("cell");
  await expect(cells.nth(0)).toContainText("vacant");
  await expect(cells.nth(1)).toContainText("vacant");

  await page.getByRole("button", { name: `Plan a holder for ${title}` }).click();
  const form = page.getByRole("region", { name: `Plan a holder for ${title}` });
  await form.getByLabel("Somebody in the organisation").check();
  await form.getByLabel("Person").selectOption({ index: 1 });
  await form.getByLabel("From").fill(nextMonth);
  await form.getByRole("button", { name: "Plan the holding" }).click();
  await expect(form.getByRole("status")).toContainText(`holds ${title} from ${nextMonth}`);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await expect(cells.nth(1)).not.toContainText("vacant");
  await expect(cells.nth(0)).toContainText("vacant");

  // The planned holding goes again, so the next run finds the seat free.
  const positionId = (await row.getByRole("link", { name: title }).getAttribute("href"))?.split("/").pop();
  const status = await page.evaluate(
    async ({ positionId, nextMonth }) => {
      const { holdings } = await (await fetch(`/api/v1/holdings?positionId=${positionId}`)).json();
      const planned = holdings.find((h: { from: string }) => h.from === nextMonth);
      return (await fetch(`/api/v1/holdings/${planned.id}`, { method: "DELETE" })).status;
    },
    { positionId, nextMonth },
  );
  expect(status).toBe(204);
});
