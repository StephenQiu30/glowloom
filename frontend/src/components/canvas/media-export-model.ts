import { z } from "zod";
import { ApiError } from "@/lib/request";
import { mediaSchema } from "./queries";

const revision = z.number().int().positive().max(Number.MAX_SAFE_INTEGER);
export const exportSourceSchema = z
  .object({
    canvas_id: z.string().uuid(),
    node_id: z.string().uuid(),
    revision,
  })
  .strict();
export const exportJobSchema = z
  .object({
    id: z.string().uuid(),
    project_id: z.string().uuid(),
    source: exportSourceSchema,
    status: z.enum([
      "queued",
      "running",
      "review_required",
      "succeeded",
      "failed",
      "cancel_requested",
      "cancelled",
    ]),
    stage: z.string().max(128),
    progress: z.number().int().min(0).max(100),
    attempt: z.number().int().min(1).max(100),
    revision,
    asset_id: z.string().uuid().nullable(),
    sha256: z
      .string()
      .regex(/^[a-f0-9]{64}$/)
      .nullable(),
    failure_code: z.string().max(128).nullable(),
    created_at: z.iso.datetime({ offset: true }),
    updated_at: z.iso.datetime({ offset: true }),
  })
  .superRefine((job, context) => {
    if (
      (job.status === "review_required" || job.status === "succeeded") &&
      (!job.asset_id || !job.sha256)
    )
      context.addIssue({ code: "custom", message: "导出缺少真实输出身份。" });
  });
export type ExportJob = z.infer<typeof exportJobSchema>;
export type ExportSource = z.infer<typeof exportSourceSchema>;
const previewSchema = z.object({
  job_id: z.string().uuid(),
  revision,
  sha256: z.string().regex(/^[a-f0-9]{64}$/),
  url: z.string().url(),
  expires_at: z.iso.datetime({ offset: true }),
  asset: mediaSchema.extend({
    kind: z.literal("video"),
    mime_type: z.literal("video/mp4"),
    byte_size: z.number().int().min(1),
  }),
});
export type ExportPreview = z.infer<typeof previewSchema>;
export function parseExportPreview(
  value: unknown,
  job: ExportJob,
): ExportPreview {
  const result = previewSchema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  const preview = result.data;
  const url = new URL(preview.url);
  if (
    preview.job_id !== job.id ||
    preview.revision !== job.revision ||
    preview.sha256 !== job.sha256 ||
    preview.asset.project_id !== job.project_id ||
    preview.asset.id !== job.asset_id ||
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    Date.parse(preview.expires_at) <= Date.now() + 5000 ||
    !["review_required", "succeeded"].includes(job.status)
  )
    throw new ApiError(502, "invalid_response");
  return preview;
}
