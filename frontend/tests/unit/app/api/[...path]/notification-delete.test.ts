import { afterEach, expect, it, vi } from "vitest";
import { GET, POST } from "@/app/api/[...path]/route";

afterEach(() => vi.unstubAllGlobals());

const path =
  "platform/notifications/11111111-1111-4111-8111-111111111111/delete";
const context = { params: Promise.resolve({ path: path.split("/") }) };

it("公告草稿删除透传请求体、会话与并发命令头，保留后端拒绝结果", async () => {
  const body = JSON.stringify({
    command_id: "22222222-2222-4222-8222-222222222222",
    expected_version: 3,
  });
  const fetcher = vi.fn().mockResolvedValue(
    new Response('{"error":"version_conflict"}', {
      status: 409,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetcher);
  const response = await POST(
    new Request(`http://localhost/api/${path}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Cookie: "session=fixture",
        Origin: "http://localhost",
        "X-CSRF-Token": "csrf-proof",
        "Idempotency-Key": "frozen-delete-command",
      },
      body,
    }),
    context,
  );
  expect(response.status).toBe(409);
  expect(await response.json()).toEqual({ error: "version_conflict" });
  const [url, init] = fetcher.mock.calls[0];
  expect((url as URL).pathname).toBe(`/api/${path}`);
  expect(init.method).toBe("POST");
  expect(await new Response(init.body).text()).toBe(body);
  expect(init.headers.get("Cookie")).toBe("session=fixture");
  expect(init.headers.get("Origin")).toBe("http://localhost");
  expect(init.headers.get("X-CSRF-Token")).toBe("csrf-proof");
  expect(init.headers.get("Idempotency-Key")).toBe("frozen-delete-command");
});

it("公告删除不开放读取方法或额外查询参数", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  expect(
    (await GET(new Request(`http://localhost/api/${path}`), context)).status,
  ).toBe(405);
  expect(
    (
      await POST(
        new Request(`http://localhost/api/${path}?actor_id=foreign`, {
          method: "POST",
          body: "{}",
        }),
        context,
      )
    ).status,
  ).toBe(422);
  expect(fetcher).not.toHaveBeenCalled();
});
