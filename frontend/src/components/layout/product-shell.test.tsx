import {
  cleanup,
  fireEvent,
  render as renderUI,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ProductShell } from "./product-shell";
function render(children: ReactNode) {
  return renderUI(<TooltipProvider>{children}</TooltipProvider>);
}
const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => mocks }));
vi.mock("sonner", () => ({
  toast: { success: mocks.success, error: mocks.error },
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
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            actor: {
              id: "11111111-1111-4111-8111-111111111111",
              login_name: "admin",
              display_name: "Admin",
              role: "admin",
              must_change_password: false,
              credential_revision: 1,
            },
            session_id: "22222222-2222-4222-8222-222222222222",
            last_active_at: new Date().toISOString(),
            absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
            persistent: false,
          }),
          { status: 200 },
        ),
      ),
    ),
  );
});
it("R6 主导航与画布入口指向真实路由，支持收起和展开", async () => {
  render(
    <ProductShell screen="home">
      <h1>创作工作区</h1>
    </ProductShell>,
  );
  expect(
    (await screen.findByRole("link", { name: "创作" })).getAttribute("href"),
  ).toBe("/");
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
});

it("会话失效时不渲染工作台内容", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "session_invalid" }), {
        status: 401,
      }),
    ),
  );
  render(
    <ProductShell screen="home">
      <p>私有工作台内容</p>
    </ProductShell>,
  );
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/login"));
  expect(screen.queryByText("私有工作台内容")).toBeNull();
});

it("收起后保留可访问导航和当前页，重新挂载保持偏好", async () => {
  const view = render(
    <ProductShell screen="explore">
      <h1>探索内容</h1>
    </ProductShell>,
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
    <ProductShell screen="home">
      <h1>首页内容</h1>
    </ProductShell>,
  );
  expect(await screen.findByRole("button", { name: "展开导航" })).toBeTruthy();
});

it("快捷键切换导航，编辑输入时不抢占快捷键", async () => {
  render(
    <ProductShell screen="home">
      <input aria-label="提示词" />
    </ProductShell>,
  );
  await screen.findByRole("button", { name: "收起导航" });
  fireEvent.keyDown(screen.getByRole("textbox", { name: "提示词" }), {
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
    <ProductShell screen="home">
      <h1>手机首页</h1>
    </ProductShell>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "打开导航" }));
  const dialog = screen.getByRole("dialog", { name: "导航" });
  expect(within(dialog).getByRole("button", { name: "关闭导航" })).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("link", { name: "创作" }));
  expect(screen.queryByRole("dialog", { name: "导航" })).toBeNull();
  expect(document.cookie).toContain("sidebar_state=false");
});

it("游客从抽屉打开登录弹窗，关闭后返回移动导航入口", async () => {
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockImplementation(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "session_invalid" }), {
        status: 401,
      }),
    ),
  );
  render(
    <ProductShell screen="home" allowGuest>
      <h1>游客首页</h1>
    </ProductShell>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "打开导航" }));
  fireEvent.click(
    within(screen.getByRole("dialog", { name: "导航" })).getByRole("button", {
      name: "登录 / 注册",
    }),
  );
  expect(await screen.findByRole("dialog", { name: "登录织光" })).toBeTruthy();
  expect(screen.queryByRole("dialog", { name: "导航" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "关闭登录注册弹窗" }));
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "打开导航" }),
    ),
  );
});
