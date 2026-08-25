import { defineConfig, devices } from "@playwright/test";

const backendPort = process.env.LANVERSE_E2E_BACKEND_PORT ?? "8687";
const agentPort = process.env.LANVERSE_E2E_AGENT_PORT ?? "8688";
const frontendPort = process.env.LANVERSE_E2E_FRONTEND_PORT ?? "8124";
const backendBaseUrl = `http://127.0.0.1:${backendPort}`;
const agentBaseUrl = `http://127.0.0.1:${agentPort}`;
const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`;
const minioPort = process.env.LANVERSE_E2E_MINIO_PORT ?? "19010";
const minioEndpoint = `127.0.0.1:${minioPort}`;
const kafkaPort = process.env.LANVERSE_E2E_KAFKA_PORT ?? "19092";
const kafkaReadyPort = process.env.LANVERSE_E2E_KAFKA_READY_PORT ?? "19094";

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
      command: "./scripts/e2e-kafka.sh",
      env: {
        LANVERSE_E2E_KAFKA_PORT: kafkaPort,
        LANVERSE_E2E_KAFKA_READY_PORT: kafkaReadyPort,
      },
      url: `http://127.0.0.1:${kafkaReadyPort}`,
      reuseExistingServer: false,
      timeout: 90_000,
    },
    {
      command:
        `storage_dir=$(mktemp -d); trap 'rm -rf "$storage_dir"' EXIT; MINIO_ROOT_USER=lanverse-e2e MINIO_ROOT_PASSWORD=lanverse-e2e-only minio server --quiet --address 127.0.0.1:${minioPort} --console-address 127.0.0.1:19011 "$storage_dir"`,
      url: `http://${minioEndpoint}/minio/health/live`,
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command:
        "cd ../backend && go run ./cmd/migrate && cd ../agent && .venv/bin/python -m app.runtime.commands.database && .venv/bin/python -m tests.support.e2e_server",
      env: {
        API_HOST: "127.0.0.1",
        API_PORT: agentPort,
        CORS_ORIGINS: JSON.stringify([frontendBaseUrl]),
        DATABASE_URL: "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
        ENVIRONMENT: "test",
        JWT_SECRET_KEY: "playwright-only-jwt-secret-with-at-least-32-bytes",
        MINIO_BUCKET: "lanverse-e2e",
        MINIO_ENDPOINT: minioEndpoint,
        MINIO_PUBLIC_ENDPOINT: minioEndpoint,
        MINIO_PUBLIC_SECURE: "false",
        MINIO_ACCESS_KEY: "lanverse-e2e",
        MINIO_SECRET_KEY: "lanverse-e2e-only",
        MINIO_SECURE: "false",
        KAFKA_BOOTSTRAP_SERVERS:
          `127.0.0.1:${kafkaPort}`,
        EMAIL_VERIFICATION_SOURCE_LIMIT: "1000",
      },
      url: `${agentBaseUrl}/readyz`,
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: "cd ../backend && go run ./cmd/api",
      env: {
        API_HOST: "127.0.0.1",
        API_PORT: backendPort,
        LEGACY_API_URL: agentBaseUrl,
        MIGRATION_DATABASE_URL:
          "postgresql://postgres@127.0.0.1:5432/lanverse_test",
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
