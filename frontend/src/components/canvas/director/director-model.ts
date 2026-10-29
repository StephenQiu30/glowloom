import { directorBuiltinModel } from "./director-model-catalog";
import {
  validateDirectorMotionLibrary,
  type DirectorMotionSource,
} from "./director-motion-library-model";
import {
  supportsDirectorBody,
  validateDirectorBodyProfile,
  type DirectorBodyProfile,
} from "./director-body-profile";
import { validateDirectorMotion } from "./director-motion-model";
import { canvasGraphFingerprint } from "../lib/canvas-json";
import { isProjectID } from "@/components/projects/project";
// Persistent scene/material colors are data, independent of UI theme tokens.
export const DIRECTOR_COLORS = {
  ground: "#242424",
  white: "#ffffff",
  primitive: "#d4d4d8",
} as const;
export type DirectorTransform = Required<API.DirectorTransform>;
export type DirectorMotionClip = API.DirectorMotionClip & {
  source_offset?: number;
  blend_in?: number;
};
export type DirectorFrozenNode = {
  transform: DirectorTransform;
  morph_weights?: number[];
};
export type DirectorFrame = API.DirectorKeyframe & {
  id: string;
  time: number;
  easing: string;
  transform: DirectorTransform;
};
type TransformEntity = {
  id: string;
  name: string;
  transform: DirectorTransform;
  locked?: boolean;
};
export type DirectorObject = Omit<
  API.DirectorObject,
  | keyof TransformEntity
  | "keyframes"
  | "kind"
  | "color"
  | "visible"
  | "cast_shadow"
  | "receive_shadow"
  | "frozen_nodes"
  | "body_profile"
  | "motion_clips"
  | "motion_library"
  | "builtin_model"
> &
  TransformEntity & {
    kind: string;
    color: string;
    visible: boolean;
    cast_shadow: boolean;
    receive_shadow: boolean;
    keyframes: DirectorFrame[];
    frozen_nodes?: Record<string, DirectorFrozenNode>;
    body_profile?: Partial<DirectorBodyProfile>;
    motion_clips?: DirectorMotionClip[];
    motion_library?: DirectorMotionSource[];
    builtin_model?: string;
  };
export type DirectorCamera = Omit<
  API.DirectorCamera,
  | keyof TransformEntity
  | "keyframes"
  | "target"
  | "fov"
  | "near"
  | "far"
  | "focal_length"
  | "aperture"
  | "focus_distance"
> &
  TransformEntity & {
    target: number[];
    fov: number;
    near: number;
    far: number;
    focal_length: number;
    aperture: number;
    focus_distance: number;
    keyframes: DirectorFrame[];
  };
export type DirectorLight = Omit<
  API.DirectorLight,
  keyof TransformEntity | "type" | "color" | "intensity" | "cast_shadow"
> &
  TransformEntity & {
    type: string;
    color: string;
    intensity: number;
    cast_shadow: boolean;
  };
export type DirectorShot = Required<Omit<API.DirectorShot, "screenshots">> &
  Pick<API.DirectorShot, "screenshots">;
export type Director = Omit<
  API.DirectorConfig,
  "objects" | "cameras" | "lights" | "shots" | "ground"
> & {
  id: string;
  version: number;
  title: string;
  mode?: "static" | "dynamic";
  background: string;
  environment_intensity: number;
  grid_visible: boolean;
  objects: DirectorObject[];
  cameras: DirectorCamera[];
  lights: DirectorLight[];
  shots: DirectorShot[];
  active_shot_id: string;
  ground?: API.DirectorGround & { color?: string };
};
export type DirectorSelection = {
  kind: "object" | "camera" | "light";
  id: string;
};
export const directorKey = (selection: DirectorSelection) =>
  `${selection.kind}:${selection.id}`;
export const directorTransform = (position = [0, 0, 0]): DirectorTransform => ({
  position,
  rotation: [0, 0, 0],
  scale: [1, 1, 1],
});
export function newDirectorCamera(name = "机位"): DirectorCamera {
  return {
    id: crypto.randomUUID(),
    name,
    transform: directorTransform([5, 3, 7]),
    target: [0, 1, 0],
    focal_length: 35,
    fov: 50,
    aperture: 2.8,
    focus_distance: 8,
    near: 0.1,
    far: 1000,
    look_at_mode: "coordinates",
    keyframes: [],
    locked: false,
  };
}
export function newDirectorShot(cameraID: string, name = "镜头"): DirectorShot {
  return {
    id: crypto.randomUUID(),
    name,
    camera_id: cameraID,
    duration: 20,
    fps: 30,
    shot_size: "medium",
    camera_move: "static",
    prompt: "",
  };
}
export function emptyDirectorConfig(): Director {
  const camera = newDirectorCamera(),
    shot = newDirectorShot(camera.id);
  return {
    id: crypto.randomUUID(),
    version: 1,
    title: "场景",
    mode: "static",
    background: "#171717",
    environment_intensity: 1,
    grid_visible: true,
    grid_snap: false,
    labels_visible: true,
    aspect_ratio: "16:9",
    ground: {
      visible: true,
      color: DIRECTOR_COLORS.ground,
      opacity: 1,
      height: 0,
    },
    objects: [],
    cameras: [camera],
    lights: [],
    shots: [shot],
    active_shot_id: shot.id,
  };
}
const finite = (n: unknown, min: number, max: number): n is number =>
  typeof n === "number" && Number.isFinite(n) && n >= min && n <= max;
const vector = (v: unknown, min = -10000, max = 10000): v is number[] =>
  Array.isArray(v) && v.length === 3 && v.every((n) => finite(n, min, max));
const color = (v: unknown) => typeof v === "string" && /^#[\da-f]{6}$/i.test(v);
const title = (v: unknown) =>
  typeof v === "string" && !!v.trim() && [...v].length <= 128;
const transform = (
  v: API.DirectorTransform | undefined,
): v is DirectorTransform =>
  !!v && vector(v.position) && vector(v.rotation) && vector(v.scale, 0.01, 100);
function requireFrames(frames: API.DirectorKeyframe[] | undefined) {
  if (!Array.isArray(frames) || frames.length > 256)
    throw new Error("关键帧无效");
  const seen = new Set();
  for (const frame of frames) {
    if (
      !isProjectID(frame.id) ||
      seen.has(frame.id) ||
      !finite(frame.time, 0, 3600) ||
      !transform(frame.transform) ||
      [frame.position_keyed, frame.rotation_keyed, frame.scale_keyed].some(
        (v) => v !== undefined && typeof v !== "boolean",
      ) ||
      (frame.target !== undefined && !vector(frame.target)) ||
      (frame.fov !== undefined && !finite(frame.fov, 1, 179)) ||
      (frame.roll !== undefined && !finite(frame.roll, -Math.PI, Math.PI)) ||
      ![undefined, "", "linear", "smooth", "step"].includes(frame.easing)
    )
      throw new Error("关键帧无效");
    seen.add(frame.id);
  }
}
function requireEntities(items: unknown, limit: number) {
  if (!Array.isArray(items) || items.length > limit)
    throw new Error("场景对象数量无效");
  const seen = new Set();
  for (const item of items as TransformEntity[]) {
    if (
      !isProjectID(item.id) ||
      seen.has(item.id) ||
      !title(item.name) ||
      !transform(item.transform) ||
      (item.locked !== undefined && typeof item.locked !== "boolean")
    )
      throw new Error("场景对象无效");
    seen.add(item.id);
  }
}
/** Read the formal scene without dropping fields owned by other director features. */
export function readDirector(value: API.DirectorConfig | undefined): Director {
  const c = value as Director | undefined;
  if (
    !c ||
    c.version !== 1 ||
    !isProjectID(c.id) ||
    !title(c.title) ||
    !color(c.background) ||
    (c.exposure !== undefined && !finite(c.exposure, 0.1, 5)) ||
    !finite(c.environment_intensity, 0, 10) ||
    typeof c.grid_visible !== "boolean" ||
    ![undefined, "", "static", "dynamic"].includes(c.mode) ||
    ![
      undefined,
      "",
      "adaptive",
      "21:9",
      "16:9",
      "4:3",
      "1:1",
      "3:4",
      "9:16",
      "2.39:1",
    ].includes(c.aspect_ratio)
  )
    throw new Error("导演场景配置无效");
  requireEntities(c.objects, 128);
  requireEntities(c.cameras, 16);
  requireEntities(c.lights, 32);
  if (
    !c.cameras.length ||
    !Array.isArray(c.shots) ||
    !c.shots.length ||
    c.shots.length > 128
  )
    throw new Error("场景需要机位与镜头");
  if (
    c.ground &&
    (!finite(c.ground.height, -2, 2) ||
      !finite(c.ground.opacity, 0, 1) ||
      (c.ground.color && !color(c.ground.color)))
  )
    throw new Error("地面配置无效");
  for (const object of c.objects) {
    if (
      !["primitive", "actor", "model", "billboard"].includes(object.kind) ||
      !color(object.color) ||
      (object.opacity !== undefined && !finite(object.opacity, 0, 1)) ||
      typeof object.visible !== "boolean" ||
      typeof object.cast_shadow !== "boolean" ||
      typeof object.receive_shadow !== "boolean" ||
      (object.asset_id && !isProjectID(object.asset_id)) ||
      (object.source_node_id && !isProjectID(object.source_node_id))
    )
      throw new Error("对象配置无效");
    if (
      (object.kind === "primitive" &&
        ![
          "box",
          "sphere",
          "cylinder",
          "plane",
          "character",
          "cone",
          "torus",
          "capsule",
          "circle",
          "ring",
          "tetrahedron",
          "octahedron",
          "dodecahedron",
          "icosahedron",
          "cuboid",
          "pyramid",
          "half_arc",
          "half_cylinder",
        ].includes(object.primitive ?? "")) ||
      (object.kind === "model" && !object.asset_id && !object.builtin_model) ||
      (object.kind === "actor" &&
        !object.asset_id &&
        object.builtin_actor !== "mannequin" &&
        !/^(male|female)_(standard|teen|child|slim|athletic|broad)$/.test(
          object.builtin_actor ?? "",
        )) ||
      (object.kind === "billboard" &&
        !object.asset_id &&
        !object.source_node_id)
    )
      throw new Error("对象来源无效");
    if (object.builtin_model !== undefined) {
      directorBuiltinModel(object.builtin_model);
      if (
        object.kind !== "model" ||
        object.asset_id ||
        object.builtin_actor ||
        object.primitive
      )
        throw new Error("内置模型来源不能混用");
    }
    requireFrames(object.keyframes);
    if (
      object.motion_library?.length &&
      !["actor", "model"].includes(object.kind)
    )
      throw new Error("只有骨骼角色或模型支持动作库");
    validateDirectorMotionLibrary(object.motion_library);
    validateDirectorMotion(object);
    if (object.body_profile !== undefined) {
      if (!supportsDirectorBody(object))
        throw new Error("身体参数仅适用于内置角色");
      validateDirectorBodyProfile(object.body_profile);
    }
  }
  for (const camera of c.cameras) {
    if (camera.roll !== undefined && !finite(camera.roll, -Math.PI, Math.PI))
      throw new Error("机位倾斜无效");
    if (
      !vector(camera.target) ||
      !finite(camera.fov, 1, 179) ||
      !finite(camera.focal_length, 1, 1000) ||
      !finite(camera.aperture, 0.5, 64) ||
      !finite(camera.focus_distance, 0.01, 10000) ||
      !finite(camera.near, 0.001, 100) ||
      !finite(camera.far, 0.01, 10000) ||
      camera.near >= camera.far ||
      (camera.follow_object_id &&
        !c.objects.some((o) => o.id === camera.follow_object_id)) ||
      (camera.look_at_object_id &&
        !c.objects.some((o) => o.id === camera.look_at_object_id))
    )
      throw new Error("机位配置无效");
    requireFrames(camera.keyframes);
  }
  for (const light of c.lights)
    if (
      !["directional", "point", "spot", "ambient"].includes(light.type) ||
      !color(light.color) ||
      !finite(light.intensity, 0, 100) ||
      (light.angle != null && !finite(light.angle, 0.01, Math.PI / 2)) ||
      (light.penumbra != null && !finite(light.penumbra, 0, 1))
    )
      throw new Error("灯光配置无效");
  const shots = new Set<string>();
  for (const shot of c.shots) {
    if (
      !isProjectID(shot.id) ||
      shots.has(shot.id) ||
      !title(shot.name) ||
      !c.cameras.some((camera) => camera.id === shot.camera_id) ||
      !finite(shot.duration, 0.1, 3600) ||
      ![24, 25, 30, 50, 60].includes(shot.fps) ||
      typeof shot.prompt !== "string" ||
      [...shot.prompt].length > 10000
    )
      throw new Error("镜头配置无效");
    shots.add(shot.id);
  }
  const screenshots = new Set<string>();
  for (const shot of c.shots) {
    if ((shot.screenshots?.length ?? 0) > 64)
      throw new Error("镜头图片数量无效");
    for (const image of shot.screenshots ?? []) {
      if (
        ![undefined, "", "image", "video"].includes(image.kind) ||
        !isProjectID(image.id) ||
        screenshots.has(image.id) ||
        !isProjectID(image.asset_id) ||
        !title(image.name) ||
        image.name!.includes("\0") ||
        !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(
          image.created_at ?? "",
        ) ||
        !Number.isFinite(Date.parse(image.created_at!))
      )
        throw new Error("镜头图片身份无效");
      screenshots.add(image.id);
    }
  }
  if (
    screenshots.size > 512 ||
    (c.cover &&
      (!isProjectID(c.cover.asset_id) || !shots.has(c.cover.shot_id ?? "")))
  )
    throw new Error("镜头输出身份无效");
  if (
    !shots.has(c.active_shot_id) ||
    new TextEncoder().encode(JSON.stringify(c)).length > 512 * 1024
  )
    throw new Error("场景配置无效或过大");
  return structuredClone(c);
}
export function directorEntity(
  scene: Director,
  selected: DirectorSelection | null,
) {
  if (!selected) return undefined;
  return (
    selected.kind === "object"
      ? scene.objects
      : selected.kind === "camera"
        ? scene.cameras
        : scene.lights
  ).find((item) => item.id === selected.id);
}
export type DirectorEdit =
  | { type: "add-object"; primitive: string }
  | { type: "add-camera" }
  | { type: "add-light"; light: string }
  | {
      type: "entity";
      selection: DirectorSelection;
      patch: Partial<DirectorObject & DirectorCamera & DirectorLight>;
    }
  | { type: "remove"; selection: DirectorSelection }
  | {
      type: "keyframe";
      selection: DirectorSelection;
      time: number;
      channel?: "position" | "rotation" | "scale";
      transform: DirectorTransform;
      easing?: string;
    }
  | { type: "remove-keyframe"; selection: DirectorSelection; id: string }
  | { type: "add-shot"; cameraID: string }
  | { type: "shot"; id: string; patch: Partial<DirectorShot> }
  | { type: "remove-shot"; id: string };
export function editDirector(value: Director, edit: DirectorEdit): Director {
  const next = readDirector(value);
  if (edit.type === "add-object")
    next.objects.push({
      id: crypto.randomUUID(),
      name:
        edit.primitive === "character"
          ? "简化人体"
          : (
              {
                box: "立方体",
                sphere: "球体",
                cylinder: "圆柱体",
                plane: "基础平面",
                cuboid: "长方体",
                pyramid: "四棱锥",
                half_arc: "半圆弧",
                half_cylinder: "半圆柱",
                cone: "圆锥体",
                torus: "圆环体",
                capsule: "胶囊体",
                circle: "圆盘",
                ring: "环面",
                tetrahedron: "四面体",
                octahedron: "八面体",
                dodecahedron: "十二面体",
                icosahedron: "二十面体",
              } as Record<string, string>
            )[edit.primitive],
      kind: "primitive",
      primitive: edit.primitive,
      transform: directorTransform([
        0,
        edit.primitive === "plane" ? 0 : 0.5,
        0,
      ]),
      color: "#c2c7ce",
      visible: true,
      cast_shadow: true,
      receive_shadow: true,
      keyframes: [],
      locked: false,
    });
  else if (edit.type === "add-camera")
    next.cameras.push(newDirectorCamera(`机位 ${next.cameras.length + 1}`));
  else if (edit.type === "add-light")
    next.lights.push({
      id: crypto.randomUUID(),
      name: `灯光 ${next.lights.length + 1}`,
      type: edit.light,
      transform: directorTransform([3, 5, 4]),
      color: DIRECTOR_COLORS.white,
      intensity: edit.light === "ambient" ? 1 : 3,
      cast_shadow: true,
      locked: false,
    });
  else if (edit.type === "add-shot") {
    const shot = newDirectorShot(
      edit.cameraID,
      `镜头 ${next.shots.length + 1}`,
    );
    next.shots.push(shot);
    next.active_shot_id = shot.id;
  } else if (edit.type === "shot") {
    const shot = next.shots.find((s) => s.id === edit.id);
    if (!shot) throw new Error("镜头已不存在");
    Object.assign(shot, edit.patch);
  } else if (edit.type === "remove-shot") {
    if (next.shots.length === 1) throw new Error("至少保留一个镜头");
    next.shots = next.shots.filter((s) => s.id !== edit.id);
    if (next.active_shot_id === edit.id) next.active_shot_id = next.shots[0].id;
  } else {
    const entity = directorEntity(next, edit.selection);
    if (!entity) throw new Error("对象已不存在");
    const unlocking =
      edit.type === "entity" &&
      Object.keys(edit.patch).every((key) =>
        ["locked", "visible"].includes(key),
      );
    if (entity.locked && !unlocking) throw new Error("对象已锁定");
    if (edit.type === "entity") {
      if (Object.hasOwn(edit.patch, "body_profile")) {
        if (next.mode === "dynamic")
          throw new Error("请在静态预演中修改身体参数");
        if (
          edit.selection.kind !== "object" ||
          !supportsDirectorBody({ ...entity, ...edit.patch } as DirectorObject)
        )
          throw new Error("身体参数仅适用于内置角色");
        Object.assign(entity, edit.patch);
        delete (entity as DirectorObject).frozen_bones;
        delete (entity as DirectorObject).frozen_nodes;
      } else Object.assign(entity, edit.patch);
    } else if (edit.type === "remove") {
      if (edit.selection.kind === "camera") {
        if (next.shots.some((s) => s.camera_id === entity.id))
          throw new Error("机位仍被镜头使用");
        next.cameras = next.cameras.filter((c) => c.id !== entity.id);
      } else if (edit.selection.kind === "light")
        next.lights = next.lights.filter((l) => l.id !== entity.id);
      else {
        next.objects = next.objects.filter((o) => o.id !== entity.id);
        for (const camera of next.cameras) {
          if (camera.follow_object_id === entity.id) {
            delete camera.follow_object_id;
            delete camera.follow_anchor;
          }
          if (camera.look_at_object_id === entity.id) {
            delete camera.look_at_object_id;
            camera.look_at_mode = "coordinates";
          }
        }
      }
    } else {
      if (edit.selection.kind === "light")
        throw new Error("灯光没有变换关键帧合同");
      const animated = entity as DirectorObject | DirectorCamera;
      if (edit.type === "remove-keyframe")
        animated.keyframes = animated.keyframes.filter((f) => f.id !== edit.id);
      else {
        const channels = edit.channel
          ? [edit.channel]
          : (["position", "rotation", "scale"] as const);
        if (edit.easing !== undefined) {
          for (const frame of animated.keyframes) {
            if (
              Math.abs(frame.time - edit.time) < 1e-6 &&
              (["position", "rotation", "scale"] as const).every(
                (channel) =>
                  frame[`${channel}_keyed`] === false ||
                  channels.includes(channel),
              ) &&
              frame.target === undefined &&
              frame.fov === undefined &&
              frame.roll === undefined
            )
              frame.easing = edit.easing;
          }
        }
        for (const channel of channels) {
          const sameTime = animated.keyframes.filter(
            (frame) => Math.abs(frame.time - edit.time) < 1e-6,
          );
          const owners = sameTime.filter(
            (frame) => frame[`${channel}_keyed`] !== false,
          );
          const easing = edit.easing ?? owners[0]?.easing ?? "linear";
          // A frame's easing is shared by every channel it owns. Reuse it only
          // when that interpolation matches, otherwise keep a separate frame.
          for (const owner of owners) owner[`${channel}_keyed`] = false;
          let frame =
            owners.find((candidate) => candidate.easing === easing) ??
            sameTime.find((candidate) => candidate.easing === easing);
          if (!frame) {
            frame = {
              id: crypto.randomUUID(),
              time: edit.time,
              transform: structuredClone(edit.transform),
              easing,
              position_keyed: false,
              rotation_keyed: false,
              scale_keyed: false,
            };
            animated.keyframes.push(frame);
          }
          frame.transform[channel] = [...edit.transform[channel]];
          frame[`${channel}_keyed`] = true;
        }
        animated.keyframes = animated.keyframes.filter(
          (frame) =>
            frame.position_keyed !== false ||
            frame.rotation_keyed !== false ||
            frame.scale_keyed !== false ||
            frame.target !== undefined ||
            frame.fov !== undefined ||
            frame.roll !== undefined,
        );
        animated.keyframes.sort((a, b) => a.time - b.time);
      }
    }
  }
  return readDirector(next);
}

/** Go RFC3339Nano trims fractional trailing zeros in authoritative readback. */
export function directorFingerprint(value: unknown): string {
  if (!value || typeof value !== "object") return canvasGraphFingerprint(value);
  const copy = structuredClone(value) as Record<string, unknown>;
  if (copy.workspaces && typeof copy.workspaces === "object")
    for (const [mode, scene] of Object.entries(copy.workspaces))
      (copy.workspaces as Record<string, unknown>)[mode] = JSON.parse(
        directorFingerprint(scene),
      );
  if (Array.isArray(copy.objects))
    for (const object of copy.objects) {
      if (!object || typeof object !== "object") continue;
      if (Array.isArray(object.motion_clips))
        for (const clip of object.motion_clips)
          if (clip) {
            if (clip.source_offset === 0) delete clip.source_offset;
            if (clip.blend_in === 0) delete clip.blend_in;
          }
      for (const field of [
        "motion_clips",
        "motion_library",
        "bone_tracks",
        "bone_overrides",
        "frozen_bones",
        "frozen_nodes",
        "body_profile",
      ]) {
        const value = object[field];
        if (
          value &&
          typeof value === "object" &&
          Object.keys(value).length === 0
        )
          delete object[field];
      }
    }
  if (Array.isArray(copy.cameras))
    for (const camera of copy.cameras)
      if (camera && camera.roll === 0) delete camera.roll;
  if (Array.isArray(copy.shots))
    for (const shot of copy.shots) {
      if (!shot || !Array.isArray(shot.screenshots)) continue;
      if (!shot.screenshots.length) {
        delete shot.screenshots;
        continue;
      }
      for (const image of shot.screenshots) {
        if (
          image &&
          typeof image.created_at === "string" &&
          Number.isFinite(Date.parse(image.created_at))
        ) {
          image.created_at = new Date(image.created_at).toISOString();
        }
      }
    }
  return canvasGraphFingerprint(copy);
}
