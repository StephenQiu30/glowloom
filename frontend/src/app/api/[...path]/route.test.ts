import { afterEach, expect, it, vi } from "vitest";
import { GET, POST } from "./route";
afterEach(() => vi.unstubAllGlobals());
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});
it("认证代理拒绝非注册资源且不发出上游请求", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request("http://localhost/api/admin/accounts"),
    context("admin/accounts"),
  );
  expect(response.status).toBe(404);
  expect(fetcher).not.toHaveBeenCalled();
});
it("代理传递真实会话Cookie、写入来源和回执Cookie并禁止缓存", async () => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response('{"session_id":"receipt"}', {
      status: 201,
      headers: {
        "Set-Cookie": "lanverse_session=test; HttpOnly; Path=/",
        "Content-Type": "application/json",
      },
    }),
  );
  vi.stubGlobal("fetch", fetcher);
  const response = await POST(
    new Request("http://localhost/api/sessions", {
      method: "POST",
      headers: {
        Cookie: "lanverse_session=old",
        Origin: "http://localhost",
        "Idempotency-Key": "test",
      },
      body: '{"login_name":"alice"}',
    }),
    context("sessions"),
  );
  expect(response.status).toBe(201);
  expect(response.headers.get("Cache-Control")).toBe("no-store");
  expect(response.headers.get("Set-Cookie")).toContain("HttpOnly");
  const [, init] = fetcher.mock.calls[0];
  expect(init.headers.get("Cookie")).toBe("lanverse_session=old");
  expect(init.headers.get("Origin")).toBe("http://localhost");
});
it("上游失败返回明确503，超大请求不进入上游", async () => {
  const fetcher = vi.fn().mockRejectedValue(new Error("offline"));
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request("http://localhost/api/session"),
    context("session"),
  );
  expect(response.status).toBe(503);
  fetcher.mockClear();
  const tooLarge = await POST(
    new Request("http://localhost/api/sessions", {
      method: "POST",
      body: "x".repeat(1048577),
    }),
    context("sessions"),
  );
  expect(tooLarge.status).toBe(413);
  expect(fetcher).not.toHaveBeenCalled();
});
