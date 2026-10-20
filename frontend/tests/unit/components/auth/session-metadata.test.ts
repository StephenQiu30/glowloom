import { expect, it } from "vitest";
import { requireSession, canonicalAuthLink } from "@/components/auth/contracts";
const legacy = {
  actor: {
    id: "11111111-1111-4111-8111-111111111111",
    email: "a@b.test",
    display_name: "a",
    platform_role: "creator",
    status: "active",
    revision: "1",
    capabilities: [],
  },
  session_id: "22222222-2222-4222-8222-222222222222",
  csrf_token: "proof",
  credential_epoch: "1",
  persistent: false,
  last_active_at: "2026-10-18T00:00:00Z",
  absolute_expires_at: "2026-10-25T00:00:00Z",
};
it("missing metadata stays v1 without upgrade, malformed partial metadata is unavailable", () => {
  expect(requireSession(legacy)).toMatchObject({
    session_protocol: "v1",
    upgrade_available: false,
  });
  expect(() => requireSession({ ...legacy, session_protocol: "v2" })).toThrow(
    "invalid_receipt",
  );
  expect(() => requireSession({ ...legacy, session_protocol: "v3" })).toThrow(
    "invalid_receipt",
  );
});
it("canonical link accepts only documented alias and safe canonical metadata", () => {
  expect(
    canonicalAuthLink(
      { canonical_origin: "http://127.0.0.1:3200" },
      "http://localhost:3200/login",
    ),
  ).toBe("http://127.0.0.1:3200/login");
  expect(
    canonicalAuthLink(
      { canonical_origin: "https://evil.test" },
      "http://localhost:3200/login",
    ),
  ).toBeNull();
  expect(
    canonicalAuthLink(
      { canonical_origin: "javascript:alert(1)" },
      "http://localhost:3200/login",
    ),
  ).toBeNull();
});
it.each(["origin_forbidden", "origin_mismatch"])(
  "%s has the trusted-site error message",
  async (code) => {
    const { authErrorMessage } = await import("@/components/auth/contracts");
    const { RequestError } = await import("@/lib/request");
    expect(authErrorMessage(new RequestError(code, 403))).toBe(
      "当前访问地址不受信任，请使用系统配置的网站地址重新打开。",
    );
  },
);

it.each([undefined, "-1", "01", "9223372036854775808", 1])(
  "rejects malformed v2 durable generation %s",
  (generation) => {
    expect(() =>
      requireSession({
        ...legacy,
        session_protocol: "v2",
        server_time: "2026-10-18T00:00:00Z",
        idle_expires_at: "2026-10-18T12:00:00Z",
        access_expires_at: "2026-10-18T00:15:00Z",
        refresh_after_at: "2026-10-18T00:14:00Z",
        upgrade_available: false,
        refresh_generation: generation,
      }),
    ).toThrow("invalid_receipt");
  },
);
