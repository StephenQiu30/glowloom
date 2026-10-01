import { z } from "zod";
import { timelineSchema, type TimelineProject } from "./timeline";

const time = z.number().int().min(0).max(86_400_000);
const sha = z.string().regex(/^[0-9a-f]{64}$/);
export const transcriptionSourceSchema = z.object({
  canvas_id: z.string().uuid(),
  node_id: z.string().uuid(),
  revision: z.number().int().positive(),
});
export type TranscriptionSource = z.infer<typeof transcriptionSourceSchema>;

export const transcriptSchema = z
  .object({
    version: z.literal(1),
    language: z.string().min(1).max(64),
    duration_ms: time.refine((value) => value > 0),
    segments: z
      .array(
        z.object({
          start_ms: time,
          end_ms: time,
          text: z
            .string()
            .min(1)
            .refine(
              (value) =>
                Boolean(value.trim()) &&
                new TextEncoder().encode(value).length <= 32768 &&
                !/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/.test(
                  value,
                ),
              "字幕文本含有无效字符或过长。",
            ),
        }),
      )
      .min(1)
      .max(10000),
  })
  .superRefine((value, context) => {
    let previous = 0;
    for (const segment of value.segments) {
      if (
        segment.start_ms < previous ||
        segment.end_ms <= segment.start_ms ||
        segment.end_ms > value.duration_ms
      )
        context.addIssue({ code: "custom", message: "字幕时间无效或重叠。" });
      previous = segment.end_ms;
    }
    if (
      new TextEncoder().encode(JSON.stringify(value)).length >
      8 * 1024 * 1024
    )
      context.addIssue({ code: "custom", message: "字幕稿超过 8 MiB。" });
  });
export type Transcript = z.infer<typeof transcriptSchema>;

export const transcriptionJobSchema = z.object({
  id: z.string().uuid(),
  project_id: z.string().uuid(),
  source: transcriptionSourceSchema,
  language: z.string().min(1).max(64),
  status: z.enum([
    "queued",
    "running",
    "succeeded",
    "failed",
    "cancel_requested",
    "cancelled",
  ]),
  stage: z.string(),
  progress: z.number().int().min(0).max(100),
  attempt: z.number().int().min(1).max(100),
  revision: z.number().int().positive(),
  result_sha256: sha.nullable(),
  failure_code: z.string().nullable(),
  created_at: z.iso.datetime({ offset: true }),
  updated_at: z.iso.datetime({ offset: true }),
});
export type TranscriptionJob = z.infer<typeof transcriptionJobSchema>;
export const transcriptionResultSchema = z.object({
  job_id: z.string().uuid(),
  revision: z.number().int().positive(),
  sha256: sha,
  source_asset_id: z.string().uuid(),
  source_asset_revision: z.number().int().positive(),
  source_sha256: sha,
  draft: transcriptSchema,
  srt: z
    .string()
    .min(1)
    .max(8 * 1024 * 1024),
});
export type TranscriptionResult = z.infer<typeof transcriptionResultSchema>;

// The reviewer selects one placement and one subtitle track. Recognition never
// changes the source clip, other tracks, saved canvas, or execution facts.
export function applyTranscript(
  value: TimelineProject,
  transcript: Transcript,
  sourceClipId: string,
  targetTrackId: string,
  sourceAssetId: string,
  sourceNodeId: string,
): TimelineProject {
  const timeline = timelineSchema.parse(value);
  const draft = transcriptSchema.parse(transcript);
  const source = timeline.clips.find((clip) => clip.id === sourceClipId);
  const target = timeline.tracks.find((track) => track.id === targetTrackId);
  if (
    !source ||
    !["audio", "video"].includes(source.kind) ||
    (source.assetId !== null
      ? source.assetId !== sourceAssetId
      : source.nodeId !== sourceNodeId) ||
    (source.nodeId !== null && source.nodeId !== sourceNodeId) ||
    source.sourceStartMs + source.durationMs > draft.duration_ms
  )
    throw new Error("所选片段与已冻结的转写来源不一致。");
  if (!target || target.kind !== "subtitle" || target.locked)
    throw new Error("请选择未锁定的字幕轨道。");
  const entries = draft.segments.flatMap((segment) => {
    const start = Math.max(segment.start_ms, source.sourceStartMs);
    const end = Math.min(
      segment.end_ms,
      source.sourceStartMs + source.durationMs,
    );
    return end > start ? [{ ...segment, start_ms: start, end_ms: end }] : [];
  });
  if (!entries.length) throw new Error("所选片段范围内没有识别字幕。");
  if (entries.some((entry) => entry.end_ms - entry.start_ms < 100))
    throw new Error("片段裁剪后有字幕短于 100ms，请先编辑字幕再导入。");
  const kept = timeline.clips.filter((clip) => clip.trackId !== targetTrackId);
  if (kept.length + entries.length > 1000)
    throw new Error("采用后将超过 1000 个片段，原时间线保持不变。");
  return timelineSchema.parse({
    ...timeline,
    clips: [
      ...kept,
      ...entries.map((entry, index) => ({
        id: crypto.randomUUID(),
        trackId: targetTrackId,
        kind: "subtitle",
        nodeId: null,
        assetId: null,
        title: `字幕 ${index + 1}`,
        startMs: source.startMs + entry.start_ms - source.sourceStartMs,
        durationMs: entry.end_ms - entry.start_ms,
        sourceStartMs: 0,
        sourceDurationMs: null,
        volume: 1,
        fadeInMs: 0,
        fadeOutMs: 0,
        text: entry.text,
      })),
    ],
  });
}
