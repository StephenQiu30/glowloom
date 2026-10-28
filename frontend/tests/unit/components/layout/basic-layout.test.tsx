import { SpaceProvider } from "@/components/spaces/space-context";
import { SessionProvider } from "@/components/auth/session";
import {
  cleanup,
  act,
  fireEvent,
  render as renderUI,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AssetWorkspace } from "@/app/assets/_components/asset-workspace";
import { BasicLayout } from "@/components/layout/basic-layout";
import { ApplicationLayout } from "@/components/layout/application-layout";
import { RouteLoading } from "@/components/layout/route-loading";
import { PageLayout } from "@/components/layout/page-layout";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/request")>();
  return { ...actual, request: vi.fn() };
});
function TestProviders({ children }: { children: ReactNode }) {
  return (
    <SessionProvider>
      <SpaceProvider blockUntilReady={false}>
        <TooltipProvider>{children}</TooltipProvider>
      </SpaceProvider>
    </SessionProvider>
  );
}
function render(children: ReactNode) {
  return renderUI(children, { wrapper: TestProviders });
}
const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  pathname: "/",
}));
vi.mock("next/navigation", () => ({
  usePathname: () => mocks.pathname,
  useRouter: () => mocks,
}));
vi.mock("sonner", () => ({
  toast: Object.assign(vi.fn(), {
    success: mocks.success,
    error: mocks.error,
    dismiss: vi.fn(),
  }),
}));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
beforeEach(() => {
  vi.clearAllMocks();
  mocks.pathname = "/";
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  document.cookie = "sidebar_state=; path=/; max-age=0";
  vi.mocked(request)
    .mockReset()
    .mockResolvedValue({
      items: [
        {
          id: "33333333-3333-4333-8333-333333333333",
          kind: "personal",
          name: "个人空间",
          role: "creator",
          writable: true,
          team_id: "",
        },
      ],
      actor: {
        id: "11111111-1111-4111-8111-111111111111",
        email: "admin@example.com",
        platform_role: "platform_admin",
        status: "active",
        revision: "1",
        capabilities: ["platform:manage"],
        display_name: "Admin",
      },
      session_id: "22222222-2222-4222-8222-222222222222",
      last_active_at: new Date().toISOString(),
      absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
      persistent: false,
      must_change_password: false,
      csrf_token: "proof",
      credential_epoch: "1",
    });
});
it("R6 主导航与画布入口指向真实路由，支持收起和展开", async () => {
  render(
    <BasicLayout screen="home">
      <h1>创作工作区</h1>
    </BasicLayout>,
  );
  expect(
    (await screen.findByRole("link", { name: "创作" })).getAttribute("href"),
  ).toBe("/");
  expect(screen.getByRole("main").id).toBe("main-content");
  expect(
    screen.getByRole("banner", { name: "页面顶栏" }).closest("main"),
  ).toBeNull();
  expect(screen.getByRole("link", { name: "资产" }).getAttribute("href")).toBe(
    "/assets",
  );
  expect(
    screen.getByRole("link", { name: "查看我的画布" }).getAttribute("href"),
  ).toBe("/projects");
  fireEvent.click(screen.getByRole("button", { name: "收起导航" }));
  expect(screen.getByRole("button", { name: "展开导航" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "展开导航" }));
  expect(screen.queryByRole("button", { name: "展开导航" })).toBeNull();
  const header = screen.getByRole("banner", { name: "页面顶栏" });
  expect(within(header).getByRole("button", { name: "搜索灵感" })).toBeTruthy();
  expect(within(header).getByRole("button", { name: "账号菜单" })).toBeTruthy();
  expect(screen.queryByRole("combobox", { name: "当前创作空间" })).toBeNull();
  expect(screen.queryByRole("link", { name: "团队" })).toBeNull();
  expect(screen.queryByRole("link", { name: "邀请与授权" })).toBeNull();
});

it("会话失效时不渲染工作台内容", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("session_invalid", 401),
  );
  render(
    <BasicLayout screen="home">
      <p>私有工作台内容</p>
    </BasicLayout>,
  );
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/login"));
  expect(screen.queryByText("私有工作台内容")).toBeNull();
});

it("素材库允许未登录页面外壳，私人数据仍由页面会话边界控制", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("unauthenticated", 401),
  );
  render(
    <BasicLayout screen="assets" allowGuest>
      <p>素材库登录引导</p>
    </BasicLayout>,
  );
  expect(await screen.findByText("素材库登录引导")).toBeTruthy();
  expect(mocks.replace).not.toHaveBeenCalled();
  expect(screen.getByRole("link", { name: "资产" }).getAttribute("href")).toBe(
    "/assets",
  );
});

it("收起后保留可访问导航和当前页，重新挂载保持偏好", async () => {
  const view = render(
    <BasicLayout screen="explore">
      <h1>探索内容</h1>
    </BasicLayout>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "收起导航" }));
  expect(
    screen.getByRole("link", { name: "探索" }).getAttribute("aria-current"),
  ).toBe("page");
  expect(
    screen.getByRole("link", { name: "查看我的画布" }).getAttribute("href"),
  ).toBe("/projects");
  expect(document.querySelector('[data-collapsible="icon"]')).toBeTruthy();
  view.unmount();
  render(
    <BasicLayout screen="home">
      <h1>首页内容</h1>
    </BasicLayout>,
  );
  expect(await screen.findByRole("button", { name: "展开导航" })).toBeTruthy();
});

it("快捷键切换导航，编辑输入时不抢占快捷键", async () => {
  render(
    <BasicLayout screen="home">
      <input aria-label="提示词" />
    </BasicLayout>,
  );
  await screen.findByRole("button", { name: "收起导航" });
  fireEvent.keyDown(await screen.findByRole("textbox", { name: "提示词" }), {
    key: "b",
    ctrlKey: true,
  });
  expect(screen.getByRole("button", { name: "收起导航" })).toBeTruthy();
  fireEvent.keyDown(window, { key: "b", ctrlKey: true });
  expect(screen.getByRole("button", { name: "展开导航" })).toBeTruthy();
});

it("手机抽屉保持完整导航，同路由点击关闭抽屉且保留桌面偏好", async () => {
  document.cookie = "sidebar_state=false; path=/";
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockImplementation(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  render(
    <BasicLayout screen="home">
      <h1>手机首页</h1>
    </BasicLayout>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "打开导航" }));
  const dialog = screen.getByRole("dialog", { name: "导航" });
  expect(within(dialog).getByRole("button", { name: "关闭导航" })).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("link", { name: "创作" }));
  expect(screen.queryByRole("dialog", { name: "导航" })).toBeNull();
  expect(document.cookie).toContain("sidebar_state=false");
});

it("游客仅从顶栏登录，手机抽屉只展示导航，关闭认证后恢复登录按钮焦点", async () => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockImplementation(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  vi.mocked(request).mockRejectedValue(
    new RequestError("session_invalid", 401),
  );
  render(
    <BasicLayout screen="home" allowGuest>
      <h1>游客首页</h1>
    </BasicLayout>,
  );
  expect(await screen.findByRole("button", { name: "切换主题" })).toBeTruthy();
  fireEvent.click(await screen.findByRole("button", { name: "打开导航" }));
  const navigation = screen.getByRole("dialog", { name: "导航" });
  expect(within(navigation).queryByRole("button", { name: "登录" })).toBeNull();
  expect(within(navigation).queryByText("登录后保存你的创作")).toBeNull();
  fireEvent.click(within(navigation).getByRole("button", { name: "关闭导航" }));
  const login = within(
    screen.getByRole("banner", { name: "页面顶栏" }),
  ).getByRole("button", { name: "登录" });
  expect(screen.getAllByRole("button", { name: "登录" })).toHaveLength(1);
  login.focus();
  fireEvent.click(login);
  expect(await screen.findByRole("dialog", { name: "登录织光" })).toBeTruthy();
  expect(screen.queryByRole("dialog", { name: "导航" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "关闭登录注册弹窗" }));
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() => expect(document.activeElement).toBe(login));
});

it("手机顶栏保留主题与通知，积分入口不显示虚构会员促销", async () => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockImplementation(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  render(
    <BasicLayout screen="subjects">
      <h1>主体内容</h1>
    </BasicLayout>,
  );
  const header = await screen.findByRole("banner", { name: "页面顶栏" });
  expect(within(header).getByRole("button", { name: "搜索灵感" })).toBeTruthy();
  expect(within(header).getByRole("button", { name: "消息中心" })).toBeTruthy();
  expect(within(header).queryByRole("button", { name: "积分商城" })).toBeNull();
  expect(within(header).getByRole("button", { name: "账号菜单" })).toBeTruthy();
  expect(within(header).getByRole("button", { name: "切换主题" })).toBeTruthy();
  expect(within(header).queryByRole("button", { name: "更多操作" })).toBeNull();
  expect(within(header).queryByText("开通会员")).toBeNull();
  expect(within(header).queryByRole("button", { name: "积分余额" })).toBeNull();
  fireEvent.click(within(header).getByRole("button", { name: "消息中心" }));
  expect(
    await screen.findByRole("dialog", { name: "消息中心暂未开放" }),
  ).toBeTruthy();
});

it("空间读取期间保留顶栏和侧栏，只阻止正文私有内容", async () => {
  let finish!: (value: unknown) => void;
  const pending = new Promise((resolve) => {
    finish = resolve;
  });
  const original = vi.mocked(request).getMockImplementation()!;
  vi.mocked(request).mockImplementation((url, options) =>
    url === "/api/me/spaces" ? pending : original(url, options),
  );
  render(
    <BasicLayout screen="subjects">
      <p>private subject content</p>
    </BasicLayout>,
  );
  const header = await screen.findByRole("banner", { name: "页面顶栏" });
  expect(screen.getByRole("link", { name: "资产" })).toBeTruthy();
  expect(await screen.findByRole("status")).toHaveProperty(
    "textContent",
    "正在确认创作归属…",
  );
  expect(screen.queryByText("private subject content")).toBeNull();
  await act(async () =>
    finish({
      items: [
        {
          id: "33333333-3333-4333-8333-333333333333",
          kind: "personal",
          name: "个人空间",
          role: "creator",
          writable: true,
          team_id: "",
        },
      ],
    }),
  );
  await screen.findByText("private subject content");
  expect(screen.getByRole("banner", { name: "页面顶栏" })).toBe(header);
});

it("游客侧栏展开和收起均只保留顶栏登录入口", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("unauthenticated", 401),
  );
  render(
    <BasicLayout screen="explore" allowGuest>
      <p>公共探索</p>
    </BasicLayout>,
  );
  await screen.findByText("公共探索");
  const header = screen.getByRole("banner", { name: "页面顶栏" });
  expect(within(header).getByRole("button", { name: "登录" })).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "登录" })).toHaveLength(1);
  expect(screen.queryByText("登录后保存你的创作")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "收起导航" }));
  expect(screen.getAllByRole("button", { name: "登录" })).toHaveLength(1);
  expect(screen.getByRole("link", { name: "资产" })).toBeTruthy();
});

it("普通页面换页保留同一个页头与侧栏节点，并清理前一页搜索回调", async () => {
  const oldSearch = vi.fn();
  const nextSearch = vi.fn();
  const view = render(
    <ApplicationLayout>
      <PageLayout
        screen="home"
        search={{
          value: "旧内容",
          label: "搜索首页",
          onValueChange: vi.fn(),
          onSearch: oldSearch,
        }}
      >
        <h1>首页正文</h1>
      </PageLayout>
    </ApplicationLayout>,
  );
  const toggle = await screen.findByRole("button", { name: "收起导航" });
  fireEvent.click(toggle);
  const header = screen.getByRole("banner", { name: "页面顶栏" });
  const navigation = screen.getByRole("navigation", { name: "主导航" });
  expect(screen.getByRole("button", { name: "搜索首页" })).toBeTruthy();
  mocks.pathname = "/explore";
  view.rerender(
    <ApplicationLayout>
      <PageLayout
        screen="explore"
        search={{
          value: "新内容",
          label: "搜索探索",
          onValueChange: vi.fn(),
          onSearch: nextSearch,
        }}
      >
        <h1>探索正文</h1>
      </PageLayout>
    </ApplicationLayout>,
  );
  expect(await screen.findByRole("heading", { name: "探索正文" })).toBeTruthy();
  expect(screen.getByRole("banner", { name: "页面顶栏" })).toBe(header);
  expect(screen.getByRole("navigation", { name: "主导航" })).toBe(navigation);
  expect(screen.getByRole("button", { name: "展开导航" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "搜索探索" }));
  fireEvent.keyDown(await screen.findByRole("combobox"), { key: "Enter" });
  expect(nextSearch).toHaveBeenCalledWith("新内容");
  expect(oldSearch).not.toHaveBeenCalled();
});

it("管理页切换与路由加载保留同一导航节点和焦点，正文按页面独立替换", async () => {
  mocks.pathname = "/users";
  const view = render(
    <ApplicationLayout>
      <PageLayout screen="users">
        <h1>账号目录正文</h1>
      </PageLayout>
    </ApplicationLayout>,
  );
  const nav = await screen.findByRole("navigation", { name: "管理页面" });
  const teamLink = within(nav).getByRole("link", { name: "团队管理" });
  teamLink.focus();
  mocks.pathname = "/platform/teams";
  view.rerender(
    <ApplicationLayout>
      <RouteLoading />
    </ApplicationLayout>,
  );
  expect(screen.getByRole("navigation", { name: "管理页面" })).toBe(nav);
  expect(document.activeElement).toBe(teamLink);
  expect(teamLink.getAttribute("aria-current")).toBe("page");
  expect(screen.queryByRole("heading", { name: "账号目录正文" })).toBeNull();
  view.rerender(
    <ApplicationLayout>
      <PageLayout screen="platform/teams">
        <h1>团队目录正文</h1>
      </PageLayout>
    </ApplicationLayout>,
  );
  expect(
    await screen.findByRole("heading", { name: "团队目录正文" }),
  ).toBeTruthy();
  expect(screen.getAllByRole("navigation", { name: "管理页面" })).toEqual([
    nav,
  ]);
  expect(document.activeElement).toBe(teamLink);
  mocks.pathname = "/models";
  view.rerender(
    <ApplicationLayout>
      <PageLayout screen="models">
        <h1>模型目录正文</h1>
      </PageLayout>
    </ApplicationLayout>,
  );
  expect(screen.getByRole("navigation", { name: "管理页面" })).toBe(nav);
  expect(
    within(nav)
      .getByRole("link", { name: "模型管理" })
      .getAttribute("aria-current"),
  ).toBe("page");
  mocks.pathname = "/";
  view.rerender(
    <ApplicationLayout>
      <PageLayout screen="home">
        <h1>创作首页正文</h1>
      </PageLayout>
    </ApplicationLayout>,
  );
  expect(screen.queryByRole("navigation", { name: "管理页面" })).toBeNull();
});

it("asset header options stay stable under the shared login context", async () => {
  const fallback = await request("/api/session");
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/operations/history"
      ? { items: [], next_cursor: null }
      : fallback,
  );
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  render(
    <BasicLayout screen="assets">
      <AssetWorkspace />
    </BasicLayout>,
  );
  await screen.findByText("暂无相关资产");
  await act(async () => {
    await Promise.resolve();
  });
  expect(screen.getByRole("button", { name: "搜索本人生成历史" })).toBeTruthy();
  const header = screen.getByRole("banner", { name: "页面顶栏" });
  expect(
    within(header).getByRole("tablist", { name: "资产来源" }),
  ).toBeTruthy();
  expect(within(header).getByRole("button", { name: "批量操作" })).toBeTruthy();
  expect(within(header).getByRole("button", { name: "账号菜单" })).toBeTruthy();
  expect(within(header).queryByLabelText("资产筛选")).toBeNull();
  expect(screen.getByLabelText("资产筛选").closest("main")).toBeTruthy();
  expect(screen.getByRole("heading", { name: /^资产$/ }).className).toContain(
    "sr-only",
  );
  expect(
    errors.mock.calls.some((args) =>
      args.some((value) => String(value).includes("Maximum update depth")),
    ),
  ).toBe(false);
  errors.mockRestore();
});

function BatchHeaderPage({ batch }: { batch: boolean }) {
  return (
    <>
      <PageToolbar inHeader>
        <button>Page filter</button>
      </PageToolbar>
      <PageToolbar
        inHeader
        slot="actions"
        presentation={batch ? "replace-base" : "adjacent"}
      >
        <button>Page action</button>
      </PageToolbar>
    </>
  );
}

it("replacing public actions closes its search portal and releases shortcuts; unmount restores the shell", async () => {
  const search = {
    value: "",
    onValueChange: vi.fn(),
    label: "Header search",
    placeholder: "Search",
    onSearch: vi.fn(),
  };
  const view = render(
    <BasicLayout screen="subjects" search={search}>
      <BatchHeaderPage batch={false} />
    </BasicLayout>,
  );
  const header = await screen.findByRole("banner");
  await screen.findByRole("button", { name: "Header search" });
  fireEvent.keyDown(document, { key: "k", metaKey: true });
  expect(await screen.findByRole("combobox")).toBeTruthy();
  view.rerender(
    <BasicLayout screen="subjects" search={search}>
      <BatchHeaderPage batch />
    </BasicLayout>,
  );
  expect(screen.queryByRole("combobox")).toBeNull();
  expect(
    within(header).getByRole("button", { name: "Page filter" }),
  ).toBeTruthy();
  expect(
    within(header).queryByRole("button", { name: "Header search" }),
  ).toBeNull();
  fireEvent.keyDown(document, { key: "k", metaKey: true });
  expect(screen.queryByRole("combobox")).toBeNull();
  view.rerender(
    <BasicLayout screen="subjects" search={search}>
      <h1>Next page</h1>
    </BasicLayout>,
  );
  expect(
    within(header).queryByRole("button", { name: "Page action" }),
  ).toBeNull();
  fireEvent.keyDown(document, { key: "k", metaKey: true });
  expect(await screen.findByRole("combobox")).toBeTruthy();
  expect(screen.getAllByRole("combobox")).toHaveLength(1);
});
