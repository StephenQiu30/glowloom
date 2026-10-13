import { afterEach, expect, it, vi } from "vitest";
import { GET, POST, PUT } from "./route";
afterEach(() => vi.unstubAllGlobals());

it("项目查询只透传允许的筛选参数，详情只读且必须是UUID", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValue(new Response('{"items":[],"next_cursor":null}'));
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request(
      "http://localhost/api/projects?q=film&limit=50&org_id=other&host=https://example.com",
    ),
    context("projects"),
  );
  expect(response.status).toBe(200);
  const upstream = fetcher.mock.calls[0][0] as URL;
  expect(upstream.searchParams.get("q")).toBe("film");
  expect(upstream.searchParams.get("limit")).toBe("50");
  expect(upstream.searchParams.has("org_id")).toBe(false);
  expect(upstream.searchParams.has("host")).toBe(false);
  fetcher.mockClear();
  expect(
    (
      await GET(
        new Request("http://localhost/api/canvases/wrong"),
        context("canvases/wrong"),
      )
    ).status,
  ).toBe(404);
  const detail = "projects/00000000-0000-4000-8000-000000000001";
  expect(
    (
      await POST(
        new Request(`http://localhost/api/${detail}`, { method: "POST" }),
        context(detail),
      )
    ).status,
  ).toBe(405);
  expect(fetcher).not.toHaveBeenCalled();
});
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});
it("画布命令限定UUID和POST，透传同一请求正文与幂等键", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response('{"revision":2}'));
  vi.stubGlobal("fetch", fetcher);
  const path = "canvases/00000000-0000-4000-8000-000000000001/commands";
  const body = '{"expected_revision":1,"commands":[]}';
  const response = await POST(
    new Request(`http://localhost/api/${path}`, {
      method: "POST",
      headers: { "Idempotency-Key": "00000000-0000-4000-8000-000000000002" },
      body,
    }),
    context(path),
  );
  expect(response.status).toBe(200);
  const [url, init] = fetcher.mock.calls[0];
  expect(url.pathname).toBe(`/api/${path}`);
  expect(new TextDecoder().decode(init.body)).toBe(body);
  expect(init.headers.get("Idempotency-Key")).toBe(
    "00000000-0000-4000-8000-000000000002",
  );
  fetcher.mockClear();
  expect(
    (await GET(new Request(`http://localhost/api/${path}`), context(path)))
      .status,
  ).toBe(405);
  const invalid = "canvases/wrong/commands";
  expect(
    (
      await POST(
        new Request(`http://localhost/api/${invalid}`, { method: "POST" }),
        context(invalid),
      )
    ).status,
  ).toBe(404);
  expect(fetcher).not.toHaveBeenCalled();
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
it("个人素材查询不透传外部地址或其他身份，私有下载保留附件头", async () => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response("file", {
      headers: {
        "Content-Disposition": "attachment; filename=test.png",
        "X-Content-Type-Options": "nosniff",
      },
    }),
  );
  vi.stubGlobal("fetch", fetcher);
  const response = await GET(
    new Request(
      "http://localhost/api/media/library?scope=project&project_id=foreign&actor_id=other&search=flower",
    ),
    context("media/library"),
  );
  expect(response.status).toBe(200);
  const [url] = fetcher.mock.calls[0];
  expect(url.searchParams.get("scope")).toBe("personal");
  expect(url.searchParams.has("project_id")).toBe(false);
  expect(url.searchParams.has("actor_id")).toBe(false);
  expect(url.searchParams.get("search")).toBe("flower");
  expect(response.headers.get("Content-Disposition")).toContain("attachment");
  expect(response.headers.get("Cache-Control")).toBe("no-store");
});
it("素材上传流式透传multipart正文和原幂等键，不受JSON 1MB上限影响", async () => {
  const fetcher = vi.fn().mockImplementation(async (_url, init) => {
    const bytes = await new Response(init.body).arrayBuffer();
    expect(bytes.byteLength).toBe(1048580);
    expect(init.duplex).toBe("half");
    expect(init.headers.get("Idempotency-Key")).toBe("original-key");
    return new Response('{"asset":{}}', { status: 201 });
  });
  vi.stubGlobal("fetch", fetcher);
  const result = await POST(
    new Request("http://localhost/api/media/library/uploads", {
      method: "POST",
      headers: {
        "Content-Type": "multipart/form-data; boundary=fixture",
        "Idempotency-Key": "original-key",
      },
      body: "x".repeat(1048580),
    }),
    context("media/library/uploads"),
  );
  expect(result.status).toBe(201);
});

it("生成、主体与管理员接口保持封闭路径和查询范围", async () => {
  const fetcher = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response("{}")));
  vi.stubGlobal("fetch", fetcher);
  const pid = "11111111-1111-4111-8111-111111111111";
  await GET(
    new Request(
      `http://localhost/api/models?project_id=${pid}&capability=image.generate&org_id=other`,
    ),
    context("models"),
  );
  expect((fetcher.mock.calls[0][0] as URL).searchParams.get("capability")).toBe(
    "image.generate",
  );
  expect((fetcher.mock.calls[0][0] as URL).searchParams.has("org_id")).toBe(
    false,
  );
  const route = `admin/users/${pid}`;
  expect(
    (
      await PUT(
        new Request(`http://localhost/api/${route}`, {
          method: "PUT",
          body: '{"expected_revision":2}',
        }),
        context(route),
      )
    ).status,
  ).toBe(200);
  expect(
    (
      await POST(
        new Request(`http://localhost/api/admin/users/${pid}/delete`, {
          method: "POST",
        }),
        context(`admin/users/${pid}/delete`),
      )
    ).status,
  ).toBe(404);
});
