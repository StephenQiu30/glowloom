import { beforeEach, expect, it, vi } from "vitest";
import * as api from "@/api/mediaDepths";
import { ApiError } from "@/lib/request";
import {
  listDepths,
  getDepth,
  runDepthIntent,
  downloadDepth,
} from "./media-depth-queries";
vi.mock("@/api/mediaDepths", () => ({
  createMediaDepth: vi.fn(),
  listMediaDepths: vi.fn(),
  getMediaDepth: vi.fn(),
  cancelMediaDepth: vi.fn(),
  retryMediaDepth: vi.fn(),
  reconcileMediaDepth: vi.fn(),
  reviewMediaDepth: vi.fn(),
  previewMediaDepth: vi.fn(),
  downloadMediaDepth: vi.fn(),
}));
const id = "fba9de81-80c4-49ce-8f59-1dac39717a04";
const project = "b580ad59-17a5-4ed2-985b-cb0cdd04b4c4";
const canvas = "5db0c65d-d391-45a0-ab1a-d7f6eb4c868d";
const node = "0c6e50ee-0912-4da2-96b3-c84e27b61389";
const job = {
  id,
  project_id: project,
  source: { canvas_id: canvas, node_id: node, revision: 4 },
  source_asset_id: "b6f81c20-2489-4276-909d-04a9d13b3e43",
  source_asset_revision: 1,
  source_sha256: "a".repeat(64),
  profile_id: "vda-small-relative-v1" as const,
  status: "queued" as const,
  stage: "queued" as const,
  revision: 1,
  attempt: 1,
  asset_id: null,
  sha256: null,
  failure_code: null,
  retryable: false,
  needs_reconciliation: false,
  reconciliation_requested: false,
  execution_unconfirmed: false,
  cancellation_requested: false,
  created_at: "2026-10-01T19:00:00Z",
  updated_at: "2026-10-01T19:00:00Z",
};
const scope = {
  origin: "http://127.0.0.1:3000",
  actorId: id,
  orgId: project,
  projectId: project,
  canvasId: canvas,
  nodeId: node,
};
const intent = {
  ...scope,
  version: 1 as const,
  key: id,
  action: "create" as const,
  body: job.source,
};
beforeEach(() => vi.clearAllMocks());
it("只调用生成客户端，分页严格核当前 Principal 和来源作用域", async () => {
  vi.mocked(api.listMediaDepths).mockResolvedValue({
    items: [job],
    next_cursor: "next",
    current_actor_id: id,
    current_org_id: project,
  });
  expect(
    (await listDepths(project, canvas, node, "opaque")).items,
  ).toHaveLength(1);
  expect(api.listMediaDepths).toHaveBeenCalledWith(
    {
      pid: project,
      canvas_id: canvas,
      node_id: node,
      limit: 25,
      cursor: "opaque",
    },
    { signal: undefined },
  );
  vi.mocked(api.listMediaDepths).mockResolvedValue({
    items: [{ ...job, project_id: id }],
    next_cursor: null,
    current_actor_id: id,
    current_org_id: project,
  });
  await expect(listDepths(project, canvas, node)).rejects.toBeInstanceOf(
    ApiError,
  );
});
it("未知 create 按原 UUID、正文、Origin 核验，不安装或重造任务", async () => {
  vi.mocked(api.createMediaDepth).mockResolvedValue(job);
  await runDepthIntent(intent);
  await runDepthIntent(JSON.parse(JSON.stringify(intent)));
  expect(api.createMediaDepth).toHaveBeenLastCalledWith(
    { pid: project },
    job.source,
    { headers: { "Idempotency-Key": id, Origin: scope.origin } },
  );
  expect(api.createMediaDepth).toHaveBeenCalledTimes(2);
  vi.mocked(api.createMediaDepth).mockResolvedValue({
    ...job,
    source: { ...job.source, revision: 5 },
  });
  await expect(runDepthIntent(intent)).rejects.toMatchObject({ status: 502 });
});
it("控制和审核保留旧 CAS 正文，非法 DML DTO 留给未知效果恢复", async () => {
  vi.mocked(api.reconcileMediaDepth).mockResolvedValue(job);
  await runDepthIntent({
    ...scope,
    version: 1,
    key: id,
    action: "reconcile",
    jobId: id,
    body: { project_id: project, revision: 8 },
  });
  expect(api.reconcileMediaDepth).toHaveBeenCalledWith(
    { job_id: id },
    { project_id: project, revision: 8 },
    { headers: { "Idempotency-Key": id, Origin: scope.origin } },
  );
  vi.mocked(api.reviewMediaDepth).mockResolvedValue(job);
  await runDepthIntent({
    ...scope,
    version: 1,
    key: id,
    action: "review",
    jobId: id,
    body: {
      project_id: project,
      revision: 8,
      sha256: "b".repeat(64),
      local_review_confirmed: true,
    },
  });
  expect(api.reviewMediaDepth).toHaveBeenCalledWith(
    { job_id: id },
    {
      project_id: project,
      revision: 8,
      sha256: "b".repeat(64),
      local_review_confirmed: true,
    },
    { headers: { "Idempotency-Key": id, Origin: scope.origin } },
  );
  vi.mocked(api.cancelMediaDepth).mockResolvedValue({ accepted: true });
  await expect(
    runDepthIntent({
      ...scope,
      version: 1,
      key: id,
      action: "cancel",
      jobId: id,
      body: { project_id: project, revision: 8 },
    }),
  ).rejects.toMatchObject({ status: 502 });
});
it("详情不能信任其他任务或项目返回，取消和读取错误传递", async () => {
  vi.mocked(api.getMediaDepth).mockResolvedValue({ ...job, id: project });
  await expect(getDepth(project, id)).rejects.toMatchObject({ status: 502 });
  const failure = new ApiError(404, "not_found");
  vi.mocked(api.getMediaDepth).mockRejectedValue(failure);
  await expect(getDepth(project, id)).rejects.toBe(failure);
});
it("正式下载必须已审核且实际为非空有限 MP4 Blob", async () => {
  await expect(downloadDepth(job)).rejects.toThrow();
  expect(api.downloadMediaDepth).not.toHaveBeenCalled();
  const succeeded = {
    ...job,
    status: "succeeded" as const,
    stage: "complete" as const,
    asset_id: canvas,
    sha256: "b".repeat(64),
  };
  vi.mocked(api.downloadMediaDepth).mockResolvedValue("fake");
  await expect(downloadDepth(succeeded)).rejects.toMatchObject({ status: 502 });
  const blob = new Blob(["synthetic-mp4"], { type: "video/mp4" });
  vi.mocked(api.downloadMediaDepth).mockResolvedValue(
    blob as unknown as string,
  );
  expect(await downloadDepth(succeeded)).toBe(blob);
  expect(api.downloadMediaDepth).toHaveBeenLastCalledWith(
    { job_id: id, project_id: project },
    { responseType: "blob", signal: undefined },
  );
});
