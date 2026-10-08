import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AuthView } from "./auth-view";
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
it("注册时密码不一致不可提交，满足校验才进入演示首页", () => {
  render(<AuthView mode="register" />);
  const submit = screen.getByRole("button", { name: "创建账号" });
  expect(submit.hasAttribute("disabled")).toBe(true);
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), {
    target: { value: "Preview12345" },
  });
  fireEvent.change(screen.getByLabelText("确认密码"), {
    target: { value: "different" },
  });
  expect(screen.getByRole("alert").textContent).toContain("不一致");
  expect(submit.hasAttribute("disabled")).toBe(true);
  fireEvent.change(screen.getByLabelText("确认密码"), {
    target: { value: "Preview12345" },
  });
  fireEvent.click(submit);
  expect(mocks.push).toHaveBeenCalledWith("/");
});
