import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StoryboardPage } from "./storyboard-page";
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
it("分镜取消批选会禁用批量操作，重新勾选后恢复", () => {
  render(<StoryboardPage />);
  fireEvent.click(screen.getByRole("button", { name: "取消选择" }));
  expect(
    screen
      .getByRole("button", { name: "批量生成 0 项" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("checkbox", { name: "选择镜头 02-01" }));
  fireEvent.click(screen.getByRole("button", { name: "批量生成 1 项" }));
  expect(mocks.success).toHaveBeenCalledWith(
    "已为 1 个镜头创建演示任务（本地演示）",
  );
});

// 本测试仅覆盖页面交互；真实身份守卫由product-shell.test.tsx与浏览器核验。
vi.mock("@/components/layout/product-shell", () => ({
  ProductShell: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
