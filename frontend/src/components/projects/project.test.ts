import { afterEach, expect, it, vi } from "vitest";
import { request } from "@/components/auth/request";
import { requireProject, requireProjectList } from "./project";
afterEach(() => vi.unstubAllGlobals());
it("传输保留创建重试键并按类型序列化查询", async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response("{}"));
  vi.stubGlobal("fetch", fetcher);
  await request("/api/projects", {
    method: "POST",
    data: { name: "test" },
    headers: { "Idempotency-Key": "original-key" },
    params: { limit: 50, deleted: false, cursor: undefined },
  });
  expect(fetcher.mock.calls[0][0]).toBe("/api/projects?limit=50&deleted=false");
  expect(fetcher.mock.calls[0][1].headers.get("Idempotency-Key")).toBe(
    "original-key",
  );
  expect(fetcher.mock.calls[0][1].credentials).toBe("same-origin");
});
it("空列表合法，缺失分页字段和无效画布回执不能当成功", () => {
  expect(requireProjectList({ items: [], next_cursor: null })).toEqual({
    items: [],
    next_cursor: null,
  });
  expect(() => requireProjectList({ items: [] })).toThrow();
  expect(() =>
    requireProject(
      {
        id: "00000000-0000-4000-8000-000000000001",
        canvas_id: null,
        name: "test",
        aspect_ratio: "1:1",
        style_key: null,
        revision: 1,
        update_time: "2026-10-09T00:00:00Z",
      },
      true,
    ),
  ).toThrow();
});
