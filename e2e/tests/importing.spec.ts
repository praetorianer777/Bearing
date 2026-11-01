import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

// Each run imports a unit of its own under Quality, so runs and projects do not meet.
test("an editor exports the units, imports a new one from a file, and the worker makes it", async ({ page }, info) => {
  const name = `Imported ${info.project.name} ${Date.now()}`;
  await signIn(page);
  await page.goto("/people");
  await page.getByRole("link", { name: "Import or export as CSV" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Import and export" })).toBeVisible();

  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("region", { name: "Export" }).getByRole("link", { name: "Units" }).click(),
  ]);
  const units = await (await download.createReadStream()).toArray().then((chunks) => Buffer.concat(chunks).toString("utf8"));
  expect(units.split("\n")[0]).toBe("ref,name,short_name,type,parent_ref,description");
  const quality = units.split("\n").find((line) => line.split(",")[1] === "Quality");
  if (!quality) throw new Error("Quality is not in the export");
  const [qualityRef, , , type] = quality.split(",");

  const section = page.getByRole("region", { name: "Import" });
  await section
    .getByLabel("Units")
    .setInputFiles({ name: "units.csv", mimeType: "text/csv", buffer: Buffer.from(`ref,name,type,parent_ref\nnew,${name},${type},${qualityRef}\n`) });
  await section.getByRole("button", { name: "Preview" }).click();
  const preview = page.getByRole("region", { name: "Preview" });
  await expect(preview).toContainText("1 to add, 0 to change, 0 unchanged.");
  await expect(preview.getByRole("table")).toContainText(name);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);

  await preview.getByRole("button", { name: "Run the import" }).click();
  await expect(preview.getByRole("status")).toHaveText("The import is done.", { timeout: 15_000 });

  await page.goto(`/search?q=${encodeURIComponent(name)}`);
  await expect(page.getByRole("region", { name: /^Units/ }).getByRole("link", { name })).toBeVisible();
});
