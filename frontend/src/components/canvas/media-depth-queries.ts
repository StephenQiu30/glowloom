import { z } from "zod";
import * as depths from "@/api/mediaDepths";
import { ApiError } from "@/lib/request";
import { depthIntentSchema, type DepthIntent } from "./media-depth-intent";
import {
  depthActions,
  depthJobSchema,
  depthUUID,
  parseDepthPreview,
  type DepthJob,
} from "./media-depth-model";

export const MEDIA_DEPTHS_KEY = ["canvas", "media-depths"] as const;
function job(value: unknown, projectId: string, jobId?: string): DepthJob {
  const checked = depthJobSchema.safeParse(value);
  if (
    !checked.success ||
    checked.data.project_id !== projectId ||
    (jobId && checked.data.id !== jobId)
  )
    throw new ApiError(502, "invalid_response");
  return checked.data;
}
export async function listDepths(
  projectId: string,
  canvasId?: string,
  nodeId?: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  const result = z
    .object({
      items: z.array(depthJobSchema).max(25),
      next_cursor: z.string().min(1).nullable(),
      current_actor_id: depthUUID,
      current_org_id: depthUUID,
    })
    .strict()
    .safeParse(
      await depths.listMediaDepths(
        {
          pid: projectId,
          canvas_id: canvasId,
          node_id: nodeId,
          limit: 25,
          cursor,
        },
        { signal },
      ),
    );
  if (
    !result.success ||
    result.data.items.some(
      (item) =>
        item.project_id !== projectId ||
        (canvasId && item.source.canvas_id !== canvasId) ||
        (nodeId && item.source.node_id !== nodeId),
    )
  )
    throw new ApiError(502, "invalid_response");
  return result.data;
}
export async function getDepth(
  projectId: string,
  id: string,
  signal?: AbortSignal,
) {
  return job(
    await depths.getMediaDepth(
      { job_id: id, project_id: projectId },
      { signal },
    ),
    projectId,
    id,
  );
}
export async function runDepthIntent(intent: DepthIntent) {
  const checked = depthIntentSchema.parse(intent);
  const options = {
    headers: { "Idempotency-Key": checked.key, Origin: checked.origin },
  };
  let response: unknown;
  if (checked.action === "create")
    response = await depths.createMediaDepth(
      { pid: checked.projectId },
      checked.body,
      options,
    );
  else if (checked.action === "review")
    response = await depths.reviewMediaDepth(
      { job_id: checked.jobId },
      checked.body,
      options,
    );
  else
    response = await {
      cancel: depths.cancelMediaDepth,
      retry: depths.retryMediaDepth,
      reconcile: depths.reconcileMediaDepth,
    }[checked.action]({ job_id: checked.jobId }, checked.body, options);
  const receipt = job(
    response,
    checked.projectId,
    checked.action === "create" ? undefined : checked.jobId,
  );
  if (
    receipt.source.canvas_id !== checked.canvasId ||
    receipt.source.node_id !== checked.nodeId ||
    (checked.action === "create" &&
      receipt.source.revision !== checked.body.revision)
  )
    throw new ApiError(502, "invalid_response");
  return receipt;
}
export async function previewDepth(value: DepthJob, signal?: AbortSignal) {
  return parseDepthPreview(
    await depths.previewMediaDepth(
      { job_id: value.id, project_id: value.project_id },
      { signal },
    ),
    value,
  );
}
export async function downloadDepth(value: DepthJob, signal?: AbortSignal) {
  if (!depthActions(value).download)
    throw new Error("人工审核通过后才能正式下载深度视频。");
  const result: unknown = await depths.downloadMediaDepth(
    { job_id: value.id, project_id: value.project_id },
    { responseType: "blob", signal },
  );
  if (
    !(result instanceof Blob) ||
    result.size < 1 ||
    result.size > 500 * 1024 * 1024 ||
    result.type.split(";")[0] !== "video/mp4"
  )
    throw new ApiError(502, "invalid_response");
  return result;
}
