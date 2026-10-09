import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProductShell } from "./product-shell";
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
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
beforeEach(() => {
  vi.clearAllMocks();
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
it("管理导航与搜索使用正式路由，关闭上下文栏仍保留主导航", async () => {
  render(
    <ProductShell screen="users" contextual>
      <h1>账号</h1>
    </ProductShell>,
  );
  expect(
    (await screen.findByRole("link", { name: "项目" })).getAttribute("href"),
  ).toBe("/");
  expect(
    within(screen.getByRole("complementary"))
      .getByRole("link", { name: "供应商凭据" })
      .getAttribute("href"),
  ).toBe("/providers");
  fireEvent.click(screen.getByRole("button", { name: "切换上下文导航" }));
  expect(screen.queryByRole("complementary")).toBeNull();
  expect(screen.getByRole("link", { name: "项目" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  const dialog = screen.getByRole("dialog");
  fireEvent.change(
    within(dialog).getByRole("textbox", { name: "输入页面名称" }),
    { target: { value: "供应商" } },
  );
  expect(within(dialog).getAllByRole("link")).toHaveLength(1);
  expect(
    within(dialog)
      .getByRole("link", { name: "供应商凭据" })
      .getAttribute("href"),
  ).toBe("/providers");
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
