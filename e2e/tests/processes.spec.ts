import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

const node = (page: Page, label: string) => page.getByRole("article", { name: label });

/** Draws an arrow as a person does: from the lower handle of one node to the upper handle of another. */
async function connect(page: Page, from: string, to: string) {
  await node(page, from).locator(".react-flow__handle-bottom").dragTo(node(page, to).locator(".react-flow__handle-top"));
}

/** Arranges the flow and waits until the view has stopped moving to fit it, so drags start where the nodes are. */
async function arrange(page: Page) {
  await page.getByRole("button", { name: "Arrange" }).click();
  const viewport = page.locator(".react-flow__viewport");
  let last = "";
  await expect(async () => {
    const now = (await viewport.getAttribute("style")) ?? "";
    const settled = now === last;
    last = now;
    expect(settled).toBe(true);
  }).toPass({ intervals: [150] });
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
  const responsibilities = page.getByRole("region", { name: "Node" }).getByRole("region", { name: "Responsibilities" });
  await responsibilities.getByLabel("How they stand to it").selectOption({ label: "Accountable" });
  await responsibilities.getByLabel("Who").selectOption({ label: "Head of Finance and People" });
  await responsibilities.getByRole("button", { name: "Add" }).click();
  await responsibilities.getByLabel("How they stand to it").selectOption({ label: "Responsible" });
  await responsibilities.getByLabel("Who").selectOption({ label: "Invoice approver" });
  await responsibilities.getByRole("button", { name: "Add" }).click();
  await expect(node(page, "Step: Approve")).toContainText("A · Head of Finance and People");
  await arrange(page);

  await connect(page, "Start: Start", "Decision: Over the limit?");
  await connect(page, "Decision: Over the limit?", "Step: Approve");
  await connect(page, "Decision: Over the limit?", "End: End");
  await connect(page, "Step: Approve", "End: End");
  await arrange(page);
  await condition(page, "Arrow from Over the limit? to Approve", "Yes");
  await condition(page, "Arrow from Over the limit? to End", "No");

  await page.getByRole("button", { name: "Save draft" }).click();
  await expect(page.getByRole("button", { name: "Saved" })).toBeVisible();
  await expect(page.getByText("Nothing keeps the draft from being published.")).toBeVisible();
  await page.getByLabel("What changed").fill("First version.");
  await page.getByRole("button", { name: "Publish as the next version" }).click();
  const versions = page.getByRole("region", { name: "Versions" });
  await expect(versions.getByRole("button", { name: "Version 1" })).toBeVisible();
  await expect(versions).toContainText("First version.");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await page.reload();
  // Arranged before saving, every arrow keeps the route the layout gave it.
  const id = new URL(page.url()).pathname.split("/").at(-1);
  const saved = await page.evaluate(async (processId) => {
    const answer = await fetch(`/api/v1/processes/${processId}`);
    return ((await answer.json()) as { process: { draft: { arrows: { route: number[] }[] } } }).process.draft.arrows.map((a) => a.route.length);
  }, id);
  expect(saved.length).toBe(4);
  expect(saved.every((n) => n >= 4)).toBe(true);
  await expect(node(page, "Step: Approve")).toContainText("A · Head of Finance and People");
  await expect(page.getByLabel("Arrow from Over the limit? to Approve", { exact: true })).toBeAttached();
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

test("the demo's supplier invoice says who does what in a RACI matrix", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  await page.getByRole("link", { name: "Supplier invoice" }).click();
  await page.getByRole("button", { name: "RACI matrix" }).click();
  const table = page.getByRole("table");
  await expect(table.getByRole("columnheader", { name: /Invoice approver/ })).toBeVisible();
  const approve = table.getByRole("row", { name: /Approve the invoice/ });
  await expect(approve.getByTitle("Responsible")).toHaveText("R");
  await expect(approve.getByTitle("Accountable")).toHaveText("A");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  await approve.getByRole("button", { name: "Approve the invoice" }).click();
  await expect(node(page, "Step: Approve the invoice")).toContainText("A · Head of Finance and People");
});

test("the demo's supplier invoice reads in swimlanes, one per responsible party", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  await page.getByRole("link", { name: "Supplier invoice" }).click();
  await page.getByRole("button", { name: "Swimlanes" }).click();
  await expect(page.getByRole("region", { name: "Lane: Invoice approver" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Lane: Accountant" })).toBeVisible();
  await expect(page.getByRole("toolbar", { name: "Add" })).toHaveCount(0);
  const approve = await node(page, "Step: Approve the invoice").boundingBox();
  const lane = await page.getByRole("region", { name: "Lane: Invoice approver" }).boundingBox();
  expect(approve && lane && approve.x >= lane.x && approve.x + approve.width <= lane.x + lane.width).toBe(true);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
});

test("the demo's order to delivery shows what its second version changed", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  await page.getByRole("link", { name: "Order to delivery" }).click();
  await page.getByLabel("Showing").selectOption("2");
  await page.getByLabel("Compare with").selectOption("1");
  const changes = page.getByRole("region", { name: "Changes from Version 1 to Version 2" });
  const nodes = changes.getByRole("list", { name: "Nodes changed" });
  await expect(nodes.getByRole("listitem")).toHaveCount(2);
  await expect(nodes.getByRole("listitem").filter({ hasText: "Decision: Within the credit limit?" })).toContainText("added");
  await expect(nodes.getByRole("listitem").filter({ hasText: "Step: Ask for prepayment" })).toContainText("added");
  await expect(changes.getByRole("list", { name: "Arrows changed" }).getByRole("listitem").filter({ hasText: "removed" })).toHaveCount(1);
  await expect(node(page, "Decision: Within the credit limit?").getByText("added")).toBeVisible();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  await nodes.getByRole("button", { name: "Step: Ask for prepayment" }).click();
  await expect(page.getByRole("region", { name: "Node" }).getByRole("list", { name: "Responsibilities" })).toContainText("Accountant");
});

// In the demo the Head of Quality is vacant; the Test lab lead stands in as the first deputy.
test("a step's responsibilities come down to people, a deputy standing in for a vacant position", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  await page.getByRole("link", { name: "Customer complaint" }).click();
  await node(page, "Step: Find the root cause").click();
  const named = page.getByRole("region", { name: "Node" }).getByRole("list", { name: "Responsibilities" });
  await expect(named.getByRole("link", { name: "Carol Checker" })).toBeVisible();
  await expect(named).toContainText("standing in for Head of Quality");
  await named.getByRole("link", { name: "Carol Checker" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Carol Checker" })).toBeVisible();
});

test("who does what: a step found by a term, a person's duties and a role's page", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  await page.getByRole("link", { name: "Who does what" }).click();
  await page.getByLabel("Step or process").fill("root cause");
  const found = page.getByRole("list", { name: "Steps found" });
  await expect(found.getByRole("heading", { name: "Find the root cause" })).toBeVisible();
  await expect(found).toContainText("Carol Checker (standing in)");
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await found.getByRole("link", { name: "Carol Checker" }).first().click();
  await expect(page.getByRole("heading", { level: 1, name: "Carol Checker" })).toBeVisible();
  const duties = page.getByRole("region", { name: "Responsible for" });
  await expect(duties.getByRole("link", { name: "Test the batch" })).toBeVisible();
  await expect(duties).toContainText("standing in for Head of Quality");

  await page.goto("/processes");
  await page.getByRole("link", { name: "Supplier invoice" }).click();
  await node(page, "Step: Approve the invoice").click();
  await page.getByRole("region", { name: "Node" }).getByRole("list", { name: "Responsibilities" }).getByText("Invoice approver").waitFor();
  await page.goto("/people?q=Maren");
  await page.getByRole("link", { name: "Maren Holt" }).click();
  await page.getByRole("region", { name: "Roles" }).getByRole("link", { name: "Invoice approver" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Invoice approver" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Responsible for" }).getByRole("link", { name: "Approve the invoice" })).toBeVisible();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
});

test("a published version reads tidily without changing it", async ({ page }) => {
  await signIn(page);
  await page.goto("/processes");
  await page.getByRole("link", { name: "Supplier invoice" }).click();
  await page.getByRole("region", { name: "Versions" }).getByRole("button", { name: "Version 1" }).click();
  await expect(node(page, "Decision: Does it match?")).toBeVisible();
  await page.getByRole("button", { name: "Tidy view" }).click();
  await expect(page.getByRole("button", { name: "As drawn" })).toHaveAttribute("aria-pressed", "true");
  await expect(node(page, "Step: Clarify with the supplier")).toBeVisible();
  await page.getByRole("button", { name: "As drawn" }).click();
  await expect(page.getByRole("button", { name: "Tidy view" })).toBeVisible();
});
