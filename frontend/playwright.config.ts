import { defineConfig, devices } from "@playwright/test";

const backendPort = process.env.LANVERSE_E2E_BACKEND_PORT ?? "8687";
const agentPort = process.env.LANVERSE_E2E_AGENT_PORT ?? "8688";
const frontendPort = process.env.LANVERSE_E2E_FRONTEND_PORT ?? "8124";
const postgresPort = process.env.LANVERSE_E2E_POSTGRES_PORT ?? "15432";
const minioPort = process.env.LANVERSE_E2E_MINIO_PORT ?? "19010";
const backendBaseUrl = `http://127.0.0.1:${backendPort}`;
const agentBaseUrl = `http://127.0.0.1:${agentPort}`;
const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`;
const minioEndpoint = `127.0.0.1:${minioPort}`;
const executionSecret = "playwright-agent-execution-secret-with-32-bytes";

export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: ["**/*.spec.ts"],
  globalTeardown: "./tests/e2e/global-teardown.ts",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: frontendBaseUrl,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command: "./scripts/e2e-postgres.sh",
      env: { LANVERSE_E2E_POSTGRES_PORT: postgresPort },
      port: Number(postgresPort),
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: "./scripts/e2e-minio.sh",
      env: { LANVERSE_E2E_MINIO_PORT: minioPort },
      url: `http://${minioEndpoint}/minio/health/live`,
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command:
        "cd ../agent && uv run --all-extras python -m uvicorn app.candidate_runtime.api:app --host 127.0.0.1 --port " +
        agentPort,
      env: {
        AGENT_EXECUTION_SECRET: executionSecret,
        CODEX_BIN: process.env.CODEX_BIN ?? "codex",
      },
      url: `${agentBaseUrl}/healthz`,
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: "cd ../backend && go run ./cmd",
      env: {
        AGENT_EXECUTION_SECRET: executionSecret,
        AGENT_POLL_INTERVAL_MS: "250",
        AGENT_URL: agentBaseUrl,
        API_HOST: "127.0.0.1",
        API_PORT: backendPort,
        CORS_ORIGINS: JSON.stringify([frontendBaseUrl]),
        DATABASE_URL: `postgresql://lanverse_e2e@127.0.0.1:${postgresPort}/postgres?sslmode=disable`,
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
        MINIO_ACCESS_KEY: "lanverse-e2e",
        MINIO_BUCKET: "lanverse-e2e",
        MINIO_ENDPOINT: minioEndpoint,
        MINIO_PUBLIC_ENDPOINT: minioEndpoint,
        MINIO_PUBLIC_SECURE: "false",
        MINIO_SECRET_KEY: "lanverse-e2e-only",
        MINIO_SECURE: "false",
        REGISTRATION_VERIFICATION_CODE: "123456",
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
        NEXT_PUBLIC_API_BASE_URL: backendBaseUrl,
        PORT: frontendPort,
      },
      url: frontendBaseUrl,
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
