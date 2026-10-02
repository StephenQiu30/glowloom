import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import {
  copyActions,
  getCopy,
  listCopies,
  newerCopy,
  runCopyIntent,
} from "./copy-queries";
import type { CopyIntent } from "./copy-intent";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  cancel: vi.fn(),
  retry: vi.fn(),
  reconcile: vi.fn(),
}));
vi.mock("@/gen/api/projectCopies", () => ({
  listProjectCopies: api.list,
  getProjectCopy: api.get,
  createProjectCopy: api.create,
  cancelProjectCopy: api.cancel,
  retryProjectCopy: api.retry,
  reconcileProjectCopy: api.reconcile,
}));
const sourceId = "e61dbdb9-d428-45dc-a034-53f3f1bb1068";
const job = {
  id: "93021a72-3281-42b2-91cf-6a4413ee8956",
  source_project_id: sourceId,
  target_project_id: "c5d5711d-ab9a-4347-be88-933b51e399ae",
  target_name: "完整副本",
  source_revision: 7,
  status: "queued" as const,
  stage: "media" as const,
  revision: 1,
  attempt: 1,
  documents: 2,
  assets: 4,
  renditions: 8,
  completed_documents: 0,
  completed_assets: 0,
  completed_renditions: 0,
  retryable: false,
  needs_reconciliation: false,
  reconciliation_requested: false,
  execution_unconfirmed: false,
  cancellation_requested: false,
};
const intent: CopyIntent = {
  version: 1,
  origin: "http://localhost:3000",
  actorId: "b51e05a5-3e86-4685-a624-bf28841d5618",
  orgId: "9c06e9e9-c029-4404-b6c0-68509d031341",
  sourceId,
  action: "create",
  key: "511054be-4f0d-4a9a-b9ad-2a6af5a3979f",
  body: { expected_revision: 7, target_name: "完整副本" },
};
const scriptCounts = {
  sources: 4,
  versions: 3,
  version_sources: 7,
  project_states: 1,
  version_heads: 3,
  split_sets: 5,
  split_confirmations: 2,
  episodes: 6,
  structures: 8,
  scenes: 9,
  dialogue_lines: 11,
  action_lines: 12,
  objects: 15,
};
beforeEach(() => {
  vi.resetAllMocks();
  api.list.mockResolvedValue({
    copies: [job],
    next_cursor: "opaque-cursor",
    current_actor_id: intent.actorId,
    current_org_id: intent.orgId,
  });
  api.get.mockResolvedValue(job);
  for (const command of [api.create, api.cancel, api.retry, api.reconcile])
    command.mockResolvedValue(job);
});
it("以 25 条真实游标读取作用域，严格校验来源与完成数量", async () => {
  const page = await listCopies(sourceId, "cursor");
  expect(api.list).toHaveBeenCalledWith(
    { pid: sourceId, limit: 25, cursor: "cursor" },
    { signal: undefined },
  );
  expect(page.current_actor_id).toBe(intent.actorId);
  api.list.mockResolvedValueOnce({
    ...page,
    copies: [{ ...job, source_project_id: job.target_project_id }],
  });
  await expect(listCopies(sourceId)).rejects.toMatchObject({
    code: "invalid_response",
  });
  api.get.mockResolvedValueOnce({ ...job, completed_assets: 5 });
  await expect(getCopy(sourceId, job.id)).rejects.toBeInstanceOf(ApiError);
});
it("详情来源、任务及目标身份严格核对，非法 DTO 写入视为效果未知", async () => {
  api.get.mockResolvedValueOnce({ ...job, id: job.target_project_id });
  await expect(getCopy(sourceId, job.id)).rejects.toMatchObject({
    code: "invalid_response",
  });
  api.create.mockResolvedValueOnce({ ...job, target_project_id: sourceId });
  await expect(runCopyIntent(intent)).rejects.toMatchObject({
    code: "invalid_response",
    status: 502,
  });
  api.create.mockResolvedValueOnce({ ...job, source_revision: 8 });
  await expect(runCopyIntent(intent)).rejects.toMatchObject({
    code: "invalid_response",
  });
  api.get.mockResolvedValueOnce({
    ...job,
    status: "succeeded",
    stage: "complete",
  });
  await expect(getCopy(sourceId, job.id)).rejects.toMatchObject({
    code: "invalid_response",
  });
});
it("创建使用原正文及 UUID/Origin，原 202 仅作为受理回执", async () => {
  expect(await runCopyIntent(intent)).toEqual(job);
  expect(api.create).toHaveBeenCalledWith({ pid: sourceId }, intent.body, {
    headers: { "Idempotency-Key": intent.key, Origin: intent.origin },
  });
});
it("初始 queued 202 尚未启动 worker，attempt=0 是合法受理回执", async () => {
  api.create.mockResolvedValueOnce({ ...job, attempt: 0 });
  await expect(runCopyIntent(intent)).resolves.toMatchObject({
    status: "queued",
    attempt: 0,
  });
});
it.each(["cancel", "retry", "reconcile"] as const)(
  "%s 仅调用生成 API 和原控制 CAS",
  async (action) => {
    await runCopyIntent({
      ...intent,
      action,
      jobId: job.id,
      body: { expected_revision: 3 },
    });
    expect(api[action]).toHaveBeenCalledWith(
      { id: job.id },
      { expected_revision: 3 },
      { headers: { "Idempotency-Key": intent.key, Origin: intent.origin } },
    );
  },
);
it("旧受理回执不得覆盖更高修订和实际成功缓存", () => {
  const current = {
    ...job,
    revision: 8,
    status: "succeeded" as const,
    stage: "complete" as const,
    completed_documents: 2,
    completed_assets: 4,
    completed_renditions: 8,
  };
  expect(newerCopy(current, job)).toBe(current);
  expect(newerCopy(job, current)).toBe(current);
  expect(() =>
    newerCopy(current, {
      ...current,
      target_project_id: intent.actorId,
      revision: 9,
    }),
  ).toThrow(ApiError);
});
it("取消竞态以服务端事实为准；重试、核验和执行未退出各有门禁", () => {
  expect(copyActions(job)).toEqual({
    cancel: true,
    retry: false,
    reconcile: false,
  });
  const failed = { ...job, status: "failed" as const, retryable: true };
  expect(copyActions(failed).retry).toBe(true);
  expect(copyActions({ ...failed, cancellation_requested: true }).retry).toBe(
    false,
  );
  expect(copyActions({ ...failed, needs_reconciliation: true }).reconcile).toBe(
    true,
  );
  expect(copyActions({ ...failed, needs_reconciliation: true }).retry).toBe(
    false,
  );
  expect(
    copyActions({
      ...failed,
      needs_reconciliation: true,
      execution_unconfirmed: true,
    }),
  ).toEqual({ cancel: true, retry: false, reconcile: false });
  expect(copyActions({ ...job, status: "succeeded" }).cancel).toBe(false);
  expect(
    copyActions({
      ...job,
      status: "cancel_requested",
      cancellation_requested: true,
    }).cancel,
  ).toBe(false);
});

it("接受正式剧本阶段与全部十三种历史计数，原任务仍可省略剧本", async () => {
  const admitted = {
    ...job,
    stage: "script",
    script: { counts: scriptCounts },
  };
  api.get.mockResolvedValueOnce(admitted);
  await expect(getCopy(sourceId, job.id)).resolves.toEqual(admitted);
  await expect(getCopy(sourceId, job.id)).resolves.toEqual(job);
});

it.each([
  { stage: "script" },
  { script: { counts: { ...scriptCounts, project_states: 2 } } },
  { script: { counts: { ...scriptCounts, version_heads: 2 } } },
  { script: { counts: { ...scriptCounts, objects: 2147483648 } } },
  { script: { counts: { ...scriptCounts, object_key: "private-key" } } },
  {
    script: {
      counts: scriptCounts,
      completed_counts: { ...scriptCounts, action_lines: 11 },
    },
  },
  { stage: "canvases", script: { counts: scriptCounts } },
])("拒绝不完整、越界或提前越过剧本的进度 %j", async (invalid) => {
  api.get.mockResolvedValueOnce({ ...job, ...invalid });
  await expect(getCopy(sourceId, job.id)).rejects.toMatchObject({
    code: "invalid_response",
  });
});

it("完整十三计数回执后才接受后续阶段，缺一项不得宣称复制成功", async () => {
  const completed = {
    ...job,
    status: "succeeded",
    stage: "complete",
    completed_documents: job.documents,
    completed_assets: job.assets,
    completed_renditions: job.renditions,
    script: { counts: scriptCounts, completed_counts: scriptCounts },
  };
  api.get.mockResolvedValueOnce(completed);
  await expect(getCopy(sourceId, job.id)).resolves.toEqual(completed);
  api.get.mockResolvedValueOnce({
    ...completed,
    script: { counts: scriptCounts },
  });
  await expect(getCopy(sourceId, job.id)).rejects.toMatchObject({
    code: "invalid_response",
  });
});

it("同一复制任务的剧本冻结计数不能随新响应变化或消失", () => {
  const withScript = { ...job, script: { counts: scriptCounts } };
  expect(() => newerCopy(withScript, { ...job, revision: 2 })).toThrow(
    ApiError,
  );
  expect(() =>
    newerCopy(withScript, {
      ...withScript,
      revision: 2,
      script: { counts: { ...scriptCounts, sources: 5 } },
    }),
  ).toThrow(ApiError);
});
