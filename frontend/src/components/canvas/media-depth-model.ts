import { z } from "zod";
import { ApiError } from "@/lib/request";
import { mediaSchema, type MediaAsset } from "./queries";
import { mediaAssetsToCanvasNodes } from "./media-import";
import {
  CanvasNodeType,
  type CanvasCommand,
  type CanvasDocument,
} from "./model";

export const depthUUID = z
  .string()
  .uuid()
  .refine((id) => id !== "00000000-0000-0000-0000-000000000000");
export const depthRevision = z
  .number()
  .int()
  .positive()
  .max(Number.MAX_SAFE_INTEGER);
export const depthSHA = z.string().regex(/^[a-f0-9]{64}$/);
export const depthSourceSchema = z
  .object({ canvas_id: depthUUID, node_id: depthUUID, revision: depthRevision })
  .strict();
export type DepthSource = z.infer<typeof depthSourceSchema>;
export const depthStageLabels = {
  queued: "等待执行",
  downloading: "读取冻结视频原件",
  checking: "核验模型与运行条件",
  preparing: "预处理完整视频",
  loading: "加载固定 Small 模型",
  inferring: "推理相对深度",
  encoding: "编码完整深度视频",
  verifying: "全片解码与字节核验",
  previews: "生成真实预览",
  storing: "保存私有结果",
  review: "等待人工审核",
  complete: "审核完成",
  cleanup: "核验并清理未审核结果",
  cancelling: "取消请求已记录",
  awaiting_reconciliation: "等待原结果核验",
  reconciling: "核验原尝试结果",
  failed: "执行失败",
  cancelled: "已取消并完成清理",
} as const;
export const depthJobSchema = z
  .object({
    id: depthUUID,
    project_id: depthUUID,
    source: depthSourceSchema,
    source_asset_id: depthUUID,
    source_asset_revision: depthRevision,
    source_sha256: depthSHA,
    profile_id: z.literal("vda-small-relative-v1"),
    status: z.enum([
      "queued",
      "running",
      "review_required",
      "succeeded",
      "failed",
      "cancel_requested",
      "cancelled",
    ]),
    stage: z.enum(
      Object.keys(depthStageLabels) as [
        keyof typeof depthStageLabels,
        ...(keyof typeof depthStageLabels)[],
      ],
    ),
    attempt: z.number().int().min(1).max(100),
    revision: depthRevision,
    asset_id: depthUUID.nullable(),
    sha256: depthSHA.nullable(),
    failure_code: z.string().min(1).max(128).nullable(),
    retryable: z.boolean(),
    needs_reconciliation: z.boolean(),
    reconciliation_requested: z.boolean(),
    execution_unconfirmed: z.boolean(),
    cancellation_requested: z.boolean(),
    created_at: z.iso.datetime({ offset: true }),
    updated_at: z.iso.datetime({ offset: true }),
  })
  .strict()
  .refine(
    (job) =>
      Boolean(job.asset_id) === Boolean(job.sha256) &&
      job.asset_id !== job.source_asset_id &&
      (!["review_required", "succeeded"].includes(job.status) ||
        Boolean(job.asset_id && job.sha256)) &&
      (!job.execution_unconfirmed ||
        (job.needs_reconciliation && !job.retryable)) &&
      (!job.reconciliation_requested || job.needs_reconciliation),
  );
export type DepthJob = z.infer<typeof depthJobSchema>;
export function depthActions(job: DepthJob) {
  const stopped = !job.execution_unconfirmed;
  return {
    cancel:
      !["succeeded", "cancelled", "cancel_requested"].includes(job.status) &&
      !job.cancellation_requested,
    retry:
      stopped &&
      ["failed", "cancelled"].includes(job.status) &&
      job.retryable &&
      !job.needs_reconciliation &&
      !job.reconciliation_requested &&
      (job.status === "cancelled" || !job.cancellation_requested),
    reconcile:
      stopped &&
      job.status === "failed" &&
      job.needs_reconciliation &&
      !job.reconciliation_requested,
    review:
      job.status === "review_required" &&
      !job.cancellation_requested &&
      !job.execution_unconfirmed &&
      !job.needs_reconciliation,
    download: job.status === "succeeded",
    adopt: job.status === "succeeded",
  };
}
const depthPreviewSchema = z
  .object({
    job_id: depthUUID,
    revision: depthRevision,
    sha256: depthSHA,
    url: z.string().url(),
    expires_at: z.iso.datetime({ offset: true }),
    asset: mediaSchema
      .extend({
        kind: z.literal("video"),
        mime_type: z.literal("video/mp4"),
        byte_size: z
          .number()
          .int()
          .min(1)
          .max(500 * 1024 * 1024),
        duration_ms: z.number().int().min(1).max(15100),
        width: z.literal(1920),
        height: z.literal(1080),
      })
      .strict(),
  })
  .strict();
export type DepthPreview = z.infer<typeof depthPreviewSchema>;
export function parseDepthPreview(value: unknown, job: DepthJob): DepthPreview {
  const parsed = depthPreviewSchema.safeParse(value);
  if (!parsed.success) throw new ApiError(502, "invalid_response");
  const preview = parsed.data,
    url = new URL(preview.url);
  if (
    !["review_required", "succeeded"].includes(job.status) ||
    preview.job_id !== job.id ||
    preview.revision !== job.revision ||
    preview.sha256 !== job.sha256 ||
    preview.asset.id !== job.asset_id ||
    preview.asset.project_id !== job.project_id ||
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    Date.parse(preview.expires_at) <= Date.now() + 5000
  )
    throw new ApiError(502, "invalid_response");
  return preview;
}
export function newerDepth(
  current: DepthJob | undefined,
  incoming: DepthJob,
): DepthJob {
  if (current?.id !== incoming.id) return incoming;
  if (
    current.project_id !== incoming.project_id ||
    current.source.canvas_id !== incoming.source.canvas_id ||
    current.source.node_id !== incoming.source.node_id ||
    current.source.revision !== incoming.source.revision ||
    current.source_asset_id !== incoming.source_asset_id ||
    current.source_asset_revision !== incoming.source_asset_revision ||
    current.source_sha256 !== incoming.source_sha256 ||
    current.profile_id !== incoming.profile_id
  )
    throw new ApiError(502, "invalid_response");
  if (current.revision >= incoming.revision) return current;
  if (
    incoming.attempt < current.attempt ||
    (incoming.attempt === current.attempt &&
      ((current.asset_id &&
        incoming.asset_id &&
        current.asset_id !== incoming.asset_id) ||
        (current.sha256 &&
          incoming.sha256 &&
          current.sha256 !== incoming.sha256)))
  )
    throw new ApiError(502, "invalid_response");
  return incoming;
}
export function depthResultCommands(
  document: CanvasDocument,
  job: DepthJob,
  asset: MediaAsset,
): CanvasCommand[] {
  const source = document.nodes.find((node) => node.id === job.source.node_id);
  if (
    job.status !== "succeeded" ||
    document.id !== job.source.canvas_id ||
    document.projectId !== job.project_id ||
    !source ||
    source.type !== CanvasNodeType.Video ||
    source.assetId !== job.source_asset_id ||
    asset.id !== job.asset_id ||
    asset.project_id !== job.project_id ||
    asset.kind !== "video" ||
    asset.mime_type !== "video/mp4" ||
    asset.width !== 1920 ||
    asset.height !== 1080
  )
    throw new Error(
      "请选择与冻结来源一致的画布和视频，且只采纳当前项目已审核的深度结果。",
    );
  const existing = document.nodes.filter(
    (node) => node.type === CanvasNodeType.Video && node.assetId === asset.id,
  );
  if (
    existing.some((node) =>
      document.connections.some(
        (edge) => edge.fromNodeId === source.id && edge.toNodeId === node.id,
      ),
    )
  )
    return [];
  if (
    (!existing.length && document.nodes.length >= 2000) ||
    document.connections.length >= 4000
  )
    throw new Error("画布节点或关系已达到容量上限。");
  const node =
    existing[0] ??
    mediaAssetsToCanvasNodes([asset], {
      x: source.position.x + source.width + 48,
      y: source.position.y,
    })[0];
  return [
    ...(!existing.length ? [{ type: "AddNodes" as const, nodes: [node] }] : []),
    {
      type: "Connect",
      edges: [
        { id: crypto.randomUUID(), fromNodeId: source.id, toNodeId: node.id },
      ],
    },
  ];
}

export const depthStatusLabels: Record<DepthJob["status"], string> = {
  queued: "等待执行",
  running: "正在处理",
  review_required: "等待人工审核",
  succeeded: "已审核完成",
  failed: "执行失败",
  cancel_requested: "取消请求已记录",
  cancelled: "已取消并完成清理",
};
