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
