import { isProjectID } from "@/components/projects/project";
import { isTimelineAspectRatio } from "./timeline-aspect-ratio";

export type TimelineTrack = Required<API.TimelineTrack>;
export type TimelineClip = Required<
  Omit<API.TimelineClip, "asset_id" | "node_id" | "source_duration_ms" | "crop">
> &
  Pick<
    API.TimelineClip,
    "asset_id" | "node_id" | "source_duration_ms" | "crop"
  >;
export type Timeline = Required<
  Omit<API.TimelineConfig, "tracks" | "clips" | "subtitle_style">
> & {
  imported_assets: string[];
  tracks: TimelineTrack[];
  clips: TimelineClip[];
  subtitle_style: Required<API.SubtitleStyle>;
};
const limit = 86400000;
const kinds = ["video", "image", "audio", "text", "subtitle"];
const visual = (kind: string) => kind === "video" || kind === "image";
const accepts = (track: string, clip: string) =>
  !!track && (track === clip || (track === "video" && clip === "image"));
const integer = (v: unknown, min = 0, max = limit): v is number =>
  Number.isSafeInteger(v) && (v as number) >= min && (v as number) <= max;
const title = (value: unknown) =>
  typeof value === "string" &&
  value.trim().length > 0 &&
  [...value].length <= 128;
export function emptyTimelineConfig(): Timeline {
  return {
    version: 1,
    aspect_ratio: "auto",
    fps: 30,
    snapping: true,
    tracks: [
      {
        id: crypto.randomUUID(),
        kind: "video",
        label: "主视觉轨",
        locked: false,
        visible: true,
        muted: false,
      },
    ],
    clips: [],
    imported_assets: [],
    subtitle_style: { font_size: 32, color: "#ffffff", position: "bottom" },
  };
}
/** Validate the existing service contract, not an inferred competitor rule. */
export function readTimeline(
  value: (API.TimelineConfig & { imported_assets?: string[] }) | undefined,
): Timeline {
  const invalid = () => {
    throw new Error("时间线配置无效");
  };
  if (
    !value ||
    value.version !== 1 ||
    !isTimelineAspectRatio(value.aspect_ratio) ||
    ![24, 25, 30, 60].includes(value.fps ?? 0) ||
    typeof value.snapping !== "boolean" ||
    !Array.isArray(value.tracks) ||
    value.tracks.length > 32 ||
    !Array.isArray(value.clips) ||
    value.clips.length > 1000
  )
    return invalid();
  const style = value.subtitle_style;
  if (
    !style ||
    !integer(style.font_size, 12, 96) ||
    !/^#[\da-fA-F]{6}$/.test(style.color ?? "") ||
    !["top", "center", "bottom"].includes(style.position ?? "")
  )
    return invalid();
  const tracks = new Map<string, API.TimelineTrack>();
  for (const track of value.tracks) {
    if (
      !isProjectID(track.id) ||
      tracks.has(track.id) ||
      !kinds.includes(track.kind ?? "") ||
      !title(track.label) ||
      [track.locked, track.visible, track.muted].some(
        (v) => typeof v !== "boolean",
      )
    )
      return invalid();
    tracks.set(track.id, track);
  }
  const ids = new Set<string>();
  for (const clip of value.clips) {
    if (
      !isProjectID(clip.id) ||
      ids.has(clip.id) ||
      !clip.track_id ||
      !accepts(tracks.get(clip.track_id)?.kind ?? "", clip.kind ?? "") ||
      !title(clip.title) ||
      !integer(clip.start_ms) ||
      !integer(clip.duration_ms, 100) ||
      clip.start_ms + clip.duration_ms > limit ||
      !integer(clip.source_start_ms) ||
      typeof clip.volume !== "number" ||
      !Number.isFinite(clip.volume) ||
      clip.volume < 0 ||
      clip.volume > 2 ||
      !integer(clip.fade_in_ms, 0, clip.duration_ms) ||
      !integer(clip.fade_out_ms, 0, clip.duration_ms) ||
      typeof clip.text !== "string" ||
      [...clip.text].length > 10000
    )
      return invalid();
    if (
      clip.source_duration_ms != null &&
      (!integer(clip.source_duration_ms, 100) ||
        clip.source_start_ms > clip.source_duration_ms ||
        (["video", "audio"].includes(clip.kind!) &&
          clip.source_start_ms + clip.duration_ms > clip.source_duration_ms))
    )
      return invalid();
    if (["text", "subtitle"].includes(clip.kind!)) {
      if (clip.asset_id || clip.node_id) return invalid();
    } else if (!isProjectID(clip.asset_id) && !isProjectID(clip.node_id))
      return invalid();
    if (
      (clip.asset_id && !isProjectID(clip.asset_id)) ||
      (clip.node_id && !isProjectID(clip.node_id))
    )
      return invalid();
    const crop = clip.crop;
    if (
      crop &&
      (!["image", "video"].includes(clip.kind!) ||
        !integer(crop.x, 0, 32768) ||
        !integer(crop.y, 0, 32768) ||
        !integer(crop.width, 2, 32768) ||
        !integer(crop.height, 2, 32768) ||
        crop.x + crop.width > 32768 ||
        crop.y + crop.height > 32768 ||
        (clip.kind === "video" &&
          (crop.width % 2 !== 0 || crop.height % 2 !== 0)))
    )
      return invalid();
    ids.add(clip.id);
  }
  for (const id of tracks.keys()) {
    const clips = value.clips
      .filter((c) => c.track_id === id)
      .sort((a, b) => a.start_ms! - b.start_ms!);
    for (let i = 1; i < clips.length; i++)
      if (
        clips[i].start_ms! <
        clips[i - 1].start_ms! + clips[i - 1].duration_ms!
      )
        throw new Error("同一轨道的片段不能重叠");
  }
  if (
    value.imported_assets != null &&
    (!Array.isArray(value.imported_assets) ||
      value.imported_assets.some((id) => !isProjectID(id)) ||
      new Set(value.imported_assets).size !== value.imported_assets.length)
  )
    return invalid();
  const imported_assets = [
    ...new Set([
      ...(value.imported_assets ?? []),
      ...value.clips.flatMap((clip) => (clip.asset_id ? [clip.asset_id] : [])),
    ]),
  ];
  if (imported_assets.length > 1000) return invalid();
  const normalized = { ...value, imported_assets };
  if (
    new TextEncoder().encode(JSON.stringify(normalized)).byteLength >
    512 * 1024
  )
    return invalid();
  return structuredClone(normalized) as Timeline;
}
export const timelineDuration = (timeline: Timeline) =>
  Math.max(0, ...timeline.clips.map((c) => c.start_ms + c.duration_ms));
export type TimelineEdit =
  | {
      type: "add";
      assets: { asset: API.AssetSummary; nodeID?: string }[];
      trackID?: string;
    }
  | { type: "import"; assets: { asset: API.AssetSummary; nodeID?: string }[] }
  | { type: "add-track"; kind: string }
  | {
      type: "track";
      id: string;
      patch: Partial<
        Pick<TimelineTrack, "label" | "locked" | "muted" | "visible">
      >;
    }
  | {
      type: "clip";
      id: string;
      patch: Partial<
        Pick<
          TimelineClip,
          | "title"
          | "start_ms"
          | "duration_ms"
          | "source_start_ms"
          | "volume"
          | "fade_in_ms"
          | "fade_out_ms"
          | "text"
          | "crop"
        >
      >;
    }
  | { type: "split" | "trim-left" | "trim-right"; id: string; at: number }
  | { type: "resize"; id: string; edge: "left" | "right"; delta: number }
  | {
      type: "move";
      id: string;
      start: number;
      trackID?: string;
      tolerance?: number;
      releaseTolerance?: number;
      previousSnap?: TimelineSnap | null;
      playhead?: number;
      newAudioTrack?: { afterTrackID: string };
    }
  | { type: "reorder"; id: string; direction: -1 | 1 }
  | { type: "remove"; id: string };
export type TimelineSnap = {
  edge: "start" | "end";
  kind: "playhead" | "clip";
  at: number;
};
/** Resolve one transient audio gesture; retain only a still-valid snap target. */
export function timelineMoveSnap(
  timeline: Timeline,
  clip: TimelineClip,
  start: number,
  playhead: number | undefined,
  tolerance: number,
  previous?: TimelineSnap | null,
  releaseTolerance = tolerance,
): { start: number; snap: TimelineSnap | null } {
  if (!timeline.snapping) return { start, snap: null };
  const anchors = new Set([
    0,
    ...timeline.clips
      .filter((item) => item.id !== clip.id)
      .flatMap((item) => [item.start_ms, item.start_ms + item.duration_ms]),
  ]);
  const position = (snap: TimelineSnap) =>
    snap.at - (snap.edge === "end" ? clip.duration_ms : 0);
  if (previous) {
    const valid =
      previous.kind === "playhead"
        ? previous.at === playhead
        : anchors.has(previous.at);
    const retained = position(previous);
    if (
      valid &&
      retained >= 0 &&
      Math.abs(retained - start) <= releaseTolerance
    )
      return { start: retained, snap: previous };
  }
  for (const kind of ["playhead", "clip"] as const) {
    const targets =
      kind === "playhead"
        ? playhead === undefined
          ? []
          : [playhead]
        : [...anchors];
    let best: TimelineSnap | null = null;
    let distance = Infinity;
    for (const edge of ["start", "end"] as const) {
      for (const at of targets) {
        const candidate = { edge, kind, at };
        const next = position(candidate);
        const difference = Math.abs(next - start);
        if (next >= 0 && difference <= tolerance && difference < distance) {
          best = candidate;
          distance = difference;
        }
      }
    }
    if (best) return { start: position(best), snap: best };
  }
  return { start, snap: null };
}
const newTrack = (kind: string, count: number): TimelineTrack => ({
  id: crypto.randomUUID(),
  kind,
  label: `${({ video: "视频", image: "图片", audio: "音频", text: "文本", subtitle: "字幕" } as Record<string, string>)[kind] ?? kind}轨 ${count + 1}`,
  locked: false,
  visible: true,
  muted: false,
});
function closeVisualLane(timeline: Timeline, trackID: string) {
  if (
    !visual(timeline.tracks.find((track) => track.id === trackID)?.kind ?? "")
  )
    return;
  let cursor = 0;
  for (const clip of timeline.clips
    .filter((item) => item.track_id === trackID)
    .sort((a, b) => a.start_ms - b.start_ms)) {
    clip.start_ms = cursor;
    cursor += clip.duration_ms;
  }
}
function removeEmptyAudioTracks(timeline: Timeline) {
  const used = new Set(timeline.clips.map((clip) => clip.track_id));
  timeline.tracks = timeline.tracks.filter(
    (track) => track.kind !== "audio" || track.locked || used.has(track.id),
  );
}
export function editTimeline(value: Timeline, edit: TimelineEdit): Timeline {
  const next = readTimeline(value);
  if (edit.type === "add-track")
    next.tracks.push(
      newTrack(
        edit.kind,
        next.tracks.filter((t) => t.kind === edit.kind).length,
      ),
    );
  else if (edit.type === "track") {
    const track = next.tracks.find((t) => t.id === edit.id);
    if (!track) throw new Error("轨道已不存在");
    Object.assign(track, edit.patch);
  } else if (edit.type === "add" || edit.type === "import") {
    for (const { asset, nodeID } of edit.assets) {
      if (
        !isProjectID(asset.id) ||
        !["video", "image", "audio"].includes(asset.kind ?? "") ||
        (nodeID && !isProjectID(nodeID))
      )
        throw new Error("素材身份无效");
      if (!next.imported_assets.includes(asset.id!))
        next.imported_assets.push(asset.id!);
      if (edit.type === "import") continue;
      const duration = asset.kind === "image" ? 5000 : asset.duration_ms;
      if (!integer(duration, 100)) throw new Error("素材时长尚未确认");
      let track = edit.trackID
        ? next.tracks.find((t) => t.id === edit.trackID)
        : next.tracks.find((t) =>
            visual(asset.kind!) ? visual(t.kind) : t.kind === asset.kind,
          );
      if (!track && edit.trackID) throw new Error("目标轨道已不存在");
      if (!track) {
        track = newTrack(
          visual(asset.kind!) ? "video" : asset.kind!,
          next.tracks.filter((t) => t.kind === asset.kind).length,
        );
        next.tracks.push(track);
      }
      if (track.locked) throw new Error("轨道已锁定");
      if (visual(track.kind) && visual(asset.kind!)) track.kind = "video";
      if (!accepts(track.kind, asset.kind!))
        throw new Error("素材与轨道类型不匹配");
      closeVisualLane(next, track.id);
      const start = Math.max(
        0,
        ...next.clips
          .filter((c) => c.track_id === track!.id)
          .map((c) => c.start_ms + c.duration_ms),
      );
      next.clips.push({
        id: crypto.randomUUID(),
        track_id: track.id,
        kind: asset.kind!,
        asset_id: asset.id,
        ...(nodeID ? { node_id: nodeID } : {}),
        title: asset.file_name?.slice(0, 128) || "素材",
        start_ms: start,
        duration_ms: duration,
        source_start_ms: 0,
        ...(asset.duration_ms ? { source_duration_ms: asset.duration_ms } : {}),
        volume: 1,
        fade_in_ms: 0,
        fade_out_ms: 0,
        text: "",
      });
    }
  } else {
    const clip = next.clips.find((c) => c.id === edit.id);
    if (!clip) throw new Error("片段已不存在");
    const track = next.tracks.find((t) => t.id === clip.track_id)!;
    if (track.locked) throw new Error("轨道已锁定");
    if (edit.type === "remove") {
      next.clips = next.clips.filter((c) => c.id !== clip.id);
      closeVisualLane(next, track.id);
      if (clip.kind === "audio") removeEmptyAudioTracks(next);
    } else if (edit.type === "clip") {
      Object.assign(clip, edit.patch);
      closeVisualLane(next, track.id);
    } else if (edit.type === "move") {
      let destination: TimelineTrack | undefined;
      if (edit.newAudioTrack) {
        if (clip.kind !== "audio") throw new Error("只有音频片段可创建音频轨");
        if (edit.trackID) throw new Error("请指定唯一的目标轨道");
        const anchor = next.tracks.findIndex(
          (item) => item.id === edit.newAudioTrack!.afterTrackID,
        );
        if (
          anchor < 0 ||
          !["video", "audio"].includes(next.tracks[anchor].kind)
        )
          throw new Error("目标轨道已不存在");
        destination = newTrack(
          "audio",
          next.tracks.filter((t) => t.kind === "audio").length,
        );
        next.tracks.splice(anchor + 1, 0, destination);
      } else {
        destination = next.tracks.find(
          (t) => t.id === (edit.trackID ?? clip.track_id),
        );
      }
      if (
        !destination ||
        !accepts(destination.kind, clip.kind) ||
        destination.locked
      )
        throw new Error("目标轨道不可编辑");
      let start = Math.round(edit.start);
      if (!integer(start)) throw new Error("片段位置无效");
      if (visual(destination.kind)) {
        const lane = next.clips
          .filter(
            (item) => item.track_id === destination.id && item.id !== clip.id,
          )
          .sort((a, b) => a.start_ms - b.start_ms);
        const index = lane.findIndex(
          (item) => start < item.start_ms + item.duration_ms / 2,
        );
        lane.splice(index < 0 ? lane.length : index, 0, clip);
        clip.track_id = destination.id;
        let cursor = 0;
        for (const item of lane) {
          item.start_ms = cursor;
          cursor += item.duration_ms;
        }
        if (track.id !== destination.id) closeVisualLane(next, track.id);
      } else {
        if (next.snapping) {
          const playhead =
            edit.playhead === undefined ? undefined : Math.round(edit.playhead);
          if (playhead !== undefined && !integer(playhead))
            throw new Error("播放头位置无效");
          start = timelineMoveSnap(
            next,
            clip,
            start,
            playhead,
            edit.tolerance ?? 100,
            edit.previousSnap,
            edit.releaseTolerance,
          ).start;
        }
        clip.start_ms = start;
        clip.track_id = destination.id;
        // Audio keeps absolute placement and inserts ahead of overlapping items.
        let cursor = clip.start_ms + clip.duration_ms;
        for (const other of next.clips
          .filter(
            (item) => item.id !== clip.id && item.track_id === destination.id,
          )
          .sort((a, b) => a.start_ms - b.start_ms)) {
          if (other.start_ms + other.duration_ms <= start) continue;
          other.start_ms = Math.max(other.start_ms, cursor);
          cursor = other.start_ms + other.duration_ms;
        }
      }
      // Audio lane lifetime follows its clips. Keep the primary visual and
      // protected lanes; neither belongs to this gesture.
      if (clip.kind === "audio") removeEmptyAudioTracks(next);
    } else if (edit.type === "reorder") {
      const lane = next.clips
          .filter((c) => c.track_id === clip.track_id)
          .sort((a, b) => a.start_ms - b.start_ms),
        index = lane.indexOf(clip),
        other = lane[index + edit.direction];
      if (other) {
        const first = edit.direction < 0 ? other : clip,
          second = edit.direction < 0 ? clip : other;
        const gap = second.start_ms - first.start_ms - first.duration_ms,
          start = first.start_ms;
        second.start_ms = start;
        first.start_ms = start + second.duration_ms + gap;
      }
      closeVisualLane(next, track.id);
    } else if (edit.type === "resize") {
      if (!Number.isFinite(edit.delta)) throw new Error("片段位置无效");
      const delta = Math.round(edit.delta);
      const sourced = clip.kind === "audio" || clip.kind === "video";
      const continuous = visual(track.kind);
      const neighbours = next.clips.filter(
        (item) => item.track_id === track.id && item.id !== clip.id,
      );
      if (edit.edge === "left") {
        const previousEnd = Math.max(
          0,
          ...neighbours
            .filter((item) => item.start_ms < clip.start_ms)
            .map((item) => item.start_ms + item.duration_ms),
        );
        const minimum = Math.max(
          sourced ? -clip.source_start_ms : clip.duration_ms - limit,
          continuous ? -limit : previousEnd - clip.start_ms,
        );
        const applied = Math.max(
          minimum,
          Math.min(clip.duration_ms - 100, delta),
        );
        if (!continuous) clip.start_ms += applied;
        if (sourced) clip.source_start_ms += applied;
        clip.duration_ms -= applied;
      } else {
        const nextStart = Math.min(
          limit,
          ...neighbours
            .filter((item) => item.start_ms > clip.start_ms)
            .map((item) => item.start_ms),
        );
        const maximum = Math.min(
          sourced && clip.source_duration_ms != null
            ? clip.source_duration_ms - clip.source_start_ms
            : limit,
          continuous ? limit : nextStart - clip.start_ms,
        );
        clip.duration_ms = Math.max(
          100,
          Math.min(maximum, clip.duration_ms + delta),
        );
      }
      clip.fade_in_ms = Math.min(clip.fade_in_ms, clip.duration_ms);
      clip.fade_out_ms = Math.min(clip.fade_out_ms, clip.duration_ms);
      closeVisualLane(next, track.id);
    } else {
      const offset = Math.round(edit.at) - clip.start_ms;
      if (offset < 100 || clip.duration_ms - offset < 100)
        throw new Error("请将播放头放在片段内部，保留至少100毫秒");
      const sourceOffset = ["audio", "video"].includes(clip.kind) ? offset : 0;
      if (edit.type === "split") {
        next.clips.push({
          ...structuredClone(clip),
          id: crypto.randomUUID(),
          start_ms: clip.start_ms + offset,
          duration_ms: clip.duration_ms - offset,
          source_start_ms: clip.source_start_ms + sourceOffset,
          fade_in_ms: 0,
        });
        clip.duration_ms = offset;
        clip.fade_out_ms = 0;
      } else if (edit.type === "trim-left") {
        clip.start_ms += offset;
        clip.duration_ms -= offset;
        clip.source_start_ms += sourceOffset;
      } else clip.duration_ms = offset;
      closeVisualLane(next, track.id);
      for (const item of next.clips) {
        item.fade_in_ms = Math.min(item.fade_in_ms, item.duration_ms);
        item.fade_out_ms = Math.min(item.fade_out_ms, item.duration_ms);
      }
    }
  }
  return readTimeline(next);
}
export function timelineTime(ms: number) {
  const centiseconds = Math.floor(Math.max(0, ms) / 10);
  return [
    Math.floor(centiseconds / 6000),
    Math.floor(centiseconds / 100) % 60,
    centiseconds % 100,
  ]
    .map((n) => String(n).padStart(2, "0"))
    .join(":");
}
export function timelineRulerTime(ms: number) {
  return timelineTime(ms).split(":").slice(0, 2).join(":");
}
