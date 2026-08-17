import { afterEach, describe, expect, it } from "vitest";

import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from "@/lib/auth-session";

describe("JWT browser session", () => {
  afterEach(() => {
    clearAccessToken();
  });

  it("keeps the short-lived access token out of Web Storage", () => {
    expect(getAccessToken()).toBeNull();

    setAccessToken("signed.jwt.token");

    expect(getAccessToken()).toBe("signed.jwt.token");
    expect(sessionStorage.length).toBe(0);
    expect(localStorage.length).toBe(0);
  });

  it("clears a revoked access token", () => {
    setAccessToken("signed.jwt.token");

    clearAccessToken();

    expect(getAccessToken()).toBeNull();
  });
});
