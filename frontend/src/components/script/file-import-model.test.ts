import { expect, it } from "vitest";
import {
  fileImportActions,
  readFileImport,
  readFileImportPage,
  readFileImportReceipt,
} from "./file-import-model";
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = { projectId: id(1), actorId: id(2), orgId: id(3) };
const job = {
  id: id(4),
  project_id: id(1),
  revision: 8,
  attempt: 1,
  status: "partial",
  stage: "completed",
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
      status: "succeeded",
      source_id: id(6),
      source_lineage_id: id(6),
      attempt: 1,
      warnings: [],
    },
    {
      position: 1,
      asset_id: id(7),
      file_name: "损坏.docx",
      status: "failed",
      failure_code: "invalid_docx",
      attempt: 1,
      warnings: [],
    },
  ],
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
it("保留真实部分成功与来源身份；拒绝重复原件/位置/私有字段和错scope分页", () => {
  expect(readFileImport(job, scope.projectId).files[0].source_id).toBe(id(6));
  for (const invalid of [
    { ...job, files: [job.files[0], job.files[0]] },
    { ...job, files: [{ ...job.files[0], position: 1 }, job.files[1]] },
    {
      ...job,
      files: [{ ...job.files[0], source_lineage_id: undefined }, job.files[1]],
    },
    { ...job, object_key: "private" },
  ])
    expect(() => readFileImport(invalid, scope.projectId)).toThrow();
  expect(
    readFileImportPage(
      { ...scopeFields, items: [job], next_after: 25 },
      scope,
      0,
    ).next_after,
  ).toBe(25);
  expect(() =>
    readFileImportPage(
      { ...scopeFields, current_actor_id: id(4), items: [job] },
      scope,
      0,
    ),
  ).toThrow();
  expect(() =>
    readFileImportPage(
      { ...scopeFields, items: [job], next_after: 25 },
      scope,
      25,
    ),
  ).toThrow();
});
const scopeFields = {
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
};
it("partial只显式重试失败；核验门禁保留真实取消意图且不猜active_io已停止", () => {
  expect(fileImportActions(readFileImport(job, scope.projectId))).toEqual({
    retry: true,
    cancel: true,
    reconcile: false,
  });
  const interrupted = readFileImport(
    {
      ...job,
      status: "cancel_requested",
      stage: "cancelling",
      cancellation_requested: true,
      active_io: true,
    },
    scope.projectId,
  );
  expect(fileImportActions(interrupted)).toEqual({
    retry: false,
    cancel: false,
    reconcile: true,
  });
  expect(
    fileImportActions({ ...interrupted, reconciliation_requested: true })
      .reconcile,
  ).toBe(false);
  expect(
    fileImportActions({ ...interrupted, can_control: false }).reconcile,
  ).toBe(false);
});
it("原202回执只匹配永久意图，创建核冻结顺序，控制不把旧revision覆盖当前", () => {
  const create = {
    action: "create" as const,
    body: {
      expected_revision: 0,
      rights_confirmed: true as const,
      asset_ids: [id(5), id(7)],
    },
  };
  const admission = {
    ...job,
    revision: 1,
    status: "queued",
    stage: "queued",
    latest_script_revision: 0,
    latest_version_id: undefined,
    retryable: false,
    files: job.files.map((file) => ({
      ...file,
      status: "queued",
      source_id: undefined,
      source_lineage_id: undefined,
      failure_code: undefined,
    })),
  };
  expect(readFileImportReceipt(admission, scope.projectId, create).id).toBe(
    job.id,
  );
  expect(() =>
    readFileImportReceipt(admission, scope.projectId, {
      ...create,
      body: { ...create.body, asset_ids: [id(7), id(5)] },
    }),
  ).toThrow();
  expect(
    readFileImportReceipt(
      { ...job, status: "queued", stage: "queued", attempt: 2 },
      scope.projectId,
      {
        action: "retry",
        jobId: job.id,
        body: { expected_revision: 7 },
      },
    ).revision,
  ).toBe(8);
  expect(() => readFileImportReceipt(job, scope.projectId, create)).toThrow();
  expect(() =>
    readFileImportReceipt(job, scope.projectId, {
      action: "cancel",
      jobId: job.id,
      body: { expected_revision: 7 },
    }),
  ).toThrow();
  expect(() =>
    readFileImportReceipt(job, scope.projectId, {
      action: "reconcile",
      jobId: job.id,
      body: { expected_revision: 7 },
    }),
  ).toThrow();
  expect(() =>
    readFileImportReceipt(job, scope.projectId, {
      action: "retry",
      jobId: id(9),
      body: { expected_revision: 7 },
    }),
  ).toThrow();
});
