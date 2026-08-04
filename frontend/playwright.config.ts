import { defineConfig, devices } from "@playwright/test";

const runDeepSeekE2E = process.env.LANVERSE_RUN_DEEPSEEK_E2E === "1";
const deepSeekApiKey = runDeepSeekE2E
  ? (process.env.DEEPSEEK_API_KEY ?? "")
  : "";
const backendPort = process.env.LANVERSE_E2E_BACKEND_PORT ?? "8001";
const frontendPort = process.env.LANVERSE_E2E_FRONTEND_PORT ?? "3000";
const backendBaseUrl = `http://127.0.0.1:${backendPort}`;
const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`;

if (runDeepSeekE2E && !deepSeekApiKey) {
  throw new Error(
    "DEEPSEEK_API_KEY is required when LANVERSE_RUN_DEEPSEEK_E2E=1",
  );
}

export default defineConfig({
  testDir: "./tests/e2e",
  testIgnore: runDeepSeekE2E ? [] : ["**/deepseek-script-to-storyboard.spec.ts"],
  testMatch: runDeepSeekE2E
    ? ["**/deepseek-script-to-storyboard.spec.ts"]
    : ["**/*.spec.ts"],
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: frontendBaseUrl,
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command:
        "cd ../backend && .venv/bin/python -m app.initialize_database && .venv/bin/python -m app.server",
      env: {
        API_HOST: "127.0.0.1",
        API_PORT: backendPort,
        CORS_ORIGINS: JSON.stringify([frontendBaseUrl]),
        DATABASE_URL: "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
        DEEPSEEK_API_KEY: deepSeekApiKey,
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
        MINIO_BUCKET: "lanverse-e2e",
        RABBITMQ_URL:
          "amqp://guest:guest@127.0.0.1:5672/lanverse_contract",
      },
      url: `${backendBaseUrl}/readyz`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command:
        "npm run build && mkdir -p .next/standalone/.next && cp -R .next/static .next/standalone/.next/static && cp -R public .next/standalone/public && node .next/standalone/server.js",
      env: {
        HOSTNAME: "127.0.0.1",
        PORT: frontendPort,
        NEXT_PUBLIC_API_BASE_URL: backendBaseUrl,
      },
      url: frontendBaseUrl,
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
