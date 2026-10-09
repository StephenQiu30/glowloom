import type { ReactNode } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UsersPage } from "./users-page";
const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));
vi.mock("sonner", () => ({
  toast: { success: mocks.success, error: mocks.error },
}));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
beforeEach(() => vi.clearAllMocks());
it("创建账号的无效输入保留弹窗，有效输入更新本地表格", () => {
  render(<UsersPage />);
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  const dialog = screen.getByRole("dialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "创建账号" }));
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(mocks.error).toHaveBeenCalledWith("请填写显示名与登录名");
  fireEvent.change(within(dialog).getByLabelText("显示名"), {
    target: { value: "演示导演" },
  });
  fireEvent.change(within(dialog).getByLabelText("登录名"), {
    target: { value: "demo.director" },
  });
  fireEvent.click(within(dialog).getByRole("button", { name: "创建账号" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByText("demo.director")).toBeTruthy();
});

// 本测试仅覆盖页面交互；真实身份守卫由product-shell.test.tsx与浏览器核验。
vi.mock("@/components/layout/product-shell", () => ({
  ProductShell: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
