import createClient from "openapi-fetch";
import { API_BASE } from "@/config";
import type { components, paths } from "./schema";

// The one place that talks to the API, typed from the server's own OpenAPI document: a path, a body or a response
// that does not exist fails to compile.

export type Meta = components["schemas"]["MetaResponse"];
type ErrorEnvelope = components["schemas"]["ErrorEnvelope"];

/** A failed call, carrying the API's own code and sentence when it gave them. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: Record<string, string>;
  readonly requestId?: string;

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.fields = fields;
    this.requestId = requestId;
  }
}

// Absolute, from the page's origin: a Request built from a relative address is refused outside a browser.
// fetch is looked up at each call rather than kept, so whatever stands in for it later is the one used.
export const api = createClient<paths>({
  baseUrl: `${globalThis.location?.origin ?? ""}${API_BASE}`,
  credentials: "same-origin",
  fetch: (request) => globalThis.fetch(request),
});

function isEnvelope(value: unknown): value is ErrorEnvelope {
  return typeof value === "object" && value !== null && "error" in value && typeof (value as ErrorEnvelope).error?.code === "string";
}

/** The data of a call that succeeded; a failure throws an ApiError, in the API's words when it gave them. */
export function unwrap<T>({ data, error, response }: { data?: T; error?: unknown; response: Response }): T {
  if (response.ok && error === undefined) return data as T;
  if (isEnvelope(error)) {
    const { code, message, fields, requestId } = error.error;
    throw new ApiError(response.status, code, message, fields ?? {}, requestId);
  }
  throw new ApiError(response.status, "unknown", response.statusText);
}

export const fetchMeta = async (): Promise<Meta> => unwrap(await api.GET("/meta"));
