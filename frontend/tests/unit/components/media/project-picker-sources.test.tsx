import { render } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  ProjectPickerCanvases,
  requirePickerCanvases,
} from "@/components/media/project-picker-sources";
const list = vi.hoisted(() => vi.fn());
vi.mock("@/api/canvases", () => ({ listAssetCanvases: list }));
const id = "11111111-1111-4111-8111-111111111111";
const other = "22222222-2222-4222-8222-222222222222";
const canvas = {
  id,
  project_id: other,
  kind: "canvas",
  collection: "current",
  name: "森林",
  favorite: false,
  revision: "2",
};
beforeEach(() => {
  list.mockReset();
});
afterEach(cleanup);
it("画布来源查询完整服务端收藏/搜索分页，仅浏览不伪造添加", async () => {
  list.mockImplementation(({ cursor, search, favorite_only }) =>
    Promise.resolve({
      items:
        search || favorite_only
          ? []
          : cursor
            ? [{ ...canvas, id: other, name: "海边" }]
            : [canvas],
      next_cursor: search || favorite_only || cursor ? null : "next",
    }),
  );
  render(<ProjectPickerCanvases />);
  expect(await screen.findByText("森林")).toBeTruthy();
  expect(screen.getByText(/整张画布添加尚未接入/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "加载更多画布" }));
  expect(await screen.findByText("海边")).toBeTruthy();
  expect(list.mock.lastCall![0]).toMatchObject({
    kind: "canvas",
    collection: "current",
    limit: 30,
    cursor: "next",
  });
  fireEvent.click(screen.getByRole("checkbox", { name: "仅收藏" }));
  await waitFor(() =>
    expect(list.mock.lastCall![0]).toMatchObject({
      favorite_only: true,
      cursor: undefined,
    }),
  );
  expect(await screen.findByText("没有匹配的画布")).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "搜索空间画布" }), {
    target: { value: "森林" },
  });
  await waitFor(() =>
    expect(list.mock.lastCall![0]).toMatchObject({
      search: "森林",
      favorite_only: true,
    }),
  );
});
it("跨集合及重复画布身份拒绝，不伪装空列表", () => {
  for (const value of [
    { items: [{ ...canvas, collection: "legacy" }], next_cursor: null },
    { items: [canvas, canvas], next_cursor: null },
    { items: [{ ...canvas, project_id: "bad" }], next_cursor: null },
  ])
    expect(() => requirePickerCanvases(value)).toThrow();
});
