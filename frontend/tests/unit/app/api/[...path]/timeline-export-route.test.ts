import { afterEach, expect, it, vi } from "vitest";
import { GET, POST, PUT } from "@/app/api/[...path]/route";

const id = "11111111-1111-4111-8111-111111111111";
const context = (path: string) => ({
  params: Promise.resolve({ path: path.split("/") }),
});
afterEach(() => vi.unstubAllGlobals());

it("时间线导出创建、恢复、控制与交付经过精确同站代理合同", async () => {
  const fetcher = vi.fn().mockImplementation(async () => new Response("{}"));
  vi.stubGlobal("fetch", fetcher);
  const list = `projects/${id}/media-exports`;
  const query = `canvas_id=${id}&node_id=${id}&limit=1&cursor=next%2Bpage`;
  expect(
    (
      await GET(
        new Request(`http://localhost/api/${list}?${query}`),
        context(list),
      )
    ).status,
  ).toBe(200);
  expect((fetcher.mock.calls[0][0] as URL).searchParams.get("cursor")).toBe(
    "next+page",
  );
  for (const path of [
    list,
    ...["cancel", "retry", "review"].map(
      (action) => `media-exports/${id}/${action}`,
    ),
  ]) {
    const response = await POST(
      new Request(`http://localhost/api/${path}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": id,
          "X-Creation-Scope": id,
          "X-CSRF-Token": "synthetic-csrf",
          Cookie: "session=synthetic",
        },
        body: '{"revision":9}',
      }),
      context(path),
    );
    expect(response.status, path).toBe(200);
    const options = fetcher.mock.lastCall![1] as RequestInit;
    expect(new Headers(options.headers).get("Idempotency-Key")).toBe(id);
    expect(new Headers(options.headers).get("X-Creation-Scope")).toBe(id);
    expect(new Headers(options.headers).get("X-CSRF-Token")).toBe(
      "synthetic-csrf",
    );
    expect(new Headers(options.headers).get("Cookie")).toBe(
      "session=synthetic",
    );
    expect(options.body?.toString()).toBe('{"revision":9}');
  }
  for (const suffix of ["", "/preview", "/download", "/subtitles"]) {
    const path = `media-exports/${id}${suffix}`;
    expect(
      (
        await GET(
          new Request(`http://localhost/api/${path}?project_id=${id}`),
          context(path),
        )
      ).status,
      path,
    ).toBe(200);
    expect(
      (fetcher.mock.lastCall![0] as URL).searchParams.get("project_id"),
    ).toBe(id);
  }
});

it("导出拒绝未知字段、重复范围、越界方法及伪造子路由，二进制附件保持流和响应头", async () => {
  const fetcher = vi
    .fn()
    .mockImplementation(
      async () =>
        new Response(new Uint8Array([0, 255, 1]), {
          headers: {
            "Content-Type": "video/mp4",
            "Content-Disposition": "attachment; filename=timeline.mp4",
          },
        }),
    );
  vi.stubGlobal("fetch", fetcher);
  const path = `media-exports/${id}/download`;
  for (const query of [
    `project_id=${id}&project_id=${id}`,
    `project_id=${id}&host=other`,
    `actor_id=${id}`,
  ])
    expect(
      (
        await GET(
          new Request(`http://localhost/api/${path}?${query}`),
          context(path),
        )
      ).status,
    ).toBe(422);
  expect(
    (
      await PUT(
        new Request(`http://localhost/api/${path}`, { method: "PUT" }),
        context(path),
      )
    ).status,
  ).toBe(405);
  const unknown = `media-exports/${id}/publish`;
  expect(
    (
      await POST(
        new Request(`http://localhost/api/${unknown}`, { method: "POST" }),
        context(unknown),
      )
    ).status,
  ).toBe(404);
  expect(fetcher).not.toHaveBeenCalled();
  const response = await GET(
    new Request(`http://localhost/api/${path}?project_id=${id}`),
    context(path),
  );
  expect(response.headers.get("Content-Type")).toBe("video/mp4");
  expect(response.headers.get("Content-Disposition")).toContain("timeline.mp4");
  expect(response.headers.get("Cache-Control")).toBe("no-store");
  expect(new Uint8Array(await response.arrayBuffer())).toEqual(
    new Uint8Array([0, 255, 1]),
  );
});
