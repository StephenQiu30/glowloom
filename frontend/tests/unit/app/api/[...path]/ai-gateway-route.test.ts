import { afterEach, expect, it, vi } from "vitest";
import { GET, POST, PUT } from "@/app/api/[...path]/route";

const candidate = "11111111-1111-4111-8111-111111111111";
const configuration = "ai/admin/configuration";
const review = `ai/admin/onboarding/${candidate}`;
const verify = `${review}/verify`;
const request = (path: string, init?: RequestInit) =>
  new Request(`http://localhost/api/${path}`, init);
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});

afterEach(() => vi.unstubAllGlobals());

it("将模型接入的配置、候选读取与验证请求交给 Go 网关", async () => {
  const upstream = vi
    .fn<(url: URL, init: RequestInit) => Promise<Response>>()
    .mockImplementation(async () => new Response("{}"));
  vi.stubGlobal("fetch", upstream);
  for (const path of [configuration, review]) {
    expect((await GET(request(path), context(path))).status).toBe(200);
    expect(upstream.mock.lastCall?.[0].pathname).toBe(`/api/${path}`);
  }

  const body = '{"key":"synthetic-fixture","credits":1,"purchase_micros":2000}';
  const response = await POST(
    request(verify, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Cookie: "glowloom_access=synthetic-session",
        Origin: "http://localhost",
        "X-CSRF-Token": "synthetic-proof",
        "Idempotency-Key": candidate,
      },
      body,
    }),
    context(verify),
  );
  expect(response.status).toBe(200);
  const [url, init] = upstream.mock.lastCall!;
  expect(url.pathname).toBe(`/api/${verify}`);
  expect(await new Response(init.body).text()).toBe(body);
  const headers = new Headers(init.headers);
  expect(headers.get("Cookie")).toBe("glowloom_access=synthetic-session");
  expect(headers.get("Origin")).toBe("http://localhost");
  expect(headers.get("X-CSRF-Token")).toBe("synthetic-proof");
  expect(headers.get("Idempotency-Key")).toBe(candidate);
});

it("限制接入代理的候选 ID、方法和查询参数", async () => {
  const upstream = vi.fn();
  vi.stubGlobal("fetch", upstream);
  const unknown = verify.replace(candidate, "invalid-id");
  expect((await GET(request(unknown), context(unknown))).status).toBe(404);
  expect((await GET(request(verify), context(verify))).status).toBe(405);
  expect(
    (await GET(request(`${review}?scope_id=${candidate}`), context(review)))
      .status,
  ).toBe(422);
  expect(upstream).not.toHaveBeenCalled();
});

it("只转发售价预览和售价、采购价发布的明确方法", async () => {
  const upstream = vi
    .fn<(url: URL, init: RequestInit) => Promise<Response>>()
    .mockImplementation(async () => new Response("{}"));
  vi.stubGlobal("fetch", upstream);
  const modelPrice = `ai/admin/models/${candidate}/price`;
  const purchasePrice = `ai/admin/routes/${candidate}/price`;
  const preview = `${modelPrice}/preview`;
  for (const [path, handler, method] of [
    [preview, POST, "POST"],
    [modelPrice, PUT, "PUT"],
    [purchasePrice, PUT, "PUT"],
  ] as const) {
    const body = '{"version":0}';
    const res = await handler(
      request(path, {
        method,
        headers: { "Content-Type": "application/json" },
        body,
      }),
      context(path),
    );
    expect(res.status).toBe(200);
    const [upstreamURL, init] = upstream.mock.lastCall!;
    expect(upstreamURL.pathname).toBe(`/api/${path}`);
    expect(init.method).toBe(method);
    expect(await new Response(init.body).text()).toBe(body);
  }
  upstream.mockClear();
  expect((await GET(request(preview), context(preview))).status).toBe(405);
  expect(
    (
      await POST(
        request(purchasePrice, { method: "POST" }),
        context(purchasePrice),
      )
    ).status,
  ).toBe(405);
  expect(upstream).not.toHaveBeenCalled();
});
