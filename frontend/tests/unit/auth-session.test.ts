import { afterEach, describe, expect, it } from "vitest";

import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from "@/lib/auth-session";

describe("JWT browser session", () => {
  afterEach(() => {
    sessionStorage.clear();
  });

  it("keeps the access token only in the current browser tab", () => {
    expect(getAccessToken()).toBeNull();

    setAccessToken("signed.jwt.token");

    expect(getAccessToken()).toBe("signed.jwt.token");
    expect(localStorage.length).toBe(0);
  });

  it("clears a revoked access token", () => {
    setAccessToken("signed.jwt.token");

    clearAccessToken();

    expect(getAccessToken()).toBeNull();
  });
});
