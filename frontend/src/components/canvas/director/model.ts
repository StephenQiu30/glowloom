import { z } from "zod";
import { DIRECTOR_ASPECT_RATIOS } from "./aspect-ratio";

const id = z.string().uuid();
const number = z.number().finite().min(-10000).max(10000);
const vector = z.tuple([number, number, number]);
const quaternion = z
  .tuple([
    z.number().min(-1).max(1),
    z.number().min(-1).max(1),
    z.number().min(-1).max(1),
    z.number().min(-1).max(1),
  ])
  .refine(
    (value) => Math.abs(Math.hypot(...value) - 1) < 0.01,
    "骨骼四元数需为单位旋转。",
  );
const color = z.string().regex(/^#[0-9a-f]{6}$/i);
const name = z.string().min(1).max(128);
const easing = z.enum(["step", "linear", "smooth"]);
const transform = z
  .object({
    position: vector,
    rotation: vector,
    scale: z.tuple([
      z.number().min(0.01).max(100),
      z.number().min(0.01).max(100),
      z.number().min(0.01).max(100),
    ]),
  })
  .strict();
const keyframe = z
  .object({
    id,
    time: z.number().min(0).max(3600),
    transform,
    easing: easing.optional(),
  })
  .strict();
const bones = [
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
  "leftThumb1",
  "leftThumb2",
  "leftThumb3",
  "leftIndex1",
  "leftIndex2",
  "leftIndex3",
  "leftMiddle1",
  "leftMiddle2",
  "leftMiddle3",
  "leftRing1",
  "leftRing2",
  "leftRing3",
  "leftPinky1",
  "leftPinky2",
  "leftPinky3",
  "rightThumb1",
  "rightThumb2",
  "rightThumb3",
  "rightIndex1",
  "rightIndex2",
  "rightIndex3",
  "rightMiddle1",
  "rightMiddle2",
  "rightMiddle3",
  "rightRing1",
  "rightRing2",
  "rightRing3",
  "rightPinky1",
  "rightPinky2",
  "rightPinky3",
] as const;
export const directorBoneNames = bones;
const bone = z.enum(bones);
const pose = z.enum([
  "neutral",
  "stand",
  "t_pose",
  "walk",
  "run",
  "sit",
  "squat",
  "kneel_single",
  "kneel_double",
  "hands_hips",
  "lean",
  "bow",
  "think",
  "fight",
  "kick",
  "throw",
  "push",
  "wave",
  "reach",
  "arms_crossed",
  "phone",
]);
const boneFrame = z
  .object({
    id,
    time: z.number().min(0).max(3600),
    rotation: quaternion,
    easing: easing.optional(),
  })
  .strict();
const boneTrack = z
  .object({ bone, keyframes: z.array(boneFrame).max(256) })
  .strict();
const object = z
  .object({
    id,
    name,
    kind: z.enum(["primitive", "model", "actor", "billboard"]),
    primitive: z
      .enum(["box", "sphere", "cylinder", "plane", "character"])
      .optional(),
    transform,
    color,
    uniformScale: z.number().min(0.1).max(10).optional(),
    visible: z.boolean(),
    castShadow: z.boolean(),
    receiveShadow: z.boolean(),
    pose: pose.optional(),
    rig: z
      .object({
        boneMap: z.partialRecord(bone, z.string().max(128)),
        animationNames: z.array(z.string().max(128)).max(128),
      })
      .strict()
      .optional(),
    motionClips: z
      .array(
        z
          .object({
            id,
            name,
            sourceAnimation: z.string().max(128),
            start: z.number().min(0).max(3600),
            duration: z.number().min(0.1).max(3600),
            playbackRate: z.number().min(0.1).max(4),
            loop: z.boolean(),
          })
          .strict(),
      )
      .max(128)
      .optional(),
    activeMotionClipId: id.optional(),
    boneOverrides: z.partialRecord(bone, quaternion).optional(),
    boneTracks: z.array(boneTrack).max(bones.length).optional(),
    sourceNodeId: id.optional(),
    assetId: id.optional(),
    builtinActor: z.literal("mannequin").optional(),
    keyframes: z.array(keyframe).max(256),
  })
  .strict();
const camera = z
  .object({
    id,
    name,
    transform,
    target: vector,
    followObjectId: id.optional(),
    followAnchor: vector.optional(),
    lookAtMode: z.enum(["coordinates", "rotation", "object"]).optional(),
    lookAtObjectId: id.optional(),
    focalLength: z.number().min(1).max(1000),
    fov: z.number().min(1).max(179),
    aperture: z.number().min(0.5).max(64),
    focusDistance: z.number().min(0.01).max(10000),
    near: z.number().min(0.001).max(100),
    far: z.number().min(0.01).max(10000),
    keyframes: z.array(keyframe).max(256),
  })
  .strict();
const light = z
  .object({
    id,
    name,
    type: z.enum(["directional", "point", "spot", "ambient"]),
    transform,
    color,
    intensity: z.number().min(0).max(100),
    angle: z
      .number()
      .min(0.01)
      .max(Math.PI / 2)
      .optional(),
    penumbra: z.number().min(0).max(1).optional(),
    castShadow: z.boolean(),
  })
  .strict();
const shot = z
  .object({
    id,
    name,
    cameraId: id,
    duration: z.number().min(0.1).max(3600),
    fps: z.union([z.literal(24), z.literal(25), z.literal(30)]),
    shotSize: z.enum([
      "extreme_wide",
      "wide",
      "full",
      "medium",
      "close_up",
      "extreme_close_up",
    ]),
    cameraMove: z.enum([
      "static",
      "push_in",
      "pull_out",
      "pan_left",
      "pan_right",
      "tilt_up",
      "tilt_down",
      "orbit_left",
      "orbit_right",
      "handheld",
    ]),
    prompt: z.string().max(10000),
    screenshots: z
      .array(
        z
          .object({
            id,
            assetId: id,
            name: z
              .string()
              .min(1)
              .max(128)
              .refine(
                (value) => Boolean(value.trim()) && !value.includes("\0"),
              ),
            createdAt: z.iso.datetime({ offset: true }),
          })
          .strict(),
      )
      .max(64)
      .optional(),
  })
  .strict();
export const directorSchema = z
  .object({
    id,
    version: z.literal(1),
    title: name,
    background: color,
    environmentIntensity: z.number().min(0).max(10),
    gridVisible: z.boolean(),
    gridSnap: z.boolean().optional(),
    ground: z
      .object({
        visible: z.boolean(),
        opacity: z.number().min(0).max(1),
        height: z.number().min(-2).max(2),
      })
      .strict()
      .optional(),
    stageTransform: z
      .object({
        scale: z.number().min(0.1).max(10),
        position: vector,
        rotation: vector,
      })
      .strict()
      .optional(),
    labelsVisible: z.boolean().optional(),
    aspectRatio: z.enum(DIRECTOR_ASPECT_RATIOS).optional(),
    panorama: z
      .object({
        assetId: id,
        name: z.string().max(128).optional(),
        rotation: z.number().min(-360).max(360),
      })
      .strict()
      .optional(),
    panoramaRotation: z.number().min(-360).max(360).optional(),
    panoramaRadius: z.number().min(1).max(200).optional(),
    objects: z.array(object).max(128),
    cameras: z.array(camera).min(1).max(16),
    lights: z.array(light).max(32),
    shots: z.array(shot).min(1).max(128),
    activeShotId: id,
    cover: z.object({ assetId: id, shotId: id }).strict().optional(),
  })
  .strict()
  .superRefine((value, context) => {
    const objects = new Set(value.objects.map((item) => item.id)),
      cameras = new Set(value.cameras.map((item) => item.id)),
      shots = new Set(value.shots.map((item) => item.id));
    if (
      objects.size !== value.objects.length ||
      cameras.size !== value.cameras.length ||
      shots.size !== value.shots.length ||
      new Set(value.lights.map((item) => item.id)).size !== value.lights.length
    )
      context.addIssue({ code: "custom", message: "场景元素身份重复。" });
    if (
      !shots.has(value.activeShotId) ||
      value.shots.some((item) => !cameras.has(item.cameraId)) ||
      (value.cover && !shots.has(value.cover.shotId))
    )
      context.addIssue({ code: "custom", message: "分镜引用不存在的摄影机。" });
    const screenshots = value.shots.flatMap((item) => item.screenshots ?? []);
    if (
      screenshots.length > 512 ||
      new Set(screenshots.map((item) => item.id)).size !== screenshots.length
    )
      context.addIssue({ code: "custom", message: "场景截图超额或身份重复。" });
    if (
      value.cameras.some(
        (item) =>
          item.near >= item.far ||
          (item.followObjectId && !objects.has(item.followObjectId)) ||
          (item.lookAtObjectId && !objects.has(item.lookAtObjectId)),
      )
    )
      context.addIssue({
        code: "custom",
        message: "摄影机跟随或裁剪范围无效。",
      });
    for (const item of value.objects) {
      if (item.kind === "primitive" && !item.primitive)
        context.addIssue({ code: "custom", message: "几何体需要具体类型。" });
      if (item.kind === "model" && !item.assetId)
        context.addIssue({ code: "custom", message: "模型需要授权资产。" });
      if (item.kind === "billboard" && !item.assetId && !item.sourceNodeId)
        context.addIssue({
          code: "custom",
          message: "布景图片需要授权资产或画布参考。",
        });
      const frames = [
        item.keyframes,
        ...(item.boneTracks ?? []).map((track) => track.keyframes),
      ];
      if (
        frames.some(
          (items) =>
            new Set(items.map((frame) => frame.id)).size !== items.length,
        )
      )
        context.addIssue({ code: "custom", message: "关键帧编号重复。" });
      const motions = new Set(item.motionClips?.map((clip) => clip.id));
      if (
        motions.size !== (item.motionClips?.length ?? 0) ||
        (item.activeMotionClipId && !motions.has(item.activeMotionClipId))
      )
        context.addIssue({
          code: "custom",
          message: "动画片段编号或当前片段无效。",
        });
      if (item.kind === "actor" && !item.assetId && !item.builtinActor)
        context.addIssue({
          code: "custom",
          message: "演员需要模型或内置人偶。",
        });
      if (
        new Set(item.boneTracks?.map((track) => track.bone)).size !==
        (item.boneTracks?.length ?? 0)
      )
        context.addIssue({ code: "custom", message: "骨骼轨道重复。" });
    }
  });
export type DirectorRenderMode =
  "beauty" | "clay" | "depth" | "normal" | "pose";
export type DirectorScene = z.infer<typeof directorSchema>;
export type DirectorVec3 = [number, number, number];
export type DirectorQuat = [number, number, number, number];
export type DirectorObject = DirectorScene["objects"][number];
export type DirectorCamera = DirectorScene["cameras"][number];
export type DirectorLight = DirectorScene["lights"][number];
export type DirectorShot = DirectorScene["shots"][number];
export type DirectorScreenshot = NonNullable<
  DirectorShot["screenshots"]
>[number];
export type DirectorTransform = z.infer<typeof transform>;
export type DirectorKeyframe = z.infer<typeof keyframe>;
export type DirectorKeyframeEasing = z.infer<typeof easing>;
export type DirectorPose = z.infer<typeof pose>;
export type DirectorHumanoidBone = (typeof bones)[number];
export type DirectorBoneKeyframe = z.infer<typeof boneFrame>;
export type DirectorBoneTrack = z.infer<typeof boneTrack>;
export type DirectorKeyframeDeleteTarget =
  | { track: "object-transform"; objectId: string; keyframeId: string }
  | {
      track: "object-bone";
      objectId: string;
      bone: DirectorHumanoidBone;
      keyframeId: string;
    }
  | { track: "camera"; cameraId: string; keyframeId: string };

type Snake<S extends string> = S extends `${infer First}${infer Rest}`
  ? `${First extends Lowercase<First> ? First : `_${Lowercase<First>}`}${Snake<Rest>}`
  : S;
type DirectorWire<T> =
  T extends Array<infer Item>
    ? DirectorWire<Item>[]
    : T extends object
      ? {
          [
            Key in keyof T as Key extends string ? Snake<Key> : Key
          ]: Key extends "boneMap" | "boneOverrides"
            ? T[Key]
            : DirectorWire<T[Key]>;
        }
      : T;
// This conversion is confined to the closed director schema; bone names are enum values.
function directorKeys(
  value: unknown,
  encode: boolean,
  keepKeys = false,
): unknown {
  if (Array.isArray(value))
    return value.map((item) => directorKeys(item, encode));
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value)
      .filter(([, item]) => item !== undefined)
      .map(([key, item]) => {
        const output = keepKeys
          ? key
          : encode
            ? key.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`)
            : key.replace(/_([a-z])/g, (_, letter: string) =>
                letter.toUpperCase(),
              );
        return [
          output,
          directorKeys(
            item,
            encode,
            ["boneMap", "boneOverrides", "bone_map", "bone_overrides"].includes(
              key,
            ),
          ),
        ];
      }),
  );
}
export function encodeDirector(
  value: DirectorScene,
): DirectorWire<DirectorScene> {
  return directorKeys(
    directorSchema.parse(value),
    true,
  ) as DirectorWire<DirectorScene>;
}
export function decodeDirector(value: unknown): DirectorScene {
  return directorSchema.parse(directorKeys(value, false));
}
export const directorWireSchema = z.json().superRefine((value, context) => {
  try {
    decodeDirector(value);
  } catch {
    context.addIssue({ code: "custom", message: "导演场景配置无效。" });
  }
});
/** Pure defaults keep Three.js outside the core canvas bundle. */
export function createDirectorConfig(title = "未命名场景"): DirectorScene {
  const cameraId = crypto.randomUUID(),
    shotId = crypto.randomUUID();
  const identity: DirectorTransform = {
    position: [0, 0, 0],
    rotation: [0, 0, 0],
    scale: [1, 1, 1],
  };
  return {
    id: crypto.randomUUID(),
    version: 1,
    title,
    background: "#060608",
    environmentIntensity: 0.7,
    gridVisible: true,
    gridSnap: false,
    ground: { visible: true, opacity: 0.4, height: 0 },
    stageTransform: { scale: 1, position: [0, 0, 0], rotation: [0, 0, 0] },
    labelsVisible: true,
    aspectRatio: "16:9",
    panoramaRotation: 0,
    panoramaRadius: 60,
    objects: [
      {
        id: crypto.randomUUID(),
        name: "演员 1",
        kind: "actor",
        builtinActor: "mannequin",
        transform: structuredClone(identity),
        color: "#f1f3f5",
        visible: true,
        castShadow: true,
        receiveShadow: true,
        pose: "stand",
        keyframes: [],
        boneOverrides: {},
        boneTracks: [],
        motionClips: [],
        rig: { boneMap: {}, animationNames: [] },
      },
    ],
    cameras: [
      {
        id: cameraId,
        name: "主摄影机",
        transform: { ...structuredClone(identity), position: [4.8, 2.7, 6.8] },
        target: [0, 1, 0],
        focalLength: 35,
        fov: 54.43222311461495,
        aperture: 2.8,
        focusDistance: 5,
        near: 0.05,
        far: 500,
        keyframes: [],
      },
    ],
    lights: [
      {
        id: crypto.randomUUID(),
        name: "主光",
        type: "directional",
        transform: { ...structuredClone(identity), position: [4, 6, 4] },
        color: "#ffffff",
        intensity: 2.4,
        castShadow: true,
      },
      {
        id: crypto.randomUUID(),
        name: "环境光",
        type: "ambient",
        transform: structuredClone(identity),
        color: "#ffffff",
        intensity: 0.65,
        castShadow: false,
      },
    ],
    shots: [
      {
        id: shotId,
        name: "镜头 1",
        cameraId,
        duration: 5,
        fps: 24,
        shotSize: "medium",
        cameraMove: "static",
        prompt: "",
      },
    ],
    activeShotId: shotId,
  };
}
