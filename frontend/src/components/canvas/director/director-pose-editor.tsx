"use client";
import { Euler, Quaternion } from "three";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FieldSet, FieldLegend } from "@/components/ui/field";
import { directorPoseBoneDeltas } from "./director-poses";
import type { DirectorObject, DirectorTransform } from "./director-model";
const GROUPS = [
  ["身体整体", [["hips", "身体"]]],
  [
    "胸腰",
    [
      ["spine", "腰部"],
      ["chest", "胸部"],
    ],
  ],
  ["头部", [["head", "头部"]]],
  [
    "左臂",
    [
      ["leftUpperArm", "上臂"],
      ["leftLowerArm", "前臂"],
      ["leftHand", "手腕"],
    ],
  ],
  [
    "右臂",
    [
      ["rightUpperArm", "上臂"],
      ["rightLowerArm", "前臂"],
      ["rightHand", "手腕"],
    ],
  ],
  [
    "左腿",
    [
      ["leftUpperLeg", "大腿"],
      ["leftLowerLeg", "小腿"],
      ["leftFoot", "脚踝"],
    ],
  ],
  [
    "右腿",
    [
      ["rightUpperLeg", "大腿"],
      ["rightLowerLeg", "小腿"],
      ["rightFoot", "脚踝"],
    ],
  ],
] as const;
export function DirectorPoseEditor({
  object,
  disabled,
  onChange,
  rest,
  nodePaths,
}: {
  object: DirectorObject;
  disabled: boolean;
  rest?: Record<string, DirectorTransform>;
  nodePaths?: Record<string, string>;
  onChange: (patch: Partial<DirectorObject>) => void;
}) {
  const deltas = directorPoseBoneDeltas(object.pose ?? "stand");
  const baseRotation = (bone: string) => {
    const name = object.rig?.bone_map?.[bone],
      transform = name ? rest?.[name] : undefined;
    const q = transform
      ? new Quaternion().setFromEuler(
          new Euler(...(transform.rotation as [number, number, number])),
        )
      : new Quaternion();
    const delta = new Quaternion().fromArray(
      deltas[bone as keyof typeof deltas] ?? [0, 0, 0, 1],
    );
    if (
      bone === "leftUpperArm" &&
      object.builtin_actor &&
      object.builtin_actor !== "mannequin"
    )
      delta.invert();
    return q.multiply(delta);
  };
  const rotation = (bone: string) => {
    if (object.bone_overrides?.[bone])
      return new Quaternion().fromArray(object.bone_overrides[bone]);
    const name = object.rig?.bone_map?.[bone],
      path = name ? nodePaths?.[name] : undefined,
      frozen =
        (path ? object.frozen_nodes?.[path]?.transform : undefined) ??
        (name ? object.frozen_bones?.[name] : undefined);
    return frozen?.rotation
      ? new Quaternion().setFromEuler(
          new Euler(...(frozen.rotation as [number, number, number])),
        )
      : baseRotation(bone);
  };
  const relativeRotation = (bone: string) =>
    baseRotation(bone).invert().multiply(rotation(bone));
  const change = (bone: string, index: number, degrees: number) => {
    if (disabled || object.locked) return;
    const q = relativeRotation(bone);
    const angles = new Euler().setFromQuaternion(q);
    const values = [angles.x, angles.y, angles.z];
    values[index] = (degrees * Math.PI) / 180;
    onChange({
      bone_overrides: {
        ...object.bone_overrides,
        [bone]: baseRotation(bone)
          .multiply(
            new Quaternion().setFromEuler(
              new Euler(...(values as [number, number, number])),
            ),
          )
          .toArray(),
      },
    });
  };
  return (
    <div className="space-y-3">
      {GROUPS.map(([group, items]) => {
        const mapped = items.filter(([bone]) => object.rig?.bone_map?.[bone]);
        if (!mapped.length) return null;
        return (
          <FieldSet
            key={group}
            disabled={disabled || object.locked}
            className="gap-2"
          >
            <FieldLegend variant="label">{group}</FieldLegend>
            {mapped.map(([bone, label]) => {
              const angles = new Euler().setFromQuaternion(
                relativeRotation(bone),
              );
              return (
                <div key={bone}>
                  {[angles.x, angles.y, angles.z].map((r, i) => (
                    <Label key={i} className="director-field">
                      {label} {["前后倾", "转向", "侧倾"][i]}
                      <Input
                        key={`${object.id}:${r}`}
                        type="number"
                        min={-180}
                        max={180}
                        step={1}
                        defaultValue={Math.round((r * 180) / Math.PI)}
                        aria-label={`${group} ${label} ${["前后倾", "转向", "侧倾"][i]}`}
                        onBlur={(event) => {
                          const n = Number(event.target.value);
                          if (
                            event.target.value.trim() &&
                            Number.isFinite(n) &&
                            n >= -180 &&
                            n <= 180
                          )
                            change(bone, i, n);
                          else
                            event.target.value = String(
                              Math.round((r * 180) / Math.PI),
                            );
                        }}
                        onKeyDown={(event) => {
                          event.stopPropagation();
                          if (event.key === "Enter") event.currentTarget.blur();
                        }}
                      />
                    </Label>
                  ))}
                </div>
              );
            })}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                if (disabled || object.locked) return;
                const overrides = { ...object.bone_overrides };
                for (const [bone] of items)
                  if (object.rig?.bone_map?.[bone])
                    overrides[bone] = baseRotation(bone).toArray();
                onChange({
                  bone_overrides: overrides,
                });
              }}
            >
              重置{group}
            </Button>
          </FieldSet>
        );
      })}
    </div>
  );
}
