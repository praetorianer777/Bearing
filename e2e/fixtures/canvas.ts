import { expect, type Page } from "@playwright/test";

/** A node on the process canvas, by its kind and name, such as "Step: Approve". */
export const node = (page: Page, label: string) => page.getByRole("article", { name: label });

/** Draws an arrow as a person does: from the lower handle of one node to the upper handle of another. */
export async function connect(page: Page, from: string, to: string) {
  await node(page, from).locator(".react-flow__handle-bottom").dragTo(node(page, to).locator(".react-flow__handle-top"));
}

/** Arranges the flow and waits until the view has stopped moving to fit it, so drags start where the nodes are. */
export async function arrange(page: Page) {
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

export async function rename(page: Page, label: string, name: string) {
  await node(page, label).click();
  const field = page.getByRole("region", { name: "Node" }).getByLabel("Name");
  await field.fill(name);
  await field.blur();
}

// A straight arrow is a line with no width, which Playwright counts as hidden; the click goes to it directly.
export const pickArrow = (page: Page, arrow: string) => page.getByLabel(arrow, { exact: true }).dispatchEvent("click");

export async function condition(page: Page, arrow: string, label: string) {
  await pickArrow(page, arrow);
  await page.getByRole("region", { name: "Arrow" }).getByLabel("Condition").fill(label);
}
