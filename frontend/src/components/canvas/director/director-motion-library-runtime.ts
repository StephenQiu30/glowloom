import {
  AnimationClip,
  Bone,
  Euler,
  Object3D,
  Quaternion,
  QuaternionKeyframeTrack,
  VectorKeyframeTrack,
  Vector3,
} from "three";
import {
  directorMotionSourceName,
  validateDirectorMotionLibrary,
  type DirectorMotionSource,
} from "./director-motion-library-model";
import type { DirectorTransform } from "./director-model";
/** Each instance gets tracks bound to its own bones, preserving its shaped rest pose. */
export function directorLibraryClips(
  root: Object3D,
  sources: DirectorMotionSource[],
): AnimationClip[] {
  validateDirectorMotionLibrary(sources);
  const bones = new Map<string, Bone>();
  root.traverse((node) => {
    if (node instanceof Bone) {
      if (bones.has(node.name)) throw new Error("骨骼重名，无法应用动作");
      bones.set(node.name, node);
    }
  });
  return sources.map(
    (source) =>
      new AnimationClip(
        directorMotionSourceName(source.id),
        source.duration,
        source.tracks.map((track) => {
          const target = bones.get(track.bone);
          if (!target) throw new Error(`动作缺少兼容骨骼：${track.bone}`);
          const rest = source.rest[track.bone],
            values: number[] = [];
          if (track.property === "quaternion") {
            const inverse = new Quaternion()
              .setFromEuler(
                new Euler(...(rest.rotation as [number, number, number])),
              )
              .invert();
            const correction = target.quaternion.clone().multiply(inverse);
            for (let i = 0; i < track.values.length; i += 4)
              new Quaternion()
                .fromArray(track.values, i)
                .premultiply(correction)
                .normalize()
                .toArray(values, i);
            return new QuaternionKeyframeTrack(
              `${target.uuid}.quaternion`,
              track.times,
              values,
            );
          }
          const size = new Vector3().fromArray(rest.position).length();
          const ratio = size > 0.000001 ? target.position.length() / size : 1;
          for (let i = 0; i < track.values.length; i += 3)
            for (let axis = 0; axis < 3; axis++)
              values[i + axis] =
                track.property === "position"
                  ? target.position.getComponent(axis) +
                    (track.values[i + axis] - rest.position[axis]) * ratio
                  : (target.scale.getComponent(axis) * track.values[i + axis]) /
                    rest.scale[axis];
          return new VectorKeyframeTrack(
            `${target.uuid}.${track.property}`,
            track.times,
            values,
          );
        }),
      ),
  );
}
export function directorPoseSource(
  name: string,
  pose: Record<string, DirectorTransform>,
  rest: Record<string, DirectorTransform>,
): DirectorMotionSource {
  const tracks: DirectorMotionSource["tracks"] = [];
  for (const [bone, transform] of Object.entries(pose)) {
    if (!rest[bone]) throw new Error(`姿势缺少基础骨骼：${bone}`);
    const q = new Quaternion()
      .setFromEuler(
        new Euler(...(transform.rotation as [number, number, number])),
      )
      .toArray();
    tracks.push(
      { bone, property: "quaternion", times: [0, 1], values: [...q, ...q] },
      {
        bone,
        property: "position",
        times: [0, 1],
        values: [...transform.position, ...transform.position],
      },
      {
        bone,
        property: "scale",
        times: [0, 1],
        values: [...transform.scale, ...transform.scale],
      },
    );
  }
  const source: DirectorMotionSource = {
    id: crypto.randomUUID(),
    name,
    kind: "pose",
    duration: 1,
    tracks,
    rest: structuredClone(rest),
  };
  validateDirectorMotionLibrary([source]);
  return source;
}
