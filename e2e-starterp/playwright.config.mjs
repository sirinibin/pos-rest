import { defineConfig, devices } from "@playwright/test";

// Servers are started by run.sh (API on :2010 with the seeded test fixture,
// UI harness on :5180). Browsers must already be installed
// (PLAYWRIGHT_BROWSERS_PATH); `playwright install` is never run.
export default defineConfig({
  testDir: "tests",
  timeout: 120_000,
  expect: { timeout: 15_000 },
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    ...devices["Desktop Chrome"],
    viewport: { width: 1440, height: 900 },
    baseURL: process.env.E2E_BASE || "http://localhost:5180",
    trace: "retain-on-failure",
  },
});
