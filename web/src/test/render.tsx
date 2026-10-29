import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { vi } from "vitest";
import { buildRouter } from "@/routes";

/** Renders the whole app at path, with a query client of its own that never retries. */
export function renderApp(path = "/") {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = buildRouter(queryClient, createMemoryHistory({ initialEntries: [path] }));
  return {
    router,
    ...render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    ),
  };
}

/** An answer: a JSON body with a status, or a function of the request's body. */
type Answer = { status?: number; body?: unknown } | ((body: unknown) => { status?: number; body?: unknown });

export const ADMIN = {
  id: "0190a000-0000-7000-8000-000000000001",
  email: "ada@bearing.test",
  name: "Ada Admin",
  role: "administrator",
  disabled: false,
  hasPassword: true,
  createdAt: "2026-10-01T09:00:00Z",
} as const;

export const META = { product: "bearing", build: { version: "0.1.0+dev" }, apiVersion: "1", capabilities: [] };

/** A set-up installation with Ada signed in, unless answers say otherwise. */
export function signedInAs(account: object = ADMIN): Record<string, Answer> {
  return {
    "GET /meta": { body: META },
    "GET /installation": { body: { name: "Demo organisation", setUp: true, mail: true } },
    "GET /auth/me": { body: { account } },
  };
}

export const nobodySignedIn: Record<string, Answer> = {
  ...signedInAs(),
  "GET /auth/me": { status: 401, body: { error: { code: "unauthorized", message: "Sign in to continue." } } },
};

/** Stands in for the API: each "METHOD /path" under /api/v1 answers as given; anything else is a 404. */
export function fakeAPI(answers: Record<string, Answer>) {
  return vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const request = input instanceof Request ? input : new Request(String(input));
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    const text = await request.text();
    const answer = answers[`${request.method} ${path}`];
    const { status = 200, body } =
      typeof answer === "function"
        ? answer(text ? JSON.parse(text) : undefined)
        : (answer ?? { status: 404, body: { error: { code: "not_found", message: "Nothing here." } } });
    if (body === undefined) return new Response(null, { status });
    return Response.json(body, { status });
  });
}
