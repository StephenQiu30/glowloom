import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ThemeProvider } from "next-themes";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AccountMenu } from "./account-menu";

const actor: API.EmailSessionView["actor"] = {
  id: "11111111-1111-4111-8111-111111111111",
  email_verified_at: new Date().toISOString(),
  avatar_asset_id: "",
  status: "active",
  revision: "1",
  capabilities: [],
  display_name: "真实创作者",
  email: "creator@example.com",
  platform_role: "creator",
};
let mobile = false;

beforeEach(() => {
  mobile = false;
  localStorage.clear();
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: mobile && media.includes("max-width"),
    media,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
  }));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  document.documentElement.removeAttribute("class");
  document.documentElement.removeAttribute("style");
});

async function openMenu(value = actor, onLogout = vi.fn()) {
  render(
    <ThemeProvider attribute="class" defaultTheme="light" enableSystem>
      <AccountMenu actor={value} onLogout={onLogout} />
    </ThemeProvider>,
  );
  fireEvent.keyDown(screen.getByRole("button", { name: "账号菜单" }), {
    key: "ArrowDown",
  });
  await screen.findByRole("menu", { name: "账号菜单" });
  return onLogout;
}

it("展示当前账号摘要和真实入口，普通创作者不见管理入口", async () => {
  const onLogout = await openMenu();
  expect(screen.getByText(actor.display_name)).toBeTruthy();
  expect(screen.getByText(actor.email)).toBeTruthy();
  for (const [name, href] of [
    ["账号设置", "/account"],
    ["我的资产", "/assets"],
    ["我的画布", "/projects"],
    ["修改密码", "/reset-password"],
  ]) {
    expect(screen.getByRole("menuitem", { name }).getAttribute("href")).toBe(
      href,
    );
  }
  expect(screen.queryByRole("menuitem", { name: "用户管理" })).toBeNull();
  expect(onLogout).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("menuitem", { name: "退出登录" }));
  expect(onLogout).toHaveBeenCalledOnce();
});

it("仅向平台管理员提供管理入口", async () => {
  await openMenu({ ...actor, platform_role: "platform_admin" });
  expect(
    screen.getByRole("menuitem", { name: "用户管理" }).getAttribute("href"),
  ).toBe("/users");
});
it("历史 admin role 不授予平台管理入口", async () => {
  const legacy = { ...actor, role: "admin", login_name: "legacy-admin" };
  await openMenu(legacy);
  expect(screen.queryByRole("menuitem", { name: "用户管理" })).toBeNull();
  expect(screen.queryByText("legacy-admin")).toBeNull();
});

it("键盘进入外观子菜单，主题选择写入现有偏好", async () => {
  await openMenu();
  fireEvent.keyDown(screen.getByRole("menuitem", { name: "外观" }), {
    key: "ArrowRight",
  });
  const dark = await screen.findByRole("menuitemradio", { name: "深色" });
  fireEvent.click(dark);
  expect(localStorage.getItem("theme")).toBe("dark");
  expect(document.documentElement.classList.contains("dark")).toBe(true);
});

it("手机直接展示主题选项，保留全部账号操作", async () => {
  mobile = true;
  await openMenu();
  expect(screen.queryByRole("menuitem", { name: "外观" })).toBeNull();
  for (const name of ["浅色", "深色", "跟随系统"]) {
    expect(screen.getByRole("menuitemradio", { name })).toBeTruthy();
  }
  expect(screen.getByRole("menuitem", { name: "退出登录" })).toBeTruthy();
});

it("优先显示真实邮箱，缺失账号标识时仍能使用菜单", async () => {
  const emailActor = {
    ...actor,
    display_name: "邮箱创作者",
    email: "creator@example.com",
  };
  const view = render(<AccountMenu actor={emailActor} onLogout={vi.fn()} />);
  fireEvent.keyDown(screen.getByRole("button", { name: "账号菜单" }), {
    key: "ArrowDown",
  });
  expect(await screen.findByText("creator@example.com")).toBeTruthy();
  view.unmount();
  await openMenu({ ...actor, email: "" });
  expect(screen.getByText("账号信息暂不可用")).toBeTruthy();
});
