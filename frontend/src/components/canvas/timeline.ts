// Adapted from BeefTV 1ae25027 timeline-placement.ts and timeline-snap.ts,
// Origin: yoqu/lingji-cut, Copyright 2026 yoqu, Apache-2.0; adapted via BeefTV.
// See workspace/content/licenses/lingji-cut.txt and THIRD_PARTY_NOTICES.md.
// Lanverse uses project asset UUIDs and validates persistent config at the boundary.
import { z } from "zod";

const MAX_TIME = 86_400_000;
const kindSchema = z.enum(["video", "audio", "image", "text", "subtitle"]);
const time = z.number().int().min(0).max(MAX_TIME);
const cropSchema = z
  .object({
    x: z.number().int().min(0).max(32768),
    y: z.number().int().min(0).max(32768),
    width: z.number().int().min(2).max(32768),
    height: z.number().int().min(2).max(32768),
  })
  .strict();
const clipSchema = z
  .object({
    id: z.string().uuid(),
    trackId: z.string().uuid(),
    kind: kindSchema,
    nodeId: z.string().uuid().nullable(),
    assetId: z.string().uuid().nullable(),
    title: z.string().max(128),
    startMs: time,
    durationMs: z.number().int().min(100).max(MAX_TIME),
    sourceStartMs: time,
    sourceDurationMs: z.number().int().min(100).max(MAX_TIME).nullable(),
    volume: z.number().min(0).max(2),
    fadeInMs: time,
    fadeOutMs: time,
    text: z.string().max(10000),
    crop: cropSchema.nullable().optional(),
  })
  .strict();
export const timelineSchema = z
  .object({
    version: z.literal(1),
    aspectRatio: z.enum(["16:9", "9:16", "1:1"]),
    fps: z.union([z.literal(24), z.literal(25), z.literal(30), z.literal(60)]),
    tracks: z
      .array(
        z
          .object({
            id: z.string().uuid(),
            kind: kindSchema,
            label: z.string().min(1).max(128),
            locked: z.boolean(),
            visible: z.boolean(),
            muted: z.boolean(),
          })
          .strict(),
      )
      .max(32),
    clips: z.array(clipSchema).max(1000),
    subtitleStyle: z
      .object({
        fontSize: z.number().int().min(12).max(96),
        color: z.string().regex(/^#[0-9a-f]{6}$/i),
        position: z.enum(["top", "center", "bottom"]),
      })
      .strict(),
    snapping: z.boolean(),
  })
  .strict()
  .superRefine((value, context) => {
    const tracks = new Map(value.tracks.map((track) => [track.id, track]));
    if (
      tracks.size !== value.tracks.length ||
      new Set(value.clips.map((clip) => clip.id)).size !== value.clips.length
    )
      context.addIssue({ code: "custom", message: "轨道或片段身份重复。" });
    for (const clip of value.clips) {
      if (
        clip.crop &&
        (!["video", "image"].includes(clip.kind) ||
          clip.crop.x + clip.crop.width > 32768 ||
          clip.crop.y + clip.crop.height > 32768 ||
          (clip.kind === "video" &&
            (clip.crop.width % 2 !== 0 || clip.crop.height % 2 !== 0)))
      )
        context.addIssue({
          code: "custom",
          message:
            "裁切只支持图片与视频，视频裁切尺寸须为偶数且不能超过像素范围。",
        });
      if (tracks.get(clip.trackId)?.kind !== clip.kind)
        context.addIssue({ code: "custom", message: "片段与轨道类型不匹配。" });
      if (
        clip.startMs + clip.durationMs > MAX_TIME ||
        clip.fadeInMs > clip.durationMs ||
        clip.fadeOutMs > clip.durationMs ||
        (clip.sourceDurationMs !== null &&
          clip.sourceStartMs + clip.durationMs > clip.sourceDurationMs)
      )
        context.addIssue({ code: "custom", message: "片段时间超出允许范围。" });
      if (
        value.clips.some(
          (other) =>
            other.id !== clip.id &&
            other.trackId === clip.trackId &&
            clipsOverlap(clip, other),
        )
      )
        context.addIssue({
          code: "custom",
          message: "同一轨道的片段不能重叠。",
        });
    }
  });
export type TimelineProject = z.infer<typeof timelineSchema>;
export type TimelineClip = TimelineProject["clips"][number];
export type TimelineTrack = TimelineProject["tracks"][number];
export function createTimeline(): TimelineProject {
  return {
    version: 1,
    aspectRatio: "16:9",
    fps: 24,
    snapping: true,
    subtitleStyle: { fontSize: 24, color: "#ffffff", position: "bottom" },
    tracks: ["video", "audio", "image", "text", "subtitle"].map(
      (kind, index) => ({
        id: crypto.randomUUID(),
        kind: kind as TimelineTrack["kind"],
        label: ["视频", "音频", "图片", "文字", "字幕"][index],
        locked: false,
        visible: true,
        muted: false,
      }),
    ),
    clips: [],
  };
}
export function timelineDuration(timeline: TimelineProject) {
  return Math.max(
    0,
    ...timeline.clips.map((clip) => clip.startMs + clip.durationMs),
  );
}
export function splitTimelineClip(
  clip: TimelineClip,
  timeMs: number,
  rightId = crypto.randomUUID(),
): [TimelineClip, TimelineClip] | null {
  const offset = Math.round(timeMs - clip.startMs);
  if (offset < 100 || offset > clip.durationMs - 100) return null;
  return [
    {
      ...clip,
      durationMs: offset,
      fadeOutMs: 0,
      fadeInMs: Math.min(clip.fadeInMs, offset),
    },
    {
      ...clip,
      id: rightId,
      startMs: clip.startMs + offset,
      sourceStartMs: clip.sourceStartMs + offset,
      durationMs: clip.durationMs - offset,
      fadeInMs: 0,
      fadeOutMs: Math.min(clip.fadeOutMs, clip.durationMs - offset),
    },
  ];
}
export function encodeTimeline(value: TimelineProject) {
  const config = timelineSchema.parse(value);
  return {
    version: config.version,
    aspect_ratio: config.aspectRatio,
    fps: config.fps,
    snapping: config.snapping,
    subtitle_style: {
      font_size: config.subtitleStyle.fontSize,
      color: config.subtitleStyle.color,
      position: config.subtitleStyle.position,
    },
    tracks: config.tracks,
    clips: config.clips.map((clip) => ({
      id: clip.id,
      track_id: clip.trackId,
      kind: clip.kind,
      node_id: clip.nodeId,
      asset_id: clip.assetId,
      title: clip.title,
      start_ms: clip.startMs,
      duration_ms: clip.durationMs,
      source_start_ms: clip.sourceStartMs,
      source_duration_ms: clip.sourceDurationMs,
      volume: clip.volume,
      fade_in_ms: clip.fadeInMs,
      fade_out_ms: clip.fadeOutMs,
      text: clip.text,
      ...(clip.crop !== undefined ? { crop: clip.crop } : {}),
    })),
  };
}
export const timelineWireSchema = z
  .object({
    version: z.literal(1),
    aspect_ratio: z.string(),
    fps: z.number(),
    snapping: z.boolean(),
    subtitle_style: z
      .object({
        font_size: z.number(),
        color: z.string(),
        position: z.string(),
      })
      .strict(),
    tracks: z.array(
      z
        .object({
          id: z.string(),
          kind: z.string(),
          label: z.string(),
          locked: z.boolean(),
          visible: z.boolean(),
          muted: z.boolean(),
        })
        .strict(),
    ),
    clips: z.array(
      z
        .object({
          id: z.string(),
          track_id: z.string(),
          kind: z.string(),
          node_id: z.string().nullable(),
          asset_id: z.string().nullable(),
          title: z.string(),
          start_ms: z.number(),
          duration_ms: z.number(),
          source_start_ms: z.number(),
          source_duration_ms: z.number().nullable(),
          volume: z.number(),
          fade_in_ms: z.number(),
          fade_out_ms: z.number(),
          text: z.string(),
          crop: cropSchema.nullable().optional(),
        })
        .strict(),
    ),
  })
  .strict();
export function decodeTimeline(value: unknown): TimelineProject {
  const wire = timelineWireSchema.parse(value);
  return timelineSchema.parse({
    version: wire.version,
    aspectRatio: wire.aspect_ratio,
    fps: wire.fps,
    snapping: wire.snapping,
    tracks: wire.tracks,
    subtitleStyle: {
      fontSize: wire.subtitle_style.font_size,
      color: wire.subtitle_style.color,
      position: wire.subtitle_style.position,
    },
    clips: wire.clips.map((clip) => ({
      id: clip.id,
      trackId: clip.track_id,
      kind: clip.kind,
      nodeId: clip.node_id,
      assetId: clip.asset_id,
      title: clip.title,
      startMs: clip.start_ms,
      durationMs: clip.duration_ms,
      sourceStartMs: clip.source_start_ms,
      sourceDurationMs: clip.source_duration_ms,
      volume: clip.volume,
      fadeInMs: clip.fade_in_ms,
      fadeOutMs: clip.fade_out_ms,
      text: clip.text,
      ...(clip.crop !== undefined ? { crop: clip.crop } : {}),
    })),
  });
}
// 时间线放置算法（移植自 lingji-cut 的 timeline-placement.ts）。
// 提供同轨碰撞检测、最近可用位置查找、跨轨放置与时长钳制。

export type PlacementResult = {
  startMs: number;
  fits: boolean;
};

export type FindNearestArgs = {
  targetStartMs: number;
  durationMs: number;
  trackId: string;
  excludeClipId?: string;
  clips: TimelineClip[];
};

export type PlacementTrackResult = {
  trackId: string | null;
  startMs: number;
};

export type FindAvailableTrackArgs = {
  targetStartMs: number;
  durationMs: number;
  excludeClipId?: string;
  clips: TimelineClip[];
  tracks: TimelineTrack[];
};

export type ClampDurationArgs = {
  clipId: string;
  startMs: number;
  requestedDurationMs: number;
  trackId: string;
  clips: TimelineClip[];
  maxDurationMs?: number;
};

/** 半开区间 [startMs, startMs + durationMs) 重叠判断 */
export function clipsOverlap(
  left: { startMs: number; durationMs: number },
  right: { startMs: number; durationMs: number },
): boolean {
  return (
    left.startMs < right.startMs + right.durationMs &&
    right.startMs < left.startMs + left.durationMs
  );
}

export type CanPlaceAtArgs = {
  trackId: string;
  startMs: number;
  durationMs: number;
  excludeClipId?: string;
  clips: TimelineClip[];
};

export type CanPlaceAtResult = {
  ok: boolean;
  reason?: "overlap";
};

/**
 * 判断指定轨道的区间 [startMs, startMs+durationMs) 是否可放置，
 * 不做任何自动 snap/偏移；遇到任何同轨片段重叠即返回 ok=false。
 */
export function canPlaceAt(args: CanPlaceAtArgs): CanPlaceAtResult {
  const { trackId, startMs, durationMs, excludeClipId, clips } = args;
  const candidate = { startMs, durationMs };
  for (const other of clips) {
    if (other.trackId !== trackId) continue;
    if (other.id === excludeClipId) continue;
    if (clipsOverlap(candidate, other)) {
      return { ok: false, reason: "overlap" };
    }
  }
  return { ok: true };
}

function getSortedClipsOnTrack(
  trackId: string,
  clips: TimelineClip[],
  excludeClipId?: string,
): TimelineClip[] {
  return clips
    .filter((c) => c.trackId === trackId && c.id !== excludeClipId)
    .sort((a, b) => a.startMs - b.startMs);
}

/** 在指定轨道上寻找离 targetStartMs 最近的可用放置位置 */
export function findNearestAvailablePlacement(
  args: FindNearestArgs,
): PlacementResult {
  const { durationMs, trackId, excludeClipId, clips } = args;
  const targetStartMs = Math.max(0, args.targetStartMs);
  const managed = getSortedClipsOnTrack(trackId, clips, excludeClipId);

  if (managed.length === 0) {
    return { startMs: targetStartMs, fits: true };
  }

  const candidate = { startMs: targetStartMs, durationMs };
  const hasConflict = managed.some((c) => clipsOverlap(candidate, c));
  if (!hasConflict) {
    return { startMs: targetStartMs, fits: true };
  }

  // 目标位置有冲突，扫描所有间隙寻找最近的可用位置：
  // 第一个片段之前、片段之间、最后一个片段之后（无限大）。
  type Gap = { start: number; end: number };
  const gaps: Gap[] = [];

  if (managed[0].startMs > 0) {
    gaps.push({ start: 0, end: managed[0].startMs });
  }

  for (let i = 0; i < managed.length - 1; i++) {
    const gapStart = managed[i].startMs + managed[i].durationMs;
    const gapEnd = managed[i + 1].startMs;
    if (gapEnd > gapStart) {
      gaps.push({ start: gapStart, end: gapEnd });
    }
  }

  const lastEnd =
    managed[managed.length - 1].startMs +
    managed[managed.length - 1].durationMs;
  gaps.push({ start: lastEnd, end: Number.POSITIVE_INFINITY });

  let bestStart: number | null = null;
  let bestDistance = Number.POSITIVE_INFINITY;

  for (const gap of gaps) {
    const gapSize = gap.end - gap.start;
    if (gapSize < durationMs) continue;

    let candidateStart: number;
    if (targetStartMs >= gap.start && targetStartMs + durationMs <= gap.end) {
      candidateStart = targetStartMs;
    } else if (targetStartMs < gap.start) {
      candidateStart = gap.start;
    } else {
      candidateStart = Math.max(gap.start, gap.end - durationMs);
    }

    if (
      candidateStart + durationMs > gap.end &&
      gap.end !== Number.POSITIVE_INFINITY
    ) {
      continue;
    }

    const distance = Math.abs(candidateStart - targetStartMs);
    if (distance < bestDistance) {
      bestDistance = distance;
      bestStart = candidateStart;
    }
  }

  if (bestStart !== null) {
    return { startMs: bestStart, fits: true };
  }

  return { startMs: targetStartMs, fits: false };
}

// 时间线吸附算法（移植自 lingji-cut 的 timeline-snap.ts）。
// 候选时间点吸附到播放头或其它片段边缘（起点/终点），取阈值内最近目标。

export type SnapTargetKind = "playhead" | "clip-edge";

export type SnapTarget = {
  ms: number;
  kind: SnapTargetKind;
};

export type ComputeSnapArgs = {
  candidateMs: number;
  playheadMs: number;
  clips: TimelineClip[];
  excludeClipId?: string;
  pxPerMs: number;
  thresholdPx: number;
  enabled: boolean;
};

export type ComputeSnapResult = {
  snappedMs: number;
  targets: SnapTarget[];
};

export function computeSnap(args: ComputeSnapArgs): ComputeSnapResult {
  const {
    candidateMs,
    playheadMs,
    clips,
    excludeClipId,
    pxPerMs,
    thresholdPx,
    enabled,
  } = args;
  if (!enabled) {
    return { snappedMs: candidateMs, targets: [] };
  }

  const thresholdMs = thresholdPx / Math.max(pxPerMs, 1e-6);
  const candidates: SnapTarget[] = [];

  if (Math.abs(candidateMs - playheadMs) <= thresholdMs) {
    candidates.push({ ms: playheadMs, kind: "playhead" });
  }

  for (const clip of clips) {
    if (clip.id === excludeClipId) continue;
    const start = clip.startMs;
    const end = clip.startMs + clip.durationMs;
    if (Math.abs(candidateMs - start) <= thresholdMs) {
      candidates.push({ ms: start, kind: "clip-edge" });
    }
    if (Math.abs(candidateMs - end) <= thresholdMs) {
      candidates.push({ ms: end, kind: "clip-edge" });
    }
  }

  if (candidates.length === 0) {
    return { snappedMs: candidateMs, targets: [] };
  }

  candidates.sort(
    (a, b) => Math.abs(a.ms - candidateMs) - Math.abs(b.ms - candidateMs),
  );
  const chosen = candidates[0];
  const sameMsTargets = candidates.filter((t) => t.ms === chosen.ms);
  return { snappedMs: chosen.ms, targets: sameMsTargets };
}
