import { afterEach, expect, it, vi } from "vitest";
import { GET, POST, PUT } from "@/app/api/[...path]/route";

const scope = "11111111-1111-4111-8111-111111111111";
const command = "22222222-2222-4222-8222-222222222222";
const collection = `platform/billing/scopes/${scope}/credit-commands`;
const receipt = `${collection}/${command}`;
const wallet = `platform/billing/scopes/${scope}/wallet`;
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});
afterEach(() => vi.unstubAllGlobals());

it("平台积分命令通过正式platform路径保留身份、CSRF、幂等键和原始body", async () => {
  const fetcher = vi.fn(
    async () => new Response('{"status":"accepted"}', { status: 202 }),
  );
  vi.stubGlobal("fetch", fetcher);
  const body = JSON.stringify({
    action: "grant",
    source_id: command,
    amount_credits: 200,
    expires_at: "2026-11-10T00:00:00Z",
    reason: "仅本机验收，非充值",
  });
  const response = await POST(
    new Request(`http://localhost/api/${collection}`, {
      method: "POST",
      body,
      headers: {
        "Content-Type": "application/json",
        Cookie: "glowloom_access=synthetic-session",
        Origin: "http://localhost",
        "X-CSRF-Token": "synthetic-proof",
        "X-Creation-Scope": scope,
        "Idempotency-Key": command,
      },
    }),
    context(collection),
  );
  expect(response.status).toBe(202);
  const [url, init] = fetcher.mock.calls[0] as unknown as [URL, RequestInit];
  expect(url.pathname).toBe(`/api/${collection}`);
  expect(await new Response(init.body).text()).toBe(body);
  const headers = new Headers(init.headers);
  expect(headers.get("Cookie")).toBe("glowloom_access=synthetic-session");
  expect(headers.get("Origin")).toBe("http://localhost");
  expect(headers.get("X-CSRF-Token")).toBe("synthetic-proof");
  expect(headers.get("X-Creation-Scope")).toBe(scope);
  expect(headers.get("Idempotency-Key")).toBe(command);
});

it("平台命令回执和钱包只读路由透传后端拒绝与唯一cursor", async () => {
  const fetcher = vi.fn(
    async () => new Response('{"code":"not_found"}', { status: 404 }),
  );
  vi.stubGlobal("fetch", fetcher);
  for (const path of [receipt, wallet]) {
    const search = path === wallet ? `?cursor=${command}` : "";
    const response = await GET(
      new Request(`http://localhost/api/${path}${search}`),
      context(path),
    );
    expect(response.status).toBe(404);
    const [url] = fetcher.mock.calls.at(-1)! as unknown as [URL];
    expect(url.pathname).toBe(`/api/${path}`);
    expect(url.search).toBe(search);
  }
  expect(fetcher).toHaveBeenCalledTimes(2);
});

it("平台积分白名单拒绝内部入口、伪admin别名、未声明方法和权限查询", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  for (const path of [
    collection.replace("platform/", "internal/"),
    collection.replace("platform/", "admin/"),
    collection.replace(scope, "not-a-uuid"),
  ]) {
    expect(
      (
        await POST(
          new Request(`http://localhost/api/${path}`, {
            method: "POST",
            body: "{}",
          }),
          context(path),
        )
      ).status,
    ).toBe(404);
  }
  expect(
    (
      await GET(
        new Request(`http://localhost/api/${collection}`),
        context(collection),
      )
    ).status,
  ).toBe(405);
  for (const path of [receipt, wallet]) {
    expect(
      (
        await POST(
          new Request(`http://localhost/api/${path}`, {
            method: "POST",
            body: "{}",
          }),
          context(path),
        )
      ).status,
    ).toBe(405);
    expect(
      (
        await PUT(
          new Request(`http://localhost/api/${path}`, {
            method: "PUT",
            body: "{}",
          }),
          context(path),
        )
      ).status,
    ).toBe(405);
  }
  for (const [path, query] of [
    [receipt, `cursor=${command}`],
    [wallet, `actor_id=${command}`],
    [wallet, `cursor=${command}&cursor=${scope}`],
  ]) {
    expect(
      (
        await GET(
          new Request(`http://localhost/api/${path}?${query}`),
          context(path),
        )
      ).status,
    ).toBe(422);
  }
  expect(
    (
      await POST(
        new Request(`http://localhost/api/${collection}?scope_id=${scope}`, {
          method: "POST",
          body: "{}",
        }),
        context(collection),
      )
    ).status,
  ).toBe(422);
  expect(fetcher).not.toHaveBeenCalled();
});
