import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import * as library from "@/components/media/library-queries";
import * as projects from "@/components/project/queries";
import * as transfer from "./transfer-queries";
import { TransferDialog } from "./transfer-dialog";
import { TransferProgress } from "./transfer-progress";
import {
  transferIDs,
  transferIdentity,
  transferInput,
  transferJob,
  transferLibrary,
  transferProject,
  transferProjectSummary,
} from "./transfer-test-fixtures";
vi.mock("@/components/media/library-queries", () => ({
  freshLibrary: vi.fn(),
  listLibrary: vi.fn(),
}));
vi.mock("@/components/project/queries", () => ({ getProject: vi.fn() }));
vi.mock("./transfer-queries", async (original) => ({
  ...(await original<typeof import("./transfer-queries")>()),
  listTransfers: vi.fn(),
  getTransfer: vi.fn(),
  runTransferIntent: vi.fn(),
}));
vi.mock("@/components/media/library-select", () => ({
  LibrarySelect: ({
    label,
    value,
    options,
    onChange,
    disabled,
  }: {
    label: string;
    value: string;
    options: { value: string; label: string; disabled?: boolean }[];
    onChange: (value: string) => void;
    disabled?: boolean;
  }) => (
    <select
      aria-label={label}
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
    >
      <option value="">请选择</option>
      {options.map((option) => (
        <option
          key={option.value}
          value={option.value}
          disabled={option.disabled}
        >
          {option.label}
        </option>
      ))}
    </select>
  ),
}));
const targetPage = {
  ...transferLibrary,
  scope: transferInput.target,
  library_id: transferIDs.target,
  revision: 7,
  folders: [
    {
      id: transferIDs.folder,
      library_id: transferIDs.target,
      library_kind: "project" as const,
      parent_id: null,
      name: "正式目标目录",
      position: 0,
      style: "paper",
      theme: "pearl",
      revision: 4,
      created_at: transferJob.created_at,
      updated_at: transferJob.updated_at,
    },
  ],
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(library.freshLibrary).mockResolvedValue(transferLibrary);
  vi.mocked(library.listLibrary).mockResolvedValue(targetPage);
  vi.mocked(projects.getProject).mockResolvedValue(transferProject);
  vi.mocked(transfer.listTransfers).mockResolvedValue({
    current_actor_id: transferIDs.actor,
    current_org_id: transferIDs.org,
    page: 1,
    page_size: 20,
    items: [],
  });
  vi.mocked(transfer.getTransfer).mockResolvedValue(transferJob);
  vi.mocked(transfer.runTransferIntent).mockResolvedValue(transferJob);
});
afterEach(cleanup);
function mount(selection = true) {
  const close = vi.fn();
  const onChanged = vi.fn().mockResolvedValue(undefined);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const rendered = render(
    <QueryClientProvider client={client}>
      <TransferDialog
        identity={transferIdentity}
        selection={
          selection
            ? { items: transferInput.items, sourceRevision: 0 }
            : undefined
        }
        projects={[transferProjectSummary]}
        hasMoreProjects={false}
        projectsLoading={false}
        onMoreProjects={vi.fn()}
        onClose={close}
        onChanged={onChanged}
      />
    </QueryClientProvider>,
  );
  return { close, onChanged, ...rendered };
}
async function choose() {
  fireEvent.change(await screen.findByLabelText("迁移目标项目"), {
    target: { value: transferIDs.project },
  });
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "确认独立迁移 1 项" })
        .matches(":disabled"),
    ).toBe(false),
  );
}
it("真实RHF表单冻结目标目录及双库/项目CAS，受理后Get当前任务而非写旧202缓存", async () => {
  const { onChanged } = mount();
  await choose();
  fireEvent.change(screen.getByLabelText("迁移目标目录"), {
    target: { value: transferIDs.folder },
  });
  fireEvent.click(screen.getByRole("button", { name: "确认独立迁移 1 项" }));
  await waitFor(() =>
    expect(transfer.runTransferIntent).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "create",
        body: {
          ...transferInput,
          expected_target_revision: 7,
          target_folder_id: transferIDs.folder,
          expected_folder_revision: 4,
        },
      }),
    ),
  );
  await screen.findByText(/原请求已受理，正在读取/);
  expect(onChanged).toHaveBeenCalledOnce();
  await waitFor(() =>
    expect(transfer.getTransfer).toHaveBeenCalledWith(
      transferIdentity,
      transferJob.id,
      expect.any(AbortSignal),
    ),
  );
  expect(screen.queryByText("全部完成")).toBeNull();
});
it("目录/目标CAS变化保留草稿且零写，必须重新审阅后显式提交", async () => {
  mount();
  await choose();
  vi.mocked(library.listLibrary).mockResolvedValue({
    ...targetPage,
    revision: 8,
  });
  fireEvent.click(screen.getByRole("button", { name: "确认独立迁移 1 项" }));
  await screen.findByText(/目标目录、素材库或项目版本已变化/);
  expect(transfer.runTransferIntent).not.toHaveBeenCalled();
  expect(screen.getByLabelText("迁移目标项目")).toHaveProperty(
    "value",
    transferIDs.project,
  );
  fireEvent.click(screen.getByRole("button", { name: "确认独立迁移 1 项" }));
  await waitFor(() =>
    expect(transfer.runTransferIntent).toHaveBeenCalledOnce(),
  );
  expect(
    vi.mocked(transfer.runTransferIntent).mock.calls[0][0].body,
  ).toMatchObject({ expected_target_revision: 8 });
});
it("unknown后Escape不丢原意图；刷新不自动发，人工同键重放", async () => {
  vi.mocked(transfer.runTransferIntent).mockRejectedValue(
    new ApiError(0, "network"),
  );
  const first = mount();
  await choose();
  fireEvent.click(screen.getByRole("button", { name: "确认独立迁移 1 项" }));
  await screen.findByRole("button", { name: "使用原键与原正文核验" });
  const original = vi.mocked(transfer.runTransferIntent).mock.calls[0][0];
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(first.close).not.toHaveBeenCalled();
  first.unmount();
  mount(false);
  await screen.findByText(`原键 ${original.key}`);
  expect(transfer.runTransferIntent).toHaveBeenCalledOnce();
  vi.mocked(transfer.runTransferIntent).mockResolvedValueOnce(transferJob);
  fireEvent.click(screen.getByRole("button", { name: "使用原键与原正文核验" }));
  await waitFor(() =>
    expect(transfer.runTransferIntent).toHaveBeenLastCalledWith(original),
  );
});
it("部分成功仅按钮显式重试失败行，成功行仍有正式目标入口；未知围栏不提供重试", async () => {
  const control = vi.fn();
  const failed = {
    ...transferJob.items[0],
    index: 1,
    source_item_id: transferIDs.folder,
    target_item_id: transferIDs.project,
    status: "failed" as const,
    failure_code: "source_changed",
  };
  const partial = {
    ...transferJob,
    status: "partial_failed" as const,
    stage: "completed" as const,
    items: [{ ...transferJob.items[0], status: "succeeded" as const }, failed],
  };
  const mounted = render(
    <TransferProgress job={partial} disabled={false} onControl={control} />,
  );
  expect(screen.getByText(/已发布 1 \/ 2 项/)).toBeTruthy();
  expect(
    screen
      .getByRole("link", { name: "打开已发布目标素材" })
      .getAttribute("href"),
  ).toContain(transferIDs.target);
  fireEvent.click(screen.getByRole("button", { name: "重试失败项" }));
  expect(control).toHaveBeenCalledWith("retry");
  mounted.rerender(
    <TransferProgress
      job={{
        ...partial,
        execution_unconfirmed: true,
        needs_reconciliation: true,
        status: "needs_reconciliation",
      }}
      disabled={false}
      onControl={control}
    />,
  );
  expect(
    screen.getByRole("button", { name: "重试失败项" }).matches(":disabled"),
  ).toBe(true);
  expect(
    screen.getByRole("button", { name: "核验原对象" }).matches(":disabled"),
  ).toBe(true);
});
it("取消动作只冻结已观察修订，不提前改当前状态或撤回成功行", async () => {
  vi.mocked(transfer.listTransfers).mockResolvedValue({
    current_actor_id: transferIDs.actor,
    current_org_id: transferIDs.org,
    page: 1,
    page_size: 20,
    items: [transferJob],
  });
  vi.mocked(transfer.runTransferIntent).mockResolvedValue({
    ...transferJob,
    revision: 2,
    cancellation_requested: true,
    status: "cancel_requested",
    stage: "cleanup",
  });
  mount(false);
  fireEvent.click(
    await screen.findByRole("button", {
      name: `查看迁移任务 ${transferJob.id}`,
    }),
  );
  await screen.findByRole("button", { name: "取消未完成迁移" });
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "取消未完成迁移" })
        .matches(":disabled"),
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "取消未完成迁移" }));
  await waitFor(() =>
    expect(transfer.runTransferIntent).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "cancel",
        jobId: transferJob.id,
        attempt: 1,
        body: { revision: 1 },
      }),
    ),
  );
  expect(screen.queryByText("已取消")).toBeNull();
});
