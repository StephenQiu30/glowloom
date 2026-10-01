import { expect, it } from "vitest";
import { applyCommands, createNode } from "./document";
import { CanvasNodeType, type CanvasDocument } from "./model";
import {
  depthActions,
  depthJobSchema,
  depthResultCommands,
  newerDepth,
  parseDepthPreview,
} from "./media-depth-model";

const project = "b580ad59-17a5-4ed2-985b-cb0cdd04b4c4";
const canvas = "5db0c65d-d391-45a0-ab1a-d7f6eb4c868d";
const source = "0c6e50ee-0912-4da2-96b3-c84e27b61389";
const sourceAsset = "b6f81c20-2489-4276-909d-04a9d13b3e43";
const resultAsset = "c3b56d1d-b172-41cb-af65-13b8ef8d50c7";
const job = {
  id: "fba9de81-80c4-49ce-8f59-1dac39717a04",
  project_id: project,
  source: { canvas_id: canvas, node_id: source, revision: 4 },
  source_asset_id: sourceAsset,
  source_asset_revision: 1,
  source_sha256: "a".repeat(64),
  profile_id: "vda-small-relative-v1" as const,
  status: "review_required" as const,
  stage: "review" as const,
  attempt: 1,
  revision: 8,
  asset_id: resultAsset,
  sha256: "b".repeat(64),
  failure_code: null,
  retryable: false,
  needs_reconciliation: false,
  reconciliation_requested: false,
  execution_unconfirmed: false,
  cancellation_requested: false,
  created_at: "2026-10-01T19:00:00Z",
  updated_at: "2026-10-01T19:01:00Z",
};
const asset = {
  id: resultAsset,
  project_id: project,
  kind: "video" as const,
  mime_type: "video/mp4",
  file_name: "depth.mp4",
  byte_size: 20000,
  width: 1920,
  height: 1080,
  duration_ms: 2000,
  revision: 2,
};
const preview = {
  job_id: job.id,
  revision: job.revision,
  sha256: job.sha256,
  url: "http://127.0.0.1:9000/private-synthetic.mp4",
  expires_at: "2099-01-01T00:00:00Z",
  asset,
};
function document(): CanvasDocument {
  const node = createNode(CanvasNodeType.Video, { x: 200, y: 100 });
  node.id = source;
  node.assetId = sourceAsset;
  return {
    id: canvas,
    projectId: project,
    name: "合成深度画布",
    revision: 7,
    scope: {},
    viewport: { x: 0, y: 0, k: 1 },
    nodes: [node],
    connections: [],
  };
}
it("安全 DTO 只接受固定 profile 和真实阶段，不接受进度或私有执行字段", () => {
  expect(depthJobSchema.safeParse(job).success).toBe(true);
  for (const extra of [
    { progress: 80 },
    { worker_id: source },
    { receipt: {} },
  ])
    expect(depthJobSchema.safeParse({ ...job, ...extra }).success).toBe(false);
  expect(
    depthJobSchema.safeParse({ ...job, profile_id: "fake-model" }).success,
  ).toBe(false);
  expect(
    depthJobSchema.safeParse({ ...job, stage: "almost_finished" }).success,
  ).toBe(false);
});
it("待审核和已成功任务必须有独立、完整的真实输出身份", () => {
  expect(depthJobSchema.safeParse({ ...job, asset_id: null }).success).toBe(
    false,
  );
  expect(depthJobSchema.safeParse({ ...job, sha256: "partial" }).success).toBe(
    false,
  );
  expect(
    depthJobSchema.safeParse({ ...job, asset_id: sourceAsset }).success,
  ).toBe(false);
});
it("未知原生停止只能记录取消意图，不能核验或重试", () => {
  const unknown = {
    ...job,
    status: "failed" as const,
    stage: "awaiting_reconciliation" as const,
    retryable: true,
    needs_reconciliation: true,
    execution_unconfirmed: true,
  };
  expect(depthActions(unknown)).toMatchObject({
    cancel: true,
    retry: false,
    reconcile: false,
  });
});
it("已请求核验或失败取消意图跨刷新均不再次开放普通重试", () => {
  const failed = {
    ...job,
    status: "failed" as const,
    retryable: true,
    needs_reconciliation: true,
  };
  expect(depthActions(failed)).toMatchObject({ retry: false, reconcile: true });
  expect(
    depthActions({ ...failed, reconciliation_requested: true }).reconcile,
  ).toBe(false);
  expect(
    depthActions({
      ...failed,
      needs_reconciliation: false,
      cancellation_requested: true,
    }).retry,
  ).toBe(false);
  expect(
    depthActions({
      ...failed,
      status: "cancelled",
      needs_reconciliation: false,
      cancellation_requested: true,
    }).retry,
  ).toBe(true);
});
it("预览绑定确切 job/revision/SHA/项目/输出身份，不信任任意链接或过期授权", () => {
  expect(parseDepthPreview(preview, job).asset.id).toBe(resultAsset);
  for (const change of [
    { revision: 7 },
    { sha256: "c".repeat(64) },
    { asset: { ...asset, id: sourceAsset } },
    { url: "https://user:password@example.invalid/result.mp4" },
    { expires_at: "2000-01-01T00:00:00Z" },
  ])
    expect(() => parseDepthPreview({ ...preview, ...change }, job)).toThrow();
  expect(() =>
    parseDepthPreview(preview, { ...job, status: "failed" }),
  ).toThrow();
});
it("重放旧受理回执不会覆盖较新 GET，冻结来源不允许发生变化", () => {
  const succeeded = {
    ...job,
    status: "succeeded" as const,
    stage: "complete" as const,
    revision: 9,
  };
  expect(newerDepth(succeeded, job)).toBe(succeeded);
  expect(() =>
    newerDepth(job, { ...job, source_sha256: "c".repeat(64), revision: 10 }),
  ).toThrow();
  expect(() =>
    newerDepth(job, { ...job, asset_id: sourceAsset, revision: 10 }),
  ).toThrow();
  expect(
    newerDepth(job, {
      ...job,
      attempt: 2,
      revision: 10,
      asset_id: null,
      sha256: null,
      status: "queued",
      stage: "queued",
    }).attempt,
  ).toBe(2);
});
it("审核成功后以资源和来源边采纳，保留原节点；未知画布回执后实际读回去重", () => {
  const initial = document();
  const before = structuredClone(initial.nodes[0]);
  const succeeded = {
    ...job,
    status: "succeeded" as const,
    stage: "complete" as const,
  };
  const commands = depthResultCommands(initial, succeeded, asset);
  const saved = applyCommands(initial, commands);
  expect(saved.nodes[0]).toEqual(before);
  expect(saved.nodes.filter((n) => n.assetId === resultAsset)).toHaveLength(1);
  expect(saved.connections).toHaveLength(1);
  expect(depthResultCommands(saved, succeeded, asset)).toEqual([]);
});
it("未审核、跨项目或来源已变更时不能采纳，不能覆盖来源视频", () => {
  const initial = document();
  expect(() => depthResultCommands(initial, job, asset)).toThrow();
  const succeeded = {
    ...job,
    status: "succeeded" as const,
    stage: "complete" as const,
  };
  expect(() =>
    depthResultCommands(initial, succeeded, { ...asset, project_id: source }),
  ).toThrow();
  initial.nodes[0].assetId = resultAsset;
  expect(() => depthResultCommands(initial, succeeded, asset)).toThrow();
});
