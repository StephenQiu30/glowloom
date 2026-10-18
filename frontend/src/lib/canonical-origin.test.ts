import { afterEach, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { canonicalNavigation } from "./canonical-origin";
import { proxy } from "../proxy";
const canonical = "http://127.0.0.1:3200";
afterEach(() => vi.unstubAllEnvs());
it.each(["GET", "HEAD"])("redirects safe alias %s", (method) => {
  expect(
    canonicalNavigation(
      new URL("http://localhost:3200/login?return_to=%2Faccount"),
      method,
      canonical,
    )?.href,
  ).toBe(`${canonical}/login?return_to=%2Faccount`);
});
it.each(["POST", "PUT", "PATCH", "DELETE"])("never replays %s", (method) => {
  expect(
    canonicalNavigation(
      new URL("http://localhost:3200/api/sessions"),
      method,
      canonical,
    ),
  ).toBeNull();
});
it("canonical has no loop", () =>
  expect(canonicalNavigation(new URL(canonical), "GET", canonical)).toBeNull());
it.each([
  "https://evil.test",
  "//evil.test",
  "\\evil.test",
  "%2F%2Fevil.test",
  "/%5cevil.test",
  "/%0aevil",
])("rejects unsafe target %s before canonical noop", (target) => {
  const url = new URL(`${canonical}/login`);
  url.searchParams.set("return_to", target);
  expect(() => canonicalNavigation(url, "GET", canonical)).toThrow();
});
it("rejects arbitrary host", () =>
  expect(() =>
    canonicalNavigation(new URL("http://evil.test"), "GET", canonical),
  ).toThrow());
it("validates actual Docker Host and ignores forwarded host and Origin", () => {
  vi.stubEnv("LV_PUBLIC_ORIGIN", canonical);
  const result = proxy(
    new NextRequest("http://0.0.0.0:3000/account", {
      headers: {
        host: "localhost:3200",
        origin: "https://evil.test",
        "x-forwarded-host": "evil.test",
      },
    }),
  );
  expect(result.status).toBe(307);
  expect(result.headers.get("location")).toBe(`${canonical}/account`);
  expect(
    proxy(
      new NextRequest(`${canonical}/account`, {
        headers: { host: "evil.test" },
      }),
    ).status,
  ).toBe(400);
});
it("alias POST returns trusted problem metadata without copying cookies", async () => {
  vi.stubEnv("LV_PUBLIC_ORIGIN", canonical);
  const result = proxy(
    new NextRequest("http://0.0.0.0:3000/api/session/refresh", {
      method: "POST",
      headers: { host: "localhost:3200", cookie: "session=alias" },
    }),
  );
  expect(result.status).toBe(403);
  expect(result.headers.get("location")).toBeNull();
  expect(result.headers.get("set-cookie")).toBeNull();
  expect(await result.json()).toMatchObject({
    code: "origin_mismatch",
    meta: { canonical_origin: canonical },
  });
  expect(result.headers.get("cache-control")).toBe("no-store");
});
it("canonical Docker Host proceeds preserving Origin", () => {
  vi.stubEnv("LV_PUBLIC_ORIGIN", canonical);
  const request = new NextRequest("http://0.0.0.0:3000/api/session", {
    headers: { host: "127.0.0.1:3200", origin: "https://evil.test" },
  });
  expect(proxy(request).headers.get("x-middleware-next")).toBe("1");
  expect(request.headers.get("origin")).toBe("https://evil.test");
});
it("only the exact internal health probe bypasses canonical Host", () => {
  vi.stubEnv("LV_PUBLIC_ORIGIN", canonical);
  const request = (path: string, host: string, method = "GET") =>
    new NextRequest(`http://127.0.0.1:3000${path}`, {
      method,
      headers: { host },
    });
  expect(
    proxy(request("/healthz", "127.0.0.1:3000")).headers.get(
      "x-middleware-next",
    ),
  ).toBe("1");
  expect(
    proxy(request("/healthz", "127.0.0.1:3000", "HEAD")).headers.get(
      "x-middleware-next",
    ),
  ).toBe("1");
  for (const [path, host, method] of [
    ["/healthz", "evil.test", "GET"],
    ["/healthz?x=1", "127.0.0.1:3000", "GET"],
    ["/api/session", "127.0.0.1:3000", "GET"],
    ["/healthz", "127.0.0.1:3000", "POST"],
  ])
    expect(proxy(request(path, host, method)).status).toBe(400);
});
it("development default works while production configuration is required", () => {
  vi.stubEnv("LV_PUBLIC_ORIGIN", undefined);
  vi.stubEnv("NODE_ENV", "development");
  const request = new NextRequest(`${canonical}/account`, {
    headers: { host: "127.0.0.1:3200" },
  });
  expect(proxy(request).headers.get("x-middleware-next")).toBe("1");
  vi.stubEnv("NODE_ENV", "production");
  expect(proxy(request).status).toBe(503);
});
it.each([
  "localhost:3200.evil.test",
  "127.0.0.2:3200",
  "127.0.0.1:3200@evil.test",
  "localhost:3000",
  "127.0.0.1:3200,evil.test",
])("rejects malformed or untrusted Host %s", (host) => {
  vi.stubEnv("LV_PUBLIC_ORIGIN", canonical);
  const result = proxy(
    new NextRequest(`${canonical}/account`, { headers: { host } }),
  );
  expect(result.status).toBe(400);
  expect(result.headers.get("location")).toBeNull();
});
it("canonical rejects unsafe return_to through actual proxy", () => {
  vi.stubEnv("LV_PUBLIC_ORIGIN", canonical);
  expect(
    proxy(
      new NextRequest(`${canonical}/login?return_to=https%3A%2F%2Fevil.test`, {
        headers: { host: "127.0.0.1:3200" },
      }),
    ).status,
  ).toBe(400);
});
