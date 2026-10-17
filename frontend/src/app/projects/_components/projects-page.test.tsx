import { render, findFeedback } from "../../../../tests/feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectsPage } from "./projects-page";
const api = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  push: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: api.push }) }));
vi.mock("@/components/layout/product-shell", () => ({
  ProductShell: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/api/projects", () => ({
  createProject: api.create,
  listProjects: api.list,
}));
const project = {
  id: "00000000-0000-4000-8000-000000000001",
  canvas_id: "00000000-0000-4000-8000-000000000002",
  name: "真实项目",
  aspect_ratio: "9:16",
  style_key: null,
  revision: 1,
  update_time: "2026-10-09T00:00:00Z",
};
beforeEach(() => {
  vi.clearAllMocks();
  api.list.mockResolvedValue({ items: [], next_cursor: null });
});
afterEach(cleanup);
it("丢回执重试保持请求键与输入，只在真实回执后跳转", async () => {
  api.create
    .mockRejectedValueOnce(new Error("lost receipt"))
    .mockResolvedValueOnce(project);
  render(<ProjectsPage />);
  fireEvent.click(screen.getByRole("button", { name: "新建画布" }));
  fireEvent.change(screen.getByLabelText("画布名称"), {
    target: { value: "真实项目" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建画布" }));
  await findFeedback();
  expect(api.push).not.toHaveBeenCalled();
  expect((screen.getByLabelText("画布名称") as HTMLInputElement).value).toBe(
    "真实项目",
  );
  fireEvent.click(screen.getByRole("button", { name: "创建画布" }));
  await waitFor(() =>
    expect(api.push).toHaveBeenCalledWith(
      `/canvas?project_id=${project.id}&canvas_id=${project.canvas_id}`,
    ),
  );
  expect(api.create.mock.calls[0][1].headers["Idempotency-Key"]).toBe(
    api.create.mock.calls[1][1].headers["Idempotency-Key"],
  );
});
it("创建不重入，残缺回执不会回退到固定项目", async () => {
  let finish!: (value: unknown) => void;
  api.create.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  render(<ProjectsPage />);
  fireEvent.click(screen.getByRole("button", { name: "新建画布" }));
  fireEvent.change(screen.getByLabelText("画布名称"), {
    target: { value: "真实项目" },
  });
  const button = screen.getByRole("button", { name: "创建画布" });
  fireEvent.click(button);
  fireEvent.click(button);
  expect(api.create).toHaveBeenCalledTimes(1);
  finish({ ...project, canvas_id: null });
  await findFeedback();
  expect(api.push).not.toHaveBeenCalled();
});
it("列表加载失败不显示示例项目，名称搜索传到服务端", async () => {
  api.list
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue({ items: [project], next_cursor: null });
  render(<ProjectsPage />);
  await findFeedback();
  expect(screen.queryByText("雾港来信")).toBeNull();
  fireEvent.change(screen.getByLabelText("搜索画布"), {
    target: { value: "真实" },
  });
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await screen.findByText("真实项目");
  expect(api.list.mock.calls.at(-1)?.[0]).toEqual({ q: "真实", limit: 50 });
  expect(
    screen.getByRole("link", { name: /打开画布/ }).getAttribute("href"),
  ).toBe(`/canvas?project_id=${project.id}&canvas_id=${project.canvas_id}`);
});
