import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  outputDir: process.env.LV_E2E_OUTPUT_DIR ?? "/tmp/lanverse-canvas-e2e",
  reporter: "list",
  use: {
    baseURL: process.env.LV_E2E_BASE_URL ?? "http://127.0.0.1:3140",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: process.env.LV_E2E_CHROMIUM_PATH
      ? { executablePath: process.env.LV_E2E_CHROMIUM_PATH }
      : {},
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1440, height: 1000 },
      },
    },
  ],
});
