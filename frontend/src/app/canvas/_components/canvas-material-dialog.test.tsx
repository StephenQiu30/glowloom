import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasMaterialDialog } from "./canvas-material-dialog";
const api = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/components/creation/generated/creation", () => ({
  listMediaAssets: api.list,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
vi.mock("@/components/media/upload-dialog", () => ({
  UploadDialog: () => null,
}));
vi.mock("@/components/subjects/subject-list", () => ({
  SubjectList: () => null,
}));
const pid = "00000000-0000-4000-8000-000000000001";
const a = {
  id: "00000000-0000-4000-8000-000000000011",
  project_id: pid,
  kind: "image",
  file_name: "海边香氛.png",
};
const b = {
  ...a,
  id: "00000000-0000-4000-8000-000000000012",
  file_name: "蓝蝶.png",
};
beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);
it("多选、搜索与分页保留已选资源身份，确认前不写入画布", async () => {
  api.list
    .mockResolvedValueOnce({ items: [a], next_cursor: "next" })
    .mockResolvedValueOnce({ items: [b] });
  const add = vi.fn();
  render(
    <CanvasMaterialDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={add}
      onSubject={vi.fn()}
    />,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 海边香氛.png" }),
  );
  expect(add).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "选择素材 蓝蝶.png" }),
  );
  fireEvent.change(screen.getByLabelText("搜索已加载素材"), {
    target: { value: "蓝蝶" },
  });
  expect(
    screen.queryByRole("button", { name: "选择素材 海边香氛.png" }),
  ).toBeNull();
  expect(screen.getByText("已选择 2 个素材（最多 24 个）")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "添加到画布" }));
  expect(add).toHaveBeenCalledExactlyOnceWith([a, b]);
  expect(api.list.mock.calls[1][0]).toEqual({
    pid,
    kind: "image",
    limit: 40,
    cursor: "next",
  });
});
it("跨项目回执不进入可选素材，重新读取可恢复", async () => {
  api.list
    .mockResolvedValueOnce({ items: [{ ...a, project_id: a.id }] })
    .mockResolvedValueOnce({ items: [a] });
  render(
    <CanvasMaterialDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={vi.fn()}
      onSubject={vi.fn()}
    />,
  );
  await screen.findByRole("alert");
  expect(
    screen.queryByRole("button", { name: "选择素材 海边香氛.png" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
  await screen.findByRole("button", { name: "选择素材 海边香氛.png" });
});
it("关闭弹窗中止读取，迟到回执不能继续更新弹窗", async () => {
  let finish!: (v: unknown) => void;
  api.list.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const view = render(
    <CanvasMaterialDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={vi.fn()}
      onSubject={vi.fn()}
    />,
  );
  await waitFor(() => expect(api.list).toHaveBeenCalledOnce());
  const signal = api.list.mock.calls[0][1].signal as AbortSignal;
  view.unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => finish({ items: [a] }));
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("真实空列表和已加载素材搜索无结果具有不同提示", async () => {
  api.list.mockResolvedValue({ items: [a] });
  render(
    <CanvasMaterialDialog
      projectID={pid}
      onClose={vi.fn()}
      onAdd={vi.fn()}
      onSubject={vi.fn()}
    />,
  );
  await screen.findByRole("button", { name: "选择素材 海边香氛.png" });
  fireEvent.change(screen.getByLabelText("搜索已加载素材"), {
    target: { value: "不存在" },
  });
  expect(screen.getByText("没有匹配的素材")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "添加到画布" }).hasAttribute("disabled"),
  ).toBe(true);
});
