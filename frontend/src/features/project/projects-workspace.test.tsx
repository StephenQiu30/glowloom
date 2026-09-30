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
  session: vi.fn(),
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
vi.mock("@/features/auth/queries", () => ({
  CURRENT_SESSION_KEY: ["identity", "current-user"],
  getCurrentSession: ports.session,
}));
vi.mock("./queries", () => ({
  PROJECTS_KEY: ["projects"],
  listProjects: ports.list,
  listStylePresets: ports.presets,
  createProject: ports.create,
}));
vi.mock("@/components/theme-toggle", () => ({ ThemeToggle: () => null }));
const session = {
  user: {
    id: "584ad191-2932-4d7c-bccb-0b9d481c5a76",
    must_change_password: false,
    display_name: "制作者",
  },
};
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
  ports.session.mockResolvedValue(session);
  ports.list.mockResolvedValue({ items: [project], next_cursor: null });
  ports.presets.mockResolvedValue({ items: [], next_cursor: null });
  ports.create.mockResolvedValue({ ...project, name: "新项目" });
});
afterEach(cleanup);
function setup(cached = false) {
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  if (cached) cache.setQueryData(["identity", "current-user"], session);
  render(
    <QueryClientProvider client={cache}>
      <ProjectsWorkspace />
    </QueryClientProvider>,
  );
  return cache;
}
it("身份尚未确认或旧缓存被403拒绝时不读取项目，不显示旧服务端内容", async () => {
  let fail!: (error: Error) => void;
  ports.session.mockImplementation(
    () =>
      new Promise((_resolve, reject) => {
        fail = reject;
      }),
  );
  setup(true);
  expect(screen.getByRole("status").textContent).toContain("确认身份");
  expect(ports.list).not.toHaveBeenCalled();
  fail(new ApiError(403, "forbidden"));
  await screen.findByText("当前账号没有此操作的权限。");
  expect(ports.list).not.toHaveBeenCalled();
  expect(screen.queryByText("逆光")).toBeNull();
});
it("列表401引导重新登录，403或依赖错误保留明确重试路径", async () => {
  ports.list.mockRejectedValue(new ApiError(401, "session_expired"));
  setup();
  await waitFor(() =>
    expect(ports.replace).toHaveBeenCalledWith("/login?returnTo=%2Fprojects"),
  );
  expect(screen.queryByText("逆光")).toBeNull();
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

it("顶层首次改密标记阻止读取，即使嵌套用户旧标记为false", async () => {
  ports.session.mockResolvedValue({ ...session, must_change_password: true });
  setup();
  await waitFor(() =>
    expect(ports.replace).toHaveBeenCalledWith("/login?returnTo=%2Fprojects"),
  );
  expect(ports.list).not.toHaveBeenCalled();
});

it("创建中收到401保留草稿并停用写入，关闭确认后才回登录", async () => {
  ports.create.mockRejectedValue(new ApiError(401, "session_expired"));
  setup();
  await screen.findByText("逆光");
  fireEvent.click(screen.getByRole("button", { name: "新建项目" }));
  fireEvent.change(screen.getByLabelText("项目名称"), {
    target: { value: "保留配置" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建并进入画布" }));
  await screen.findByText("会话需要重新确认");
  expect((screen.getByLabelText("项目名称") as HTMLInputElement).value).toBe(
    "保留配置",
  );
  expect(
    screen.getByRole("button", { name: "重试创建" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(ports.replace).not.toHaveBeenCalled();
  expect(screen.queryByText("逆光")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  fireEvent.click(screen.getByRole("button", { name: "放弃并离开" }));
  await waitFor(() =>
    expect(ports.replace).toHaveBeenCalledWith("/login?returnTo=%2Fprojects"),
  );
  expect(ports.create).toHaveBeenCalledTimes(1);
});

it("预设401不会伪称无预设并继续创建", async () => {
  ports.presets.mockRejectedValue(new ApiError(401, "session_expired"));
  setup();
  await screen.findByText("逆光");
  fireEvent.click(screen.getByRole("button", { name: "新建项目" }));
  await screen.findByText("会话需要重新确认");
  expect(
    screen
      .getByRole("button", { name: "创建并进入画布" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(ports.create).not.toHaveBeenCalled();
  expect(ports.replace).not.toHaveBeenCalled();
});
