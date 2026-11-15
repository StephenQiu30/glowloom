import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: 'e2e',
  workers: 1,
  timeout: 45000,
  reporter: 'line',
  use: {
    baseURL: process.env.WORKSPACE_TEST_URL ?? 'http://127.0.0.1:1130',
    viewport: { width: 1440, height: 1000 },
    screenshot: 'only-on-failure',
    launchOptions:
      process.platform === 'darwin'
        ? {
            executablePath:
              '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
          }
        : undefined,
  },
});
