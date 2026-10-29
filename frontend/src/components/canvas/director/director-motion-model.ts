import { isProjectID } from "@/components/projects/project";
import type { DirectorObject, DirectorMotionClip } from "./director-model";
export const DIRECTOR_BONES = [
  "root",
  "hips",
  "spine",
  "chest",
  "neck",
  "head",
  "leftShoulder",
  "leftUpperArm",
  "leftLowerArm",
  "leftHand",
  "rightShoulder",
  "rightUpperArm",
  "rightLowerArm",
  "rightHand",
  "leftUpperLeg",
  "leftLowerLeg",
  "leftFoot",
  "rightUpperLeg",
  "rightLowerLeg",
  "rightFoot",
  ...["left", "right"].flatMap((side) =>
    ["Thumb", "Index", "Middle", "Ring", "Pinky"].flatMap((finger) =>
      [1, 2, 3].map((n) => `${side}${finger}${n}`),
    ),
  ),
] as const;
const finite = (v: unknown, min: number, max: number): v is number =>
  typeof v === "number" && Number.isFinite(v) && v >= min && v <= max;
const name = (v: unknown): v is string =>
  typeof v === "string" &&
  !!v.trim() &&
  [...v].length <= 128 &&
  !v.includes("\0");
const quaternion = (v: unknown): v is number[] =>
  Array.isArray(v) &&
  v.length === 4 &&
  v.every((n) => finite(n, -1, 1)) &&
  Math.abs(Math.hypot(...v) - 1) < 0.01;
export function validateDirectorMotion(object: DirectorObject) {
  const frozen = object.frozen_bones ?? {};
  if (Object.keys(frozen).length > 256) throw new Error("冻结骨架过大");
  for (const [bone, transform] of Object.entries(frozen)) {
    if (
      !name(bone) ||
      !transform ||
      ![transform.position, transform.rotation, transform.scale].every(
        (v) =>
          Array.isArray(v) &&
          v.length === 3 &&
          v.every((n: unknown) => finite(n, -10000, 10000)),
      ) ||
      transform.scale!.some((n: unknown) => !finite(n, 0.01, 100))
    )
      throw new Error("冻结骨架无效");
  }

  const nodes = object.frozen_nodes ?? {};
  if (
    typeof nodes !== "object" ||
    Array.isArray(nodes) ||
    Object.keys(nodes).length > 1024
  )
    throw new Error("冻结节点无效");
  for (const [path, pose] of Object.entries(nodes)) {
    if (
      path.length > 256 ||
      !/^root(?:\/(?:0|[1-9][0-9]{0,5}))*$/.test(path) ||
      !pose ||
      ![
        pose.transform?.position,
        pose.transform?.rotation,
        pose.transform?.scale,
      ].every(
        (v) =>
          Array.isArray(v) &&
          v.length === 3 &&
          v.every((n) => finite(n, -10000, 10000)),
      ) ||
      (pose.morph_weights !== undefined &&
        (!Array.isArray(pose.morph_weights) ||
          pose.morph_weights.length > 256 ||
          pose.morph_weights.some((n) => !finite(n, -10000, 10000))))
    )
      throw new Error("冻结节点无效");
  }

  const clips = object.motion_clips ?? [],
    ids = new Set<string>();
  if (!Array.isArray(clips) || clips.length > 128)
    throw new Error("动作片段数量无效");
  for (const clip of clips) {
    if (
      !isProjectID(clip.id) ||
      ids.has(clip.id) ||
      !name(clip.name) ||
      !name(clip.source_animation) ||
      !finite(clip.start, 0, 3600) ||
      !finite(clip.duration, 0.1, 3600) ||
      clip.start + clip.duration > 3600 ||
      !finite(clip.playback_rate, 0.1, 4) ||
      !finite(clip.source_offset ?? 0, 0, 3600) ||
      !finite(clip.blend_in ?? 0, 0, clip.duration) ||
      typeof clip.loop !== "boolean"
    )
      throw new Error("动作片段配置无效");
    ids.add(clip.id);
  }
  validateDirectorBlends(clips);
  for (const clip of clips)
    if (
      clip.source_animation?.startsWith("user:") &&
      !(object.motion_library ?? []).some(
        (source) => `user:${source.id}` === clip.source_animation,
      )
    )
      throw new Error("片段动作来源已不存在");
  if (object.active_motion_clip_id && !ids.has(object.active_motion_clip_id))
    throw new Error("所选动作不存在");
  const bones = new Set<string>(DIRECTOR_BONES);
  if (object.rig) {
    const mapping = object.rig.bone_map ?? {},
      animations = object.rig.animation_names ?? [];
    if (
      Object.keys(mapping).length > bones.size ||
      !Array.isArray(animations) ||
      animations.length > 128 ||
      animations.some((item) => !name(item))
    )
      throw new Error("骨架配置无效");
    for (const [bone, target] of Object.entries(mapping))
      if (!bones.has(bone) || !name(target)) throw new Error("骨架映射无效");
  }
  for (const [bone, value] of Object.entries(object.bone_overrides ?? {}))
    if (!bones.has(bone) || !quaternion(value)) throw new Error("骨骼姿势无效");
  const tracks = object.bone_tracks ?? [],
    seen = new Set<string>();
  if (!Array.isArray(tracks) || tracks.length > bones.size)
    throw new Error("骨骼轨道数量无效");
  for (const track of tracks) {
    if (
      !track.bone ||
      !bones.has(track.bone) ||
      seen.has(track.bone) ||
      !Array.isArray(track.keyframes) ||
      track.keyframes.length > 256
    )
      throw new Error("骨骼轨道无效");
    seen.add(track.bone);
    const frames = new Set<string>(),
      times = new Set<number>();
    for (const frame of track.keyframes) {
      if (
        !isProjectID(frame.id) ||
        frames.has(frame.id) ||
        !finite(frame.time, 0, 3600) ||
        times.has(frame.time) ||
        !quaternion(frame.rotation) ||
        ![undefined, "", "linear", "smooth", "step"].includes(frame.easing)
      )
        throw new Error("骨骼关键帧无效");
      frames.add(frame.id);
      times.add(frame.time);
    }
  }
}

/** New edits cannot overlap; existing legacy blended clips remain readable. */
export function placeDirectorMotionClip(
  clips: DirectorMotionClip[],
  candidate: DirectorMotionClip,
  duration: number,
): DirectorMotionClip[] {
  const start = candidate.start!,
    end = start + candidate.duration!;
  if (
    !finite(candidate.source_offset ?? 0, 0, 3600) ||
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    start < 0 ||
    candidate.duration! < 0.1 ||
    end > duration
  )
    throw new Error("动作超出当前场景时间范围");
  const others = clips.filter((clip) => clip.id !== candidate.id);
  if (
    others.some(
      (clip) =>
        start < clip.start! + clip.duration! - 1e-6 && end > clip.start! + 1e-6,
    )
  )
    throw new Error("同一角色的动作不能重叠，请调整开始时间或时长");
  if (others.length >= 128) throw new Error("动作片段已达上限");
  const placed = [...others, candidate].sort((a, b) => a.start! - b.start!);
  validateDirectorBlends(placed);
  return placed;
}

export function validateDirectorBlends(clips: DirectorMotionClip[]) {
  for (const clip of clips) {
    const blend = clip.blend_in ?? 0;
    if (!finite(blend, 0, clip.duration ?? 0))
      throw new Error("动作混合时长无效");
    if (!blend) continue;
    const previous = clips.filter(
      (item) =>
        item.id !== clip.id &&
        Math.abs(item.start! + item.duration! - clip.start!) < 1e-6,
    );
    if (
      previous.length !== 1 ||
      blend > previous[0].duration! ||
      clips.some(
        (item) =>
          item.id !== clip.id &&
          item.id !== previous[0].id &&
          item.start! < clip.start! + blend &&
          item.start! + item.duration! > clip.start!,
      )
    )
      throw new Error("混合需要唯一相邻动作，修改边界前请先取消混合");
  }
}
