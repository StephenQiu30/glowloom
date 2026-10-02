import { expect, it } from "vitest";
import { ApiError } from "@/lib/request";
import {
  readTransfer,
  readTransferPage,
  readTransferReceipt,
  transferActions,
  transferInputSchema,
} from "./transfer-model";
const id = "11111111-1111-4111-8111-111111111111",
  org = "22222222-2222-4222-8222-222222222222",
  project = "33333333-3333-4333-8333-333333333333",
  item = "44444444-4444-4444-8444-444444444444",
  target = "55555555-5555-4555-8555-555555555555";
export const identity = {
  origin: "http://localhost:3000",
  actorId: id,
  orgId: org,
  libraryId: target,
  scope: { kind: "personal" as const },
};
export const input = {
  source: identity.scope,
  target: { kind: "project" as const, project_id: project },
  items: [{ id: item, revision: 0 }],
  expected_source_revision: 0,
  expected_target_revision: 1,
  expected_project_revision: 2,
  target_folder_id: null,
  expected_folder_revision: 0,
};
export const job = {
  id,
  current_actor_id: id,
  current_org_id: org,
  source: input.source,
  target: input.target,
  target_folder_id: null,
  status: "queued",
  stage: "frozen",
  attempt: 1,
  revision: 1,
  needs_reconciliation: false,
  cancellation_requested: false,
  execution_unconfirmed: false,
  items: [
    {
      index: 0,
      source_item_id: item,
      target_item_id: target,
      target_asset_id: null,
      status: "queued",
      failure_code: null,
    },
  ],
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
it("闭合双scope、200个真实CAS条目与root/目录revision；legacy0不伪造1", () => {
  expect(transferInputSchema.parse(input).items[0].revision).toBe(0);
  for (const bad of [
    { ...input, target: input.source },
    { ...input, items: [] },
    { ...input, items: [input.items[0], input.items[0]] },
    { ...input, target_folder_id: item },
    { ...input, expected_project_revision: 0 },
    { ...input, object_key: "private" },
  ])
    expect(transferInputSchema.safeParse(bad).success).toBe(false);
});
it("读取实际逐项身份、scope与当前主体，不接受外来字段/伪重复进度", () => {
  expect(readTransfer(job, identity).items).toHaveLength(1);
  for (const bad of [
    { ...job, current_actor_id: target },
    { ...job, source: input.target },
    { ...job, items: [{ ...job.items[0], index: 1 }] },
    { ...job, object_key: "private" },
    { ...job, items: [job.items[0], job.items[0]] },
  ])
    expect(() => readTransfer(bad, identity)).toThrow(ApiError);
  expect(() =>
    readTransferPage(
      {
        current_actor_id: id,
        current_org_id: org,
        page: 2,
        page_size: 20,
        items: [],
      },
      identity,
      1,
    ),
  ).toThrow(ApiError);
});
it("202是永久原受理事实；既有replay不能把完成响应当创建回执", () => {
  const intent = {
    ...identity,
    version: 1 as const,
    key: target,
    action: "create" as const,
    body: input,
  };
  expect(readTransferReceipt(job, intent).id).toBe(id);
  expect(() =>
    readTransferReceipt(
      { ...job, status: "succeeded", stage: "completed" },
      intent,
    ),
  ).toThrow(ApiError);
  expect(() =>
    readTransferReceipt(
      { ...job, items: [{ ...job.items[0], source_item_id: id }] },
      intent,
    ),
  ).toThrow(ApiError);
});
it("未知执行不可重试或核验；部分失败重试只指失败行，取消不撤成功行", () => {
  const partial = readTransfer(
    {
      ...job,
      status: "partial_failed",
      stage: "completed",
      items: [
        { ...job.items[0], status: "failed", failure_code: "source_changed" },
      ],
    },
    identity,
  );
  expect(transferActions(partial)).toEqual({
    cancel: false,
    retry: true,
    reconcile: false,
  });
  expect(transferActions({ ...partial, execution_unconfirmed: true })).toEqual({
    cancel: false,
    retry: false,
    reconcile: false,
  });
  expect(
    transferActions({
      ...partial,
      status: "needs_reconciliation",
      needs_reconciliation: true,
    }),
  ).toEqual({ cancel: true, retry: false, reconcile: true });
  expect(
    transferActions({ ...partial, status: "running", stage: "copying" }).cancel,
  ).toBe(true);
});
it("未知实际执行可受理取消意图，202必须保留未知事实而不是推定停止", () => {
  const intent = {
    ...identity,
    version: 1 as const,
    key: target,
    action: "cancel" as const,
    jobId: id,
    attempt: 1,
    source: job.source,
    target: job.target,
    targetFolderId: null,
    body: { revision: 1 },
  };
  expect(
    readTransferReceipt(
      {
        ...job,
        revision: 2,
        status: "cancel_requested",
        stage: "cleanup",
        cancellation_requested: true,
        execution_unconfirmed: true,
      },
      intent,
    ).execution_unconfirmed,
  ).toBe(true);
});
