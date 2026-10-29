import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, fetchMeta } from "./client";

afterEach(() => vi.restoreAllMocks());

describe("the API client", () => {
  it("answers what the API said", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({ product: "bearing", build: { version: "1.0.0" }, apiVersion: "1", capabilities: [] }));
    await expect(fetchMeta()).resolves.toMatchObject({ product: "bearing", apiVersion: "1" });
  });

  it("turns the error envelope into an ApiError", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({ error: { code: "not_found", message: "Nothing here.", requestId: "r1" } }, { status: 404 }),
    );
    const err = await fetchMeta().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 404, code: "not_found", message: "Nothing here.", requestId: "r1" });
  });

  it("copes with a failure that is not JSON", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("<html>", { status: 502, statusText: "Bad Gateway" }));
    await expect(fetchMeta()).rejects.toMatchObject({ status: 502, code: "unknown", message: "Bad Gateway" });
  });
});
