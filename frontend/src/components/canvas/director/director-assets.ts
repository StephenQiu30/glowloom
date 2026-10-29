import { directorTransform, type DirectorObject } from "./director-model";
import actorHeights from "../../../../public/director/actors/heights.json";
export const DIRECTOR_GEOMETRIES = [
  ["character", "简化人体"],
  ["box", "立方体"],
  ["sphere", "球体"],
  ["cuboid", "长方体"],
  ["cylinder", "圆柱体"],
  ["cone", "圆锥体"],
  ["pyramid", "四棱锥"],
  ["torus", "圆环体"],
  ["plane", "基础平面"],
  ["capsule", "胶囊体"],
  ["circle", "圆盘"],
  ["half_arc", "半圆弧"],
  ["half_cylinder", "半圆柱"],
] as const;
export const DIRECTOR_ACTORS = (
  [
    ["male", "男性"],
    ["female", "女性"],
  ] as const
).flatMap(([sex, label]) =>
  (
    [
      ["standard", "标准"],
      ["teen", "少年"],
      ["child", "儿童"],
      ["slim", "纤瘦"],
      ["athletic", "健壮"],
      ["broad", "丰腴"],
    ] as const
  ).map(([shape, name]) => ({
    id: `${sex}_${shape}`,
    name: `${label} · ${name}`,
    sex,
    shape,
    thumbnail: `/director/actors/${sex}_${shape}.png`,
    heightM: actorHeights[`${sex}_${shape}`],
  })),
);
export function actorModelURL(id: string) {
  const actor = DIRECTOR_ACTORS.find((item) => item.id === id);
  if (!actor) throw new Error("角色预设不存在");
  return `/director/models/quaternius-standard-${actor.sex}.glb`;
}
export function createDirectorActor(id: string): DirectorObject {
  const preset = DIRECTOR_ACTORS.find((item) => item.id === id);
  if (!preset) throw new Error("角色预设不存在");
  const bone_map: Record<string, string> = {
    root: "root",
    hips: "pelvis",
    spine: "spine_01",
    chest: "spine_02",
    neck: "neck_01",
    head: "Head",
  };
  for (const [side, suffix] of [
    ["left", "l"],
    ["right", "r"],
  ]) {
    for (const [bone, name] of [
      ["Shoulder", "clavicle"],
      ["UpperArm", "upperarm"],
      ["LowerArm", "lowerarm"],
      ["Hand", "hand"],
      ["UpperLeg", "thigh"],
      ["LowerLeg", "calf"],
      ["Foot", "foot"],
    ])
      bone_map[`${side}${bone}`] = `${name}_${suffix}`;
    for (const finger of ["Thumb", "Index", "Middle", "Ring", "Pinky"])
      for (let n = 1; n <= 3; n++)
        bone_map[`${side}${finger}${n}`] =
          `${finger.toLowerCase()}_0${n}_${suffix}`;
  }
  return {
    id: crypto.randomUUID(),
    name: preset.name,
    kind: "actor",
    builtin_actor: id,
    pose: "stand",
    rig: { bone_map, animation_names: [] },
    transform: directorTransform(),
    color: "#d9d9de",
    visible: true,
    cast_shadow: true,
    receive_shadow: true,
    keyframes: [],
  };
}
