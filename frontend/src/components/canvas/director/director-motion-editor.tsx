"use client";
import { useState } from "react";
import { DIRECTOR_BUILTIN_MOTION_PRESETS } from "./director-motion-catalog";
const motionLabel = (name: string) =>
  DIRECTOR_BUILTIN_MOTION_PRESETS.find((preset) => preset.name === name)
    ?.label ?? name;
import { Euler, Quaternion } from "three";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DIRECTOR_BONES,
  placeDirectorMotionClip,
} from "./director-motion-model";
import { sampleDirectorBone, type DirectorModelInfo } from "./director-motion";
import type { DirectorObject, DirectorMotionClip } from "./director-model";
function NumberValue({
  label,
  value,
  min,
  max,
  disabled,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  disabled: boolean;
  onChange: (value: number) => void;
}) {
  return (
    <Label className="director-field">
      {label}
      <Input
        key={value}
        aria-label={label}
        type="number"
        defaultValue={value}
        min={min}
        max={max}
        step="0.1"
        disabled={disabled}
        onBlur={(event) => {
          const next = Number(event.target.value);
          if (
            event.target.value.trim() &&
            Number.isFinite(next) &&
            next >= min &&
            next <= max &&
            next !== value
          )
            onChange(next);
          else event.target.value = String(value);
        }}
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Enter") event.currentTarget.blur();
        }}
      />
    </Label>
  );
}
function Choice({
  label,
  value,
  items,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  items: [string, string][];
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <Label className="director-field">
      {label}
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent
          data-canvas-no-zoom
          onMouseDown={(event) => event.stopPropagation()}
        >
          {items.map(([key, text]) => (
            <SelectItem key={key} value={key}>
              {text}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Label>
  );
}
export function DirectorMotionEditor({
  object,
  info,
  dynamic,
  time,
  duration,
  disabled,
  onChange,
}: {
  object: DirectorObject;
  info?: DirectorModelInfo;
  dynamic: boolean;
  time: number;
  duration: number;
  disabled: boolean;
  onChange: (patch: Partial<DirectorObject>) => void;
}) {
  const [error, setError] = useState(""),
    [source, setSource] = useState(""),
    [bone, setBone] = useState<string>("hips");
  const available =
    info &&
    info.assetID === (object.asset_id ?? "") &&
    info.objectID === object.id
      ? info
      : undefined;
  const animation =
    available?.animations.find((item) => item.name === source) ??
    available?.animations[0];
  const clips = object.motion_clips ?? [];
  function placeClip(
    candidate: DirectorMotionClip,
    patch: Partial<DirectorObject> = {},
  ) {
    try {
      const motion_clips = placeDirectorMotionClip(clips, candidate, duration);
      onChange({ ...patch, motion_clips });
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "动作无法放置");
    }
  }
  const patchClip = (id: string, patch: Partial<DirectorMotionClip>) => {
    const clip = clips.find((item) => item.id === id);
    if (clip) placeClip({ ...clip, ...patch });
  };
  const track = object.bone_tracks?.find((item) => item.bone === bone),
    mapped = object.rig?.bone_map?.[bone];
  const pose = dynamic
    ? sampleDirectorBone(track?.keyframes ?? [], time)
    : null;
  const rotation =
    pose ??
    new Quaternion().fromArray(object.bone_overrides?.[bone] ?? [0, 0, 0, 1]);
  const angles = new Euler()
    .setFromQuaternion(rotation)
    .toArray()
    .slice(0, 3) as number[];
  function changeBone(index: number, degrees: number) {
    const values = [...angles];
    values[index] = (degrees * Math.PI) / 180;
    const value = new Quaternion()
      .setFromEuler(new Euler(values[0], values[1], values[2]))
      .toArray();
    if (!dynamic)
      onChange({ bone_overrides: { ...object.bone_overrides, [bone]: value } });
    else {
      const frames = track?.keyframes ?? [],
        existing = frames.find(
          (frame) => Math.abs(frame.time! - time) < 0.000001,
        );
      const next: API.DirectorBoneTrack = {
        bone,
        keyframes: [
          ...frames.filter((frame) => frame !== existing),
          {
            id: existing?.id ?? crypto.randomUUID(),
            time,
            rotation: value,
            easing: existing?.easing ?? "linear",
          },
        ].sort((a, b) => a.time! - b.time!),
      };
      onChange({
        bone_tracks: [
          ...(object.bone_tracks ?? []).filter((item) => item.bone !== bone),
          next,
        ],
      });
    }
  }
  return (
    <div className="director-fields">
      <h3>模型动作</h3>
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      {!available ? (
        <p className="text-foreground/45">等待模型加载与动作检查</p>
      ) : !available.animations.length ? (
        <p className="text-foreground/45">此模型没有可用的内置动画</p>
      ) : (
        <>
          <Choice
            label="内置动画"
            value={animation?.name ?? ""}
            items={available.animations.map((item) => [
              item.name,
              motionLabel(item.name),
            ])}
            disabled={disabled}
            onChange={setSource}
          />
          <Button
            size="sm"
            disabled={
              disabled ||
              !dynamic ||
              !animation ||
              time >= duration ||
              clips.length >= 128
            }
            onClick={() => {
              if (!animation) return;
              const id = crypto.randomUUID();
              placeClip(
                {
                  id,
                  name: motionLabel(animation.name),
                  source_animation: animation.name,
                  start: time,
                  duration: Math.min(animation.duration, duration - time),
                  playback_rate: 1,
                  loop: false,
                },
                {
                  rig: {
                    bone_map: object.rig?.bone_map ?? {},
                    animation_names: available.animations.map(
                      (item) => item.name,
                    ),
                  },
                  active_motion_clip_id: id,
                },
              );
            }}
          >
            在当前时间添加动作
          </Button>
          {!dynamic && (
            <p className="text-foreground/45">切换动态预演可编排动作</p>
          )}
        </>
      )}
      {clips.map((clip) => (
        <div key={clip.id} className="director-fields rounded border p-2">
          <span>{clip.name}</span>
          <NumberValue
            label={`${clip.name} 开始`}
            value={clip.start!}
            min={0}
            max={duration - clip.duration!}
            disabled={disabled}
            onChange={(start) => patchClip(clip.id!, { start })}
          />
          <NumberValue
            label={`${clip.name} 时长`}
            value={clip.duration!}
            min={0.1}
            max={duration - clip.start!}
            disabled={disabled}
            onChange={(duration) => patchClip(clip.id!, { duration })}
          />
          <NumberValue
            label={`${clip.name} 速度`}
            value={clip.playback_rate!}
            min={0.1}
            max={4}
            disabled={disabled}
            onChange={(playback_rate) => patchClip(clip.id!, { playback_rate })}
          />
          <Label className="director-field">
            与前一动作混合
            <Checkbox
              checked={(clip.blend_in ?? 0) > 0}
              disabled={
                disabled ||
                !clips.some(
                  (item) =>
                    item.id !== clip.id &&
                    Math.abs(item.start! + item.duration! - clip.start!) < 1e-6,
                )
              }
              onCheckedChange={(enabled) => {
                const previous = clips.find(
                  (item) =>
                    item.id !== clip.id &&
                    Math.abs(item.start! + item.duration! - clip.start!) < 1e-6,
                );
                patchClip(clip.id!, {
                  blend_in:
                    enabled === true && previous
                      ? Math.min(0.3, clip.duration!, previous.duration!)
                      : 0,
                });
              }}
            />
          </Label>
          {(clip.blend_in ?? 0) > 0 && (
            <>
              <NumberValue
                label={`${clip.name} 混合时长`}
                value={clip.blend_in!}
                min={0.01}
                max={Math.min(
                  clip.duration!,
                  clips.find(
                    (item) =>
                      item.id !== clip.id &&
                      Math.abs(item.start! + item.duration! - clip.start!) <
                        1e-6,
                  )?.duration ?? 0,
                )}
                disabled={disabled}
                onChange={(blend_in) => patchClip(clip.id!, { blend_in })}
              />
              <Button
                size="sm"
                variant="ghost"
                disabled={disabled}
                onClick={() => patchClip(clip.id!, { blend_in: 0 })}
              >
                取消混合
              </Button>
            </>
          )}
          <Label className="director-field">
            循环
            <Checkbox
              checked={clip.loop}
              disabled={disabled}
              onCheckedChange={(loop) =>
                patchClip(clip.id!, { loop: loop === true })
              }
            />
          </Label>
          <Button
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() =>
              placeClip({
                ...clip,
                id: crypto.randomUUID(),
                start: clip.start! + clip.duration!,
              })
            }
          >
            复制动作
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={disabled}
            onClick={() =>
              onChange({
                motion_clips: clips.filter((item) => item.id !== clip.id),
                ...(object.active_motion_clip_id === clip.id
                  ? { active_motion_clip_id: undefined }
                  : {}),
              })
            }
          >
            删除动作
          </Button>
        </div>
      ))}
      {!!available?.bones.length && (
        <>
          <h3>骨骼姿势</h3>
          <Choice
            label="标准骨骼"
            value={bone}
            items={DIRECTOR_BONES.map((name) => [name, name])}
            disabled={disabled}
            onChange={setBone}
          />
          <Choice
            label="模型骨骼"
            value={mapped ?? "__none"}
            items={[
              ["__none", "未绑定"],
              ...available.bones.map((name): [string, string] => [name, name]),
            ]}
            disabled={disabled}
            onChange={(target) => {
              const map = { ...object.rig?.bone_map };
              if (target === "__none") delete map[bone];
              else map[bone] = target;
              onChange({
                rig: {
                  bone_map: map,
                  animation_names:
                    object.rig?.animation_names ??
                    available.animations.map((item) => item.name),
                },
              });
            }}
          />
          {mapped && (
            <>
              <p>
                {dynamic
                  ? `在 ${time.toFixed(2)} 秒记录骨骼关键帧`
                  : "静态骨骼姿势"}
              </p>
              {angles.map((angle, index) => (
                <NumberValue
                  key={index}
                  label={`骨骼旋转 ${["X", "Y", "Z"][index]}`}
                  value={Math.round(((angle * 180) / Math.PI) * 100) / 100}
                  min={-180}
                  max={180}
                  disabled={disabled}
                  onChange={(degrees) => changeBone(index, degrees)}
                />
              ))}
              <Button
                variant="ghost"
                size="sm"
                disabled={disabled}
                onClick={() => {
                  if (dynamic)
                    onChange({
                      bone_tracks: (object.bone_tracks ?? []).map((item) =>
                        item.bone === bone
                          ? {
                              ...item,
                              keyframes: item.keyframes?.filter(
                                (frame) =>
                                  Math.abs(frame.time! - time) >= 0.000001,
                              ),
                            }
                          : item,
                      ),
                    });
                  else {
                    const values = { ...object.bone_overrides };
                    delete values[bone];
                    onChange({ bone_overrides: values });
                  }
                }}
              >
                {dynamic ? "删除当前骨骼关键帧" : "恢复模型骨骼姿势"}
              </Button>
            </>
          )}
        </>
      )}
    </div>
  );
}
