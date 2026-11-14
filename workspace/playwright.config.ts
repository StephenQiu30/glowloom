import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "tests/browser",
  workers: 1,
  timeout: 45000,
  outputDir: "test-results",
  reporter: "line",
  use: {
    baseURL: process.env.WORKSPACE_TEST_URL ?? "http://127.0.0.1:1131",
    viewport: { width: 1460, height: 1040 },
    launchOptions:
      process.platform === "darwin"
        ? {
            executablePath:
              "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
          }
        : undefined,
    screenshot: "only-on-failure",
  },
});
