import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AuthDialog } from "@/components/auth/auth-dialog";

vi.mock("@/components/auth/auth-view", () => ({
  AuthView: ({
    mode,
    onPendingChange,
  }: {
    mode: string;
    onPendingChange: (value: boolean) => void;
  }) => {
    return (
      <div>
        <label htmlFor="name">邮箱</label>
        <input id="name" />
        <button onClick={() => onPendingChange(true)}>提交{mode}</button>
      </div>
    );
  },
}));
afterEach(cleanup);

it("弹窗用底部入口切换邮箱认证，默认聚焦邮箱，提交期间锁定切换但允许关闭", () => {
  const close = vi.fn();
  render(<AuthDialog open onOpenChange={close} onAuthenticated={vi.fn()} />);
  expect(screen.getByRole("dialog", { name: "欢迎登录织光" })).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByLabelText("邮箱"));
  expect(screen.queryByRole("tablist")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "创建账号" }));
  expect(screen.getByRole("dialog", { name: "创建织光账号" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "提交register" }));
  expect(screen.getByRole("button", { name: "登录" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "关闭对话框" }));
  expect(close).toHaveBeenCalledWith(false);
});

it("直接注册入口打开相同弹窗，Escape 可以关闭", () => {
  const close = vi.fn();
  render(
    <AuthDialog
      open
      initialMode="register"
      onOpenChange={close}
      onAuthenticated={vi.fn()}
    />,
  );
  expect(screen.getByRole("dialog", { name: "创建织光账号" })).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(close).toHaveBeenCalledWith(false);
});
