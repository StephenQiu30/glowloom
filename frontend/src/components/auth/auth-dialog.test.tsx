import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AuthDialog } from "./auth-dialog";

vi.mock("./auth-view", () => ({
  AuthView: ({
    mode,
    onContinueGuest,
    onPendingChange,
  }: {
    mode: string;
    onContinueGuest: () => void;
    onPendingChange: (value: boolean) => void;
  }) => {
    return (
      <div>
        <label htmlFor="name">邮箱</label>
        <input id="name" />
        <button onClick={onContinueGuest}>暂不登录，继续浏览</button>
        <button onClick={() => onPendingChange(true)}>提交{mode}</button>
      </div>
    );
  },
}));
afterEach(cleanup);

it("弹窗有可访问标题和关联面板，默认聚焦邮箱，提交期间锁定切换但允许继续浏览", () => {
  const close = vi.fn();
  render(<AuthDialog open onOpenChange={close} onAuthenticated={vi.fn()} />);
  expect(screen.getByRole("dialog", { name: "登录织光" })).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByLabelText("邮箱"));
  fireEvent.mouseDown(screen.getByRole("tab", { name: "注册" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(screen.getByRole("dialog", { name: "创建织光账号" })).toBeTruthy();
  expect(screen.getByRole("tabpanel").getAttribute("aria-labelledby")).toBe(
    screen.getByRole("tab", { name: "注册" }).id,
  );
  fireEvent.click(screen.getByRole("button", { name: "提交register" }));
  expect(screen.getByRole("tab", { name: "登录" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "暂不登录，继续浏览" }));
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
