// Adapted MIT implementation; source attribution: notices/third-party-MIT.txt.
// UUIDs, range validation and channel-preserving transactions follow Glowloom contracts.
// License and source attribution: notices/third-party-MIT.txt.
import { Vector3 } from "three";
import {
  resampleDirectorGroundPath,
  type DirectorGroundPathPoint,
} from "./director-path-sampling";
import type {
  DirectorCamera,
  DirectorFrame,
  DirectorTransform,
} from "./director-model";
export const CAMERA_MOTIONS = [
  ["push_in", "推近"],
  ["pull_out", "拉远"],
  ["truck", "横移"],
  ["lift", "上升"],
  ["descend", "下降"],
  ["orbit", "环绕"],
  ["half_arc", "半弧"],
  ["spiral", "螺旋"],
  ["dolly_zoom", "滑动变焦"],
  ["roll", "倾斜"],
  ["pan_left", "向左摇"],
  ["pan_right", "向右摇"],
  ["tilt_up", "向上摇"],
  ["tilt_down", "向下摇"],
] as const;
export type CameraMotion = (typeof CAMERA_MOTIONS)[number][0];
function timeRange(start: number, duration: number) {
  if (
    !Number.isFinite(start) ||
    !Number.isFinite(duration) ||
    start < 0 ||
    duration < 0.1 ||
    start + duration > 3600
  )
    throw new Error("路径时间须位于 0～3600 秒内，持续至少 0.1 秒");
}
export function actorPathFrames(
  base: DirectorTransform,
  points: DirectorGroundPathPoint[],
  start: number,
  duration: number,
  facing: boolean,
): DirectorFrame[] {
  timeRange(start, duration);
  const route = resampleDirectorGroundPath(points, 32);
  if (
    route.length < 2 ||
    (Math.hypot(route[0].x - route.at(-1)!.x, route[0].z - route.at(-1)!.z) <
      1e-6 &&
      route.every((p) => Math.hypot(p.x - route[0].x, p.z - route[0].z) < 1e-6))
  )
    throw new Error("请绘制至少两个不同的路径点");
  return route.map((point, index) => {
    const previous = route[index === route.length - 1 ? index - 1 : index],
      next = route[index === route.length - 1 ? index : index + 1];
    return {
      id: crypto.randomUUID(),
      time: start + (duration * index) / (route.length - 1),
      easing: "linear",
      position_keyed: true,
      rotation_keyed: facing,
      scale_keyed: false,
      transform: {
        ...structuredClone(base),
        position: [
          base.position[0] + point.x - route[0].x,
          base.position[1],
          base.position[2] + point.z - route[0].z,
        ],
        rotation: facing
          ? [
              base.rotation[0],
              Math.atan2(next.x - previous.x, next.z - previous.z),
              base.rotation[2],
            ]
          : [...base.rotation],
      },
    };
  });
}
export function cameraMotionFrames(
  camera: DirectorCamera,
  kind: CameraMotion,
  start: number,
  duration: number,
): DirectorFrame[] {
  timeRange(start, duration);
  const origin = new Vector3().fromArray(camera.transform.position),
    target = new Vector3().fromArray(camera.target);
  const forward = target.clone().sub(origin),
    distance = forward.length();
  if (distance < 0.001) throw new Error("机位与注视点不能重合");
  forward.normalize();
  const right = new Vector3()
    .crossVectors(forward, new Vector3(0, 1, 0))
    .normalize();
  const orbit = ["orbit", "half_arc", "spiral"].includes(kind),
    fixed =
      kind.startsWith("pan_") || kind.startsWith("tilt_") || kind === "roll";
  const count = orbit || fixed ? 32 : 1;
  return Array.from({ length: count + 1 }, (_, i) => {
    const t = i / count,
      position = origin.clone(),
      aim = target.clone();
    let fov = camera.fov,
      roll = camera.roll ?? 0;
    if (orbit) {
      const angle = (kind === "half_arc" ? Math.PI : 2 * Math.PI) * t;
      const x = origin.x - target.x,
        z = origin.z - target.z;
      position.set(
        target.x + x * Math.cos(angle) - z * Math.sin(angle),
        origin.y + (kind === "spiral" ? 2 * t : 0),
        target.z + x * Math.sin(angle) + z * Math.cos(angle),
      );
      if (kind === "orbit" && i === count) position.copy(origin);
    } else if (kind === "push_in" || kind === "dolly_zoom")
      position.addScaledVector(forward, Math.min(2, distance * 0.7) * t);
    else if (kind === "pull_out") position.addScaledVector(forward, -2 * t);
    else if (kind === "truck") position.addScaledVector(right, 2 * t);
    else if (kind === "lift" || kind === "descend")
      position.y += (kind === "lift" ? 2 : -2) * t;
    else if (kind === "roll")
      roll = Math.max(-Math.PI, Math.min(Math.PI, roll + (Math.PI / 6) * t));
    else {
      const axis = kind.startsWith("pan_") ? new Vector3(0, 1, 0) : right;
      const sign = kind === "pan_left" || kind === "tilt_up" ? 1 : -1;
      aim.copy(origin).add(
        target
          .clone()
          .sub(origin)
          .applyAxisAngle(axis, ((sign * Math.PI) / 4) * t),
      );
    }
    if (kind === "dolly_zoom")
      fov =
        (2 *
          Math.atan(
            (Math.tan((camera.fov * Math.PI) / 360) * distance) /
              position.distanceTo(target),
          ) *
          180) /
        Math.PI;
    return {
      id: crypto.randomUUID(),
      time: start + t * duration,
      easing: "linear",
      transform: {
        ...structuredClone(camera.transform),
        position: position.toArray(),
      },
      position_keyed: !fixed,
      rotation_keyed: false,
      scale_keyed: false,
      target: aim.toArray(),
      ...(kind === "dolly_zoom" ? { fov } : {}),
      ...(kind === "roll" ? { roll } : {}),
    };
  });
}
const channels = [
  "position",
  "rotation",
  "scale",
  "target",
  "fov",
  "roll",
] as const;
type Channel = (typeof channels)[number];
const owns = (frame: DirectorFrame, channel: Channel) =>
  channel === "position" || channel === "rotation" || channel === "scale"
    ? frame[`${channel}_keyed`] !== false
    : frame[channel] !== undefined;
export function mergeDirectorPath(
  previous: DirectorFrame[],
  generated: DirectorFrame[],
  replace: boolean,
): DirectorFrame[] {
  if (!generated.length) throw new Error("路径没有关键帧");
  const start = generated[0].time,
    end = generated.at(-1)!.time;
  const owned = channels.filter((channel) =>
    generated.some((frame) => owns(frame, channel)),
  );
  const overlaps = (frame: DirectorFrame) =>
    frame.time >= start && frame.time <= end;
  if (
    !replace &&
    previous.some(
      (frame) =>
        overlaps(frame) && owned.some((channel) => owns(frame, channel)),
    )
  )
    throw new Error("该时间范围已有关键帧，请确认替换相关轨道");
  const result = structuredClone(previous).map((frame) => {
    if (overlaps(frame))
      for (const channel of owned) {
        if (
          channel === "position" ||
          channel === "rotation" ||
          channel === "scale"
        )
          frame[`${channel}_keyed`] = false;
        else delete frame[channel];
      }
    return frame;
  });
  // Keep generated and retained channels in separate frames so their curves remain independent.
  result.push(...structuredClone(generated));
  const filtered = result
    .filter((frame) => channels.some((channel) => owns(frame, channel)))
    .sort((a, b) => a.time - b.time);
  if (filtered.length > 256) throw new Error("路径超过 256 个关键帧上限");
  return filtered;
}

export type CustomCameraMotion = "fixed-target" | "fixed-camera" | "free";
/** Two independent routes, both sampled by distance, share the same time interval. */
export function customCameraPathFrames(
  camera: DirectorCamera,
  mode: CustomCameraMotion,
  cameraPoints: DirectorGroundPathPoint[],
  targetPoints: DirectorGroundPathPoint[],
  start: number,
  duration: number,
): DirectorFrame[] {
  timeRange(start, duration);
  const positions =
    mode === "fixed-camera"
      ? null
      : actorPathFrames(camera.transform, cameraPoints, start, duration, false);
  const targets =
    mode === "fixed-target"
      ? null
      : actorPathFrames(
          { ...camera.transform, position: camera.target },
          targetPoints,
          start,
          duration,
          false,
        );
  return Array.from({ length: 32 }, (_, index) => {
    const position =
      positions?.[index].transform.position ?? camera.transform.position;
    const target = targets?.[index].transform.position ?? camera.target;
    if (
      new Vector3()
        .fromArray(position)
        .distanceTo(new Vector3().fromArray(target)) < 0.001
    )
      throw new Error("机位路径与注视点不能重合");
    return {
      id: crypto.randomUUID(),
      time: start + (duration * index) / 31,
      easing: "linear",
      transform: {
        ...structuredClone(camera.transform),
        position: [...position],
      },
      position_keyed: !!positions,
      rotation_keyed: false,
      scale_keyed: false,
      target: [...target],
    };
  });
}
