import { render, findFeedback } from "../../../../feedback";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectsPage } from "@/app/projects/_components/projects-page";
import { ProjectCreationProvider } from "@/components/projects/project-creation";
import { projectIntentStore } from "@/components/projects/project-intent-store";
import { projectPresentationStore } from "@/components/projects/project-presentation-store";
import { RequestError } from "@/lib/request";
import { setCreationScope } from "@/lib/request";
const api = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  project: vi.fn(),
  canvas: vi.fn(),
  recover: vi.fn(),
  update: vi.fn(),
  archive: vi.fn(),
  restore: vi.fn(),
  push: vi.fn(),
}));
const account = vi.hoisted(() => ({
  actor: {
    id: "00000000-0000-4000-8000-000000000010",
    display_name: "Fixture",
    email: "fixture@example.invalid",
    status: "active",
    revision: "1",
    platform_role: "member",
    capabilities: [],
  },
  session_id: "00000000-0000-4000-8000-000000000011",
  session_protocol: "v2",
  credential_epoch: "1",
  csrf_token: "fixture-proof",
  persistent: false,
  upgrade_available: false,
  server_time: "2026-10-19T00:00:00Z",
  last_active_at: "2026-10-19T00:00:00Z",
  absolute_expires_at: "2026-11-19T00:00:00Z",
  idle_expires_at: "2026-10-20T00:00:00Z",
}));
const scope = vi.hoisted(() => ({
  id: "00000000-0000-4000-8000-000000000012",
  kind: "personal",
  name: "个人空间",
  role: "creator",
  team_id: "",
  writable: true,
}));
const navigation = vi.hoisted(() => ({
  search: "",
  subscribers: new Set<() => void>(),
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", async () => {
  const { useSyncExternalStore } = await import("react");
  return {
    useRouter: () => router,
    useSearchParams: () =>
      new URLSearchParams(
        useSyncExternalStore(
          (listener) => {
            navigation.subscribers.add(listener);
            return () => {
              navigation.subscribers.delete(listener);
            };
          },
          () => navigation.search,
        ),
      ),
  };
});
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => account,
  useSessionUpdate: () => vi.fn(),
  ensureSessionFresh: async () => account,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    spaces: [scope],
    selected: scope,
    pending: false,
    refresh: async () => {},
    select: async () => {},
  }),
}));
vi.mock("@/components/projects/project-intent-store", async (original) => {
  const actual =
    await original<
      typeof import("@/components/projects/project-intent-store")
    >();
  class Memory {
    state: import("@/components/projects/project-intent-store").IntentDatabase =
      { fence: 0, intents: [] };
    async transact<T>(
      _write: boolean,
      run: (
        state: import("@/components/projects/project-intent-store").IntentDatabase,
      ) => T,
    ) {
      const state = structuredClone(this.state),
        result = run(state);
      this.state = state;
      return structuredClone(result);
    }
  }
  return {
    ...actual,
    projectIntentStore: new actual.ProjectIntentStore(new Memory()),
  };
});
vi.mock("@/components/layout/page-layout", async () => {
  const { ProductSearch } = await import("@/components/layout/product-search");
  return {
    PageLayout: ({
      children,
      search,
    }: {
      children: React.ReactNode;
      search: import("@/components/layout/product-search").PageSearch;
    }) => (
      <>
        <ProductSearch search={search} />
        {children}
      </>
    ),
  };
});
vi.mock("@/api/projects", () => ({
  createProject: api.create,
  listProjects: api.list,
  getProject: api.project,
  getProjectCreation: api.recover,
  updateProject: api.update,
  archiveProject: api.archive,
  unarchiveProject: vi.fn(),
  deleteProject: vi.fn(),
  restoreProject: api.restore,
}));
vi.mock("@/api/canvases", () => ({ getCanvas: api.canvas }));
const project = {
  id: "00000000-0000-4000-8000-000000000001",
  canvas_id: "00000000-0000-4000-8000-000000000002",
  name: "真实项目",
  aspect_ratio: "16:9",
  style_type: "realistic",
  style_key: null,
  revision: 1,
  status: "active",
  is_delete: false,
  last_modified_at: "2026-10-19T00:00:00Z",
  last_modified_basis: "created",
  create_time: "2026-10-19T00:00:00Z",
  update_time: "2026-10-19T00:00:00Z",
  description: "",
  resolution: "1080p",
  allow_overseas_models: false,
  default_models: {},
  cover_asset_id: null,
  cover_unavailable: false,
  archived_at: null,
  delete_time: null,
  purge_after: null,
  style_preset_id: null,
};
function page() {
  return render(
    <ProjectCreationProvider>
      <ProjectsPage />
    </ProjectCreationProvider>,
  );
}
beforeEach(async () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  Element.prototype.scrollIntoView = vi.fn();
  vi.clearAllMocks();
  navigation.search = "";
  router.push.mockImplementation((url: string) => {
    navigation.search = new URL(url, "http://localhost").search;
    navigation.subscribers.forEach((listener) => listener());
  });
  api.create.mockReset();
  api.list.mockReset().mockResolvedValue({ items: [], next_cursor: null });
  api.recover.mockReset().mockResolvedValue({
    state: "unrecorded",
    project_id: null,
    canvas_id: null,
    project_status: null,
  });
  api.project.mockReset().mockResolvedValue(project);
  api.canvas.mockReset().mockResolvedValue({
    id: project.canvas_id,
    project_id: project.id,
    name: project.name,
    revision: 1,
    scope: {},
    viewport: { x: 0, y: 0, zoom: 1 },
    nodes: [],
    edges: [],
  });
  await projectIntentStore.invalidate("identity_change");
  projectPresentationStore.clear();
  setCreationScope(scope.id);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("快捷创建无表单，丢回执先只读确认，再以原默认值和key明确继续", async () => {
  api.create
    .mockRejectedValueOnce(new Error("lost receipt"))
    .mockResolvedValueOnce({ ...project, name: "未命名画布" });
  page();
  fireEvent.click(screen.getByRole("button", { name: "新建画布" }));
  expect(screen.queryByLabelText("画布名称")).toBeNull();
  await waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
  await screen.findByRole("button", { name: "确认创建结果" });
  fireEvent.click(screen.getByRole("button", { name: "确认创建结果" }));
  await screen.findByRole("button", { name: "继续原创建" });
  expect(api.create).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "继续原创建" }));
  await waitFor(() =>
    expect(router.push).toHaveBeenCalledWith(
      `/canvas?project_id=${project.id}&canvas_id=${project.canvas_id}`,
    ),
  );
  expect(api.create.mock.calls[0][0]).toEqual({
    name: "未命名画布",
    description: "",
    aspect_ratio: "16:9",
    style_type: "realistic",
  });
  expect(api.create.mock.calls[1][0]).toEqual(api.create.mock.calls[0][0]);
  expect(api.create.mock.calls[1][1].headers["Idempotency-Key"]).toBe(
    api.create.mock.calls[0][1].headers["Idempotency-Key"],
  );
  expect(api.create.mock.calls[0][1].sessionRecovery).toBe("never");
});
it("创建不重入，残缺回执保持未知且不跳转固定项目", async () => {
  let finish!: (value: unknown) => void;
  api.create.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  page();
  const button = screen.getByRole("button", { name: "新建画布" });
  fireEvent.click(button);
  fireEvent.click(button);
  await waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
  await act(async () => finish({ ...project, canvas_id: null }));
  await screen.findByRole("button", { name: "确认创建结果" });
  expect(router.push).not.toHaveBeenCalled();
});
it("列表加载失败不显示示例项目，名称搜索与真实最近修改口径传到服务端", async () => {
  api.list
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue({ items: [project], next_cursor: null });
  page();
  await findFeedback();
  expect(screen.queryByText("雾港来信")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "搜索画布" }));
  fireEvent.change(await screen.findByRole("combobox", { name: "搜索画布" }), {
    target: { value: "真实" },
  });
  fireEvent.click(screen.getByRole("option", { name: /搜索画布/ }));
  await screen.findByText("真实项目");
  expect(api.list.mock.calls.at(-1)?.[0]).toEqual({
    q: "真实",
    limit: 12,
    order: "recent_content",
    status: "active",
  });
  expect(
    screen.getByRole("link", { name: /打开画布/ }).getAttribute("href"),
  ).toBe(`/canvas?project_id=${project.id}&canvas_id=${project.canvas_id}`);
});

it("画布按12项游标翻页，返回上一页不累计旧卡片", async () => {
  const second = {
    ...project,
    id: "00000000-0000-4000-8000-000000000003",
    name: "第二页画布",
  };
  api.list.mockImplementation(async (params) =>
    params.cursor
      ? { items: [second], next_cursor: null }
      : { items: [project], next_cursor: "next-canvas-page" },
  );
  page();
  await screen.findByText("真实项目");
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await screen.findByText("第二页画布");
  expect(screen.queryByText("真实项目")).toBeNull();
  expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({
    limit: 12,
    cursor: "next-canvas-page",
  });
  fireEvent.click(screen.getByRole("button", { name: "上一页" }));
  await screen.findByText("真实项目");
  expect(api.list.mock.calls.at(-1)?.[0].cursor).toBeUndefined();
  expect(screen.queryByText("第二页画布")).toBeNull();
});
it.each([
  ["transition", "created", /恢复信息保存失败/, /浏览器未能保存恢复信息/],
  ["acknowledge", null, /恢复信息待清理/, /浏览器未能清理恢复信息/],
] as const)(
  "创建成功后 %s 失败说明本地恢复环节并只重开原目标",
  async (method, phase, heading, description) => {
    api.create.mockResolvedValue(project);
    const original = projectIntentStore[method].bind(projectIntentStore);
    const fault = vi
      .spyOn(projectIntentStore, method)
      .mockImplementation(async (...args: unknown[]) => {
        if (phase === null || args[3] === phase)
          throw new DOMException("QA storage failure", "QuotaExceededError");
        return Reflect.apply(original, projectIntentStore, args);
      });
    try {
      page();
      fireEvent.click(screen.getByRole("button", { name: "新建画布" }));
      await screen.findByRole("heading", { name: heading });
      expect(screen.getByText(description)).toBeTruthy();
      expect(screen.queryByRole("button", { name: "继续原创建" })).toBeNull();
      expect(screen.queryByText(/结果尚未确认/)).toBeNull();
      fault.mockRestore();
      fireEvent.click(screen.getByRole("button", { name: "重新打开" }));
      await waitFor(() =>
        expect(router.push).toHaveBeenCalledWith(
          `/canvas?project_id=${project.id}&canvas_id=${project.canvas_id}`,
        ),
      );
      expect(api.create).toHaveBeenCalledTimes(1);
    } finally {
      fault.mockRestore();
    }
  },
);

it("批量选择不进入画布，确认后调用已有归档接口", async () => {
  api.list.mockResolvedValue({ items: [project], next_cursor: null });
  api.archive.mockResolvedValue({
    ...project,
    revision: 2,
    status: "archived",
  });
  page();
  await screen.findByText("真实项目");
  fireEvent.click(screen.getByRole("button", { name: "批量管理" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择本页" }));
  expect(screen.getByText("已选择 1 项")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "归档所选" }));
  await screen.findByRole("dialog");
  fireEvent.click(screen.getByRole("button", { name: "归档" }));
  await screen.findByText("已归档 1 个画布。");
  expect(api.archive).toHaveBeenCalledTimes(1);
});
it("已归档与回收站按真实状态查询", async () => {
  page();
  fireEvent.click(screen.getByRole("button", { name: "已归档" }));
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({
      status: "archived",
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "回收站" }));
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({ deleted: true }),
  );
});

it("从默认画布切到回收站再恢复，结果入口返回active筛选", async () => {
  const recycled = {
    ...project,
    revision: 2,
    is_delete: true,
    delete_time: "2026-10-21T00:00:00Z",
    purge_after: "2026-11-20T00:00:00Z",
  };
  api.list.mockImplementation(async (params) => ({
    items: params.deleted ? [recycled] : [project],
    next_cursor: null,
    server_time: "2026-10-21T00:00:00Z",
  }));
  api.restore.mockResolvedValue({ ...project, revision: 3 });
  const view = page();
  await screen.findByText("真实项目");
  fireEvent.click(screen.getByRole("button", { name: "回收站" }));
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({ deleted: true }),
  );
  fireEvent.click(screen.getByRole("button", { name: "批量管理" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择本页" }));
  fireEvent.click(screen.getByRole("button", { name: "恢复所选" }));
  fireEvent.click(await screen.findByRole("button", { name: /^恢复$/ }));
  await screen.findByText("已恢复 1 个画布。");
  const destination = screen.getByRole("link", { name: "查看画布" });
  expect(destination.getAttribute("href")).toBe("/projects?state=active");
  fireEvent.click(destination);
  act(() => router.push("/projects?state=active"));
  view.rerender(
    <ProjectCreationProvider>
      <ProjectsPage />
    </ProjectCreationProvider>,
  );
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)?.[0]).toMatchObject({ status: "active" }),
  );
  expect(
    screen.getByRole("button", { name: /^画布$/ }).getAttribute("aria-pressed"),
  ).toBe("true");
});

it("未知项原操作重试成功后仍保留首轮被拒绝项与原因", async () => {
  const second = {
    ...project,
    id: "00000000-0000-4000-8000-000000000003",
    name: "第二项",
  };
  api.list.mockResolvedValue({ items: [project, second], next_cursor: null });
  api.project.mockImplementation(async ({ pid }) =>
    pid === second.id ? second : project,
  );
  api.archive
    .mockRejectedValueOnce(new RequestError("inflight_work", 409))
    .mockRejectedValueOnce(new Error("not delivered"))
    .mockResolvedValueOnce({ ...second, revision: 2, status: "archived" });
  page();
  await screen.findByText("第二项");
  fireEvent.click(screen.getByRole("button", { name: "批量管理" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择本页" }));
  fireEvent.click(screen.getByRole("button", { name: "归档所选" }));
  fireEvent.click(await screen.findByRole("button", { name: /^归档$/ }));
  await screen.findByText(/已完成 0 项，1 项被拒绝，1 项结果未确认/);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await screen.findByText(/已完成 1 项，1 项被拒绝，0 项结果未确认/);
  expect(screen.getByText(/真实项目：画布仍有进行中的任务/)).toBeTruthy();
  expect(api.archive.mock.calls[2]).toEqual(api.archive.mock.calls[1]);
  fireEvent.click(screen.getByRole("button", { name: "完成" }));
  expect(await screen.findByText("已选择 1 项")).toBeTruthy();
});
