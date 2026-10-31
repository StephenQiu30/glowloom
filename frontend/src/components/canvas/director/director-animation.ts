// Independent-channel semantics; source attribution: notices/third-party-MIT.txt.
// Glowloom uses UUID identities, generated DTOs and whole-document Yjs transactions.
// License and source attribution: notices/third-party-MIT.txt.
import { Euler, Quaternion } from "three";
import type {
  DirectorCamera,
  DirectorFrame,
  DirectorTransform,
} from "./director-model";
function interval(
  frames: DirectorFrame[],
  time: number,
): [DirectorFrame, DirectorFrame, number] | null {
  const ordered = [...frames].sort((a, b) => a.time - b.time);
  if (!ordered.length) return null;
  if (time <= ordered[0].time) return [ordered[0], ordered[0], 0];
  const index = ordered.findIndex((frame) => frame.time > time);
  if (index < 0) return [ordered.at(-1)!, ordered.at(-1)!, 0];
  const a = ordered[index - 1],
    b = ordered[index];
  let t = (time - a.time) / (b.time - a.time);
  if (a.easing === "step") t = 0;
  else if (a.easing === "smooth") t = t * t * (3 - 2 * t);
  return [a, b, t];
}
export function sampleDirectorTransform(
  base: DirectorTransform,
  frames: DirectorFrame[],
  time: number,
): DirectorTransform {
  const result = structuredClone(base);
  for (const channel of ["position", "rotation", "scale"] as const) {
    const sampled = interval(
      frames.filter((frame) => frame[`${channel}_keyed`] !== false),
      time,
    );
    if (!sampled) continue;
    const [a, b, t] = sampled;
    if (a === b) {
      result[channel] = [...a.transform[channel]];
      continue;
    }
    if (channel === "rotation") {
      const qa = new Quaternion().setFromEuler(
        new Euler(...(a.transform.rotation as [number, number, number])),
      );
      const qb = new Quaternion().setFromEuler(
        new Euler(...(b.transform.rotation as [number, number, number])),
      );
      const rotation = new Euler().setFromQuaternion(qa.slerp(qb, t));
      result.rotation = [rotation.x, rotation.y, rotation.z];
    } else
      result[channel] = a.transform[channel].map(
        (v, i) => v + (b.transform[channel][i] - v) * t,
      );
  }
  return result;
}
export function sampleDirectorCamera(camera: DirectorCamera, time: number) {
  const result = {
    transform: sampleDirectorTransform(
      camera.transform,
      camera.keyframes,
      time,
    ),
    target: [...camera.target],
    fov: camera.fov,
    roll: camera.roll ?? 0,
  };
  for (const channel of ["target", "fov", "roll"] as const) {
    const sampled = interval(
      camera.keyframes.filter((frame) => frame[channel] !== undefined),
      time,
    );
    if (!sampled) continue;
    const [a, b, t] = sampled;
    if (channel === "target")
      result.target = a.target!.map((v, i) => v + (b.target![i] - v) * t);
    else result[channel] = a[channel]! + (b[channel]! - a[channel]!) * t;
  }
  return result;
}
/** Only the changed optical channel is keyed; existing transform keys retain ownership. */
export function keyDirectorCamera(
  camera: DirectorCamera,
  time: number,
  patch: { target?: number[]; fov?: number; roll?: number },
): DirectorCamera {
  const next = structuredClone(camera);
  for (const channel of ["target", "fov", "roll"] as const) {
    const value = patch[channel];
    if (value === undefined) continue;
    const sameTime = next.keyframes.filter(
      (frame) => Math.abs(frame.time - time) < 1e-6,
    );
    const owners = sameTime.filter((frame) => frame[channel] !== undefined);
    const easing = owners[0]?.easing ?? "linear";
    for (const owner of owners) delete owner[channel];
    let frame = sameTime.find((candidate) => candidate.easing === easing);
    if (!frame) {
      frame = {
        id: crypto.randomUUID(),
        time,
        transform: sampleDirectorTransform(
          camera.transform,
          camera.keyframes,
          time,
        ),
        easing,
        position_keyed: false,
        rotation_keyed: false,
        scale_keyed: false,
      };
      next.keyframes.push(frame);
    }
    if (channel === "target") frame.target = [...patch.target!];
    else frame[channel] = patch[channel];
  }
  next.keyframes = next.keyframes.filter(
    (frame) =>
      frame.position_keyed !== false ||
      frame.rotation_keyed !== false ||
      frame.scale_keyed !== false ||
      frame.target !== undefined ||
      frame.fov !== undefined ||
      frame.roll !== undefined,
  );
  next.keyframes.sort((a, b) => a.time - b.time);
  return next;
}
