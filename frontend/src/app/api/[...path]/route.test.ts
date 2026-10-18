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
  const route = `platform/users/${pid}`;
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
  ).toBe(405);
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

it.each(["GET", "POST"])(
  "项目画布集合支持 %s 且拒绝无效项目标识",
  async (method) => {
    const fetcher = vi
      .fn()
      .mockImplementation(() => Promise.resolve(new Response("{}")));
    vi.stubGlobal("fetch", fetcher);
    const path = "projects/00000000-0000-4000-8000-000000000001/canvases";
    const handler = method === "GET" ? GET : POST;
    const response = await handler(
      new Request(`http://localhost/api/${path}`, { method }),
      context(path),
    );
    expect(response.status).toBe(200);
    expect(fetcher.mock.calls[0][0].pathname).toBe(`/api/${path}`);
    expect(fetcher.mock.calls[0][1].method).toBe(method);
    fetcher.mockClear();
    const invalid = "projects/wrong/canvases";
    expect(
      (
        await handler(
          new Request(`http://localhost/api/${invalid}`, { method }),
          context(invalid),
        )
      ).status,
    ).toBe(404);
    expect(fetcher).not.toHaveBeenCalled();
  },
);

it("邮箱策略、本人 PATCH 与授权头实际经过 BFF，未知查询拒绝", async () => {
  const fetcher = vi
    .fn()
    .mockImplementation(
      async () =>
        new Response("{}", { headers: { "Content-Type": "application/json" } }),
    );
  vi.stubGlobal("fetch", fetcher);
  const policy = await GET(
    new Request("http://localhost/api/auth-policy"),
    context("auth-policy"),
  );
  expect(policy.status).toBe(200);
  const { PATCH } = await import("./route");
  const result = await PATCH(
    new Request("http://localhost/api/me/preferences", {
      method: "PATCH",
      headers: {
        Origin: "http://localhost",
        "X-CSRF-Token": "bound-proof",
        "X-Creation-Scope": "11111111-1111-4111-8111-111111111111",
      },
      body: '{"theme":"dark"}',
    }),
    context("me/preferences"),
  );
  expect(result.status).toBe(200);
  expect(fetcher.mock.calls.at(-1)![1].headers.get("X-CSRF-Token")).toBe(
    "bound-proof",
  );
  expect(fetcher.mock.calls.at(-1)![1].headers.get("X-Creation-Scope")).toBe(
    "11111111-1111-4111-8111-111111111111",
  );
  const invalid = await GET(
    new Request("http://localhost/api/platform/users?host=other"),
    context("platform/users"),
  );
  expect(invalid.status).toBe(422);
});

it("all current registered identity handler contracts cross the bounded BFF", async () => {
  const { readFile } = await import("node:fs/promises");
  const handlers = await import("./route");
  const sources = await Promise.all(
    [
      "identity/adapter/http/auth_policy.go",
      "identity/adapter/http/email_auth.go",
      "identity/adapter/http/self_profile.go",
      "identity/adapter/http/session_directory.go",
      "identity/adapter/http/account_deletion.go",
      "identity/adapter/http/email_change.go",
      "identity/adapter/http/directory_export.go",
      "identity/adapter/http/governance_directory.go",
      "identity/adapter/http/platform_password_reset.go",
      "identity/adapter/http/platform_team_invitation.go",
      "workspace/adapter/http/platform_team.go",
      "workspace/adapter/http/team_avatar.go",
      "media/adapter/http/avatar.go",
      "identity/adapter/http/event_query.go",
      "identity/adapter/http/platform_users.go",
      "identity/adapter/http/governance.go",
      "identity/adapter/http/invitation.go",
      "workspace/adapter/http/team.go",
      "workspace/adapter/http/team_lifecycle.go",
    ].map((file) => readFile(`../backend/internal/${file}`, "utf8")),
  );
  const fetcher = vi.fn().mockImplementation(async () => new Response("{}"));
  vi.stubGlobal("fetch", fetcher);
  for (const source of sources)
    for (const match of source.matchAll(
      /@Router (\/api\/\S+) \[(get|post|patch|put|delete)\]/g,
    )) {
      const path = match[1]
        .slice(5)
        .replace(/\{[^}]+\}/g, "11111111-1111-4111-8111-111111111111");
      const method = match[2].toUpperCase() as
        "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
      const response = await handlers[method](
        new Request(`http://localhost/api/${path}`, { method }),
        context(path),
      );
      expect(response.status, `${method} ${path}`).toBe(200);
    }
});
it.each([
  "accounts/register",
  "accounts/availability",
  "admin/users",
  "admin/users/11111111-1111-4111-8111-111111111111",
  "teams/wrong/members",
  "platform/users/11111111-1111-4111-8111-111111111111/delete",
])("retired or unknown route %s is never forwarded", async (path) => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  const response = await POST(
    new Request(`http://localhost/api/${path}`, { method: "POST" }),
    context(path),
  );
  expect(response.status).toBe(404);
  expect(fetcher).not.toHaveBeenCalled();
});
it("identity query filters preserve exact values and reject duplicates", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response("{}"));
  vi.stubGlobal("fetch", fetcher);
  const path = "platform/users";
  await GET(
    new Request(
      `http://localhost/api/${path}?search=alice%2Btag%40example.com&status=active&verified=true`,
    ),
    context(path),
  );
  expect(fetcher.mock.calls[0][0].searchParams.get("search")).toBe(
    "alice+tag@example.com",
  );
  fetcher.mockClear();
  const duplicate = await GET(
    new Request(`http://localhost/api/${path}?limit=1&limit=100`),
    context(path),
  );
  expect(duplicate.status).toBe(422);
  expect(fetcher).not.toHaveBeenCalled();
});

it("平台团队精确筛选经过 BFF 且拒绝未知或重复参数", async () => {
  const fetcher = vi.fn().mockImplementation(async () => new Response("{}"));
  vi.stubGlobal("fetch", fetcher);
  const path = "platform/teams";
  const response = await GET(
    new Request(
      `http://localhost/api/${path}?id=11111111-1111-4111-8111-111111111111&administrator_id=11111111-1111-4111-8111-111111111111&created_from=2026-01-01T00:00:00Z&created_to=2027-01-01T00:00:00Z`,
    ),
    context(path),
  );
  expect(response.status).toBe(200);
  expect(fetcher.mock.calls[0][0].searchParams.get("created_from")).toBe(
    "2026-01-01T00:00:00Z",
  );
  expect(
    (
      await GET(
        new Request(`http://localhost/api/${path}?id=one&id=two`),
        context(path),
      )
    ).status,
  ).toBe(422);
  expect(
    (
      await GET(
        new Request(`http://localhost/api/${path}?scope_id=private`),
        context(path),
      )
    ).status,
  ).toBe(422);
});

it("统计查询不能携带分页或越范围字段，CSV下载保留授权与附件头", async () => {
  const fetcher = vi.fn().mockImplementation(
    async () =>
      new Response("id,name\n1,safe", {
        headers: {
          "Content-Type": "text/csv",
          "Content-Disposition": "attachment; filename=directory-export.csv",
        },
      }),
  );
  vi.stubGlobal("fetch", fetcher);
  const id = "11111111-1111-4111-8111-111111111111";
  const stats = `teams/${id}/member-statistics`;
  expect(
    (
      await GET(
        new Request(
          `http://localhost/api/${stats}?role=member&status=suspended&from=2026-01-01T00:00:00Z`,
        ),
        context(stats),
      )
    ).status,
  ).toBe(200);
  expect(fetcher.mock.calls[0][0].searchParams.get("role")).toBe("member");
  expect(
    (
      await GET(
        new Request(
          `http://localhost/api/${stats}?platform_role=platform_admin`,
        ),
        context(stats),
      )
    ).status,
  ).toBe(422);
  expect(
    (
      await GET(
        new Request(
          "http://localhost/api/platform/user-statistics?cursor=foreign",
        ),
        context("platform/user-statistics"),
      )
    ).status,
  ).toBe(422);
  const path = `platform/user-exports/${id}/download`;
  const response = await GET(
    new Request(`http://localhost/api/${path}`, {
      headers: { Cookie: "session=bound" },
    }),
    context(path),
  );
  expect(response.status).toBe(200);
  expect(response.headers.get("Content-Disposition")).toContain(
    "directory-export.csv",
  );
  expect(response.headers.get("Cache-Control")).toBe("no-store");
  expect(await response.text()).toContain("safe");
});

it.each(["refresh", "upgrade"])(
  "session %s preserves protocol status, cookies and Origin",
  async (action) => {
    for (const status of [200, 401, 403, 409, 429, 503]) {
      const cookies = [
        "access=a; HttpOnly; Path=/",
        "refresh=r; HttpOnly; Path=/api/sessions",
        "csrf=c; Path=/",
      ];
      const headers = new Headers({
        "Content-Type": "application/json",
        "Retry-After": "3",
      });
      cookies.forEach((cookie) => headers.append("Set-Cookie", cookie));
      const payload = JSON.stringify({
        code: "session_result",
        meta: { retryable: false },
      });
      const fetcher = vi
        .fn()
        .mockResolvedValue(new Response(payload, { status, headers }));
      vi.stubGlobal("fetch", fetcher);
      const path = `session/${action}`;
      const result = await POST(
        new Request(`http://127.0.0.1:3200/api/${path}`, {
          method: "POST",
          headers: {
            Origin: "http://localhost:3200",
            Cookie: "refresh=r",
            "X-CSRF-Token": "c",
            "Idempotency-Key": "request-1",
          },
        }),
        context(path),
      );
      expect(result.status).toBe(status);
      expect(await result.text()).toBe(payload);
      expect(result.headers.getSetCookie()).toEqual(cookies);
      expect(result.headers.get("Cache-Control")).toBe("no-store");
      expect(result.headers.get("Retry-After")).toBe("3");
      const init = fetcher.mock.calls[0][1];
      expect(init.cache).toBe("no-store");
      expect(init.redirect).toBe("manual");
      expect(init.headers.get("Origin")).toBe("http://localhost:3200");
      expect(init.headers.get("Cookie")).toBe("refresh=r");
      expect(init.headers.get("X-CSRF-Token")).toBe("c");
      expect(init.headers.get("Idempotency-Key")).toBe("request-1");
      fetcher.mockClear();
      expect(
        (
          await GET(
            new Request(`http://127.0.0.1:3200/api/${path}`),
            context(path),
          )
        ).status,
      ).toBe(405);
      expect(fetcher).not.toHaveBeenCalled();
    }
  },
);

it.each([200, 401, 403, 404])(
  "recovery draft GET preserves authority headers, status %s and no-store",
  async (status) => {
    const path =
      "projects/11111111-1111-1111-1111-111111111111/prompt-drafts/22222222-2222-2222-2222-222222222222";
    const payload = JSON.stringify({ code: "recovery_permission_result" });
    const fetcher = vi.fn().mockResolvedValue(
      new Response(payload, {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetcher);
    const headers = {
      Cookie: "access=recovery-test; refresh=recovery-test",
      Origin: "http://127.0.0.1:3200",
      "X-CSRF-Token": "recovery-test",
      "X-Creation-Scope": "personal",
    };
    const result = await GET(
      new Request(`http://127.0.0.1:3200/api/${path}`, { headers }),
      context(path),
    );
    expect(result.status).toBe(status);
    expect(await result.text()).toBe(payload);
    expect(result.headers.get("Cache-Control")).toBe("no-store");
    expect(fetcher).toHaveBeenCalledTimes(1);
    const [url, init] = fetcher.mock.calls[0];
    expect(url.pathname).toBe(`/api/${path}`);
    expect(url.search).toBe("");
    expect(init.method).toBe("GET");
    expect(init.cache).toBe("no-store");
    expect(init.redirect).toBe("manual");
    for (const [name, value] of Object.entries(headers))
      expect(init.headers.get(name)).toBe(value);
  },
);

it.each(["POST", "DELETE"])(
  "recovery draft detail rejects %s without forwarding",
  async (method) => {
    const path =
      "projects/11111111-1111-1111-1111-111111111111/prompt-drafts/22222222-2222-2222-2222-222222222222";
    const fetcher = vi.fn();
    vi.stubGlobal("fetch", fetcher);
    const handlers = await import("./route");
    const handler = method === "POST" ? handlers.POST : handlers.DELETE;
    const result = await handler(
      new Request(`http://127.0.0.1:3200/api/${path}`, { method }),
      context(path),
    );
    expect(result.status).toBe(405);
    expect(result.headers.get("Cache-Control")).toBe("no-store");
    expect(fetcher).not.toHaveBeenCalled();
  },
);

it("recovery draft GET rejects unknown query without forwarding", async () => {
  const path =
    "projects/11111111-1111-1111-1111-111111111111/prompt-drafts/22222222-2222-2222-2222-222222222222";
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  const result = await GET(
    new Request(`http://127.0.0.1:3200/api/${path}?cursor=forbidden`),
    context(path),
  );
  expect(result.status).toBe(422);
  expect(result.headers.get("Cache-Control")).toBe("no-store");
  expect(fetcher).not.toHaveBeenCalled();
});

it.each([
  "projects/not-a-uuid/prompt-drafts/22222222-2222-2222-2222-222222222222",
  "projects/11111111-1111-1111-1111-111111111111/prompt-drafts/not-a-uuid",
  "projects/11111111-1111-1111-1111-111111111111/prompt-drafts/22222222-2222-2222-2222-222222222222/extra",
])("recovery draft GET rejects unbounded path %s", async (path) => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  const result = await GET(
    new Request(`http://127.0.0.1:3200/api/${path}`),
    context(path),
  );
  expect(result.status).toBe(404);
  expect(result.headers.get("Cache-Control")).toBe("no-store");
  expect(fetcher).not.toHaveBeenCalled();
});

it("recovery-only candidate rejects draft PUT without forwarding", async () => {
  const path =
    "projects/11111111-1111-1111-1111-111111111111/prompt-drafts/22222222-2222-2222-2222-222222222222";
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  const result = await PUT(
    new Request(`http://127.0.0.1:3200/api/${path}`, { method: "PUT" }),
    context(path),
  );
  expect(result.status).toBe(405);
  expect(result.headers.get("Allow")).toBe("GET");
  expect(result.headers.get("Cache-Control")).toBe("no-store");
  expect(fetcher).not.toHaveBeenCalled();
});
