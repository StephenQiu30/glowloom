import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AuthPage } from "./auth-pages";
import { UsersPage } from "./admin-pages";
import { StoryboardPage } from "./storyboard-pages";
import { BiblePage } from "./library-pages";

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
afterEach(cleanup);
beforeEach(() => vi.clearAllMocks());

it("注册时密码不一致不可提交，满足校验才进入演示首页", () => {
  render(<AuthPage mode="register" />);
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

it("设定集未确认筛选与搜索共同作用", () => {
  render(<BiblePage />);
  const list = screen.getByRole("navigation", { name: "设定条目" });
  fireEvent.click(screen.getByRole("switch", { name: "只看未确认" }));
  expect(within(list).queryByRole("button", { name: /^林舟/ })).toBeNull();
  fireEvent.change(screen.getByRole("textbox", { name: "搜索名称或别名" }), {
    target: { value: "周警官" },
  });
  expect(within(list).getAllByRole("button")).toHaveLength(1);
  fireEvent.click(within(list).getByRole("button", { name: /周警官/ }));
  expect(
    within(list)
      .getByRole("button", { name: /周警官/ })
      .getAttribute("aria-current"),
  ).toBe("true");
});

it("管理导航与搜索使用正式路由，关闭上下文栏仍保留主导航", () => {
  render(<UsersPage />);
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
