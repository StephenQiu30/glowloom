import { directorLibraryClips } from "./director-motion-library-runtime";
import { directorMotionSourceName } from "./director-motion-library-model";
import {
  AnimationMixer,
  Bone,
  Mesh,
  LoopOnce,
  Object3D,
  Quaternion,
} from "three";
import { directorPoseBoneDeltas } from "./director-poses";
import type {
  DirectorObject,
  DirectorTransform,
  DirectorFrozenNode,
  DirectorMotionClip,
} from "./director-model";

export type DirectorModelInfo = {
  objectID: string;
  assetID: string;
  animations: { name: string; duration: number }[];
  bones: string[];
  rest?: Record<string, DirectorTransform>;
  bonePaths?: Record<string, string>;
};
const finite = (v: unknown, min: number, max: number): v is number =>
  typeof v === "number" && Number.isFinite(v) && v >= min && v <= max;
const name = (v: unknown): v is string =>
  typeof v === "string" &&
  !!v.trim() &&
  [...v].length <= 128 &&
  !v.includes("\0");
export function directorModelInfo(
  root: Object3D,
  object: DirectorObject,
): DirectorModelInfo {
  const bones: string[] = [],
    counts = new Map<string, number>();
  for (const clip of root.animations)
    counts.set(clip.name, (counts.get(clip.name) ?? 0) + 1);
  root.traverse((node) => {
    if (node instanceof Bone && name(node.name)) bones.push(node.name);
  });
  const rest: Record<string, DirectorTransform> = {};
  root.traverse((node) => {
    if (node instanceof Bone)
      rest[node.name] = {
        position: node.position.toArray(),
        rotation: [node.rotation.x, node.rotation.y, node.rotation.z],
        scale: node.scale.toArray(),
      };
  });
  const bonePaths: Record<string, string> = {};
  const indexNodes = (node: Object3D, path: string) => {
    if (
      node instanceof Bone &&
      bones.filter((name) => name === node.name).length === 1
    )
      bonePaths[node.name] = path;
    node.children.forEach((child, index) =>
      indexNodes(child, `${path}/${index}`),
    );
  };
  indexNodes(root, "root");
  return {
    rest,
    bonePaths,
    objectID: object.id,
    assetID: object.asset_id ?? "",
    animations: root.animations
      .filter(
        (clip) =>
          name(clip.name) &&
          counts.get(clip.name) === 1 &&
          finite(clip.duration, 0.001, 3600),
      )
      .slice(0, 128)
      .map((clip) => ({ name: clip.name, duration: clip.duration }))
      .concat(
        (object.motion_library ?? []).map((source) => ({
          name: directorMotionSourceName(source.id),
          duration: source.duration,
        })),
      ),
    bones: [...new Set(bones)].filter(
      (name) => bones.filter((item) => item === name).length === 1,
    ),
  };
}
export function sampleDirectorBone(
  frames: API.DirectorBoneFrame[],
  time: number,
): Quaternion | null {
  const ordered = [...frames].sort((a, b) => a.time! - b.time!);
  if (!ordered.length) return null;
  let start = ordered[0],
    end = ordered[ordered.length - 1];
  if (time <= start.time!) return new Quaternion().fromArray(start.rotation!);
  if (time >= end.time!) return new Quaternion().fromArray(end.rotation!);
  for (let i = 1; i < ordered.length; i++)
    if (ordered[i].time! >= time) {
      start = ordered[i - 1];
      end = ordered[i];
      break;
    }
  let alpha = (time - start.time!) / (end.time! - start.time!);
  if (start.easing === "step") alpha = 0;
  else if (start.easing === "smooth") alpha = alpha * alpha * (3 - 2 * alpha);
  return new Quaternion()
    .fromArray(start.rotation!)
    .slerp(new Quaternion().fromArray(end.rotation!), alpha);
}
/** Deterministic timeline sampling; seeking never accumulates wall-clock playback. */
export class DirectorModelMotion {
  private mixer: AnimationMixer;
  private base = new Map<Object3D, Quaternion>();
  private nodes = new Map<
    string,
    { node: Object3D; base: DirectorFrozenNode }
  >();
  private actions: {
    config: DirectorMotionClip;
    action: NonNullable<ReturnType<AnimationMixer["clipAction"]>>;
    duration: number;
  }[] = [];
  constructor(
    private root: Object3D,
    private object: DirectorObject,
  ) {
    this.mixer = new AnimationMixer(root);
    root.traverse((node) => {
      if (node instanceof Bone) this.base.set(node, node.quaternion.clone());
    });
    const index = (node: Object3D, path: string) => {
      this.nodes.set(path, { node, base: this.readNode(node) });
      node.children.forEach((child, i) => index(child, `${path}/${i}`));
    };
    index(root, "root");
    const library = directorLibraryClips(root, object.motion_library ?? []);
    for (const config of object.motion_clips ?? []) {
      const sources = [...root.animations, ...library].filter(
        (clip) => clip.name === config.source_animation,
      );
      if (sources.length !== 1 || sources[0].duration <= 0)
        throw new Error(`找不到唯一模型动作：${config.source_animation}`);
      const clip = sources[0].clone();
      clip.name = config.id!;
      const action = this.mixer.clipAction(clip);
      action.setLoop(LoopOnce, 1);
      action.clampWhenFinished = true;
      this.actions.push({ config, action, duration: clip.duration });
    }
  }
  sample(time: number, dynamic: boolean) {
    this.mixer.stopAllAction();
    for (const { node, base } of this.nodes.values())
      this.applyNode(node, base);
    for (const [bone, pose] of this.base) bone.quaternion.copy(pose);
    let hasActive = false;
    if (dynamic) {
      const active = this.actions.filter(
        ({ config }) =>
          time >= config.start! && time < config.start! + config.duration!,
      );
      hasActive = active.length > 0;
      const samples = active.map((entry) => ({
        entry,
        time,
        weight: 1 / active.length,
      }));
      if (active.length === 1) {
        const current = active[0],
          blend = current.config.blend_in ?? 0;
        if (blend > 0 && time < current.config.start! + blend) {
          const previous = this.actions.find(
            (entry) =>
              entry !== current &&
              Math.abs(
                entry.config.start! +
                  entry.config.duration! -
                  current.config.start!,
              ) < 1e-6,
          );
          if (previous) {
            const alpha = (time - current.config.start!) / blend;
            samples[0].weight = alpha;
            samples.push({
              entry: previous,
              time: previous.config.start! + previous.config.duration! - 1e-7,
              weight: 1 - alpha,
            });
          }
        }
      }
      for (const {
        entry: { config, action, duration },
        time: sampleTime,
        weight,
      } of samples) {
        const elapsed =
          (config.source_offset ?? 0) +
          (sampleTime - config.start!) * config.playback_rate!;
        action.reset();
        action.time = config.loop
          ? elapsed % duration
          : Math.min(elapsed, duration);
        action.setEffectiveWeight(weight);
        action.paused = true;
        action.play();
      }
      this.mixer.update(0);
    }
    const mapping = this.object.rig?.bone_map ?? {};
    if (!hasActive && this.object.kind === "actor") {
      const deltas = directorPoseBoneDeltas(this.object.pose ?? "stand");
      for (const [bone, value] of Object.entries(deltas)) {
        const target = mapping[bone]
          ? this.root.getObjectByName(mapping[bone])
          : null;
        if (!(target instanceof Bone)) continue;
        const delta = new Quaternion().fromArray(value);
        if (
          bone === "leftUpperArm" &&
          this.object.builtin_actor &&
          this.object.builtin_actor !== "mannequin"
        )
          delta.invert();
        target.quaternion.multiply(delta);
      }
    }
    if (!dynamic)
      for (const [name, transform] of Object.entries(
        this.object.frozen_bones ?? {},
      )) {
        const node = this.root.getObjectByName(name);
        if (!(node instanceof Bone)) throw new Error(`冻结骨骼 ${name} 不存在`);
        node.position.fromArray(transform.position!);
        node.rotation.set(...(transform.rotation! as [number, number, number]));
        node.scale.fromArray(transform.scale!);
      }
    if (!dynamic) {
      for (const [path, pose] of Object.entries(
        this.object.frozen_nodes ?? {},
      )) {
        const target = this.nodes.get(path);
        if (!target) throw new Error(`冻结节点 ${path} 不存在`);
        this.applyNode(target.node, pose);
      }
    }
    for (const [bone, value] of Object.entries(
      this.object.bone_overrides ?? {},
    )) {
      const node = mapping[bone]
        ? this.root.getObjectByName(mapping[bone])
        : null;
      if (node instanceof Bone) node.quaternion.fromArray(value);
    }
    if (dynamic)
      for (const track of this.object.bone_tracks ?? []) {
        const node =
          track.bone && mapping[track.bone]
            ? this.root.getObjectByName(mapping[track.bone])
            : null;
        const pose = sampleDirectorBone(track.keyframes ?? [], time);
        if (node instanceof Bone && pose) node.quaternion.copy(pose);
      }
    this.root.updateMatrixWorld(true);
  }
  private readNode(node: Object3D): DirectorFrozenNode {
    return {
      transform: {
        position: node.position.toArray(),
        rotation: [node.rotation.x, node.rotation.y, node.rotation.z],
        scale: node.scale.toArray(),
      },
      ...(node instanceof Mesh && node.morphTargetInfluences
        ? { morph_weights: [...node.morphTargetInfluences] }
        : {}),
    };
  }
  private applyNode(node: Object3D, pose: DirectorFrozenNode) {
    node.position.fromArray(pose.transform.position);
    node.rotation.set(...(pose.transform.rotation as [number, number, number]));
    node.scale.fromArray(pose.transform.scale);
    if (pose.morph_weights) {
      if (
        !(node instanceof Mesh) ||
        node.morphTargetInfluences?.length !== pose.morph_weights.length
      )
        throw new Error("冻结节点的形变目标已改变");
      pose.morph_weights.forEach((weight, index) => {
        node.morphTargetInfluences![index] = weight;
      });
    }
  }
  /** Child-index paths are stable across reloads of the same immutable asset. */
  snapshotNodes(): Record<string, DirectorFrozenNode> {
    if (
      this.nodes.size > 1024 ||
      [...this.nodes.keys()].some((path) => path.length > 256)
    )
      throw new Error("冻结节点数量或层级超过上限");
    return Object.fromEntries(
      [...this.nodes].map(([path, { node }]) => [path, this.readNode(node)]),
    );
  }
  snapshot(): Record<string, DirectorTransform> {
    const poses: Record<string, DirectorTransform> = {};
    for (const node of this.base.keys()) {
      if (poses[node.name] || !name(node.name))
        throw new Error("骨架名称不唯一，无法冻结");
      poses[node.name] = {
        position: node.position.toArray(),
        rotation: [node.rotation.x, node.rotation.y, node.rotation.z],
        scale: node.scale.toArray(),
      };
    }
    return poses;
  }
  dispose() {
    this.mixer.stopAllAction();
    this.mixer.uncacheRoot(this.root);
    this.base.clear();
    this.nodes.clear();
  }
}
