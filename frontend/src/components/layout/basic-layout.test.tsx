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
import { TooltipProvider } from "@/components/ui/tooltip";
import { BasicLayout } from "./basic-layout";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/request")>();
  return { ...actual, request: vi.fn() };
});
function render(children: ReactNode) {
  return renderUI(
    <SessionProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </SessionProvider>,
  );
}
const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  usePathname: () => "/",
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
  expect(
    screen.getByRole("link", { name: "素材库" }).getAttribute("href"),
  ).toBe("/assets");
  expect(
    screen.getByRole("link", { name: "查看我的画布" }).getAttribute("href"),
  ).toBe("/projects");
  fireEvent.click(screen.getByRole("button", { name: "收起导航" }));
  expect(screen.getByRole("button", { name: "展开导航" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "展开导航" }));
  expect(screen.queryByRole("button", { name: "展开导航" })).toBeNull();
  const header = screen.getByRole("banner", { name: "页面顶栏" });
  expect(within(header).getByText("创作")).toBeTruthy();
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
  expect(
    screen.getByRole("link", { name: "素材库" }).getAttribute("href"),
  ).toBe("/assets");
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
  expect(
    within(navigation).queryByRole("button", { name: "登录 / 注册" }),
  ).toBeNull();
  expect(within(navigation).queryByText("登录后保存你的创作")).toBeNull();
  fireEvent.click(within(navigation).getByRole("button", { name: "关闭导航" }));
  const login = within(
    screen.getByRole("banner", { name: "页面顶栏" }),
  ).getByRole("button", { name: "登录 / 注册" });
  expect(screen.getAllByRole("button", { name: "登录 / 注册" })).toHaveLength(
    1,
  );
  login.focus();
  fireEvent.click(login);
  expect(await screen.findByRole("dialog", { name: "登录织光" })).toBeTruthy();
  expect(screen.queryByRole("dialog", { name: "导航" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "关闭登录注册弹窗" }));
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() => expect(document.activeElement).toBe(login));
});

it("手机顶栏提供独立主题入口，移除积分会员并保留通知反馈", async () => {
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
  expect(within(header).getByText("主体")).toBeTruthy();
  expect(within(header).getByRole("button", { name: "消息中心" })).toBeTruthy();
  expect(within(header).queryByRole("button", { name: "积分商城" })).toBeNull();
  expect(within(header).getByRole("button", { name: "账号菜单" })).toBeTruthy();
  expect(within(header).getByRole("button", { name: "切换主题" })).toBeTruthy();
  expect(within(header).queryByRole("button", { name: "更多操作" })).toBeNull();
  expect(within(header).queryByText("开通会员")).toBeNull();
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
  expect(screen.getByRole("link", { name: "素材库" })).toBeTruthy();
  expect(await screen.findByRole("status")).toHaveProperty(
    "textContent",
    "正在加载…",
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
  expect(
    within(header).getByRole("button", { name: "登录 / 注册" }),
  ).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "登录 / 注册" })).toHaveLength(
    1,
  );
  expect(screen.queryByText("登录后保存你的创作")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "收起导航" }));
  expect(screen.getAllByRole("button", { name: "登录 / 注册" })).toHaveLength(
    1,
  );
  expect(screen.getByRole("link", { name: "素材库" })).toBeTruthy();
});
