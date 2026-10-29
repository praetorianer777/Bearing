import { API_BASE } from "@/config";

/** What the API says about itself at /meta. */
export type Meta = {
  product: string;
  build: { version: string; commit?: string; builtAt?: string };
  apiVersion: string;
  capabilities: string[];
};

/** The error envelope every failing endpoint answers with. */
export type ApiErrorBody = { error: { code: string; message: string; fields?: Record<string, string>; requestId?: string } };

/** A failed call, carrying the API's own code and sentence when it gave them. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

/** Reads path under the API's base as JSON, turning the error envelope into an ApiError. */
export async function getJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(API_BASE + path, { credentials: "same-origin", ...init, headers: { Accept: "application/json", ...init?.headers } });
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ApiErrorBody | null;
    throw new ApiError(response.status, body?.error.code ?? "unknown", body?.error.message ?? response.statusText);
  }
  return (await response.json()) as T;
}

export const fetchMeta = (): Promise<Meta> => getJSON<Meta>("/meta");
