import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { ProjectsWorkspace } from "./projects-workspace";

const ports = vi.hoisted(() => ({
  list: vi.fn(),
  presets: vi.fn(),
  create: vi.fn(),
  replace: vi.fn(),
  push: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: ports.replace, push: ports.push }),
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("./queries", () => ({
  PROJECTS_KEY: ["projects"],
  listProjects: ports.list,
  listStylePresets: ports.presets,
  createProject: ports.create,
}));
vi.mock("@/components/theme-toggle", () => ({ ThemeToggle: () => null }));
const project = {
  id: "d77d3c2a-7092-4e30-bf41-a3c5d4d26418",
  name: "逆光",
  aspect_ratio: "16:9",
  style_type: "realistic",
  status: "active",
  is_delete: false,
  revision: 1,
};
beforeEach(() => {
  vi.resetAllMocks();
  ports.list.mockResolvedValue({ items: [project], next_cursor: null });
  ports.presets.mockResolvedValue({ items: [], next_cursor: null });
  ports.create.mockResolvedValue({ ...project, name: "新项目" });
});
afterEach(cleanup);
function setup() {
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={cache}>
      <ProjectsWorkspace />
    </QueryClientProvider>,
  );
  return cache;
}
it("工作台直接读取项目且不请求登录会话", async () => {
  setup();
  await screen.findByText("逆光");
  expect(ports.list).toHaveBeenCalled();
  expect(ports.replace).not.toHaveBeenCalled();
});
it("服务错误明确展示且可重试，不跳转登录页", async () => {
  ports.list.mockRejectedValueOnce(new ApiError(503, "dependency_unavailable"));
  setup();
  await screen.findByText("项目列表未能加载");
  expect(ports.replace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "重试读取项目" }));
  await screen.findByText("逆光");
});
it("按服务端游标翻页，搜索新条件从第一页读取", async () => {
  ports.list
    .mockResolvedValueOnce({ items: [project], next_cursor: "next-page" })
    .mockResolvedValue({ items: [], next_cursor: null });
  setup();
  await screen.findByText("逆光");
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await waitFor(() =>
    expect(
      ports.list.mock.calls.some(([params]) => params.cursor === "next-page"),
    ).toBe(true),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "搜索项目" }), {
    target: { value: "国风" },
  });
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() =>
    expect(
      ports.list.mock.calls.some(
        ([params]) => params.q === "国风" && params.cursor === undefined,
      ),
    ).toBe(true),
  );
});
it("创建成功失效真实项目查询并跳转正式 UUID 画布，表单不出现第二次离开确认", async () => {
  const cache = setup();
  await screen.findByText("逆光");
  const invalidate = vi.spyOn(cache, "invalidateQueries");
  fireEvent.click(screen.getByRole("button", { name: "新建项目" }));
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "新项目" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建并进入画布" }));
  await waitFor(() =>
    expect(ports.push).toHaveBeenCalledExactlyOnceWith(
      `/projects/${project.id}/canvas`,
    ),
  );
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["projects"] });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["canvas", "projects"] });
  expect(screen.queryByRole("dialog", { name: "离开项目创建？" })).toBeNull();
  expect(screen.queryByText(/样例进度|创建预览/)).toBeNull();
});

it("创建失败保留草稿并能重试，不出现登录入口", async () => {
  ports.create.mockRejectedValueOnce(
    new ApiError(503, "dependency_unavailable"),
  );
  setup();
  await screen.findByText("逆光");
  fireEvent.click(screen.getByRole("button", { name: "新建项目" }));
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "保留配置" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建并进入画布" }));
  await screen.findByText("项目尚未确认创建");
  expect((screen.getByLabelText("项目名称") as HTMLInputElement).value).toBe(
    "保留配置",
  );
  expect(screen.queryByRole("link", { name: "重新登录" })).toBeNull();
  expect(ports.replace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "重试创建" }));
  await waitFor(() =>
    expect(ports.push).toHaveBeenCalledWith(`/projects/${project.id}/canvas`),
  );
  expect(ports.create.mock.calls[0][1]).toBe(ports.create.mock.calls[1][1]);
});
