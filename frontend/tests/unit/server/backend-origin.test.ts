import { describe, expect, it } from "vitest";

import { getBackendOrigin } from "@/server/backend-origin";

describe("getBackendOrigin", () => {
  it("uses the configured backend service origin", () => {
    expect(
      getBackendOrigin({ GLOWLOOM_BACKEND_ORIGIN: "http://backend:18080" }),
    ).toBe("http://backend:18080");
  });

  it.each([
    "",
    "not-an-origin",
    "ftp://backend:18080",
    "http://user:password@backend:18080",
    "http://backend:18080/api",
    "http://backend:18080?debug=true",
  ])("rejects invalid or missing backend origins: %s", (value) => {
    expect(() =>
      getBackendOrigin({ GLOWLOOM_BACKEND_ORIGIN: value }),
    ).toThrow();
  });

  it("does not revive a legacy origin when the canonical value is empty", () => {
    expect(() =>
      getBackendOrigin({
        GLOWLOOM_BACKEND_ORIGIN: "",
        LV_BACKEND_ORIGIN: "http://backend:18080",
      }),
    ).toThrow("GLOWLOOM_BACKEND_ORIGIN is required");
  });
});
