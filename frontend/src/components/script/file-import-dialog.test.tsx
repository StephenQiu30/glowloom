import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { FileImportDialog, FileImportCreateDialog } from "./file-import-dialog";
import { getFileImport, listFileImports } from "./file-import-queries";
import { fileImportScopeKey } from "./file-import-queries";
import type { useFileImport } from "./use-file-import";
import type { FileImportJob } from "./file-import-model";
vi.mock("./file-import-queries", async (original) => ({
  ...(await original<typeof import("./file-import-queries")>()),
  getFileImport: vi.fn(),
  listFileImports: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  projectId: id(1),
  actorId: id(2),
  orgId: id(3),
};
const job = {
  id: id(4),
  project_id: id(1),
  revision: 12,
  attempt: 2,
  status: "partial" as const,
  stage: "completed" as const,
  expected_script_revision: 0,
  latest_script_revision: 1,
  latest_version_id: id(8),
  cancellation_requested: false,
  reconciliation_requested: false,
  needs_reconciliation: false,
  active_io: false,
  retryable: true,
  can_control: true,
  files: [
    {
      position: 0,
      asset_id: id(5),
      file_name: "一.txt",
      status: "succeeded" as const,
      source_id: id(6),
      source_lineage_id: id(6),
      attempt: 1,
      warnings: [],
    },
    {
      position: 1,
      asset_id: id(7),
      file_name: "损坏.docx",
      status: "failed" as const,
      failure_code: "invalid_docx",
      attempt: 2,
      warnings: [],
    },
  ],
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
function writer() {
  return {
    ready: true,
    busy: false,
    intent: null,
    rejected: null,
    error: undefined,
    storageError: undefined,
    locked: false,
    submit: vi.fn(async () => {}),
    replay: vi.fn(async () => {}),
    restoreStorage: vi.fn(async () => {}),
    acknowledgeLatest: vi.fn(),
  };
}
beforeEach(() =>
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  ),
);
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("partial展示实际发布数与失败原件，重试冻结当前jobCAS；回执不乐观写缓存", async () => {
  vi.mocked(listFileImports).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [job],
  });
  vi.mocked(getFileImport).mockResolvedValue(job);
  const current = writer();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <FileImportDialog
        scope={scope}
        writer={current}
        onClose={vi.fn()}
        onReadCurrent={vi.fn()}
      />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: /选择导入任务/ }));
  expect(await screen.findByText("已发布 1 / 2 个原件")).toBeTruthy();
  expect(screen.getByText("invalid_docx")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "仅重试失败原件" }));
  expect(current.submit).toHaveBeenCalledWith({
    action: "retry",
    jobId: job.id,
    body: { expected_revision: 12 },
  });
  expect(screen.queryByText(/100%|50%/)).toBeNull();
});
it("创建冻结原件完整顺序与CAS，版权未确认零DML", async () => {
  const current = writer();
  const assets = [5, 7].map((n) => ({
    id: id(n),
    project_id: scope.projectId,
    kind: "document" as const,
    file_name: `${n}.txt`,
    mime_type: "text/plain" as const,
    byte_size: 8,
    revision: 1,
  }));
  render(
    <FileImportCreateDialog
      scope={scope}
      assets={assets}
      base={{ expected_revision: 4, base_version_id: id(8) }}
      writer={current}
      onClose={vi.fn()}
      onRebase={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "确认创建导入任务" }));
  expect(await screen.findByText("请确认所有原件均有使用权限。")).toBeTruthy();
  expect(current.submit).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("checkbox", { name: "已获授权使用所选全部原件" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认创建导入任务" }));
  await waitFor(() =>
    expect(current.submit).toHaveBeenCalledWith({
      action: "create",
      body: {
        expected_revision: 4,
        base_version_id: id(8),
        rights_confirmed: true,
        asset_ids: [id(5), id(7)],
      },
    }),
  );
});
it("取消意图有active_io仍显式核验；202不宣称停止或覆盖已读revision", async () => {
  const cancelling = {
    ...job,
    status: "cancel_requested" as const,
    stage: "cancelling" as const,
    cancellation_requested: true,
    active_io: true,
    needs_reconciliation: false,
  };
  vi.mocked(listFileImports).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [cancelling],
  });
  vi.mocked(getFileImport).mockResolvedValue(cancelling);
  const current = writer();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <FileImportDialog
        scope={scope}
        writer={current}
        initialJobId={job.id}
        onClose={vi.fn()}
        onReadCurrent={vi.fn()}
      />
    </QueryClientProvider>,
  );
  const reconcile = await screen.findByRole("button", {
    name: "明确核验原导入任务",
  });
  fireEvent.click(reconcile);
  expect(current.submit).toHaveBeenCalledWith({
    action: "reconcile",
    jobId: job.id,
    body: { expected_revision: 12 },
  });
  expect(
    screen.getByText(
      "当前有实际输入输出所有者；取消受理后仍需读取真实停止与清理结果。",
    ),
  ).toBeTruthy();
  expect(
    client.getQueryData<FileImportJob>([
      ...fileImportScopeKey(scope),
      "job",
      job.id,
    ])?.revision,
  ).toBe(12);
  expect(screen.queryByRole("button", { name: "仅重试失败原件" })).toBeNull();
});
it("创建unknown原键核验在模态框内，继承fieldset禁用时转焦点且Escape不能丢原体", async () => {
  const current = writer();
  const close = vi.fn();
  const assets = [
    {
      id: id(5),
      project_id: scope.projectId,
      kind: "document" as const,
      file_name: "原.txt",
      mime_type: "text/plain" as const,
      byte_size: 8,
      revision: 1,
    },
  ];
  const props = {
    scope,
    assets,
    base: { expected_revision: 4 },
    writer: current,
    onClose: close,
    onRebase: vi.fn(),
  };
  const view = render(<FileImportCreateDialog {...props} />);
  screen.getByRole("button", { name: "确认创建导入任务" }).focus();
  const pending: ReturnType<typeof useFileImport> = {
    ...current,
    locked: true,
    intent: {
      ...scope,
      version: 1,
      key: id(8),
      action: "create",
      body: {
        expected_revision: 4,
        rights_confirmed: true,
        asset_ids: [id(5)],
      },
    },
  };
  view.rerender(<FileImportCreateDialog {...props} writer={pending} />);
  const replay = screen.getByRole("button", {
    name: "人工使用原键核验导入请求",
  });
  await waitFor(() => expect(document.activeElement).toBe(replay));
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(close).not.toHaveBeenCalled();
  expect(
    screen
      .getByRole("button", { name: "放弃本地选择并关闭" })
      .matches(":disabled"),
  ).toBe(true);
  fireEvent.click(replay);
  expect(current.replay).toHaveBeenCalledTimes(1);
});
