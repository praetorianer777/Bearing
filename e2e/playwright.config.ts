import { defineConfig, devices } from "@playwright/test";
import { WEB_URL } from "./fixtures/stack";

const MOBILE_VIEWPORT = { width: 360, height: 740 };
const REPORTS_DIR = "../reports";

export default defineConfig({
  testDir: "./tests",
  outputDir: "./test-results",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  timeout: 20_000,
  expect: { timeout: 5_000 },
  reporter: [["list"], ["html", { outputFolder: "playwright-report", open: "never" }], ["json", { outputFile: `${REPORTS_DIR}/e2e.json` }]],
  grep: process.env.ONLY ? new RegExp(process.env.ONLY) : undefined,
  use: {
    baseURL: WEB_URL,
    // The interface follows the browser's language, and the specs read its English words.
    locale: "en-US",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "chromium", grepInvert: /@mobile/, use: { ...devices["Desktop Chrome"] } },
    { name: "mobile", grepInvert: /@desktop/, use: { ...devices["Desktop Chrome"], viewport: MOBILE_VIEWPORT, isMobile: true, hasTouch: true } },
  ],
});
