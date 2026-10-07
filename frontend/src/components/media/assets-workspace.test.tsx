import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AssetsWorkspace } from "./assets-workspace";
import * as library from "./library-queries";
import * as projects from "@/components/project/queries";
import { ApiError } from "@/lib/request";
import { saveLibraryUploads } from "./library-upload-intent";
const nav = vi.hoisted(() => ({ params: "", replace: vi.fn() }));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(nav.params),
  usePathname: () => "/assets",
  useRouter: () => ({ replace: nav.replace }),
}));
vi.mock("./library-queries", async (original) => ({
  ...(await original<typeof import("./library-queries")>()),
  listLibrary: vi.fn(),
  freshLibrary: vi.fn(),
  getLibraryDetail: vi.fn(),
  previewLibrary: vi.fn(),
  applyLibraryIntent: vi.fn(),
}));
vi.mock("@/components/project/queries", () => ({
  listProjects: vi.fn(),
  getProject: vi.fn(),
  PROJECTS_KEY: ["projects"],
}));
const actor = "11111111-1111-4111-8111-111111111111",
  org = "22222222-2222-4222-8222-222222222222",
  lid = "33333333-3333-5333-8333-333333333333",
  id = "44444444-4444-4444-8444-444444444444";
const time = "2026-10-02T00:00:00Z";
const item = {
  id,
  asset_id: null,
  folder_id: null,
  kind: "text" as const,
  title: "正式库正文",
  category: "material" as const,
  tags: [],
  source_label: "",
  note: "",
  favorite: false,
  catalog_state: "active" as const,
  trashed_at: null,
  position: 0,
  revision: 1,
  created_at: time,
  updated_at: time,
};
const page = {
  current_actor_id: actor,
  current_org_id: org,
  library_id: lid,
  scope: { kind: "personal" as const },
  revision: 1,
  page: 1,
  page_size: 40,
  total: 81,
  items: [item],
  folders: [],
  category_counts: { material: 81 },
  folder_counts: { root: 81 },
};
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  nav.params = "";
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.mocked(library.listLibrary).mockImplementation(async (scope, query) => ({
    ...page,
    scope,
    page: query?.page ?? 1,
    page_size: query?.page_size ?? 40,
  }));
  vi.mocked(library.freshLibrary).mockResolvedValue({ ...page, page_size: 1 });
  vi.mocked(projects.listProjects).mockResolvedValue({
    items: [],
    next_cursor: null,
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function mount(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  return render(
    <QueryClientProvider client={client}>
      <AssetsWorkspace />
    </QueryClientProvider>,
  );
}
it("个人库默认40服务端分页，正文懒读；全部筛选写URL保留原Copy定位", async () => {
  nav.params = `copy_source=${id}&copy_job=${id}`;
  mount();
  await screen.findByRole("button", { name: "查看 正式库正文" });
  expect(library.getLibraryDetail).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("搜索完整素材库"), {
    target: { value: "远页全文" },
  });
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  expect(nav.replace).toHaveBeenCalledWith(
    expect.stringContaining(`copy_source=${id}&copy_job=${id}`),
    { scroll: false },
  );
  expect(nav.replace).toHaveBeenCalledWith(
    expect.stringContaining("search=%E8%BF%9C%E9%A1%B5%E5%85%A8%E6%96%87"),
    { scroll: false },
  );
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  expect(nav.replace).toHaveBeenCalledWith(expect.stringContaining("page=2"), {
    scroll: false,
  });
});
it("挂载先获取当前actor/org证明；旧bootstrap私有缓存和403不显示正文", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(
    ["media-library-context", window.location.origin, "personal"],
    page,
  );
  let reject!: (error: Error) => void;
  vi.mocked(library.listLibrary).mockImplementation(
    () =>
      new Promise((_, fail) => {
        reject = fail;
      }),
  );
  mount(client);
  await waitFor(() => expect(library.listLibrary).toHaveBeenCalled());
  expect(screen.queryByText("正式库正文")).toBeNull();
  expect(projects.listProjects).not.toHaveBeenCalled();
  await act(async () => reject(new ApiError(403, "forbidden")));
  await screen.findByRole("button", { name: "重试读取素材库身份" });
  expect(screen.queryByText("正式库正文")).toBeNull();
  expect(projects.listProjects).not.toHaveBeenCalled();
});
it("当前页全选保留真实版本，批量回收一次冻结成员并显示全部ID", async () => {
  mount();
  await screen.findByRole("button", { name: "查看 正式库正文" });
  fireEvent.click(screen.getByRole("button", { name: "选择当前页" }));
  fireEvent.click(screen.getByRole("button", { name: "移入回收站" }));
  await screen.findByRole("dialog", { name: "移入回收站" });
  expect(screen.getByText(id)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "确认移入回收站" }));
  await waitFor(() =>
    expect(library.applyLibraryIntent).toHaveBeenCalledWith(
      expect.objectContaining({
        body: {
          scope: page.scope,
          expected_revision: 1,
          action: "recycle_items",
          items: [{ id, revision: 1 }],
        },
      }),
    ),
  );
});
it("跨刷新未确认上传自动打开恢复入口，原指纹保留且目录写与scope切换锁定", async () => {
  const identity = {
    origin: window.location.origin,
    actorId: actor,
    orgId: org,
    libraryId: lid,
    scope: page.scope,
  };
  saveLibraryUploads(sessionStorage, identity, [
    {
      key: id,
      fileName: "original.txt",
      byteSize: 4,
      sha256: "a".repeat(64),
      local_review_confirmed: true,
    },
  ]);
  mount();
  await screen.findByRole("dialog", { name: "上传素材原件" });
  expect(screen.getByText(/^原键 /).textContent).toContain(id);
  expect(
    screen.getByText("新建目录").closest("button")!.matches(":disabled"),
  ).toBe(true);
  expect(
    document.querySelector('[aria-label="素材库范围"]')!.matches(":disabled"),
  ).toBe(true);
  expect(library.applyLibraryIntent).not.toHaveBeenCalled();
});
it("URL目录已移除时显示可恢复错误，不崩溃且保留Copy参数", async () => {
  nav.params = `folder=${id}&copy_source=${id}`;
  mount();
  await screen.findByText("此目录已不存在或当前不可访问");
  fireEvent.click(screen.getByRole("button", { name: "返回全部素材" }));
  expect(nav.replace).toHaveBeenCalledWith(
    expect.stringContaining(`copy_source=${id}`),
    { scroll: false },
  );
});
it.each([
  ["新建目录", "目录名称"],
  ["上传素材原件", "本地素材原件"],
])("%s按Escape关闭后返回确切启动按钮，而非页面正文", async (name, input) => {
  mount();
  const trigger = await screen.findByRole("button", { name });
  await waitFor(() => expect(trigger.matches(":disabled")).toBe(false));
  trigger.focus();
  fireEvent.click(trigger);
  const dialog = await screen.findByRole("dialog", { name });
  await screen.findByLabelText(input);
  fireEvent.keyDown(dialog, { key: "Escape" });
  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name })).toBeNull(),
  );
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
it("详情过渡到编辑时保留新弹窗焦点，最终关闭详情返回原查看按钮", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  vi.mocked(library.getLibraryDetail).mockResolvedValue({
    ...item,
    plain_text: "独立正文",
  });
  const mounted = mount(client);
  function navigate() {
    mounted.rerender(
      <QueryClientProvider client={client}>
        <AssetsWorkspace />
      </QueryClientProvider>,
    );
  }
  nav.replace.mockImplementation((url: string) => {
    nav.params = url.split("?")[1] ?? "";
  });
  const trigger = await screen.findByRole("button", {
    name: "查看 正式库正文",
  });
  await waitFor(() => expect(trigger.matches(":disabled")).toBe(false));
  trigger.focus();
  fireEvent.click(trigger);
  navigate();
  await screen.findByRole("dialog", { name: "正式库正文" });
  fireEvent.click(screen.getByRole("button", { name: "编辑素材信息" }));
  const editor = await screen.findByRole("dialog", { name: "编辑素材描述" });
  await waitFor(() =>
    expect(editor.contains(document.activeElement)).toBe(true),
  );
  fireEvent.keyDown(editor, { key: "Escape" });
  const detail = await screen.findByRole("dialog", { name: "正式库正文" });
  await waitFor(() =>
    expect(detail.contains(document.activeElement)).toBe(true),
  );
  fireEvent.keyDown(detail, { key: "Escape" });
  navigate();
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
