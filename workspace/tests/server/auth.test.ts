import { expect, it } from "vitest";
import {
  issueSession,
  verifySession,
  authorize,
  csrfFor,
} from "../../src/server/auth/session";
const secret = "fixture-local-session-secret-32-bytes-only";
it("rejects tampered and expired sessions while honoring capabilities", () => {
  const token = issueSession(secret, ["read"], 1000);
  expect(verifySession(token, secret, 1001)?.actor).toBe("local-owner");
  expect(verifySession(token + "x", secret, 1001)).toBeNull();
  expect(verifySession(token, secret, 1000 + 31 * 60 * 1000)).toBeNull();
  expect(() =>
    authorize(
      {
        token,
        origin: "http://127.0.0.1:1130",
        expectedOrigin: "http://127.0.0.1:1130",
        csrf: csrfFor(token, secret),
      },
      secret,
      "edit",
      1001,
      true,
    ),
  ).toThrow(/forbidden/);
});
it("requires both same origin and a session-bound CSRF token for writes", () => {
  const token = issueSession(secret, ["read", "edit"], 1000);
  const request = {
    token,
    origin: "https://evil.example",
    expectedOrigin: "http://127.0.0.1:1130",
    csrf: csrfFor(token, secret),
  };
  expect(() => authorize(request, secret, "edit", 1001, true)).toThrow(
    /origin/,
  );
  expect(() =>
    authorize(
      { ...request, origin: request.expectedOrigin, csrf: "wrong" },
      secret,
      "edit",
      1001,
      true,
    ),
  ).toThrow(/csrf/);
  expect(() =>
    authorize(
      { ...request, origin: request.expectedOrigin, csrf: "你".repeat(43) },
      secret,
      "edit",
      1001,
      true,
    ),
  ).toThrow(/csrf/);
  expect(
    authorize(
      { ...request, origin: request.expectedOrigin },
      secret,
      "edit",
      1001,
      true,
    ).actor,
  ).toBe("local-owner");
});
