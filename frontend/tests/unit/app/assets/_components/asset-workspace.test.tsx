import { useEffect, type ReactNode } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AssetWorkspace } from "@/app/assets/_components/asset-workspace";
import { assetDateRange } from "@/app/assets/_components/asset-query";
import { readAssetLocation } from "@/app/assets/_components/asset-location";
import { RequestError } from "@/lib/request";

const state = vi.hoisted(() => ({
  actor: "11111111-1111-4111-8111-111111111111" as string | null,
  scope: "22222222-2222-4222-8222-222222222222",
  history: vi.fn(),
  documentDownload: vi.fn(),
  historyCommand: vi.fn(),
  canvasCommand: vi.fn(),
  publish: vi.fn(),
  canvases: vi.fn(),
  create: vi.fn(),
  createAsset: vi.fn(),
  templates: vi.fn(),
  push: vi.fn(),
  login: vi.fn(),
  search: null as null | { onSearch: (value: string) => void },
  pickProject: null as null | ((value: { id: string }) => void),
}));
vi.mock("@/api/operations", () => ({
  listPersonalOperationHistory: state.history,
  applyPersonalHistoryCommand: state.historyCommand,
  publishHistoryResults: state.publish,
  getOperation: vi.fn(),
}));
vi.mock("@/api/canvases", () => ({
  listAssetCanvases: state.canvases,
  applyCanvasAssetCommand: state.canvasCommand,
  createCanvas: state.create,
  createAssetCanvas: state.createAsset,
  listCanvasAssetTemplates: state.templates,
}));
vi.mock("@/api/media", () => ({
  downloadDocumentAsset: state.documentDownload,
  downloadMediaOriginal: vi.fn(),
}));
vi.mock("@/components/layout/page-layout", () => ({
  PageLayout: ({
    children,
    search,
  }: {
    children: ReactNode;
    search: typeof state.search;
  }) => {
    state.search = search;
    return children;
  },
}));
vi.mock("@/components/layout/basic-layout", () => ({
  useLoginDialog: () => state.login,
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () =>
    state.actor ? { actor: { id: state.actor } } : null,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: { id: state.scope, writable: true },
    pending: false,
  }),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: state.push }) }));
vi.mock("@/components/projects/project-picker", () => ({
  ProjectPicker: ({ onChange }: { onChange: typeof state.pickProject }) => {
    state.pickProject = onChange;
    return <span>所属项目</span>;
  },
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => <span>媒体预览</span>,
}));
vi.mock("@/components/generation/generation-dialog", () => ({
  TaskDetail: ({
    onAvailabilityChange,
  }: {
    onAvailabilityChange?: (available: boolean) => void;
  }) => {
    useEffect(() => onAvailabilityChange?.(true), [onAvailabilityChange]);
    return <span>真实任务详情</span>;
  },
}));

beforeEach(() => {
  vi.resetAllMocks();
  window.history.replaceState({}, "", "/assets");
  state.pickProject = null;
  state.scope = "22222222-2222-4222-8222-222222222222";
  state.actor = "11111111-1111-4111-8111-111111111111";
  state.history.mockResolvedValue({ items: [], next_cursor: null });
  state.canvases.mockResolvedValue({ items: [], next_cursor: null });
  state.templates.mockResolvedValue({ items: [] });
});
afterEach(cleanup);

it("a bookmarked query restores its media, selected filters and server search on first read", async () => {
  const initialLocation = readAssetLocation(
    new URLSearchParams(
      "kind=video&q=山谷&favorite=1&resolutions=1080P&from=2026-10-01&through=2026-10-28&order=asc",
    ),
  );
  render(<AssetWorkspace initialLocation={initialLocation} />);
  await screen.findByText("当前已开启筛选，部分内容可能被隐藏");
  expect(state.history).toHaveBeenCalledWith(
    expect.objectContaining({
      kind: "video",
      search: "山谷",
      favorite_only: true,
      resolutions: "1080P",
      order: "asc",
      ...assetDateRange("2026-10-01", "2026-10-28"),
    }),
    expect.anything(),
  );
  expect(
    screen.getByRole("radio", { name: "视频" }).getAttribute("aria-checked"),
  ).toBe("true");
});

it("keyboard focus can switch asset-source tabs without losing the current source URL", async () => {
  render(<AssetWorkspace />);
  await screen.findByText("暂无相关资产");
  const historyTab = screen.getByRole("tab", { name: "生成历史" });
  act(() => historyTab.focus());
  fireEvent.keyDown(historyTab, { key: "ArrowRight" });
  await waitFor(() =>
    expect(state.canvases).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "canvas" }),
      expect.anything(),
    ),
  );
  expect(
    screen.getByRole("tab", { name: "画布" }).getAttribute("aria-selected"),
  ).toBe("true");
  expect(new URLSearchParams(location.search).get("view")).toBe("canvas");
});

it("media and legacy-canvas state survive URL navigation back to the asset source", async () => {
  render(<AssetWorkspace />);
  await screen.findByText("暂无相关资产");
  fireEvent.click(screen.getByRole("radio", { name: "视频" }));
  await waitFor(() =>
    expect(new URLSearchParams(location.search).get("kind")).toBe("video"),
  );
  fireEvent.mouseDown(screen.getByRole("tab", { name: "画布" }), {
    button: 0,
    ctrlKey: false,
  });
  await screen.findByText("暂无相关资产");
  fireEvent.click(screen.getByRole("radio", { name: "历史画布" }));
  await waitFor(() =>
    expect(new URLSearchParams(location.search).get("collection")).toBe(
      "legacy",
    ),
  );
  act(() => {
    window.history.replaceState({}, "", "/assets?kind=audio&q=配音");
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
  await waitFor(() =>
    expect(state.history.mock.calls.at(-1)?.[0]).toMatchObject({
      kind: "audio",
      search: "配音",
    }),
  );
  expect(
    screen.getByRole("radio", { name: "音频" }).getAttribute("aria-checked"),
  ).toBe("true");
});

it("date validation stays inline, associates the invalid input and returns keyboard focus", async () => {
  render(<AssetWorkspace />);
  await screen.findByText("暂无相关资产");
  fireEvent.click(screen.getByRole("button", { name: "时间" }));
  const from = screen.getByLabelText("开始日期");
  fireEvent.change(from, { target: { value: "2026-10-28" } });
  fireEvent.change(screen.getByLabelText("结束日期"), {
    target: { value: "2026-10-01" },
  });
  fireEvent.submit(from.closest("form")!);
  expect(await screen.findByText("开始日期不能晚于结束日期。")).toBeTruthy();
  expect(from.getAttribute("aria-invalid")).toBe("true");
  expect(document.activeElement).toBe(from);
});

const outputMetadata = {
  favorite: false,
  metadata_revision: 1,
  resolution: "",
  ratio: "",
  generation_mode: "",
  process_types: [],
  can_download: true,
  can_publish: false,
};

const task = {
  id: "33333333-3333-4333-8333-333333333333",
  project_id: "44444444-4444-4444-8444-444444444444",
  capability: "image.generate",
  status: "completed",
  model_name: "真实模型",
  create_time: "2026-10-28T01:00:00Z",
  prompt_preview: "本人的创作记录",
  history_revision: 3,
  outputs: [],
};

it("默认读取本人图片生成历史，原上传素材不冒充历史", async () => {
  render(<AssetWorkspace />);
  await screen.findByText("暂无相关资产");
  expect(state.history).toHaveBeenCalledWith(
    expect.objectContaining({ kind: "image", order: "desc" }),
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
  expect(screen.getAllByRole("tab").map((n) => n.textContent)).toEqual([
    "生成历史",
    "画布",
    "短剧",
  ]);
  expect(screen.getByRole("button", { name: "参考素材" })).toBeTruthy();
  expect(state.canvases).not.toHaveBeenCalled();
});

it("游客不读取私人接口，并使用既有登录入口", () => {
  state.actor = null;
  render(<AssetWorkspace />);
  expect(state.history).not.toHaveBeenCalled();
  expect(state.canvases).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "登录 / 注册" }));
  expect(state.login).toHaveBeenCalledOnce();
});

it("切换媒体和搜索时重新服务端筛选，不继续使用旧游标", async () => {
  state.history.mockResolvedValueOnce({ items: [task], next_cursor: "next" });
  render(<AssetWorkspace />);
  await screen.findByText("本人的创作记录");
  fireEvent.click(screen.getByRole("button", { name: "更多记录" }));
  await waitFor(() =>
    expect(state.history).toHaveBeenLastCalledWith(
      expect.objectContaining({ cursor: "next" }),
      expect.anything(),
    ),
  );
  fireEvent.click(screen.getByRole("radio", { name: "视频" }));
  await waitFor(() =>
    expect(state.history).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "video", cursor: undefined }),
      expect.anything(),
    ),
  );
  act(() => state.search?.onSearch("海边"));
  await waitFor(() =>
    expect(state.history).toHaveBeenLastCalledWith(
      expect.objectContaining({
        kind: "video",
        search: "海边",
        cursor: undefined,
      }),
      expect.anything(),
    ),
  );
});

it("错误与空账户分开，重试读取真实接口", async () => {
  state.history.mockRejectedValueOnce(new RequestError("unavailable", 503));
  render(<AssetWorkspace />);
  await screen.findByText("暂时无法读取生成历史");
  expect(screen.queryByText("暂无相关资产")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试读取" }));
  await screen.findByText("暂无相关资产");
  expect(state.history).toHaveBeenCalledTimes(2);
});

it("切到短剧只查询显式短剧分类，不用项目或视频列表冒充", async () => {
  render(<AssetWorkspace />);
  await screen.findByText("暂无相关资产");
  fireEvent.mouseDown(screen.getByRole("tab", { name: "短剧" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() =>
    expect(state.canvases).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "short_drama" }),
      expect.anything(),
    ),
  );
  expect(screen.queryByRole("radio", { name: "视频" })).toBeNull();
  expect(screen.queryByText("历史画布")).toBeNull();
});

it("同账号切换空间会取消旧请求，旧结果不能进入新空间", async () => {
  let finish: ((result: unknown) => void) | undefined;
  state.history.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const view = render(<AssetWorkspace />);
  const first = state.history.mock.calls[0][1].signal as AbortSignal;
  state.scope = "55555555-5555-4555-8555-555555555555";
  view.rerender(<AssetWorkspace />);
  await screen.findByText("暂无相关资产");
  expect(first.aborted).toBe(true);
  await act(async () => {
    finish?.({ items: [task], next_cursor: null });
  });
  expect(screen.queryByText("本人的创作记录")).toBeNull();
  state.scope = "22222222-2222-4222-8222-222222222222";
});

it("日期按本地日历日转成半开范围，拒绝反向和无效日期", () => {
  expect(assetDateRange("2026-10-01", "2026-10-28")).toEqual({
    created_from: new Date(2026, 9, 1).toISOString(),
    created_before: new Date(2026, 9, 29).toISOString(),
  });
  expect(() => assetDateRange("2026-10-29", "2026-10-28")).toThrow();
  expect(() => assetDateRange("2026-02-30", "")).toThrow();
});

it("文档历史摘要允许无全文的JSON候选，打开时才加载完整详情", async () => {
  state.history.mockResolvedValueOnce({
    items: [
      {
        ...task,
        capability: "text.structured",
        outputs: [
          {
            id: "66666666-6666-4666-8666-666666666666",
            kind: "json",
            ...outputMetadata,
            moderation_status: "passed",
            media_asset_id: null,
          },
        ],
      },
    ],
    next_cursor: null,
  });
  render(<AssetWorkspace />);
  await screen.findByText("本人的创作记录");
  expect(screen.queryByText("真实任务详情")).toBeNull();
  fireEvent.click(
    screen
      .getByRole("article", { name: "本人的创作记录" })
      .querySelector("button")!,
  );
  expect(await screen.findByText("真实任务详情")).toBeTruthy();
});

it("未通过审核的媒体身份不能被摘要用来加载预览", async () => {
  state.history.mockResolvedValueOnce({
    items: [
      {
        ...task,
        outputs: [
          {
            id: "66666666-6666-4666-8666-666666666666",
            kind: "image",
            ...outputMetadata,
            moderation_status: "pending",
            media_asset_id: "77777777-7777-4777-8777-777777777777",
          },
        ],
      },
    ],
    next_cursor: null,
  });
  render(<AssetWorkspace />);
  await screen.findByText("暂时无法读取生成历史");
  expect(screen.queryByText("媒体预览")).toBeNull();
  expect(screen.queryByText("暂无相关资产")).toBeNull();
});

it("画布卡片使用实际画布ID和项目上下文打开", async () => {
  state.canvases.mockResolvedValueOnce({
    items: [
      {
        id: task.id,
        project_id: task.project_id,
        name: "真实画布",
        kind: "canvas",
        revision: "7",
        created_at: task.create_time,
        updated_at: task.create_time,
        is_main: false,
        collection: "current",
        favorite: false,
        favorite_revision: "0",
        project_revision: 2,
        cover_asset_id: null,
        cover_unavailable: false,
        can_copy_project: true,
      },
    ],
    next_cursor: null,
  });
  render(<AssetWorkspace initialView="canvas" />);
  expect(
    (await screen.findByRole("link", { name: /真实画布/ })).getAttribute(
      "href",
    ),
  ).toBe(`/canvas?project_id=${task.project_id}&canvas_id=${task.id}`);
  expect(state.history).not.toHaveBeenCalled();
});

it("显式创建短剧，未知结果重试保持原命令和幂等键", async () => {
  state.create
    .mockRejectedValueOnce(new RequestError("unavailable", 503))
    .mockResolvedValueOnce({
      id: task.id,
      project_id: task.project_id,
      kind: "short_drama",
    });
  render(<AssetWorkspace initialView="short_drama" />);
  await screen.findByText("暂无相关资产");
  fireEvent.pointerDown(screen.getByRole("button", { name: "更多画布操作" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(
    await screen.findByRole("menuitem", { name: "添加到已有项目" }),
  );
  act(() => state.pickProject?.({ id: task.project_id }));
  fireEvent.click(screen.getByRole("button", { name: "创建" }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect(state.create.mock.calls[0][1]).toEqual({
    name: "未命名短剧",
    kind: "short_drama",
  });
  expect(
    screen.getByRole("textbox", { name: "名称" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await waitFor(() =>
    expect(state.push).toHaveBeenCalledWith(
      `/canvas?project_id=${task.project_id}&canvas_id=${task.id}`,
    ),
  );
  expect(state.create.mock.calls[1].slice(0, 2)).toEqual(
    state.create.mock.calls[0].slice(0, 2),
  );
  expect(state.create.mock.calls[1][2].headers).toEqual(
    state.create.mock.calls[0][2].headers,
  );
});

it("新画布查询不可用时不允许通过旧创建端点误建短剧", async () => {
  state.canvases.mockRejectedValueOnce(new RequestError("not_found", 404));
  render(<AssetWorkspace initialView="short_drama" />);
  await screen.findByText("暂时无法读取短剧");
  expect(
    screen
      .getByRole("button", { name: "新建短剧画布" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(state.create).not.toHaveBeenCalled();
});

it("短剧创建回执类别错误时保持原操作冻结，不假成功导航", async () => {
  state.create.mockResolvedValueOnce({
    id: task.id,
    project_id: task.project_id,
    kind: "canvas",
  });
  render(<AssetWorkspace initialView="short_drama" />);
  await screen.findByText("暂无相关资产");
  fireEvent.pointerDown(screen.getByRole("button", { name: "更多画布操作" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(
    await screen.findByRole("menuitem", { name: "添加到已有项目" }),
  );
  act(() => state.pickProject?.({ id: task.project_id }));
  fireEvent.click(screen.getByRole("button", { name: "创建" }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect(
    screen.getByRole("textbox", { name: "名称" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(state.push).not.toHaveBeenCalled();
});

const firstOutput = "66666666-6666-4666-8666-666666666666";
const secondOutput = "77777777-7777-4777-8777-777777777777";
const media = "88888888-8888-4888-8888-888888888888";
const multiTask = {
  ...task,
  outputs: [firstOutput, secondOutput].map((id) => ({
    ...outputMetadata,
    id,
    kind: "image",
    media_asset_id: media,
    moderation_status: "passed",
    can_publish: true,
  })),
};
function receipt(
  body: API.PersonalHistoryCommand,
  options: { headers: Record<string, string> },
  failLast = false,
) {
  return {
    request_id: options.headers["Idempotency-Key"],
    action: body.action,
    results: body.targets!.map((target, index) => ({
      ...target,
      output_id: target.output_id ?? null,
      ok: !(failLast && index === body.targets!.length - 1),
      code:
        failLast && index === body.targets!.length - 1
          ? "revision_conflict"
          : "applied",
      revision: 5,
      affected_output_ids: target.output_id ? [target.output_id] : [],
    })),
  };
}
it("多图结果分别计数，批量收藏部分失败后仅保留失败项并重读修订", async () => {
  state.history.mockResolvedValue({ items: [multiTask], next_cursor: null });
  state.historyCommand.mockImplementation(async (body, options) =>
    receipt(body, options, true),
  );
  render(<AssetWorkspace />);
  await screen.findByRole("article", { name: "本人的创作记录 · 结果2" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("button", { name: "全选" }));
  expect(screen.getByText("已选择2项内容")).toBeTruthy();
  expect(
    within(screen.getByRole("article", { name: "本人的创作记录 · 结果2" }))
      .getByRole("button")
      .getAttribute("aria-pressed"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "收藏" }));
  await screen.findByText("已选择1项内容");
  expect(state.historyCommand.mock.calls[0][0].targets).toEqual([
    { operation_id: task.id, output_id: firstOutput, expected_revision: 1 },
    { operation_id: task.id, output_id: secondOutput, expected_revision: 1 },
  ]);
  expect(
    screen
      .getByRole("checkbox", { name: "选择本人的创作记录 · 结果2" })
      .getAttribute("data-state"),
  ).toBe("checked");
  expect(state.history).toHaveBeenCalledTimes(2);
  expect(screen.getByLabelText("操作结果").textContent).toContain(
    "内容已更新，请重试此项。",
  );
  expect(screen.getByLabelText("操作结果").getAttribute("aria-live")).toBe(
    "polite",
  );
});
it("收藏未知结果冻结来源与原幂等键，重试不会改为新命令", async () => {
  state.history.mockResolvedValue({ items: [multiTask], next_cursor: null });
  state.historyCommand
    .mockRejectedValueOnce(new RequestError("unavailable", 503))
    .mockImplementation(async (body, options) => receipt(body, options));
  render(<AssetWorkspace />);
  fireEvent.click(
    await screen.findByRole("button", { name: "收藏本人的创作记录 · 结果1" }),
  );
  await screen.findByRole("button", { name: "重试原操作" });
  expect(
    screen.getByRole("tab", { name: "画布" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(state.historyCommand).toHaveBeenCalledTimes(2));
  expect(state.historyCommand.mock.calls[0][0]).toEqual(
    state.historyCommand.mock.calls[1][0],
  );
  expect(state.historyCommand.mock.calls[0][1].headers).toEqual(
    state.historyCommand.mock.calls[1][1].headers,
  );
});
it("没有输出的进行中记录不可收藏，删除只提交记录修订且必须确认", async () => {
  state.history.mockResolvedValue({
    items: [{ ...task, status: "submitted" }],
    next_cursor: null,
  });
  state.historyCommand.mockImplementation(async (body, options) =>
    receipt(body, options),
  );
  render(<AssetWorkspace />);
  await screen.findByRole("article", { name: task.prompt_preview });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("button", { name: "全选" }));
  expect(
    screen.getByRole("button", { name: "收藏" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  expect(state.historyCommand).not.toHaveBeenCalled();
  expect(screen.getByRole("dialog").textContent).toContain("任务执行、计费");
  fireEvent.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "删除" }),
  );
  await waitFor(() => expect(state.historyCommand).toHaveBeenCalled());
  expect(state.historyCommand.mock.calls[0][0].targets).toEqual([
    { operation_id: task.id, output_id: undefined, expected_revision: 3 },
  ]);
});
it("高级筛选按组发CSV，切换媒体清理不兼容条件", async () => {
  render(<AssetWorkspace />);
  await screen.findByText("暂无相关资产");
  fireEvent.pointerDown(screen.getByRole("button", { name: "筛选资产" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(await screen.findByRole("menuitemcheckbox", { name: "2K" }));
  await waitFor(() =>
    expect(state.history).toHaveBeenLastCalledWith(
      expect.objectContaining({ resolutions: "2K", cursor: undefined }),
      expect.anything(),
    ),
  );
  fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "4K" }));
  await waitFor(() =>
    expect(state.history).toHaveBeenLastCalledWith(
      expect.objectContaining({ resolutions: "2K,4K" }),
      expect.anything(),
    ),
  );
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  fireEvent.click(screen.getByRole("radio", { name: "音频" }));
  await waitFor(() =>
    expect(state.history).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: "audio", resolutions: undefined }),
      expect.anything(),
    ),
  );
});
it("历史画布查询绑定legacy而非归档，不提供旧画布新建", async () => {
  render(<AssetWorkspace initialView="canvas" />);
  await screen.findByText("暂无相关资产");
  fireEvent.click(screen.getByRole("radio", { name: "历史画布" }));
  await waitFor(() =>
    expect(state.canvases).toHaveBeenLastCalledWith(
      expect.objectContaining({ collection: "legacy" }),
      expect.anything(),
    ),
  );
  expect(screen.queryByRole("button", { name: "新建画布" })).toBeNull();
});
it("主画布删除明确项目影响，并通过统一命令携带项目CAS", async () => {
  state.canvases.mockResolvedValue({
    items: [
      {
        id: task.id,
        project_id: task.project_id,
        name: "主画布",
        kind: "canvas",
        revision: "7",
        created_at: task.create_time,
        updated_at: task.create_time,
        is_main: true,
        collection: "current",
        favorite: false,
        favorite_revision: "2",
        project_revision: 9,
        cover_asset_id: null,
        cover_unavailable: false,
        can_copy_project: true,
      },
    ],
    next_cursor: null,
  });
  state.canvasCommand.mockResolvedValue({
    results: [{ id: task.id, status: "succeeded", code: "applied" }],
    succeeded: 1,
    failed: 0,
  });
  render(<AssetWorkspace initialView="canvas" />);
  await screen.findByRole("link", { name: /主画布/ });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("button", { name: "全选" }));
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  expect(screen.getByRole("dialog").textContent).toContain("整个所属项目");
  fireEvent.click(
    within(screen.getByRole("dialog")).getByRole("button", {
      name: "确认删除",
    }),
  );
  await waitFor(() =>
    expect(state.canvasCommand).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "delete",
        items: [
          {
            id: task.id,
            expected_revision: "7",
            expected_favorite_revision: "2",
            expected_project_revision: 9,
          },
        ],
      }),
      expect.objectContaining({
        headers: { "Idempotency-Key": expect.any(String) },
      }),
    ),
  );
});
it("权限拒绝不会显示为空列表或可操作旧数据", async () => {
  state.history.mockRejectedValue(new RequestError("forbidden", 403));
  render(<AssetWorkspace />);
  await screen.findByText("无权查看此空间的生成历史");
  expect(screen.queryByText("暂无相关资产")).toBeNull();
  expect(
    screen.getByRole("button", { name: "批量操作" }).hasAttribute("disabled"),
  ).toBe(true);
});

it("默认新建直接创建当前空间项目与主画布，不依赖已有项目", async () => {
  state.createAsset.mockResolvedValue({
    id: task.id,
    project_id: task.project_id,
    kind: "short_drama",
    name: "created",
    revision: "1",
    project_revision: 1,
    collection: "current",
    template_key: "",
    is_template: false,
  });
  render(<AssetWorkspace initialView="short_drama" />);
  await screen.findByText("暂无相关资产");
  fireEvent.click(screen.getByRole("button", { name: "新建短剧画布" }));
  await waitFor(() =>
    expect(state.push).toHaveBeenCalledWith(
      `/canvas?project_id=${task.project_id}&canvas_id=${task.id}`,
    ),
  );
  expect(state.createAsset).toHaveBeenCalledWith(
    { kind: "short_drama", template_key: undefined },
    expect.objectContaining({
      headers: { "Idempotency-Key": expect.any(String) },
    }),
  );
  expect(state.pickProject).toBeNull();
  expect(state.create).not.toHaveBeenCalled();
});
it("系统模板有明确标记，使用时创建真实预填副本而不执行生成", async () => {
  state.templates.mockResolvedValue({
    items: [
      {
        key: "text-to-image-v1",
        kind: "canvas",
        name: "文字生图",
        description: "预填可编辑工作流",
        is_template: true,
        node_count: 3,
        requires_model: true,
      },
    ],
  });
  state.createAsset.mockResolvedValue({
    id: task.id,
    project_id: task.project_id,
    kind: "canvas",
    name: "created",
    revision: "1",
    project_revision: 1,
    collection: "current",
    template_key: "text-to-image-v1",
    is_template: false,
  });
  render(<AssetWorkspace initialView="canvas" />);
  fireEvent.click(await screen.findByRole("button", { name: "使用模板" }));
  await waitFor(() =>
    expect(state.createAsset).toHaveBeenCalledWith(
      { kind: "canvas", template_key: "text-to-image-v1" },
      expect.anything(),
    ),
  );
  expect(state.create).not.toHaveBeenCalled();
  expect(state.historyCommand).not.toHaveBeenCalled();
});
it("图片可选择200但收藏删除超过50禁用，超过7只禁用发布", async () => {
  const outputs = Array.from({ length: 51 }, (_, index) => ({
    ...multiTask.outputs[0],
    id: `99999999-9999-4999-8999-${String(index + 1).padStart(12, "0")}`,
  }));
  state.history.mockResolvedValue({
    items: [{ ...task, outputs }],
    next_cursor: null,
  });
  render(<AssetWorkspace />);
  await screen.findByText("本人的创作记录 · 结果51");
  const toolbar = within(screen.getByLabelText("生成历史操作"));
  fireEvent.click(toolbar.getByRole("button", { name: "批量操作" }));
  fireEvent.click(toolbar.getByRole("button", { name: "全选" }));
  expect(screen.getByText("已选择51项内容")).toBeTruthy();
  expect(
    toolbar.getByRole("button", { name: "收藏" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    toolbar.getByRole("button", { name: "删除" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    toolbar.getByRole("button", { name: "发布" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    toolbar.getByRole("button", { name: "下载" }).hasAttribute("disabled"),
  ).toBe(false);
  expect(screen.getByText("最多选择7项发布")).toBeTruthy();
});

it("direct creation aborts its old-scope request and never navigates from a late receipt", async () => {
  let finish: ((value: unknown) => void) | undefined;
  state.createAsset.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const view = render(<AssetWorkspace initialView="canvas" />);
  await screen.findByText("暂无相关资产");
  fireEvent.click(screen.getByRole("button", { name: "新建画布" }));
  await waitFor(() => expect(state.createAsset).toHaveBeenCalledOnce());
  const signal = state.createAsset.mock.calls[0][1].signal as AbortSignal;
  state.scope = "55555555-5555-4555-8555-555555555555";
  view.rerender(<AssetWorkspace initialView="canvas" />);
  await act(async () => {
    finish?.({
      id: task.id,
      project_id: task.project_id,
      kind: "canvas",
      collection: "current",
      name: "created",
      revision: "1",
      project_revision: 1,
      template_key: "",
      is_template: false,
    });
  });
  expect(signal.aborted).toBe(true);
  expect(state.push).not.toHaveBeenCalled();
});

it("original-only documents expose real original downloads in card, detail and batch, with conversion disabled", async () => {
  const createDescriptor = Object.getOwnPropertyDescriptor(
      URL,
      "createObjectURL",
    ),
    revokeDescriptor = Object.getOwnPropertyDescriptor(URL, "revokeObjectURL");
  Object.defineProperty(URL, "createObjectURL", {
    configurable: true,
    value: () => "blob:fixture-document",
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    configurable: true,
    value: () => {},
  });
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  try {
    state.history.mockResolvedValue({
      items: [
        {
          ...task,
          capability: "document.generate",
          outputs: [
            {
              ...outputMetadata,
              id: firstOutput,
              kind: "document",
              media_asset_id: media,
              moderation_status: "passed",
            },
          ],
        },
      ],
      next_cursor: null,
    });
    state.documentDownload.mockResolvedValue(
      new Blob(["original"], {
        type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      }),
    );
    render(<AssetWorkspace />);
    fireEvent.click(screen.getByRole("radio", { name: "文档" }));
    expect(
      (
        await screen.findByRole("button", { name: "下载格式 · 原件" })
      ).hasAttribute("disabled"),
    ).toBe(true);
    fireEvent.click(
      screen
        .getByRole("article", { name: task.prompt_preview })
        .querySelector("button")!,
    );
    const detail = within(screen.getByRole("dialog"));
    expect(
      detail
        .getByRole("button", { name: "下载格式 · 原件" })
        .hasAttribute("disabled"),
    ).toBe(true);
    fireEvent.click(detail.getByRole("button", { name: "下载原件" }));
    await waitFor(() =>
      expect(state.documentDownload).toHaveBeenCalledTimes(1),
    );
    await waitFor(() => expect(click).toHaveBeenCalledTimes(1));
    fireEvent.click(detail.getByRole("button", { name: "关闭对话框" }));
    fireEvent.pointerDown(
      screen.getByRole("button", { name: `更多 · ${task.prompt_preview}` }),
      { button: 0, ctrlKey: false },
    );
    fireEvent.click(await screen.findByRole("menuitem", { name: "下载原件" }));
    await waitFor(() => expect(click).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
    fireEvent.click(screen.getByRole("button", { name: "全选" }));
    fireEvent.click(screen.getByRole("button", { name: "下载原件" }));
    await waitFor(() => expect(click).toHaveBeenCalledTimes(3));
    expect(screen.getByLabelText("下载结果").textContent).toContain("原件");
  } finally {
    cleanup();
    click.mockRestore();
    if (createDescriptor)
      Object.defineProperty(URL, "createObjectURL", createDescriptor);
    else Reflect.deleteProperty(URL, "createObjectURL");
    if (revokeDescriptor)
      Object.defineProperty(URL, "revokeObjectURL", revokeDescriptor);
    else Reflect.deleteProperty(URL, "revokeObjectURL");
  }
});
