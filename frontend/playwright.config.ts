import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:3000",
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command:
        "cd ../backend && uv run --frozen --no-python-downloads python -m app.initialize_database && uv run --frozen --no-python-downloads uvicorn app.main:app --host 127.0.0.1 --port 8000",
      env: {
        DATABASE_URL: "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
      },
      url: "http://127.0.0.1:8000/healthz",
      reuseExistingServer: true,
      timeout: 60_000,
    },
    {
      command: "npm run dev",
      url: "http://127.0.0.1:3000",
      reuseExistingServer: true,
      timeout: 60_000,
    },
  ],
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
