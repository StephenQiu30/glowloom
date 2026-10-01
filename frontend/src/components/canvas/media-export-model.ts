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
    output_kind: z.enum(["video", "audio"]),
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
  waveform: z
    .object({
      url: z.string().url(),
      expires_at: z.iso.datetime({ offset: true }),
      width: z.number().int().min(1).max(8192),
      height: z.number().int().min(1).max(8192),
    })
    .optional(),
  asset: mediaSchema.extend({
    kind: z.enum(["video", "audio"]),
    mime_type: z.enum(["video/mp4", "audio/mp4"]),
    byte_size: z.number().int().min(1),
    duration_ms: z.number().int().min(1).max(86_400_000),
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
  const waveformURL = preview.waveform
    ? new URL(preview.waveform.url)
    : undefined;
  if (
    preview.job_id !== job.id ||
    preview.revision !== job.revision ||
    preview.sha256 !== job.sha256 ||
    preview.asset.project_id !== job.project_id ||
    preview.asset.id !== job.asset_id ||
    preview.asset.kind !== job.output_kind ||
    preview.asset.mime_type !==
      (job.output_kind === "audio" ? "audio/mp4" : "video/mp4") ||
    (job.output_kind === "audio" && !preview.waveform) ||
    (preview.waveform &&
      (!waveformURL ||
        !["http:", "https:"].includes(waveformURL.protocol) ||
        waveformURL.username ||
        waveformURL.password ||
        Date.parse(preview.waveform.expires_at) <= Date.now() + 5000 ||
        preview.waveform.width * preview.waveform.height > 1_048_576)) ||
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    Date.parse(preview.expires_at) <= Date.now() + 5000 ||
    !["review_required", "succeeded"].includes(job.status)
  )
    throw new ApiError(502, "invalid_response");
  return preview;
}
