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
  searchParams: "",
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: ports.replace, push: ports.push }),
  useSearchParams: () => new URLSearchParams(ports.searchParams),
}));
vi.mock("./queries", () => ({
  PROJECTS_KEY: ["projects"],
  listProjects: ports.list,
  listStylePresets: ports.presets,
  createProject: ports.create,
}));
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
  ports.searchParams = "";
  ports.list.mockResolvedValue({ items: [project], next_cursor: null });
  ports.presets.mockResolvedValue({ items: [], next_cursor: null });
  ports.create.mockResolvedValue({ ...project, name: "新项目" });
});
afterEach(cleanup);
function setup() {
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const workspace = () => (
    <QueryClientProvider client={cache}>
      <ProjectsWorkspace />
    </QueryClientProvider>
  );
  const mounted = render(workspace());
  return { cache, rerender: () => mounted.rerender(workspace()) };
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
  const { cache } = setup();
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

it("首页创建入口自动打开真实创建表单，关闭只移除创建参数并可再次打开", async () => {
  ports.searchParams = "q=逆光&status=active&view=table&create=true";
  const { rerender } = setup();
  await screen.findByRole("dialog", { name: "新建项目" });
  await waitFor(() => expect(ports.presets).toHaveBeenCalled());
  expect(ports.create).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(ports.replace).toHaveBeenCalledExactlyOnceWith(
    "/projects?q=%E9%80%86%E5%85%89&status=active&view=table",
    { scroll: false },
  );
  ports.searchParams = "q=逆光&status=active&view=table";
  rerender();
  expect(screen.queryByRole("dialog", { name: "新建项目" })).toBeNull();
  expect(screen.getByRole("textbox", { name: "搜索项目" })).toHaveProperty(
    "value",
    "逆光",
  );
  ports.searchParams += "&create=true";
  rerender();
  expect(screen.getByRole("dialog", { name: "新建项目" })).toBeTruthy();
});

it("来自首页的创建请求仍保护已填写的草稿", async () => {
  ports.searchParams = "create=true";
  const { rerender } = setup();
  await screen.findByRole("dialog", { name: "新建项目" });
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "尚未完成的故事" },
  });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.getByRole("dialog", { name: "离开项目创建？" })).toBeTruthy();
  expect(ports.replace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "继续填写" }));
  expect(screen.getByLabelText("项目名称")).toHaveProperty(
    "value",
    "尚未完成的故事",
  );
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  fireEvent.click(screen.getByRole("button", { name: "放弃并离开" }));
  expect(ports.replace).toHaveBeenCalledWith("/projects", { scroll: false });
  ports.searchParams = "";
  rerender();
  expect(screen.queryByRole("dialog")).toBeNull();
  ports.searchParams = "create=true";
  rerender();
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "再次编辑的故事" },
  });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.getByRole("dialog", { name: "离开项目创建？" })).toBeTruthy();
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
