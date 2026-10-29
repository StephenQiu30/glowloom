import {
  AnimationClip,
  Bone,
  Object3D,
  Quaternion,
  QuaternionKeyframeTrack,
} from "three";

import { DIRECTOR_BUILTIN_MOTION_PRESETS } from "./director-motion-catalog";
export { DIRECTOR_BUILTIN_MOTION_PRESETS } from "./director-motion-catalog";
const RESOURCE = "/director/motions/quaternius-ual1-standard.json?v=standard-43-v1";
const MAX_BYTES = 24 * 1024 * 1024;
type MotionTrack = { bone: string; times: number[]; values: number[] };
type MotionLibrary = {
  version: 1;
  rest_rotations: Record<string, number[]>;
  motions: { name: string; duration: number; tracks: MotionTrack[] }[];
};
let cached: MotionLibrary | undefined;
type Download = {
  controller: AbortController;
  users: number;
  promise: Promise<MotionLibrary>;
};
let downloading: Download | undefined;

function validateResource(input: unknown): MotionLibrary {
  const invalid = () => new Error("内置动作资源格式无效");
  if (!input || typeof input !== "object") throw invalid();
  const data = input as MotionLibrary;
  if (
    data.version !== 1 ||
    !data.rest_rotations ||
    typeof data.rest_rotations !== "object" ||
    !Array.isArray(data.motions) ||
    data.motions.length !== DIRECTOR_BUILTIN_MOTION_PRESETS.length
  )
    throw invalid();
  const names = new Set<string>();
  const validQuaternion = (values: number[], offset: number) => {
    const vector = values.slice(offset, offset + 4);
    return (
      vector.length === 4 &&
      vector.every(Number.isFinite) &&
      Math.abs(Math.hypot(...vector) - 1) < 0.01
    );
  };
  for (const motion of data.motions) {
    if (
      !motion ||
      !DIRECTOR_BUILTIN_MOTION_PRESETS.some(
        (preset) => preset.name === motion.name,
      ) ||
      names.has(motion.name) ||
      !Number.isFinite(motion.duration) ||
      motion.duration <= 0 ||
      motion.duration > 60 ||
      !Array.isArray(motion.tracks) ||
      !motion.tracks.length ||
      motion.tracks.length > 128
    )
      throw invalid();
    names.add(motion.name);
    const bones = new Set<string>();
    for (const track of motion.tracks) {
      if (
        !track ||
        typeof track.bone !== "string" ||
        !track.bone ||
        bones.has(track.bone) ||
        !Array.isArray(track.times) ||
        !track.times.length ||
        track.times.length > 2048 ||
        !Array.isArray(track.values) ||
        track.values.length !== track.times.length * 4
      )
        throw invalid();
      const rest = data.rest_rotations[track.bone];
      if (
        !Array.isArray(rest) ||
        rest.length !== 4 ||
        !validQuaternion(rest, 0)
      )
        throw invalid();
      bones.add(track.bone);
      for (let i = 0; i < track.times.length; i++) {
        const time = track.times[i];
        if (
          !Number.isFinite(time) ||
          time < 0 ||
          time > motion.duration + 0.0001 ||
          (i > 0 && time <= track.times[i - 1]) ||
          !validQuaternion(track.values, i * 4)
        )
          throw invalid();
      }
    }
  }
  return data;
}

async function readLibrary(signal: AbortSignal): Promise<MotionLibrary> {
  signal.throwIfAborted();
  if (cached) return cached;
  if (!downloading) {
    const controller = new AbortController();
    const flight: Download = {
      controller,
      users: 0,
      // eslint-disable-next-line no-restricted-syntax -- Same-origin public CC0 asset, not a business API.
      promise: fetch(RESOURCE, {
        signal: controller.signal,
        cache: "force-cache",
      })
        .then(async (response) => {
          if (!response.ok) throw new Error("内置动作加载失败，请重试");
          if (Number(response.headers.get("content-length")) > MAX_BYTES)
            throw new Error("内置动作资源过大");
          const text = await response.text();
          controller.signal.throwIfAborted();
          if (text.length > MAX_BYTES) throw new Error("内置动作资源过大");
          const library = validateResource(JSON.parse(text));
          cached = library;
          return library;
        })
        .finally(() => {
          if (downloading === flight) downloading = undefined;
        }),
    };
    downloading = flight;
  }
  const flight = downloading;
  flight.users++;
  try {
    return await new Promise<MotionLibrary>((resolve, reject) => {
      const abort = () =>
        reject(
          signal.reason ??
            new DOMException("Motion load aborted", "AbortError"),
        );
      signal.addEventListener("abort", abort, { once: true });
      flight.promise
        .then(resolve, reject)
        .finally(() => signal.removeEventListener("abort", abort));
      if (signal.aborted) abort();
    });
  } finally {
    flight.users--;
    if (!flight.users && downloading === flight) {
      downloading = undefined;
      flight.controller.abort();
    }
  }
}

/**
 * Call on the viewport-owned clone in its rest pose, before creating/playing its mixer.
 * Only rotation tracks are added: body proportions and path-owned translation stay intact.
 */
export async function attachBuiltinDirectorMotions(
  root: Object3D,
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  if (
    DIRECTOR_BUILTIN_MOTION_PRESETS.every((preset) =>
      root.animations.some((clip) => clip.name === preset.name),
    )
  )
    return;
  const bones = new Map<string, { bone: Bone; rest: Quaternion }>();
  root.traverse((node) => {
    if (!(node instanceof Bone)) return;
    if (bones.has(node.name))
      throw new Error(`内置动作无法匹配重名骨骼：${node.name}`);
    bones.set(node.name, {
      bone: node,
      rest: node.quaternion.clone().normalize(),
    });
  });
  const library = await readLibrary(signal);
  signal.throwIfAborted();
  const clips = DIRECTOR_BUILTIN_MOTION_PRESETS.map((preset) => {
    const motion = library.motions.find((item) => item.name === preset.name)!;
    const tracks = motion.tracks.map((track) => {
      const target = bones.get(track.bone);
      if (!target) throw new Error(`内置动作缺少目标骨骼：${track.bone}`);
      const correction = target.rest
        .clone()
        .multiply(
          new Quaternion()
            .fromArray(library.rest_rotations[track.bone])
            .normalize()
            .invert(),
        );
      const values = new Float32Array(track.values.length);
      const frame = new Quaternion(),
        previous = new Quaternion();
      for (let i = 0; i < track.times.length; i++) {
        frame
          .fromArray(track.values, i * 4)
          .normalize()
          .premultiply(correction)
          .normalize();
        if (i && previous.dot(frame) < 0)
          frame.set(-frame.x, -frame.y, -frame.z, -frame.w);
        frame.toArray(values, i * 4);
        previous.copy(frame);
      }
      return new QuaternionKeyframeTrack(
        `${target.bone.uuid}.quaternion`,
        track.times,
        values,
      );
    });
    return new AnimationClip(motion.name, motion.duration, tracks);
  });
  signal.throwIfAborted();
  root.animations = [
    ...root.animations,
    ...clips.filter(
      (clip) =>
        !root.animations.some((existing) => existing.name === clip.name),
    ),
  ];
}
