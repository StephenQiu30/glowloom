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
it("管理导航与搜索使用正式路由，关闭上下文栏仍保留主导航", () => {
  render(
    <ProductShell screen="users" contextual>
      <h1>账号</h1>
    </ProductShell>,
  );
  expect(screen.getByRole("link", { name: "项目" }).getAttribute("href")).toBe(
    "/",
  );
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
