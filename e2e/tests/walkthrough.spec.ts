import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";
import { expect, type Page, test } from "@playwright/test";
import { arrange, connect, node, pickArrow, rename } from "../fixtures/canvas";
import { connectApp } from "../fixtures/oauth";
import { ADMIN } from "../fixtures/stack";

// The proof of concept end to end, as docs/poc.md walks through it. With POC_SCREENSHOTS set, as make poc-screenshots
// sets it, each step leaves the picture the document shows.
async function shot(page: Page, name: string) {
  if (!process.env.POC_SCREENSHOTS) return;
  await page.waitForLoadState("networkidle");
  await page.screenshot({ path: `../docs/poc/${name}.png` });
}

test("the proof of concept, from signing in to an assistant asking who is responsible @desktop", async ({ page, baseURL }) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1440, height: 900 });

  await page.goto("/sign-in");
  await page.getByLabel("Email").fill(ADMIN.email);
  await page.getByLabel("Password").fill(ADMIN.password);
  await shot(page, "01-sign-in");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Overview" })).toBeVisible();
  await shot(page, "02-overview");

  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Organisation" }).click();
  const chart = page.getByLabel("Organisation", { exact: true });
  await expect(chart.getByRole("article", { name: "Head of Finance and People" })).toBeVisible();
  await page.getByLabel("Focus on", { exact: true }).selectOption({ label: "Jonas Weber" });
  await expect(chart.getByRole("article", { name: "Backend lead" })).toBeVisible();
  await shot(page, "03-organisation");

  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "People" }).click();
  await page.getByRole("link", { name: "Jonas Weber" }).first().click();
  await expect(page.getByRole("heading", { level: 1, name: "Jonas Weber" })).toBeVisible();
  await shot(page, "04-person");

  // The process the walkthrough draws goes first, should an earlier run have left it, so its name can stay plain.
  const name = "Expense claim";
  const listed = (await (await page.request.get("/api/v1/processes")).json()) as { processes: { id: string; name: string }[] };
  for (const p of listed.processes.filter((each) => each.name === name)) await page.request.delete(`/api/v1/processes/${p.id}`);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Processes" }).click();
  const form = page.getByRole("region", { name: "Add a process" });
  await form.getByLabel("Name").fill(name);
  await form.getByLabel("Area").fill("Finance");
  await form.getByRole("button", { name: "Add" }).click();
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
  await pickArrow(page, "Arrow from Start to End");
  await page.getByRole("button", { name: "Remove the arrow" }).click();
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await rename(page, "Step: New step", "Check the receipts");
  const responsibilities = page.getByRole("region", { name: "Node" }).getByRole("region", { name: "Responsibilities" });
  await responsibilities.getByLabel("How they stand to it").selectOption({ label: "Accountable" });
  await responsibilities.getByLabel("Who").selectOption({ label: "Head of Finance and People" });
  await responsibilities.getByRole("button", { name: "Add" }).click();
  await responsibilities.getByLabel("How they stand to it").selectOption({ label: "Responsible" });
  await responsibilities.getByLabel("Who").selectOption({ label: "Accountant" });
  await responsibilities.getByRole("button", { name: "Add" }).click();
  await arrange(page);
  await connect(page, "Start: Start", "Step: Check the receipts");
  await connect(page, "Step: Check the receipts", "End: End");
  await arrange(page);
  await page.getByRole("button", { name: "Save draft" }).click();
  await expect(page.getByText("Nothing keeps the draft from being published.")).toBeVisible();
  await node(page, "Step: Check the receipts").click();
  await shot(page, "05-drawing");

  await page.goto("/processes");
  await page.getByRole("link", { name: "Supplier invoice" }).click();
  await page.getByRole("button", { name: "Swimlanes" }).click();
  await expect(page.getByRole("region", { name: "Lane: Invoice approver" })).toBeVisible();
  await shot(page, "06-swimlanes");
  await page.getByRole("button", { name: "RACI matrix" }).click();
  await expect(page.getByRole("table")).toBeVisible();
  await shot(page, "07-raci");

  await page.goto("/who-does");
  await page.getByLabel("Step or process").fill("approve the invoice");
  await expect(page.getByRole("heading", { name: "Approve the invoice" }).first()).toBeVisible();
  await shot(page, "08-who-does");

  const token = await connectApp(page, "Assistant", "read", async () => shot(page, "09-consent"));
  await page.goto("/settings/connected-apps");
  await expect(page.getByRole("list", { name: "Yours" }).getByText("Assistant").first()).toBeVisible();
  await shot(page, "10-connected-apps");

  const client = new Client({ name: "walkthrough", version: "1.0.0" });
  await client.connect(new StreamableHTTPClientTransport(new URL("/api/v1/mcp", baseURL), { requestInit: { headers: { Authorization: `Bearer ${token}` } } }));
  try {
    const answer = await client.callTool({ name: "who_is_responsible", arguments: { query: "approve the invoice" } });
    const [step] = (answer.structuredContent as { steps: { name: string; responsibilities: { kind: string; people: { name: string }[] }[] }[] }).steps;
    expect(step?.name).toBe("Approve the invoice");
    expect(step?.responsibilities.find((r) => r.kind === "accountable")?.people.length).toBeGreaterThan(0);
  } finally {
    await client.close();
  }
});
