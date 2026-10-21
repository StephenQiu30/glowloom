import { expect, it } from "vitest";
import {
  requireProject,
  requireProjectList,
} from "@/components/projects/project";
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
it("回收摘要保留并校验删除时间、截止时间和服务端时间", () => {
  const p = {
    id: "00000000-0000-4000-8000-000000000001",
    canvas_id: null,
    name: "回收画布",
    aspect_ratio: "16:9",
    style_key: null,
    revision: 2,
    update_time: "2026-10-21T00:00:00Z",
    status: "archived",
    is_delete: true,
    delete_time: "2026-10-21T00:00:00Z",
    purge_after: "2026-11-20T00:00:00Z",
  };
  const page = requireProjectList({
    items: [p],
    next_cursor: null,
    server_time: "2026-10-22T00:00:00Z",
  });
  expect(page.items[0].purge_after).toBe(p.purge_after);
  expect(page.items[0].server_time).toBe("2026-10-22T00:00:00Z");
  expect(() => requireProject({ ...p, purge_after: null })).toThrow();
  expect(() => requireProject({ ...p, purge_after: p.delete_time })).toThrow();
  expect(() =>
    requireProjectList({
      items: [p],
      next_cursor: null,
      server_time: "invalid",
    }),
  ).toThrow();
});
