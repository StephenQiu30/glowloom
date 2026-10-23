import { afterEach, expect, it, vi } from "vitest";
import config from "../../next.config";
afterEach(() => vi.unstubAllEnvs());
it("routes Upgrade before the fetch catch-all to the build-owned backend", async () => {
  vi.stubEnv("GLOWLOOM_BACKEND_ORIGIN", "http://backend:18080");
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
