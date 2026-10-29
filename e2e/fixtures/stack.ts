import { readFileSync } from "node:fs";

// The stack's addresses, from the file make up writes; the suite runs on the host's network, as a person would.
function stackEnv(): Record<string, string> {
  const path = new URL("../../.cache/stack.env", import.meta.url);
  const entries = readFileSync(path, "utf8")
    .split("\n")
    .filter((line) => line.includes("="))
    .map((line) => line.split(/=(.*)/s, 2) as [string, string]);
  return Object.fromEntries(entries);
}

const env = stackEnv();

export const WEB_URL = env.BEARING_WEB_URL ?? "http://localhost:5173";

/** The first administrator the stack's seed sets up. */
export const ADMIN = { email: "admin@bearing.test", password: "bearing admin password" };
