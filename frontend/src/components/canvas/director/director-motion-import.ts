import {
  Bone,
  InterpolateLinear,
  Group,
  LoadingManager,
  PropertyBinding,
  Quaternion,
  type Object3D,
  type AnimationClip,
} from "three";
import {
  validateDirectorMotionLibrary,
  type DirectorMotionSource,
} from "./director-motion-library-model";
import { disposeDirectorObject } from "./director-scene";
export const DIRECTOR_MOTION_ACCEPT = ".glb,.gltf,.fbx,.bvh,.json";
export async function importDirectorMotion(
  file: File,
  bones: string[],
  signal: AbortSignal,
  selected?: string,
  onNames?: (names: string[]) => void,
): Promise<{ names: string[]; source: DirectorMotionSource }> {
  signal.throwIfAborted();
  if (!file.size || file.size > 64 * 1024 * 1024)
    throw new Error("动作文件须为1字节至64MiB");
  const ext = file.name.split(".").at(-1)?.toLowerCase();
  const bytes = await file.arrayBuffer();
  signal.throwIfAborted();
  if (ext === "json") {
    const source = JSON.parse(
      new TextDecoder().decode(bytes),
    ) as DirectorMotionSource;
    source.id = crypto.randomUUID();
    validateDirectorMotionLibrary([source]);
    if (source.tracks.some((track) => !bones.includes(track.bone)))
      throw new Error("动作骨骼与当前角色不兼容");
    return { names: [source.name], source };
  }
  const manager = new LoadingManager();
  manager.setURLModifier((url) => {
    if (url.startsWith("data:") || url.startsWith("blob:")) return url;
    throw new Error("动作必须自包含，不能引用外部文件");
  });
  let root: Object3D | undefined;
  let animations: AnimationClip[] = [];
  try {
    if (ext === "glb" || ext === "gltf") {
      const { GLTFLoader } = await import("three/addons/loaders/GLTFLoader.js");
      const parsed = await new GLTFLoader(manager).parseAsync(
        ext === "glb" ? bytes : new TextDecoder().decode(bytes),
        "",
      );
      root = parsed.scene;
      animations = parsed.animations;
    } else if (ext === "fbx") {
      const { FBXLoader } = await import("three/addons/loaders/FBXLoader.js");
      root = new FBXLoader(manager).parse(bytes, "");
      animations = root.animations;
    } else if (ext === "bvh") {
      const { BVHLoader } = await import("three/addons/loaders/BVHLoader.js");
      const parsed = new BVHLoader(manager).parse(
        new TextDecoder().decode(bytes),
      );
      root = new Group();
      root.add(parsed.skeleton.bones[0]);
      animations = [parsed.clip];
    } else throw new Error("支持GLB、glTF、FBX、BVH或导演台动作JSON");
    signal.throwIfAborted();
    if (!animations.length || animations.length > 128)
      throw new Error("文件中没有可用动作或动作数量过多");
    const names = animations.map(
      (clip, index) => clip.name || `动作 ${index + 1}`,
    );
    if (new Set(names).size !== names.length)
      throw new Error("动作名称重复，请在源文件中重命名");
    onNames?.(names);
    const index = selected ? names.indexOf(selected) : 0;
    if (index < 0) throw new Error("所选动作已不存在");
    const clip = animations[index];
    if (
      !Number.isFinite(clip.duration) ||
      clip.duration < 0.1 ||
      clip.duration > 60
    )
      throw new Error("单个导入动作须为0.1–60秒");
    const source: DirectorMotionSource = {
      id: crypto.randomUUID(),
      name: names[index],
      kind: "import",
      duration: clip.duration,
      tracks: [],
      rest: {},
    };
    const sourceBones = new Map<string, Bone>();
    root.traverse((node) => {
      if (node instanceof Bone) {
        if (sourceBones.has(node.name)) throw new Error("源骨架有重名骨骼");
        sourceBones.set(node.name, node);
        sourceBones.set(node.uuid, node);
      }
    });
    const unit = ext === "fbx" || ext === "bvh" ? 0.01 : 1;
    for (const track of clip.tracks) {
      const parsed = PropertyBinding.parseTrackName(track.name);
      const node = sourceBones.get(
        parsed.objectName === "bones"
          ? String(parsed.objectIndex)
          : parsed.nodeName,
      );
      if (!node || !bones.includes(node.name))
        throw new Error(`动作轨道无法匹配当前骨架：${track.name}`);
      const property = parsed.propertyName;
      if (!["position", "quaternion", "scale"].includes(property))
        throw new Error(`不支持的动作轨道：${property}`);
      const width = property === "quaternion" ? 4 : 3;
      if (
        track.getValueSize() !== width ||
        track.getInterpolation() !== InterpolateLinear
      )
        throw new Error("动作使用了不支持的采样插值，请导出线性轨道");
      source.rest[node.name] = {
        position: node.position.toArray().map((n) => n * unit),
        rotation: [node.rotation.x, node.rotation.y, node.rotation.z],
        scale: node.scale.toArray(),
      };
      let times = Array.from(track.times),
        values = Array.from(track.values);
      if (property === "position") values = values.map((n) => n * unit);
      if (property === "quaternion")
        for (let i = 0; i < values.length; i += 4)
          new Quaternion().fromArray(values, i).normalize().toArray(values, i);
      const constant = values.every(
        (value, i) => Math.abs(value - values[i % width]) < 1e-7,
      );
      if (constant) {
        times = [0, clip.duration];
        values = [...values.slice(0, width), ...values.slice(0, width)];
      }
      source.tracks.push({
        bone: node.name,
        property: property as "quaternion" | "position" | "scale",
        times,
        values,
      });
    }
    validateDirectorMotionLibrary([source]);
    signal.throwIfAborted();
    return { names, source };
  } finally {
    if (root) disposeDirectorObject(root);
  }
}
