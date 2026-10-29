import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, getJSON } from "./client";

afterEach(() => vi.restoreAllMocks());

describe("getJSON", () => {
  it("turns the error envelope into an ApiError", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ error: { code: "not_found", message: "Nothing here." } }), { status: 404 }));
    const err = await getJSON("/x").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 404, code: "not_found", message: "Nothing here." });
  });

  it("copes with a failure that is not JSON", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("<html>", { status: 502, statusText: "Bad Gateway" }));
    await expect(getJSON("/x")).rejects.toMatchObject({ status: 502, code: "unknown", message: "Bad Gateway" });
  });
});
