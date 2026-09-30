// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

// Loading application configuration must not read any local environment files here.
vi.mock("@next/env", () => ({ loadEnvConfig: vi.fn() }));

describe("upload route precedence", () => {
  afterEach(() => vi.unstubAllEnvs());

  it("puts other APIs after the concrete upload route and keeps default proxy limits", async () => {
    vi.stubEnv("LV_API_BASE_URL", "http://127.0.0.1:18991");
    const { default: config } = await import("../../next.config");
    expect(await config.rewrites?.()).toEqual({
      fallback: [
        {
          source: "/api/:path*",
          destination: "http://127.0.0.1:18991/api/:path*",
        },
      ],
    });
    expect(config.experimental?.proxyTimeout).toBeUndefined();
    expect(config.experimental?.proxyClientMaxBodySize).toBeUndefined();
  });
});
