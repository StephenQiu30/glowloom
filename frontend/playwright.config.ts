import { defineConfig, devices } from "@playwright/test";

const runDeepSeekE2E = process.env.LANVERSE_RUN_DEEPSEEK_E2E === "1";
const deepSeekApiKey = runDeepSeekE2E
  ? (process.env.DEEPSEEK_API_KEY ?? "")
  : "";

if (runDeepSeekE2E && !deepSeekApiKey) {
  throw new Error(
    "DEEPSEEK_API_KEY is required when LANVERSE_RUN_DEEPSEEK_E2E=1",
  );
}

export default defineConfig({
  testDir: "./tests/e2e",
  testIgnore: runDeepSeekE2E ? [] : ["**/s2-deepseek-provider.spec.ts"],
  testMatch: runDeepSeekE2E
    ? ["**/s2-deepseek-provider.spec.ts"]
    : ["**/*.spec.ts"],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:3000",
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command:
        "cd ../backend && .venv/bin/python -m app.initialize_database && .venv/bin/python -m app.server",
      env: {
        API_HOST: "127.0.0.1",
        API_PORT: "8001",
        DATABASE_URL: "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
        DEEPSEEK_API_KEY: deepSeekApiKey,
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
        MINIO_BUCKET: "lanverse-e2e",
        RABBITMQ_URL:
          "amqp://guest:guest@127.0.0.1:5672/lanverse_contract",
      },
      url: "http://127.0.0.1:8001/readyz",
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command:
        "npm run build && mkdir -p .next/standalone/.next && cp -R .next/static .next/standalone/.next/static && cp -R public .next/standalone/public && HOSTNAME=127.0.0.1 PORT=3000 node .next/standalone/server.js",
      env: {
        NEXT_PUBLIC_API_BASE_URL: "http://127.0.0.1:8001",
      },
      url: "http://127.0.0.1:3000",
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
