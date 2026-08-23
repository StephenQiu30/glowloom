import { defineConfig, devices } from "@playwright/test";

const backendPort = process.env.LANVERSE_E2E_BACKEND_PORT ?? "8687";
const frontendPort = process.env.LANVERSE_E2E_FRONTEND_PORT ?? "8124";
const backendBaseUrl = `http://127.0.0.1:${backendPort}`;
const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`;

export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: ["**/*.spec.ts"],
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
        "cd ../backend && .venv/bin/python -m app.runtime.commands.database && .venv/bin/python -m tests.support.e2e_server",
      env: {
        API_HOST: "127.0.0.1",
        API_PORT: backendPort,
        CORS_ORIGINS: JSON.stringify([frontendBaseUrl]),
        DATABASE_URL: "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
        MINIO_BUCKET: "lanverse-e2e",
        KAFKA_BOOTSTRAP_SERVERS:
          process.env.KAFKA_BOOTSTRAP_SERVERS ?? "127.0.0.1:9092",
        EMAIL_VERIFICATION_SOURCE_LIMIT: "1000",
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
