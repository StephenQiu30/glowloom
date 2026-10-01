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
  folders: vi.fn(),
  folder: vi.fn(),
}));
vi.mock("./folder-queries", () => ({
  FOLDERS_KEY: ["projects", "folders"],
  listFolders: ports.folders,
  findFolder: ports.folder,
}));
vi.mock("./project-folder-dialog", () => ({ ProjectFolderDialog: () => null }));
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
vi.mock("./project-copy-dialog", () => ({
  ProjectCopyDialog: ({
    sourceId,
    jobId,
    onClose,
  }: {
    sourceId: string;
    jobId?: string;
    onClose: () => void;
  }) => (
    <div
      role="dialog"
      aria-label="复制完整项目"
      data-source={sourceId}
      data-job={jobId}
    >
      <button onClick={onClose}>关闭复制任务</button>
    </div>
  ),
}));
const project = {
  id: "d77d3c2a-7092-4e30-bf41-a3c5d4d26418",
  name: "逆光",
  aspect_ratio: "16:9",
  style_type: "realistic",
  status: "active",
  is_delete: false,
  revision: 1,
  folder_id: null,
  placement_revision: 0,
};
const folder = {
  id: "9f3e948c-550b-47c8-8fa2-f2c31c88927c",
  name: "目录一",
  cover: null,
  cover_unavailable: false,
  project_count: 2,
  revision: 3,
  is_delete: false,
  delete_time: null,
  create_time: "2026-10-02T00:00:00Z",
  update_time: "2026-10-02T00:00:00Z",
};
beforeEach(() => {
  vi.resetAllMocks();
  ports.searchParams = "";
  ports.list.mockResolvedValue({ items: [project], next_cursor: null });
  ports.presets.mockResolvedValue({ items: [], next_cursor: null });
  ports.create.mockResolvedValue({ ...project, name: "新项目" });
  ports.folders.mockResolvedValue({
    current_actor_id: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
    current_org_id: "1a39f9d1-a43c-4d98-8947-901b385d5457",
    items: [folder],
    next_cursor: null,
  });
  ports.folder.mockResolvedValue(folder);
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
  expect(ports.list.mock.calls[0][0].folder_id).toBe("root");
  await screen.findByRole("button", { name: "打开目录 目录一" });
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

it("复制 URL 刷新不依赖当前搜索页命中源项目，关闭只去除复制参数", async () => {
  const jobId = "93021a72-3281-42b2-91cf-6a4413ee8956";
  ports.searchParams = `q=没有命中&view=table&copy_source=${project.id}&copy_job=${jobId}`;
  ports.list.mockResolvedValue({ items: [], next_cursor: null });
  setup();
  const dialog = await screen.findByRole("dialog", { name: "复制完整项目" });
  expect(dialog.getAttribute("data-source")).toBe(project.id);
  expect(dialog.getAttribute("data-job")).toBe(jobId);
  fireEvent.click(screen.getByRole("button", { name: "关闭复制任务" }));
  const next = new URL(ports.replace.mock.calls.at(-1)![0], "http://localhost");
  expect(next.searchParams.get("q")).toBe("没有命中");
  expect(next.searchParams.get("view")).toBe("table");
  expect(next.searchParams.has("copy_source")).toBe(false);
  expect(next.searchParams.has("copy_job")).toBe(false);
});
it("搜索与视图筛选保留复制恢复参数", async () => {
  ports.searchParams = `copy_source=${project.id}&copy_job=93021a72-3281-42b2-91cf-6a4413ee8956`;
  setup();
  await screen.findByText("逆光");
  fireEvent.change(screen.getByRole("textbox", { name: "搜索项目" }), {
    target: { value: "另一搜索" },
  });
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  const next = new URL(ports.replace.mock.calls.at(-1)![0], "http://localhost");
  expect(next.searchParams.get("copy_source")).toBe(project.id);
  expect(next.searchParams.get("copy_job")).toBe(
    "93021a72-3281-42b2-91cf-6a4413ee8956",
  );
  expect(next.searchParams.get("q")).toBe("另一搜索");
});

it("晚页目录URL刷新读取真实目录，进入回收站移除目录绑定且保留Copy恢复参数", async () => {
  ports.searchParams = `folder_id=${folder.id}&cursor=project-later&page=3&copy_source=${project.id}`;
  setup();
  await screen.findByRole("button", { name: "修改当前目录名称" });
  expect(ports.folder).toHaveBeenCalledWith(
    folder.id,
    expect.objectContaining({
      actorId: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
    }),
    expect.any(AbortSignal),
  );
  expect(ports.list.mock.calls[0][0]).toMatchObject({
    folder_id: folder.id,
    cursor: "project-later",
  });
  fireEvent.click(screen.getByRole("radio", { name: "回收站" }));
  await waitFor(() =>
    expect(ports.list.mock.calls.at(-1)![0]).toMatchObject({
      folder_id: undefined,
      cursor: undefined,
      deleted: true,
    }),
  );
  const next = new URL(
    ports.replace.mock.calls.at(-1)![0],
    window.location.origin,
  );
  expect(next.searchParams.has("folder_id")).toBe(false);
  expect(next.searchParams.has("cursor")).toBe(false);
  expect(next.searchParams.get("copy_source")).toBe(project.id);
  expect(screen.queryByRole("navigation", { name: "项目目录路径" })).toBeNull();
});

it("目录分页失败可重读，原目录和Copy参数不被错误页覆盖", async () => {
  ports.searchParams = `folder_cursor=folder-later&copy_source=${project.id}`;
  ports.folders.mockImplementation((cursor?: string) =>
    cursor === "folder-later"
      ? Promise.reject(new ApiError(503, "dependency_unavailable"))
      : Promise.resolve({
          current_actor_id: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
          current_org_id: "1a39f9d1-a43c-4d98-8947-901b385d5457",
          items: [],
          next_cursor: "folder-later",
        }),
  );
  setup();
  await screen.findByText("目录列表未能加载");
  expect(screen.queryByRole("button", { name: "打开目录 目录一" })).toBeNull();
  ports.folders.mockResolvedValue({
    current_actor_id: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
    current_org_id: "1a39f9d1-a43c-4d98-8947-901b385d5457",
    items: [folder],
    next_cursor: null,
  });
  fireEvent.click(screen.getByRole("button", { name: "重试读取目录" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "打开目录 目录一" }),
  );
  const next = new URL(
    ports.replace.mock.calls.at(-1)![0],
    window.location.origin,
  );
  expect(next.searchParams.get("folder_id")).toBe(folder.id);
  expect(next.searchParams.has("folder_cursor")).toBe(false);
  expect(next.searchParams.get("copy_source")).toBe(project.id);
});

it("无效目录UUID不发项目或目录详情GET，可人工返回真实根目录", async () => {
  ports.searchParams = "folder_id=not-a-folder";
  setup();
  await screen.findByText("目录链接无效");
  expect(ports.list).not.toHaveBeenCalled();
  expect(ports.folder).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "根目录" }));
  await screen.findByText("逆光");
  expect(ports.list.mock.calls.at(-1)![0].folder_id).toBe("root");
});
