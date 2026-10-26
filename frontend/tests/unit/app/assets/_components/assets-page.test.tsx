import type { ReactNode } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AssetsPage } from "@/app/assets/_components/assets-page";
import { request, RequestError } from "@/lib/request";

// Isolated transport tests. These responses never enter the running application.
const actor = "11111111-1111-4111-8111-111111111111";
const library = "22222222-2222-4222-8222-222222222222";
const state = vi.hoisted(() => ({
  session: null as { actor: { id: string } } | null,
  login: vi.fn(),
}));
vi.mock("@/components/layout/page-layout", () => ({
  PageLayout: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("@/components/layout/basic-layout", () => ({
  useLoginDialog: () => state.login,
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => state.session,
}));
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
const emptyPage = {
  current_actor_id: actor,
  current_org_id: library,
  library_id: library,
  scope: { kind: "personal" },
  revision: 1,
  page: 1,
  page_size: 12,
  total: 0,
  items: [],
  folders: [],
  folder_counts: {},
};
beforeEach(() => {
  vi.clearAllMocks();
  state.session = { actor: { id: actor } };
  vi.mocked(request).mockReset().mockResolvedValue(emptyPage);
});
afterEach(cleanup);

it("收藏预览中的素材后保持弹窗并更新操作状态", async () => {
  const item = {
    id: "33333333-3333-4333-8333-333333333333",
    title: "预览测试",
    kind: "image",
    revision: 1,
    favorite: false,
    catalog_state: "active",
    updated_at: "2026-10-23T00:00:00Z",
  };
  let saved = false;
  vi.mocked(request).mockImplementation(async (url) => {
    if (url.endsWith("/commands")) {
      saved = true;
      return {
        ...emptyPage,
        revision: 2,
        items: [{ id: item.id, revision: 2 }],
      };
    }
    return {
      ...emptyPage,
      revision: saved ? 2 : 1,
      total: 1,
      items: [{ ...item, revision: saved ? 2 : 1, favorite: saved }],
    };
  });
  render(<AssetsPage />);
  fireEvent.click(await screen.findByRole("button", { name: "查看 预览测试" }));
  fireEvent.click(screen.getByRole("button", { name: "收藏" }));
  await screen.findByRole("button", { name: "取消收藏" });
  expect(screen.getByRole("dialog")).toBeTruthy();
});

it("未登录保持素材库首位，不读取私人接口，登录由已有弹窗处理", () => {
  state.session = null;
  render(<AssetsPage />);
  expect(
    screen
      .getAllByRole("tab")
      .slice(0, 2)
      .map((t) => t.textContent),
  ).toEqual(["素材", "生成历史"]);
  expect(
    screen.getByRole("tab", { name: "素材" }).getAttribute("aria-selected"),
  ).toBe("true");
  expect(
    screen.getByRole("tab", { name: "素材" }).getAttribute("aria-controls"),
  ).toBe(screen.getByRole("tabpanel", { name: "素材" }).id);
  expect(screen.getByRole("radiogroup", { name: "素材类型" })).toBeTruthy();
  expect(request).not.toHaveBeenCalled();
  expect(
    screen.getByRole("button", { name: "上传素材" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "登录 / 注册" }));
  expect(state.login).toHaveBeenCalledOnce();
  expect(screen.queryByText("共 0 个素材")).toBeNull();
});

it("请求成功后才显示真实空库，最近30天参数发送到生成的客户端", async () => {
  render(<AssetsPage />);
  await screen.findByText("这里还没有素材");
  expect(request).toHaveBeenCalledWith(
    "/api/media/library",
    expect.objectContaining({
      params: expect.objectContaining({
        scope: "personal",
        kind: "image",
        page: 1,
        page_size: 12,
        order: "updated_desc",
      }),
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "最近30天" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/media/library",
      expect.objectContaining({
        params: expect.objectContaining({ recent_only: true }),
      }),
    ),
  );
});

it("读取失败与空库不同，重试重新访问真实接口", async () => {
  vi.mocked(request).mockRejectedValueOnce(
    new RequestError("unavailable", 503),
  );
  render(<AssetsPage />);
  await screen.findByText("无法读取素材");
  expect(screen.queryByText("这里还没有素材")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试读取素材" }));
  await screen.findByText("这里还没有素材");
  expect(request).toHaveBeenCalledTimes(2);
});

it("拒绝跨账号回执并在接口返回401时回到登录引导", async () => {
  vi.mocked(request).mockResolvedValueOnce({
    ...emptyPage,
    current_actor_id: library,
  });
  render(<AssetsPage />);
  await screen.findByText("无法读取素材");
  expect(screen.queryByText("这里还没有素材")).toBeNull();
  vi.mocked(request).mockRejectedValueOnce(
    new RequestError("unauthenticated", 401),
  );
  fireEvent.click(screen.getByRole("button", { name: "重试读取素材" }));
  await screen.findByText("登录查看你的素材");
  expect(screen.queryByText("共 0 个素材")).toBeNull();
});

it("生成历史要求选择项目后读取，不虚构跨项目记录", async () => {
  render(<AssetsPage />);
  await screen.findByText("这里还没有素材");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "生成历史" }), {
    button: 0,
    ctrlKey: false,
  });
  expect(await screen.findByText("选择项目，查看生成记录")).toBeTruthy();
  expect(screen.queryByText("还没有生成记录")).toBeNull();
});

it("创建文件夹携带库版本和幂等键，成功后重新读取列表", async () => {
  vi.mocked(request).mockImplementation(async (url) =>
    url.endsWith("/commands") ? { ...emptyPage, revision: 2 } : emptyPage,
  );
  render(<AssetsPage />);
  await screen.findByText("这里还没有素材");
  fireEvent.click(screen.getByRole("button", { name: "新建文件夹" }));
  fireEvent.change(screen.getByRole("textbox", { name: "文件夹名称" }), {
    target: { value: "品牌素材" },
  });
  fireEvent.click(screen.getByRole("button", { name: "创建文件夹" }));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/media/library/commands",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "Idempotency-Key": expect.any(String),
        }),
        data: expect.objectContaining({
          scope: { kind: "personal" },
          expected_revision: 1,
          action: "create_folder",
          folder: expect.objectContaining({ name: "品牌素材" }),
        }),
      }),
    ),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    vi
      .mocked(request)
      .mock.calls.filter(([url]) => url === "/api/media/library"),
  ).toHaveLength(2);
});

it("跨页读取清空本页批量选择，不把上一页素材带入下一次批量操作", async () => {
  const item = (n: number) => ({
    id: `33333333-3333-4333-8333-${String(n).padStart(12, "0")}`,
    title: `素材 ${n}`,
    kind: "image",
    revision: 1,
    favorite: false,
    catalog_state: "active",
    updated_at: "2026-10-19T00:00:00Z",
  });
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (url !== "/api/media/library") return {};
    const index = (options?.params as { page?: number })?.page ?? 1;
    return {
      ...emptyPage,
      page: index,
      total: 13,
      items:
        index === 1
          ? Array.from({ length: 12 }, (_, n) => item(n + 1))
          : [item(13)],
    };
  });
  render(<AssetsPage />);
  await screen.findByRole("button", { name: "查看 素材 1" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择本页全部素材" }));
  expect(screen.getByText("已选择 12 项")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await screen.findByRole("button", { name: "查看 素材 13" });
  expect(screen.queryByRole("button", { name: "查看 素材 1" })).toBeNull();
  expect(screen.getByText("已选择 0 项")).toBeTruthy();
  expect(screen.getByRole("button", { name: "移入回收站" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(request).toHaveBeenCalledWith(
    "/api/media/library",
    expect.objectContaining({
      params: expect.objectContaining({ page: 2, page_size: 12 }),
    }),
  );
});

it("每页数量选择发送真实分页参数，调整筛选时保留数量", async () => {
  render(<AssetsPage />);
  await screen.findByText("这里还没有素材");
  fireEvent.keyDown(
    screen.getByRole("combobox", { name: "素材分页每页数量" }),
    { key: "ArrowDown" },
  );
  fireEvent.click(await screen.findByRole("option", { name: "24" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/media/library",
      expect.objectContaining({
        params: expect.objectContaining({ page: 1, page_size: 24 }),
      }),
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "最近30天" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/media/library",
      expect.objectContaining({
        params: expect.objectContaining({
          page: 1,
          page_size: 24,
          recent_only: true,
        }),
      }),
    ),
  );
});

it("收藏筛选有明确关闭入口，返回保留类型和时间且不修改素材", async () => {
  render(<AssetsPage />);
  await screen.findByText("这里还没有素材");
  fireEvent.click(screen.getByRole("radio", { name: "视频" }));
  fireEvent.click(screen.getByRole("button", { name: "最近30天" }));
  fireEvent.click(screen.getByRole("button", { name: "收藏" }));
  expect(
    screen
      .getByRole("button", { name: "关闭收藏筛选" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  await screen.findByText("暂无匹配的收藏");
  fireEvent.click(screen.getByRole("button", { name: "返回素材" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/media/library",
      expect.objectContaining({
        params: expect.objectContaining({
          favorite_only: false,
          kind: "video",
          recent_only: true,
        }),
      }),
    ),
  );
  expect(screen.queryByRole("button", { name: "返回素材" })).toBeNull();
  expect(
    vi.mocked(request).mock.calls.every(([url]) => !url.endsWith("/commands")),
  ).toBe(true);
});

it("收藏加载中和失败后均可返回，重复点击收藏也可以关闭", async () => {
  render(<AssetsPage />);
  await screen.findByText("这里还没有素材");
  let fail!: (reason: unknown) => void;
  vi.mocked(request).mockImplementationOnce(
    () =>
      new Promise((_, reject) => {
        fail = reject;
      }),
  );
  fireEvent.click(screen.getByRole("button", { name: "收藏" }));
  expect(screen.getByRole("status", { name: "正在读取素材" })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "返回素材" }).hasAttribute("disabled"),
  ).toBe(false);
  fail(new RequestError("unavailable", 503));
  await screen.findByText("无法读取素材");
  fireEvent.click(screen.getByRole("button", { name: "关闭收藏筛选" }));
  await screen.findByText("这里还没有素材");
  expect(
    screen.getByRole("button", { name: "收藏" }).getAttribute("aria-pressed"),
  ).toBe("false");
});

it("从回收站切换收藏时恢复活动素材范围", async () => {
  render(<AssetsPage />);
  await screen.findByText("这里还没有素材");
  fireEvent.click(screen.getByRole("button", { name: "回收站" }));
  await screen.findByText("回收站为空");
  fireEvent.click(screen.getByRole("button", { name: "收藏" }));
  await waitFor(() =>
    expect(request).toHaveBeenLastCalledWith(
      "/api/media/library",
      expect.objectContaining({
        params: expect.objectContaining({
          favorite_only: true,
          catalog_state: "active",
        }),
      }),
    ),
  );
});
