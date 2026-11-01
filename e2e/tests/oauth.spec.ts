import { createHash, randomBytes } from "node:crypto";
import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { signIn } from "../fixtures/session";

const callback = "http://127.0.0.1:9/callback";

test("an app registers itself and an administrator connects it on the consent page", async ({ page }) => {
  const registered = await page.request.post("/api/v1/oauth/register", {
    data: { client_name: `Assistant ${Date.now()}`, redirect_uris: [callback], token_endpoint_auth_method: "none" },
  });
  expect(registered.status()).toBe(201);
  const app = (await registered.json()) as { client_id: string; client_name: string };
  const verifier = randomBytes(32).toString("base64url");
  const challenge = createHash("sha256").update(verifier).digest("base64url");

  await signIn(page);
  // Nothing listens at the app's address; the browser's arrival there is the answer.
  let arrived: URL | undefined;
  await page.route(`${callback}**`, async (route) => {
    arrived = new URL(route.request().url());
    await route.fulfill({ status: 200, contentType: "text/plain", body: "back at the app" });
  });
  const query = new URLSearchParams({
    response_type: "code",
    client_id: app.client_id,
    redirect_uri: callback,
    state: "e2e",
    code_challenge: challenge,
    code_challenge_method: "S256",
    scope: "read write",
  });
  await page.goto(`/api/v1/oauth/authorize?${query}`);
  await expect(page.getByRole("heading", { level: 1, name: `Connect ${app.client_name}` })).toBeVisible();
  await expect(page.getByText(/Connecting it allows it for everybody here/)).toBeVisible();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  await page.getByRole("checkbox", { name: "Let it change things too" }).uncheck();
  await page.getByRole("button", { name: "Connect" }).click();
  await expect.poll(() => arrived?.searchParams.get("state")).toBe("e2e");

  const granted = await page.request.post("/api/v1/oauth/token", {
    form: {
      grant_type: "authorization_code",
      client_id: app.client_id,
      code: arrived?.searchParams.get("code") ?? "",
      redirect_uri: callback,
      code_verifier: verifier,
    },
  });
  expect(granted.status()).toBe(200);
  const tokens = (await granted.json()) as { access_token: string; scope: string };
  expect(tokens.access_token).toMatch(/^bearing_oat_/);
  expect(tokens.scope).toBe("read");

  await page.goto("/settings/connected-apps");
  const yours = page.getByRole("list", { name: "Yours" });
  await expect(yours.getByText(app.client_name)).toBeVisible();
  await page.getByRole("button", { name: `Disconnect ${app.client_name}` }).click();
  await expect(yours.getByText(app.client_name)).toHaveCount(0);
  await page.getByRole("button", { name: `Remove ${app.client_name}` }).click();
  await expect(page.getByRole("list", { name: "Every app" }).getByText(app.client_name)).toHaveCount(0);
});
