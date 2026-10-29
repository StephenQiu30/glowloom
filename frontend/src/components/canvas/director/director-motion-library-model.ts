import { isProjectID } from "@/components/projects/project";
import type { DirectorObject, DirectorTransform } from "./director-model";
export type DirectorMotionTrack = {
  bone: string;
  property: "quaternion" | "position" | "scale";
  times: number[];
  values: number[];
};
export type DirectorMotionSource = {
  id: string;
  name: string;
  kind: "import" | "pose";
  duration: number;
  tracks: DirectorMotionTrack[];
  rest: Record<string, DirectorTransform>;
};
export const directorMotionSourceName = (id: string) => `user:${id}`;
const finite = (v: unknown, min: number, max: number): v is number =>
  typeof v === "number" && Number.isFinite(v) && v >= min && v <= max;
export function validateDirectorMotionLibrary(
  sources: DirectorMotionSource[] | undefined,
) {
  if (sources === undefined) return;
  if (!Array.isArray(sources) || sources.length > 16)
    throw new Error("我的动作最多保存16个来源");
  const ids = new Set<string>();
  for (const source of sources) {
    if (
      !source ||
      !isProjectID(source.id) ||
      ids.has(source.id) ||
      !source.name?.trim() ||
      source.name.length > 128 ||
      !["import", "pose"].includes(source.kind) ||
      !finite(source.duration, 0.1, 60) ||
      !Array.isArray(source.tracks) ||
      !source.tracks.length ||
      source.tracks.length > 256 ||
      !source.rest ||
      typeof source.rest !== "object" ||
      Object.keys(source.rest).length > 256
    )
      throw new Error("动作来源无效");
    ids.add(source.id);
    const channels = new Set<string>();
    let points = 0;
    for (const track of source.tracks) {
      const width = track.property === "quaternion" ? 4 : 3,
        key = `${track.bone}:${track.property}`;
      const rest = source.rest[track.bone];
      if (
        !track.bone?.trim() ||
        track.bone.length > 128 ||
        !["quaternion", "position", "scale"].includes(track.property) ||
        channels.has(key) ||
        !rest ||
        ![rest.position, rest.rotation, rest.scale].every(
          (v) =>
            Array.isArray(v) &&
            v.length === 3 &&
            v.every((n) => finite(n, -10000, 10000)),
        ) ||
        rest.scale.some((n) => Math.abs(n) < 0.000001) ||
        !Array.isArray(track.times) ||
        !track.times.length ||
        track.times.length > 2048 ||
        !Array.isArray(track.values) ||
        track.values.length !== track.times.length * width
      )
        throw new Error("动作骨骼轨道无效");
      channels.add(key);
      points += track.times.length;
      for (let i = 0; i < track.times.length; i++) {
        const values = track.values.slice(i * width, (i + 1) * width);
        if (
          !finite(track.times[i], 0, source.duration + 0.0001) ||
          (i > 0 && track.times[i] <= track.times[i - 1]) ||
          !values.every((n) => finite(n, -10000, 10000)) ||
          (width === 4 && Math.abs(Math.hypot(...values) - 1) > 0.01)
        )
          throw new Error("动作采样无效");
      }
    }
    if (points > 8192) throw new Error("动作采样过多，请缩短片段或降低采样率");
  }
}
export function addDirectorMotionSource(
  object: DirectorObject,
  source: DirectorMotionSource,
  bones: string[],
): Partial<DirectorObject> {
  validateDirectorMotionLibrary([source]);
  const names = new Set(bones);
  if (source.tracks.some((track) => !names.has(track.bone)))
    throw new Error("动作骨骼与当前角色不兼容");
  const motion_library = [...(object.motion_library ?? []), source];
  validateDirectorMotionLibrary(motion_library);
  return { motion_library };
}
