import { useState } from "react";
import {
  PageToolbarContext,
  PageActionsContext,
  type PageActionsRegistration,
} from "@/components/layout/page-toolbar";
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
import { UnifiedSubjectLibrary } from "@/components/subjects/unified-library";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  batch: vi.fn(),
  export: vi.fn(),
  scopeGeneration: 0,
  sessionGeneration: 0,
}));
vi.mock("@/lib/request", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/request")>()),
  getScopeGeneration: () => api.scopeGeneration,
  getSessionGeneration: () => api.sessionGeneration,
}));
vi.mock("@/api/subjects", () => ({
  listSubjects: api.list,
  batchSubjects: api.batch,
  exportSubjects: api.export,
}));
vi.mock("@/components/subjects/unified-media", () => ({
  UnifiedSubjectMedia: ({ label }: { label: string }) => (
    <span>{label}预览</span>
  ),
}));
vi.mock("@/components/subjects/unified-editor", () => ({
  UnifiedSubjectEditor: ({ initial }: { initial?: { name: string } }) => (
    <div role="dialog" aria-label="设置主体">
      {initial?.name}
    </div>
  ),
}));
const actorID = "11111111-1111-4111-8111-111111111111",
  scopeID = "22222222-2222-4222-8222-222222222222";
const first = {
  id: "33333333-3333-4333-8333-333333333333",
  owner_scope_id: scopeID,
  revision: 1,
  version_id: "44444444-4444-4444-8444-444444444444",
  name: "海边香氛",
  description: "描述",
  primary: {
    binding_id: "55555555-5555-4555-8555-555555555555",
    asset_id: "66666666-6666-4666-8666-666666666666",
    asset_revision: 1,
    kind: "image",
    mime_type: "image/png",
    file_name: "图片.png",
  },
  references: [],
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
};
const second = {
  ...first,
  id: "77777777-7777-4777-8777-777777777777",
  name: "纸飞机",
};
const page = {
  current_actor_id: actorID,
  current_scope_id: scopeID,
  page: 1,
  page_size: 20,
  total: 2,
  items: [first, second],
  capabilities: {
    intelligent_voice: {
      available: false,
      reason: "intelligent_voice_unavailable",
    },
  },
};
beforeEach(() => {
  vi.clearAllMocks();
  api.scopeGeneration = 0;
  api.sessionGeneration = 0;
  api.list.mockReset().mockResolvedValue(page);
});

it("同一帧重复下载只创建一次导出，失败保留所选版本", async () => {
  let reject!: (reason: unknown) => void;
  api.export.mockImplementation(
    () =>
      new Promise((_resolve, r) => {
        reject = r;
      }),
  );
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择主体 海边香氛" }));
  const download = screen.getByRole("button", { name: "下载" });
  // Dispatch both events before React can commit disabled state.
  download.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  download.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  expect(api.export).toHaveBeenCalledOnce();
  reject(new Error("network"));
  await screen.findByRole("alert");
  expect(screen.getByText("已选择 1 项内容")).toBeTruthy();
  expect(api.export.mock.calls[0][0]).toEqual({
    items: [{ id: first.id, version_id: first.version_id }],
  });
});

it("导出等待时切换空间，晚到Blob不得触发下载", async () => {
  let resolve!: (value: Blob) => void;
  api.export.mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const create = vi.fn(() => "blob:export");
  const previous = URL.createObjectURL;
  URL.createObjectURL = create;
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  try {
    render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
    await screen.findByRole("button", { name: "设置主体 海边香氛" });
    fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
    fireEvent.click(
      screen.getByRole("checkbox", { name: "选择主体 海边香氛" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "下载" }));
    api.scopeGeneration++;
    resolve(new Blob(["fixture"], { type: "application/zip" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "下载" })).toHaveProperty(
        "disabled",
        false,
      ),
    );
    expect(create).not.toHaveBeenCalled();
    expect(click).not.toHaveBeenCalled();
  } finally {
    URL.createObjectURL = previous;
    click.mockRestore();
  }
});
afterEach(cleanup);
it("搜索结果禁用排序并展示创建时间", async () => {
  render(
    <UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} query="海边" />,
  );
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  expect(screen.getByRole("button", { name: "排序" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getAllByText(/主体 · .*创建/)).toHaveLength(2);
});

it("改变筛选退出批量，普通卡片勾选直接进入批量", async () => {
  render(
    <UnifiedSubjectLibrary
      actorID={actorID}
      scopeID={scopeID}
      toolbarInHeader
    />,
  );
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择主体 海边香氛" }));
  expect(screen.getByText("已选择 1 项内容")).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("button", { name: "筛选" }), {
    key: "ArrowDown",
  });
  fireEvent.click(
    await screen.findByRole("menuitemcheckbox", { name: "收藏" }),
  );
  await waitFor(() =>
    expect(screen.queryByText(/已选择 .* 项内容/)).toBeNull(),
  );
  expect(screen.getByRole("button", { name: "批量操作" })).toBeTruthy();
});
it("当前空间无需项目就读取完整库，搜索传给服务端", async () => {
  const view = render(
    <UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />,
  );
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  expect(api.list.mock.calls[0][0]).toMatchObject({
    page: 1,
    page_size: 20,
    order: "updated_desc",
  });
  expect(screen.queryByText("角色")).toBeNull();
  view.rerender(
    <UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} query="香氛" />,
  );
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)![0]).toMatchObject({
      q: "香氛",
      page: 1,
    }),
  );
});

it.each([
  ["修改时间", "近-远", "updated_desc", "updated"],
  ["修改时间", "远-近", "updated_asc", "updated"],
  ["创建时间", "近-远", "created_desc", "created"],
  ["创建时间", "远-近", "created_asc", "created"],
])("四排序%s/%s使用服务器合同", async (field, direction, order, timeField) => {
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  for (const label of [field, direction]) {
    fireEvent.keyDown(screen.getByRole("button", { name: "排序" }), {
      key: "ArrowDown",
    });
    fireEvent.click(await screen.findByRole("menuitemradio", { name: label }));
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  }
  await waitFor(() =>
    expect(api.list.mock.calls.at(-1)![0]).toMatchObject({
      order,
      time_field: timeField,
      page: 1,
    }),
  );
  expect(
    screen.getAllByText(
      timeField === "created" ? /主体 · .*创建/ : /主体 · .*修改/,
    ),
  ).toHaveLength(2);
});

it("取消删除不发送命令并保留明确选中的主体", async () => {
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择主体 海边香氛" }));
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  const dialog = await screen.findByRole("alertdialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(screen.getByText("已选择 1 项内容")).toBeTruthy();
  expect(api.batch).not.toHaveBeenCalled();
});
it("批量部分失败保留失败选择，不能显示全部成功", async () => {
  api.batch.mockResolvedValue({
    items: [
      { id: first.id, status: "succeeded" },
      { id: second.id, status: "failed", code: "revision_conflict" },
    ],
  });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  expect(screen.getByRole("button", { name: "删除" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: "选择主体 海边香氛" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择主体 纸飞机" }));
  fireEvent.click(screen.getByRole("button", { name: "收藏" }));
  await screen.findByText("已选择 1 项内容");
  expect(screen.getByRole("alert").textContent).toContain(
    "纸飞机（内容已更新，请重新选择）",
  );
  await screen.findByRole("checkbox", { name: "选择主体 纸飞机" });
  expect(
    screen
      .getByRole("checkbox", { name: "选择主体 纸飞机" })
      .getAttribute("aria-checked"),
  ).toBe("true");
});
it("未知批量结果沿同一幂等键重试", async () => {
  api.batch
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({ items: [{ id: first.id, status: "succeeded" }] });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "收藏 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "收藏 海边香氛" }));
  fireEvent.click(await screen.findByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(api.batch).toHaveBeenCalledTimes(2));
  expect(api.batch.mock.calls[1][0]).toEqual(api.batch.mock.calls[0][0]);
  expect(api.batch.mock.calls[1][1].headers["Idempotency-Key"]).toBe(
    api.batch.mock.calls[0][1].headers["Idempotency-Key"],
  );
});

it("删除结果未知时在确认框内可重试同一请求，避免锁死在遮罩后", async () => {
  api.batch
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({ items: [{ id: first.id, status: "succeeded" }] });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择主体 海边香氛" }));
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  const dialog = screen.getByRole("alertdialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "删除" }));
  fireEvent.click(
    await within(dialog).findByRole("button", { name: "重试原删除" }),
  );
  await waitFor(() => expect(api.batch).toHaveBeenCalledTimes(2));
  expect(api.batch.mock.calls[1][1].headers["Idempotency-Key"]).toBe(
    api.batch.mock.calls[0][1].headers["Idempotency-Key"],
  );
});

function HeaderLibraryHarness({
  embedded = false,
  show = true,
}: {
  embedded?: boolean;
  show?: boolean;
}) {
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const [actions, setActions] = useState<HTMLDivElement | null>(null);
  const [registration, setPresentation] =
    useState<PageActionsRegistration | null>(null);
  return (
    <>
      <header aria-label="页面顶栏">
        <div ref={setHost} />
        <div ref={setActions} />
        {registration?.presentation !== "replace-base" ? (
          <button>账号菜单</button>
        ) : null}
      </header>
      <PageActionsContext.Provider value={{ target: actions, setPresentation }}>
        <PageToolbarContext.Provider value={host}>
          <main>
            {show ? (
              <UnifiedSubjectLibrary
                actorID={actorID}
                scopeID={scopeID}
                toolbarInHeader={!embedded}
              />
            ) : (
              <h1>探索</h1>
            )}
          </main>
        </PageToolbarContext.Provider>
      </PageActionsContext.Provider>
    </>
  );
}

it("主体筛选和批量选择共用顶栏，保留业务状态且路由卸载清除工具", async () => {
  const view = render(<HeaderLibraryHarness />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  const header = screen.getByRole("banner", { name: "页面顶栏" });
  expect(within(header).getByRole("button", { name: "筛选" })).toBeTruthy();
  expect(
    within(screen.getByRole("main")).queryByRole("button", { name: "筛选" }),
  ).toBeNull();
  fireEvent.click(within(header).getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择主体 海边香氛" }));
  expect(within(header).getByText("已选择 1 项内容")).toBeTruthy();
  expect(within(header).getByRole("button", { name: "筛选" })).toBeTruthy();
  expect(within(header).queryByRole("button", { name: "账号菜单" })).toBeNull();
  expect(
    within(header)
      .getByRole("button", { name: "删除" })
      .hasAttribute("disabled"),
  ).toBe(false);
  fireEvent.click(within(header).getByRole("button", { name: "取消选择" }));
  expect(within(header).getByRole("button", { name: "排序" })).toBeTruthy();
  await waitFor(() =>
    expect(document.activeElement).toBe(
      within(header).getByRole("button", { name: "批量操作" }),
    ),
  );
  expect(
    screen
      .queryByRole("checkbox", { name: "选择主体 海边香氛" })
      ?.getAttribute("aria-checked"),
  ).toBe("false");
  view.rerender(<HeaderLibraryHarness show={false} />);
  expect(within(header).queryByRole("button", { name: "筛选" })).toBeNull();
  expect(within(header).getByRole("button", { name: "账号菜单" })).toBeTruthy();
});

it("嵌入式主体选择器即使存在顶栏宿主也保留内联工具", async () => {
  render(<HeaderLibraryHarness embedded />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  expect(
    within(screen.getByRole("main")).getByRole("button", { name: "筛选" }),
  ).toBeTruthy();
  expect(
    within(screen.getByRole("banner")).queryByRole("button", { name: "筛选" }),
  ).toBeNull();
});

it("嵌入式主体库退出批量后恢复入口焦点", async () => {
  render(<HeaderLibraryHarness embedded />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.click(screen.getByRole("button", { name: "取消选择" }));
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "批量操作" }),
    ),
  );
});

const observers = new Map<Element, IntersectionObserverCallback>();
beforeEach(() => {
  observers.clear();
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(private callback: IntersectionObserverCallback) {}
      observe(target: Element) {
        observers.set(target, this.callback);
      }
      disconnect() {
        observers.clear();
      }
    },
  );
});
afterEach(() => vi.unstubAllGlobals());
function intersectTail(times = 1) {
  expect(observers.size).toBe(1);
  act(() => {
    for (let i = 0; i < times; i++)
      for (const [target, callback] of observers)
        callback(
          [
            {
              target,
              isIntersecting: true,
              intersectionRatio: 1,
            } as IntersectionObserverEntry,
          ],
          {} as IntersectionObserver,
        );
  });
}

it("触底按20条追加，重复触发不并发，末批耗尽后无分页或结束提示", async () => {
  let resolve!: (value: unknown) => void;
  api.list.mockResolvedValueOnce({ ...page, total: 21, items: [first] });
  api.list.mockImplementationOnce(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  intersectTail(2);
  expect(
    screen.getByRole("button", { name: "设置主体 海边香氛" }),
  ).toBeTruthy();
  expect(api.list).toHaveBeenCalledTimes(2);
  expect(api.list.mock.calls[1][0]).toMatchObject({ page: 2, page_size: 20 });
  await act(async () =>
    resolve({ ...page, page: 2, total: 21, items: [first, second] }),
  );
  expect(
    screen.getAllByRole("button", { name: "设置主体 海边香氛" }),
  ).toHaveLength(1);
  expect(screen.getByRole("button", { name: "设置主体 纸飞机" })).toBeTruthy();
  expect(observers.size).toBe(0);
  expect(screen.queryByLabelText("主体分页")).toBeNull();
  expect(screen.queryByText("已经到底")).toBeNull();
});

it("追加失败保留卡片，点击重试请求同一批次", async () => {
  api.list.mockResolvedValueOnce({ ...page, total: 21, items: [first] });
  api.list.mockRejectedValueOnce(new Error("network"));
  api.list.mockResolvedValueOnce({
    ...page,
    page: 2,
    total: 21,
    items: [second],
  });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  intersectTail();
  fireEvent.click(await screen.findByRole("button", { name: "重试加载" }));
  await screen.findByRole("button", { name: "设置主体 纸飞机" });
  expect(
    screen.getByRole("button", { name: "设置主体 海边香氛" }),
  ).toBeTruthy();
  expect(api.list.mock.calls.map((call) => call[0].page)).toEqual([1, 2, 2]);
});

it("追加中搜索变化取消请求，晚到旧批次不能污染新查询", async () => {
  let resolve!: (value: unknown) => void;
  api.list.mockResolvedValueOnce({ ...page, total: 21, items: [first] });
  api.list.mockImplementationOnce(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  api.list.mockResolvedValueOnce({ ...page, total: 1, items: [second] });
  const view = render(
    <UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />,
  );
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  intersectTail();
  view.rerender(
    <UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} query="纸" />,
  );
  await screen.findByRole("button", { name: "设置主体 纸飞机" });
  expect(api.list.mock.calls[1][1].signal.aborted).toBe(true);
  await act(async () => resolve({ ...page, page: 2, items: [first] }));
  expect(
    screen.queryByRole("button", { name: "设置主体 海边香氛" }),
  ).toBeNull();
});

it("空追加批次停止观察，避免总数变化导致自动请求死循环", async () => {
  api.list.mockResolvedValueOnce({ ...page, total: 80, items: [first] });
  api.list.mockResolvedValueOnce({ ...page, page: 2, total: 80, items: [] });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  intersectTail();
  await waitFor(() => expect(observers.size).toBe(0));
  expect(api.list).toHaveBeenCalledTimes(2);
  expect(
    screen.getByRole("button", { name: "设置主体 海边香氛" }),
  ).toBeTruthy();
});

it("offset并发移动造成整批重复时继续下一批，不提前隐藏后续主体", async () => {
  api.list.mockResolvedValueOnce({ ...page, total: 41, items: [first] });
  api.list.mockResolvedValueOnce({
    ...page,
    page: 2,
    total: 41,
    items: [first],
  });
  api.list.mockResolvedValueOnce({
    ...page,
    page: 3,
    total: 41,
    items: [second],
  });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  intersectTail();
  await waitFor(() => expect(observers.size).toBe(1));
  intersectTail();
  await screen.findByRole("button", { name: "设置主体 纸飞机" });
  expect(
    screen.getAllByRole("button", { name: "设置主体 海边香氛" }),
  ).toHaveLength(1);
  expect(api.list.mock.calls.map((call) => call[0].page)).toEqual([1, 2, 3]);
});

it("追加后的收藏写入刷新首批，不遗留旧版本和原排序", async () => {
  api.list.mockResolvedValueOnce({ ...page, total: 21, items: [first] });
  api.list.mockResolvedValueOnce({
    ...page,
    page: 2,
    total: 21,
    items: [second],
  });
  api.list.mockResolvedValueOnce({
    ...page,
    total: 1,
    items: [{ ...second, favorite: true, revision: 2 }],
  });
  api.batch.mockResolvedValue({
    items: [{ id: second.id, status: "succeeded" }],
  });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  intersectTail();
  fireEvent.click(await screen.findByRole("button", { name: "收藏 纸飞机" }));
  await screen.findByRole("button", { name: "取消收藏 纸飞机" });
  expect(
    screen.queryByRole("button", { name: "设置主体 海边香氛" }),
  ).toBeNull();
  expect(api.list.mock.calls.map((call) => call[0].page)).toEqual([1, 2, 1]);
});

it.each([false, true])(
  "右键主体卡片显示同一管理动作且设置打开该主体，收藏=%s",
  async (favorite) => {
    api.list.mockResolvedValue({ ...page, items: [{ ...first, favorite }] });
    render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
    const card = await screen.findByRole("button", {
      name: "设置主体 海边香氛",
    });
    fireEvent.contextMenu(card, { clientX: 100, clientY: 120 });
    const menu = await screen.findByRole("menu");
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["设置主体", "下载", favorite ? "取消收藏" : "收藏", "删除"]);
    fireEvent.click(within(menu).getByRole("menuitem", { name: "设置主体" }));
    expect(
      (await screen.findByRole("dialog", { name: "设置主体" })).textContent,
    ).toBe(first.name);
    expect(api.batch).not.toHaveBeenCalled();
    expect(api.export).not.toHaveBeenCalled();
  },
);

it("批量模式右键不弹主体管理菜单", async () => {
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "批量操作" }));
  fireEvent.contextMenu(
    screen.getByRole("button", { name: "选择主体 海边香氛" }),
  );
  expect(screen.queryByRole("menu")).toBeNull();
});

it("引用模式右键不弹主体管理菜单且不误采纳", async () => {
  const select = vi.fn();
  render(
    <UnifiedSubjectLibrary
      actorID={actorID}
      scopeID={scopeID}
      onSelect={select}
    />,
  );
  fireEvent.contextMenu(
    await screen.findByRole("button", { name: "引用主体 海边香氛" }),
  );
  expect(screen.queryByRole("menu")).toBeNull();
  expect(select).not.toHaveBeenCalled();
});

it("写入锁定时右键不弹主体管理菜单", async () => {
  api.batch.mockReturnValue(new Promise(() => {}));
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  const card = await screen.findByRole("button", { name: "设置主体 海边香氛" });
  fireEvent.click(screen.getByRole("button", { name: "收藏 海边香氛" }));
  expect(card).toHaveProperty("disabled", true);
  fireEvent.contextMenu(card.closest("article")!);
  expect(screen.queryByRole("menu")).toBeNull();
});

it("搜索无结果只呈现竞品单行空态，不增加操作指导", async () => {
  api.list.mockResolvedValue({ ...page, items: [], total: 0 });
  render(
    <UnifiedSubjectLibrary
      actorID={actorID}
      scopeID={scopeID}
      query="不存在"
    />,
  );
  expect(
    await screen.findByRole("heading", { name: "暂无相关资产" }),
  ).toBeTruthy();
  expect(screen.queryByText("调整筛选条件后重试。")).toBeNull();
  expect(screen.queryByRole("button", { name: "新建主体" })).toBeNull();
});

it.each(["", "不存在"])(
  "收藏无结果优先显示筛选提示，搜索=%s",
  async (query) => {
    api.list.mockResolvedValue({ ...page, items: [], total: 0 });
    render(
      <UnifiedSubjectLibrary
        actorID={actorID}
        scopeID={scopeID}
        query={query}
      />,
    );
    await screen.findByRole("button", { name: "筛选" });
    fireEvent.keyDown(screen.getByRole("button", { name: "筛选" }), {
      key: "ArrowDown",
    });
    fireEvent.click(
      await screen.findByRole("menuitemcheckbox", { name: "收藏" }),
    );
    expect(
      await screen.findByRole("heading", {
        name: "当前已开启筛选，部分内容可能被隐藏",
      }),
    ).toBeTruthy();
    expect(screen.queryByText("调整筛选条件后重试。")).toBeNull();
    expect(screen.queryByRole("heading", { name: "暂无相关资产" })).toBeNull();
    expect(screen.getByRole("button", { name: "收藏" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "清空筛选" }));
    expect(screen.getByRole("button", { name: "筛选" })).toBeTruthy();
    await waitFor(() =>
      expect(api.list.mock.lastCall?.[0].favorite).toBeUndefined(),
    );
  },
);

it("仅日期筛选无结果保留新建卡，清空收藏不影响日期", async () => {
  api.list.mockResolvedValue({ ...page, items: [], total: 0 });
  render(<UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />);
  fireEvent.click(await screen.findByRole("button", { name: "时间" }));
  fireEvent.click(await screen.findByRole("button", { name: "最近一周" }));
  await waitFor(() => expect(api.list.mock.lastCall?.[0].from).toBeTruthy());
  const from = api.list.mock.lastCall?.[0].from;
  expect(screen.getByRole("button", { name: "新建主体" })).toBeTruthy();
  expect(screen.queryByText("此时间范围内没有主体")).toBeNull();
  expect(screen.queryByText("调整筛选条件后重试。")).toBeNull();
  fireEvent.keyDown(screen.getByRole("button", { name: "筛选" }), {
    key: "ArrowDown",
  });
  fireEvent.click(
    await screen.findByRole("menuitemcheckbox", { name: "收藏" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "清空筛选" }));
  await waitFor(() =>
    expect(api.list.mock.lastCall?.[0]).toMatchObject({
      favorite: undefined,
      from,
    }),
  );
});
