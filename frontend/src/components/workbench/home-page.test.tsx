import { createElement, type ComponentProps } from "react";
import type { ImageProps } from "next/image";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { HomePage } from "./home-page";

const ports = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/components/project/queries", () => ({
  PROJECTS_KEY: ["projects"],
  listProjects: ports.list,
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

const project = {
  id: "d77d3c2a-7092-4e30-bf41-a3c5d4d26418",
  name: "逆光",
  aspect_ratio: "16:9",
  style_type: "realistic",
  status: "active",
  is_delete: false,
  revision: 1,
};
const clients: QueryClient[] = [];
function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <HomePage />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.resetAllMocks();
  ports.list.mockResolvedValue({ items: [project], next_cursor: null });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

it("最近项目读取活动项目并进入正式 UUID 画布，不捏造封面和日期", async () => {
  setup();
  const recent = screen.getByRole("region", { name: "最近项目" });
  const link = await within(recent).findByRole("link", { name: /逆光/ });
  expect(ports.list).toHaveBeenCalledExactlyOnceWith(
    { limit: 4, status: "active", deleted: false },
    expect.any(AbortSignal),
  );
  expect(link.getAttribute("href")).toBe(`/projects/${project.id}/canvas`);
  expect(within(recent).getByText("16:9 · 写实")).toBeTruthy();
  expect(recent.querySelector("img, video")).toBeNull();
  expect(recent.textContent).not.toMatch(/\d{4}-\d{2}-\d{2}|样例|最近更新/);
  expect(
    screen.getByRole("link", { name: "新建画布创作" }).getAttribute("href"),
  ).toBe("/projects?create=true");
});

it("项目读取失败保留指南和明确错误，只有点击重试后才恢复真实项目", async () => {
  ports.list.mockRejectedValueOnce(new ApiError(503, "dependency_unavailable"));
  setup();
  await screen.findByText("最近项目未能加载");
  expect(ports.list).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: /剧本原创与改编/ })).toBeTruthy();
  expect(screen.queryByRole("link", { name: /逆光/ })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试读取最近项目" }));
  await screen.findByRole("link", { name: /逆光/ });
  expect(ports.list).toHaveBeenCalledTimes(2);
  expect(screen.queryByText("最近项目未能加载")).toBeNull();
});

it("没有项目时显示真实创建入口，不补静态项目卡片", async () => {
  ports.list.mockResolvedValue({ items: [], next_cursor: null });
  setup();
  const recent = screen.getByRole("region", { name: "最近项目" });
  await within(recent).findByText("你的故事，从这里开始");
  expect(
    within(recent).getByRole("link", { name: "新建项目" }).getAttribute("href"),
  ).toBe("/projects?create=true");
  expect(within(recent).queryByRole("link", { name: /逆光|雾港/ })).toBeNull();
});

it("指南分类与去空白搜索组合筛选，清除后恢复全部指南", async () => {
  setup();
  await screen.findByRole("link", { name: /逆光/ });
  fireEvent.keyDown(screen.getByRole("tab", { name: "故事" }), {
    key: "Enter",
  });
  expect(
    screen.getByRole("tab", { name: "故事" }).getAttribute("aria-selected"),
  ).toBe("true");
  expect(screen.getByRole("button", { name: /剧本原创与改编/ })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /角色造型室/ })).toBeNull();
  const search = screen.getByRole("textbox", { name: "搜索创作指南" });
  fireEvent.change(search, { target: { value: "  角色  " } });
  expect(screen.getByText("没有匹配的创作指南")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "清除筛选" }));
  expect(search).toHaveProperty("value", "");
  expect(
    screen.getByRole("tab", { name: "全部" }).getAttribute("aria-selected"),
  ).toBe("true");
  expect(
    within(screen.getByRole("tabpanel")).getAllByRole("button"),
  ).toHaveLength(4);
  fireEvent.change(search, { target: { value: "  角色  " } });
  expect(screen.getByRole("button", { name: /角色造型室/ })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /剧本原创与改编/ })).toBeNull();
});

it("指南打开操作说明，关闭时恢复触发按钮焦点，创建链接也关闭模态", async () => {
  setup();
  await screen.findByRole("link", { name: /逆光/ });
  const trigger = screen.getByRole("button", { name: /剧本原创与改编/ });
  trigger.focus();
  fireEvent.click(trigger);
  const guide = screen.getByRole("dialog", { name: "剧本原创与改编" });
  expect(within(guide).getAllByRole("listitem")).toHaveLength(3);
  expect(
    within(guide).getByText("图片为创作方向示意，生成服务仍在准备中。"),
  ).toBeTruthy();
  fireEvent.click(within(guide).getByRole("button", { name: "Close" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(trigger);
  const create = within(
    screen.getByRole("dialog", { name: "剧本原创与改编" }),
  ).getByRole("link", { name: "从新项目开始" });
  expect(create.getAttribute("href")).toBe("/projects?create=true");
  fireEvent.click(create);
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("视频生成明确为准备中，提供可用路径且关闭后恢复服务按钮焦点", async () => {
  setup();
  await screen.findByRole("link", { name: /逆光/ });
  const trigger = within(
    screen.getByRole("navigation", { name: "创作服务" }),
  ).getByRole("button", { name: "视频生成" });
  trigger.focus();
  fireEvent.click(trigger);
  const service = screen.getByRole("dialog", { name: "视频生成" });
  expect(within(service).getByText("服务准备中")).toBeTruthy();
  expect(within(service).getByText(/暂不能提交生成任务/)).toBeTruthy();
  expect(
    within(service).queryByRole("button", { name: /提交|生成/ }),
  ).toBeNull();
  expect(
    within(service)
      .getByRole("link", { name: "进入创作画布" })
      .getAttribute("href"),
  ).toBe("/canvas");
  fireEvent.click(within(service).getByRole("button", { name: "Close" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(trigger);
  const create = within(
    screen.getByRole("dialog", { name: "视频生成" }),
  ).getByRole("link", { name: "新建项目" });
  expect(create.getAttribute("href")).toBe("/projects?create=true");
  fireEvent.click(create);
  expect(screen.queryByRole("dialog")).toBeNull();
});
