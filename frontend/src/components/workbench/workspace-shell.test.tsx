import { createElement, type ComponentProps } from "react";
import type { ImageProps } from "next/image";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WorkspaceShell } from "./workspace-shell";

const ports = vi.hoisted(() => ({ pathname: "/", push: vi.fn() }));
vi.mock("next/navigation", () => ({
  usePathname: () => ports.pathname,
  useRouter: () => ({ push: ports.push }),
}));
vi.mock("next/image", () => ({
  default: ({ src, alt }: ImageProps) =>
    createElement("img", {
      src: typeof src === "string" ? src : undefined,
      alt,
    }),
}));
vi.mock("next/link", () => ({
  default: ({ onClick, ...props }: ComponentProps<"a">) => (
    <a
      {...props}
      onClick={(event) => {
        onClick?.(event);
        event.preventDefault();
      }}
    />
  ),
}));
vi.mock("@/components/theme-toggle", () => ({
  ThemeToggle: () => <button aria-label="切换明暗主题" />,
}));

beforeEach(() => {
  vi.clearAllMocks();
  ports.pathname = "/";
});
afterEach(cleanup);
function setup() {
  return render(
    <WorkspaceShell>
      <h1>当前页面内容</h1>
    </WorkspaceShell>,
  );
}

it.each([
  ["/", "首页"],
  ["/projects", "项目"],
  ["/projects/d77d3c2a-7092-4e30-bf41-a3c5d4d26418", "项目"],
  ["/assets", "资产"],
  ["/tasks/task-03", "任务中心"],
])("主要导航在 %s 只标记当前 %s 页面", (pathname, active) => {
  ports.pathname = pathname;
  setup();
  const navigation = screen.getByRole("navigation", { name: "主要导航" });
  const links = within(navigation).getAllByRole("link");
  expect(
    links.filter((link) => link.getAttribute("aria-current") === "page"),
  ).toEqual([within(navigation).getByRole("link", { name: active })]);
  expect(screen.getByRole("main").getAttribute("id")).toBe("main-content");
  expect(
    screen.getByRole("link", { name: "跳转到主要内容" }).getAttribute("href"),
  ).toBe("#main-content");
});

it.each([
  ["/projects", "项目"],
  ["/assets", "资产"],
  ["/tasks", "任务中心"],
])("移动导航点击 %s 当前页面时仍关闭抽屉", (pathname, name) => {
  ports.pathname = pathname;
  setup();
  fireEvent.click(screen.getByRole("button", { name: "打开导航菜单" }));
  const drawer = screen.getByRole("dialog", { name: "工作台导航" });
  const current = within(drawer).getByRole("link", { name });
  expect(current.getAttribute("href")).toBe(pathname);
  expect(current.getAttribute("aria-current")).toBe("page");
  fireEvent.click(current);
  expect(screen.queryByRole("dialog", { name: "工作台导航" })).toBeNull();
  expect(ports.pathname).toBe(pathname);
  expect(screen.getByRole("heading", { name: "当前页面内容" })).toBeTruthy();
});

it("项目页的帮助创建链接在同一路径导航时主动关闭模态", () => {
  ports.pathname = "/projects";
  setup();
  fireEvent.click(screen.getByRole("button", { name: /^使用指南/ }));
  const help = screen.getByRole("dialog", { name: "开始你的创作" });
  expect(within(help).getAllByRole("listitem")).toHaveLength(3);
  const create = within(help).getByRole("link", { name: "创建第一个项目" });
  expect(create.getAttribute("href")).toBe("/projects?create=true");
  fireEvent.click(create);
  expect(screen.queryByRole("dialog", { name: "开始你的创作" })).toBeNull();
  expect(ports.pathname).toBe("/projects");
  fireEvent.click(screen.getByRole("button", { name: /^使用指南/ }));
  fireEvent.click(
    within(screen.getByRole("dialog", { name: "开始你的创作" })).getByRole(
      "button",
      { name: "Close" },
    ),
  );
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("正式项目画布保留独立全屏布局，不重复渲染工作台导航", () => {
  ports.pathname = "/projects/d77d3c2a-7092-4e30-bf41-a3c5d4d26418/canvas";
  setup();
  expect(screen.getByRole("heading", { name: "当前页面内容" })).toBeTruthy();
  expect(screen.queryByRole("navigation")).toBeNull();
  expect(screen.queryByRole("button", { name: "打开导航菜单" })).toBeNull();
});
