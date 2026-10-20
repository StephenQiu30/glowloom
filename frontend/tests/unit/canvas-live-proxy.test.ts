import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, expect, it, vi } from "vitest";
import config from "../../next.config";
afterEach(() => vi.unstubAllEnvs());
it("routes Upgrade before the fetch catch-all to the build-owned backend", async () => {
  vi.stubEnv("LV_BACKEND_ORIGIN", "http://backend:18080");
  const rewrites = await config.rewrites!();
  expect(rewrites).toMatchObject({
    beforeFiles: [
      {
        source: "/api/canvases/:id/live",
        destination: "http://backend:18080/api/canvases/:id/live",
      },
    ],
  });
});
it("requires an explicit backend origin for the standalone Docker build", () => {
  const docker = readFileSync(
    resolve(import.meta.dirname, "../../Dockerfile"),
    "utf8",
  );
  const build = docker.split("FROM base AS build")[1].split("FROM node:")[0];
  expect(build).toMatch(/ARG LV_BACKEND_ORIGIN/);
  expect(build).toContain('test -n "$LV_BACKEND_ORIGIN"');
});

it("CI image build supplies the origin persisted in the Upgrade rewrite", () => {
  const workflow = readFileSync(
    resolve(import.meta.dirname, "../../../.github/workflows/ci.yml"),
    "utf8",
  );
  expect(workflow).toContain(
    "--build-arg LV_BACKEND_ORIGIN=http://backend-api:8080",
  );
});
