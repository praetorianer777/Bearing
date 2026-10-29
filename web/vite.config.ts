// vitest/config re-exports Vite's defineConfig with the `test` block typed, which keeps one config file instead of
// two that can drift apart.
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { readFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";

// The root VERSION file is the one a release bumps, so the client reads it rather than keeping a second copy.
const version = readFileSync(fileURLToPath(new URL("../VERSION", import.meta.url)), "utf8").trim();

// The port inside the container; make web-dev publishes it on a port of the checkout's own.
const DEV_SERVER_PORT = 5173;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    __APP_VERSION__: JSON.stringify(version),
  },
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  server: {
    host: "0.0.0.0",
    port: DEV_SERVER_PORT,
    // Vite rejects requests whose Host header it does not recognise, which blocks anything reaching the dev server
    // by its compose service name.
    allowedHosts: ["localhost", "127.0.0.1", "web"],
    // One origin for the browser, so the session cookie is first-party and no CORS preflight is involved.
    proxy: {
      "/api": { target: process.env.VITE_API_PROXY || "http://localhost:8080", changeOrigin: true },
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/i18n/*.ts", "src/config.ts", "src/test/**", "src/**/*.test.{ts,tsx}", "src/**/*.d.ts", "src/main.tsx", "src/api/schema.d.ts"],
      reporter: ["text-summary", "json-summary"],
      reportsDirectory: "../reports/vitest-coverage",
    },
  },
});
