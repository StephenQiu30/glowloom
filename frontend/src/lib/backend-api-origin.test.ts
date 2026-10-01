import { describe, expect, it } from "vitest";

import { backendApiOrigin } from "./backend-api-origin";

describe("server backend origin", () => {
  it("uses the current loopback default and normalizes valid roots", () => {
    expect(backendApiOrigin(undefined)).toBe("http://127.0.0.1:8080");
    expect(backendApiOrigin("https://EXAMPLE.com:443/")).toBe(
      "https://example.com",
    );
  });

  it.each([
    "invalid-secret-root",
    "file:///tmp/upstream",
    "http://user:private-password@example.com",
    "http://example.com/api",
    "http://example.com?token=private-password",
    "http://example.com#private-password",
    "",
  ])(
    "rejects non-root or secret-bearing configuration without echoing it",
    (value) => {
      expect(() => backendApiOrigin(value)).toThrow(
        "LV_API_BASE_URL 必须是无凭据的 HTTP(S) 服务根地址。",
      );
    },
  );
});
