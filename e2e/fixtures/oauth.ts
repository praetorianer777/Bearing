import { createHash, randomBytes } from "node:crypto";
import { expect, type Page } from "@playwright/test";

const callback = "http://127.0.0.1:9/callback";

/**
 * Registers an app, has the person signed in on page connect it on the consent page, and redeems the code as the app
 * would: the access token is what the app then acts with.
 */
export async function connectApp(page: Page, name: string, scope = "read", onConsent?: () => Promise<void>): Promise<string> {
  const registered = await page.request.post("/api/v1/oauth/register", {
    data: { client_name: name, redirect_uris: [callback], token_endpoint_auth_method: "none" },
  });
  const app = (await registered.json()) as { client_id: string };
  const verifier = randomBytes(32).toString("base64url");
  const challenge = createHash("sha256").update(verifier).digest("base64url");
  let arrived: URL | undefined;
  await page.route(`${callback}**`, async (route) => {
    arrived = new URL(route.request().url());
    await route.fulfill({ status: 200, contentType: "text/plain", body: "back at the app" });
  });
  const query = new URLSearchParams({
    response_type: "code",
    client_id: app.client_id,
    redirect_uri: callback,
    state: "s",
    code_challenge: challenge,
    code_challenge_method: "S256",
    scope,
  });
  await page.goto(`/api/v1/oauth/authorize?${query}`);
  await expect(page.getByRole("button", { name: "Connect" })).toBeVisible();
  await onConsent?.();
  await page.getByRole("button", { name: "Connect" }).click();
  await expect.poll(() => arrived?.searchParams.get("code")).toBeTruthy();
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
  return ((await granted.json()) as { access_token: string }).access_token;
}
