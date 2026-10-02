import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LibraryPurgeDialog } from "./library-purge-dialog";
import { usePurgeWriter } from "./library-purge-writer";
import { readPurgeJob } from "./library-purge-model";
import { loadPurgeIntent } from "./library-purge-intent";
import { ApiError } from "@/lib/request";
import {
  createPurgePlan,
  savePurgePlan,
  loadPurgePlan,
} from "./library-purge-plan";
import * as queries from "./library-purge-query";
import * as library from "./library-queries";
import {
  purgeTestIdentity as identity,
  purgeTestInput as body,
  purgeTestJob as job,
  purgeTestReview as review,
} from "./library-purge-fixtures";
import {
  transferLibrary,
  transferIDs as ids,
} from "@/components/library/transfer-test-fixtures";
vi.mock("./library-queries", () => ({
  freshLibrary: vi.fn(),
  libraryKey: () => ["test-library"],
}));
vi.mock("./library-purge-query", () => ({
  reviewLibraryPurge: vi.fn(),
  getLibraryPurge: vi.fn(),
  listLibraryPurges: vi.fn(),
  runLibraryPurge: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(library.freshLibrary).mockResolvedValue(transferLibrary);
  vi.mocked(queries.reviewLibraryPurge).mockResolvedValue(review);
  vi.mocked(queries.listLibraryPurges).mockResolvedValue([job]);
  vi.mocked(queries.getLibraryPurge).mockResolvedValue(job);
  vi.mocked(queries.runLibraryPurge).mockResolvedValue(job);
});
afterEach(cleanup);
function mount(mode: "selected" | "history" = "selected", onClose = vi.fn()) {
  function Surface() {
    const writer = usePurgeWriter(identity, vi.fn());
    return (
      <LibraryPurgeDialog
        identity={identity}
        frame={mode === "selected" ? { mode, items: body.items } : { mode }}
        writer={writer}
        readOnly={false}
        onClose={onClose}
      />
    );
  }
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <Surface />
    </QueryClientProvider>,
  );
}
it("已恢复的完整计划必须明确结束，关闭和Escape不能静默丢弃或假装关闭", async () => {
  savePurgePlan(
    sessionStorage,
    createPurgePlan(identity, { ...review, mode: "all" }),
    null,
  );
  const onClose = vi.fn();
  mount("history", onClose);
  await screen.findByLabelText("完整回收站清理计划");
  const close = screen.getByRole("button", { name: "关闭清理窗口" });
  expect(close.matches(":disabled")).toBe(true);
  fireEvent.click(close);
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(onClose).not.toHaveBeenCalled();
  expect(loadPurgePlan(sessionStorage, identity)).not.toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "结束原计划并重新审阅全部" }),
  );
  await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  expect(loadPurgePlan(sessionStorage, identity)).toBeNull();
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
});
it("打开/Enter未确认不永久写；完整审阅勾选且点击最终按钮才永久送出", async () => {
  mount();
  const final = await screen.findByRole("button", {
    name: "确认永久删除 1 项",
  });
  expect(final.matches(":disabled")).toBe(true);
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
  const text = screen.getByLabelText("全部永久清理条目及冻结版本");
  expect(text).toHaveProperty("value", expect.stringContaining(ids.item));
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Enter" });
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("checkbox", {
      name: "我已审阅完整范围并确认永久删除不可恢复",
    }),
  );
  await waitFor(() => expect(final.matches(":disabled")).toBe(false));
  fireEvent.click(final);
  await waitFor(() => expect(queries.runLibraryPurge).toHaveBeenCalledTimes(1));
  expect(vi.mocked(queries.runLibraryPurge).mock.calls[0][0].body).toEqual(
    body,
  );
});
it("引用blocked、unknown、cancelled逐项显示；对账只在明确控制确认后发送", async () => {
  const actual = {
    ...job,
    status: "needs_reconciliation" as const,
    stage: "removing" as const,
    needs_reconciliation: true,
    cancellation_requested: true,
    items: [
      {
        ...job.items[0],
        status: "blocked" as const,
        failure_code: "in_use" as const,
      },
      {
        ...job.items[0],
        index: 1,
        item_id: ids.target,
        status: "needs_reconciliation" as const,
      },
      {
        ...job.items[0],
        index: 2,
        item_id: ids.folder,
        status: "cancelled" as const,
      },
    ],
  };
  vi.mocked(queries.listLibraryPurges).mockResolvedValue([actual]);
  vi.mocked(queries.getLibraryPurge).mockResolvedValue(actual);
  vi.mocked(queries.runLibraryPurge).mockResolvedValue(actual);
  mount("history");
  fireEvent.click(
    await screen.findByRole("button", { name: /结果未知.*3 项/ }),
  );
  await screen.findByText(/引用保护阻断.*正在被当前或历史内容引用/);
  await screen.findByText("已取消");
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "审阅对账未知结果" }));
  const final = screen.getByRole("button", { name: "确认对账原任务" });
  expect(final.matches(":disabled")).toBe(true);
  fireEvent.click(
    screen.getByRole("checkbox", { name: "明确确认此次任务控制动作" }),
  );
  fireEvent.click(final);
  await waitFor(() =>
    expect(queries.runLibraryPurge).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "reconcile",
        jobId: job.id,
        body: { revision: 1 },
      }),
      undefined,
    ),
  );
});
it.each(["running", "cancel_requested"] as const)(
  "公开GET状态%s且未知标志为false时，允许人工请求服务端核验原任务对账",
  async (status) => {
    const actual = readPurgeJob(
      {
        ...job,
        status,
        stage: "removing",
        revision: 3,
        cancellation_requested: status === "cancel_requested",
        needs_reconciliation: false,
        execution_unconfirmed: false,
        items: [{ ...job.items[0], status: "running" }],
      },
      identity,
      job.id,
    );
    vi.mocked(queries.listLibraryPurges).mockResolvedValue([actual]);
    vi.mocked(queries.getLibraryPurge).mockResolvedValue(actual);
    vi.mocked(queries.runLibraryPurge).mockResolvedValue(actual);
    mount("history");
    fireEvent.click(await screen.findByRole("button", { name: /1 项/ }));
    const reconcile = await screen.findByRole("button", { name: /审阅.*对账/ });
    expect(reconcile.matches(":disabled")).toBe(false);
    expect(queries.runLibraryPurge).not.toHaveBeenCalled();
    fireEvent.click(reconcile);
    expect(screen.getByText(/服务器会先核验执行已结束与当前权限/)).toBeTruthy();
    const final = screen.getByRole("button", { name: "确认对账原任务" });
    expect(final.matches(":disabled")).toBe(true);
    expect(queries.runLibraryPurge).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("checkbox", { name: "明确确认此次任务控制动作" }),
    );
    fireEvent.click(final);
    await waitFor(() => expect(queries.runLibraryPurge).toHaveBeenCalledOnce());
    expect(queries.runLibraryPurge).toHaveBeenCalledWith(
      expect.objectContaining({
        ...identity,
        action: "reconcile",
        jobId: job.id,
        body: { revision: 3 },
        key: expect.any(String),
      }),
      undefined,
    );
  },
);
it("公开GET仍标记执行未确认时，对账入口禁用且不发送", async () => {
  const actual = readPurgeJob(
    {
      ...job,
      status: "running",
      stage: "removing",
      execution_unconfirmed: true,
      items: [{ ...job.items[0], status: "running" }],
    },
    identity,
    job.id,
  );
  vi.mocked(queries.listLibraryPurges).mockResolvedValue([actual]);
  vi.mocked(queries.getLibraryPurge).mockResolvedValue(actual);
  mount("history");
  fireEvent.click(await screen.findByRole("button", { name: /1 项/ }));
  const reconcile = await screen.findByRole("button", { name: /审阅.*对账/ });
  expect(reconcile.matches(":disabled")).toBe(true);
  fireEvent.click(reconcile);
  expect(screen.queryByLabelText("清理控制确认")).toBeNull();
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
});
it("GET不能证明执行结束，服务端409拒绝对账时保留原键正文且刷新不自动重试", async () => {
  const actual = readPurgeJob(
    {
      ...job,
      status: "running",
      stage: "removing",
      revision: 3,
      items: [{ ...job.items[0], status: "running" }],
    },
    identity,
    job.id,
  );
  vi.mocked(queries.listLibraryPurges).mockResolvedValue([actual]);
  vi.mocked(queries.getLibraryPurge).mockResolvedValue(actual);
  vi.mocked(queries.runLibraryPurge).mockRejectedValue(
    new ApiError(409, "media_purge_conflict"),
  );
  const surface = mount("history");
  fireEvent.click(await screen.findByRole("button", { name: /1 项/ }));
  const reconcile = await screen.findByRole("button", { name: /审阅.*对账/ });
  expect(reconcile.matches(":disabled")).toBe(false);
  fireEvent.click(reconcile);
  expect(screen.getByText(/仍在执行或状态变化时停止，不自动重试/)).toBeTruthy();
  fireEvent.click(
    screen.getByRole("checkbox", { name: "明确确认此次任务控制动作" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认对账原任务" }));
  await screen.findByText(/服务器已明确拒绝原请求/);
  const original = vi.mocked(queries.runLibraryPurge).mock.calls[0][0];
  expect(original).toEqual(
    expect.objectContaining({
      ...identity,
      action: "reconcile",
      jobId: job.id,
      body: { revision: 3 },
      key: expect.any(String),
    }),
  );
  expect(loadPurgeIntent(sessionStorage, identity)).toEqual(original);
  expect(screen.getByLabelText("原清理完整正文")).toHaveProperty(
    "value",
    JSON.stringify(original.body, null, 2),
  );
  surface.unmount();
  mount("history");
  await screen.findByText(/结果尚未确认，不会自动发送/);
  expect(queries.runLibraryPurge).toHaveBeenCalledOnce();
  expect(loadPurgeIntent(sessionStorage, identity)).toEqual(original);
});
