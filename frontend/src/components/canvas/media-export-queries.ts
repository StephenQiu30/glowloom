import { z } from "zod";
import * as exports from "@/api/mediaExports";
import { ApiError } from "@/lib/request";
import {
  exportJobSchema,
  exportSourceSchema,
  parseExportPreview,
  type ExportJob,
  type ExportPreview,
  type ExportSource,
} from "./media-export-model";

export const MEDIA_EXPORTS_KEY = ["canvas", "media-exports"] as const;
function job(value: unknown, projectId: string, id?: string): ExportJob {
  const parsed = exportJobSchema.safeParse(value);
  if (
    !parsed.success ||
    parsed.data.project_id !== projectId ||
    (id && parsed.data.id !== id)
  )
    throw new ApiError(502, "invalid_response");
  return parsed.data;
}
export async function createTimelineExport(
  projectId: string,
  source: ExportSource,
  key: string,
  outputKind: ExportJob["output_kind"] = "video",
) {
  const frozen = exportSourceSchema.parse(source);
  const result = job(
    await exports.createMediaExport(
      { pid: projectId },
      { ...frozen, output_kind: outputKind },
      {
        headers: { "Idempotency-Key": key },
      },
    ),
    projectId,
  );
  if (
    result.source.canvas_id !== frozen.canvas_id ||
    result.source.node_id !== frozen.node_id ||
    result.source.revision !== frozen.revision ||
    result.output_kind !== outputKind
  )
    throw new ApiError(502, "invalid_response");
  return result;
}
export async function listTimelineExports(
  projectId: string,
  canvasId: string,
  nodeId: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  const result = z
    .object({
      items: z.array(exportJobSchema).max(100),
      next_cursor: z.string().nullable(),
    })
    .safeParse(
      await exports.listMediaExports(
        {
          pid: projectId,
          canvas_id: canvasId,
          node_id: nodeId,
          limit: 25,
          ...(cursor ? { cursor } : {}),
        },
        { signal },
      ),
    );
  if (
    !result.success ||
    result.data.items.some(
      (item) =>
        item.project_id !== projectId ||
        item.source.canvas_id !== canvasId ||
        item.source.node_id !== nodeId,
    )
  )
    throw new ApiError(502, "invalid_response");
  return result.data;
}
export async function getTimelineExport(
  projectId: string,
  id: string,
  signal?: AbortSignal,
) {
  return job(
    await exports.getMediaExport(
      { job_id: id, project_id: projectId },
      { signal },
    ),
    projectId,
    id,
  );
}
export async function previewTimelineExport(
  value: ExportJob,
  signal?: AbortSignal,
) {
  return parseExportPreview(
    await exports.previewMediaExport(
      { job_id: value.id, project_id: value.project_id },
      { signal },
    ),
    value,
  );
}
export async function reviewTimelineExport(
  value: ExportJob,
  preview: ExportPreview,
  key: string,
) {
  parseExportPreview(preview, value);
  const result = job(
    await exports.reviewMediaExport(
      { job_id: value.id },
      {
        project_id: value.project_id,
        revision: preview.revision,
        sha256: preview.sha256,
        local_review_confirmed: true,
      },
      { headers: { "Idempotency-Key": key } },
    ),
    value.project_id,
    value.id,
  );
  if (
    result.status !== "succeeded" ||
    result.asset_id !== value.asset_id ||
    result.sha256 !== preview.sha256 ||
    result.output_kind !== value.output_kind
  )
    throw new ApiError(502, "invalid_response");
  return result;
}
export async function controlTimelineExport(
  value: ExportJob,
  action: "cancel" | "retry",
  key: string,
) {
  const invoke =
    action === "cancel" ? exports.cancelMediaExport : exports.retryMediaExport;
  return job(
    await invoke(
      { job_id: value.id },
      { project_id: value.project_id, revision: value.revision },
      { headers: { "Idempotency-Key": key } },
    ),
    value.project_id,
    value.id,
  );
}
export async function downloadTimelineSubtitles(
  value: ExportJob,
  signal?: AbortSignal,
) {
  const response: unknown = await exports.downloadMediaExportSubtitles(
    { job_id: value.id, project_id: value.project_id },
    { responseType: "blob", signal },
  );
  if (
    !(response instanceof Blob) ||
    response.size === 0 ||
    response.size > 1024 * 1024
  )
    throw new ApiError(502, "invalid_response");
  if (!response.type.startsWith("application/x-subrip"))
    throw new ApiError(502, "invalid_response");
  return response;
}
