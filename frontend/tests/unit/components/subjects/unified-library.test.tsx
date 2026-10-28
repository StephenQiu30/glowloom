import {
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
  page_size: 12,
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
  api.list.mockResolvedValue(page);
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
it("当前空间无需项目就读取完整库，搜索传给服务端", async () => {
  const view = render(
    <UnifiedSubjectLibrary actorID={actorID} scopeID={scopeID} />,
  );
  await screen.findByRole("button", { name: "设置主体 海边香氛" });
  expect(api.list.mock.calls[0][0]).toMatchObject({
    page: 1,
    page_size: 12,
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
