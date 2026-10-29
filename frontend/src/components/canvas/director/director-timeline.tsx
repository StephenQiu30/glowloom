"use client";
import {
  DirectorTimelineKey,
  DirectorTimelineClip,
} from "./director-timeline-drag";
import { placeDirectorMotionClip } from "./director-motion-model";
import { useMemo, useState } from "react";
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
  readDirector,
  directorFingerprint,
  type Director,
  type DirectorFrame,
  type DirectorSelection,
} from "./director-model";
export const DIRECTOR_CHANNELS = [
  ["position", "位置"],
  ["rotation", "旋转"],
  ["scale", "缩放"],
  ["target", "注视点"],
  ["fov", "视野"],
  ["roll", "倾斜"],
] as const;
type Channel = (typeof DIRECTOR_CHANNELS)[number][0];
export function directorOwnsChannel(frame: DirectorFrame, channel: Channel) {
  return channel === "position" || channel === "rotation" || channel === "scale"
    ? frame[`${channel}_keyed`] !== false
    : frame[channel] !== undefined;
}
export function editDirectorChannel(
  frames: DirectorFrame[],
  id: string,
  channel: Channel,
  patch: { time?: number; easing?: string; remove?: boolean },
): DirectorFrame[] {
  const next = structuredClone(frames),
    source = next.find((frame) => frame.id === id);
  if (!source || !directorOwnsChannel(source, channel))
    throw new Error("关键帧已不存在");
  const time = patch.time ?? source.time;
  if (!Number.isFinite(time) || time < 0 || time > 3600)
    throw new Error("关键帧时间无效");
  if (
    !patch.remove &&
    next.some(
      (frame) =>
        frame !== source &&
        Math.abs(frame.time - time) < 1e-6 &&
        directorOwnsChannel(frame, channel),
    )
  )
    throw new Error("该轨道此时已有关键帧");
  const value = structuredClone(source);
  if (channel === "position" || channel === "rotation" || channel === "scale")
    source[`${channel}_keyed`] = false;
  else delete source[channel];
  if (!patch.remove) {
    const easing = patch.easing ?? value.easing;
    let destination = next.find(
      (frame) => Math.abs(frame.time - time) < 1e-6 && frame.easing === easing,
    );
    if (!destination) {
      destination = {
        id: crypto.randomUUID(),
        time,
        easing,
        transform: structuredClone(value.transform),
        position_keyed: false,
        rotation_keyed: false,
        scale_keyed: false,
      };
      next.push(destination);
    }
    if (
      channel === "position" ||
      channel === "rotation" ||
      channel === "scale"
    ) {
      destination.transform[channel] = value.transform[channel];
      destination[`${channel}_keyed`] = true;
    } else if (channel === "target") destination.target = value.target;
    else destination[channel] = value[channel];
    destination.easing = easing;
  }
  return next
    .filter((frame) =>
      DIRECTOR_CHANNELS.some(([channel]) =>
        directorOwnsChannel(frame, channel),
      ),
    )
    .sort((a, b) => a.time - b.time);
}
export function DirectorTimeline({
  scene,
  time,
  duration,
  fps,
  disabled,
  onTime,
  onChange,
  onSelect,
}: {
  scene: Director;
  time: number;
  duration: number;
  fps: number;
  disabled: boolean;
  onTime: (time: number) => void;
  onChange: (scene: Director) => void;
  onSelect: (selection: DirectorSelection) => void;
}) {
  const [zoom, setZoom] = useState(1),
    [collapsed, setCollapsed] = useState(false),
    [hideEmpty, setHideEmpty] = useState(false),
    [key, setKey] = useState<{
      selection: DirectorSelection;
      id: string;
      channel: Channel;
    } | null>(null),
    [error, setError] = useState("");
  const baseline = useMemo(
    () => `${directorFingerprint(scene)}:${duration}:${fps}`,
    [scene, duration, fps],
  );
  const tracks = [
    ...scene.objects.map((entity) => ({ entity, kind: "object" as const })),
    ...scene.cameras.map((entity) => ({ entity, kind: "camera" as const })),
  ];
  const selected = key
      ? tracks.find((track) => track.entity.id === key.selection.id)?.entity
      : undefined,
    frame = selected?.keyframes.find((frame) => frame.id === key?.id);
  function change(patch: { time?: number; easing?: string; remove?: boolean }) {
    if (!key || !selected || selected.locked || disabled) return;
    try {
      if (patch.time !== undefined && (patch.time < 0 || patch.time > duration))
        throw new Error("关键帧超出当前场景时间范围");
      const next = readDirector(scene),
        entity = (
          key.selection.kind === "object" ? next.objects : next.cameras
        ).find((entity) => entity.id === key.selection.id)!;
      entity.keyframes = editDirectorChannel(
        entity.keyframes,
        key.id,
        key.channel,
        patch,
      );
      onChange(readDirector(next));
      setError("");
      if (
        patch.remove ||
        patch.time !== undefined ||
        patch.easing !== undefined
      )
        setKey(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "关键帧修改失败");
    }
  }
  return (
    <div className="director-track-editor">
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setCollapsed(!collapsed)}
        >
          {collapsed ? "展开轨道" : "折叠轨道"}
        </Button>
        <Label>
          <Checkbox
            checked={hideEmpty}
            onCheckedChange={(value) => setHideEmpty(value === true)}
          />
          隐藏空轨道
        </Label>
        <Button
          variant="ghost"
          size="sm"
          disabled={zoom <= 1}
          onClick={() => setZoom(Math.max(1, zoom / 2))}
        >
          −
        </Button>
        <span>{zoom * 100}%</span>
        <Button
          variant="ghost"
          size="sm"
          disabled={zoom >= 8}
          onClick={() => setZoom(Math.min(8, zoom * 2))}
        >
          +
        </Button>
        <span>
          {Math.round(time * fps)} / {Math.round(duration * fps)} 帧
        </span>
      </div>
      {!collapsed && (
        <div className="director-track-scroll">
          <div style={{ minWidth: `${zoom * 100}%` }}>
            {tracks.map(({ entity, kind }) => (
              <div key={entity.id}>
                {DIRECTOR_CHANNELS.filter(
                  ([channel]) =>
                    kind === "camera" ||
                    !["target", "fov", "roll"].includes(channel),
                )
                  .filter(
                    ([channel]) =>
                      !hideEmpty ||
                      entity.keyframes.some((frame) =>
                        directorOwnsChannel(frame, channel),
                      ),
                  )
                  .map(([channel, label]) => (
                    <div key={channel} className="director-track-row">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="director-track-label"
                        onClick={() => onSelect({ kind, id: entity.id })}
                      >
                        {entity.name} · {label}
                      </Button>
                      <div className="director-track-lane">
                        <span
                          className="director-playhead"
                          style={{
                            left: `${Math.min(100, (time / duration) * 100)}%`,
                          }}
                        />
                        {entity.keyframes
                          .filter((frame) =>
                            directorOwnsChannel(frame, channel),
                          )
                          .map((frame) => (
                            <DirectorTimelineKey
                              key={frame.id}
                              time={frame.time}
                              label={`${entity.name} ${label} ${frame.time.toFixed(2)} 秒关键帧`}
                              baseline={baseline}
                              duration={duration}
                              fps={fps}
                              disabled={disabled || entity.locked === true}
                              onError={setError}
                              validate={(time) => {
                                editDirectorChannel(
                                  entity.keyframes,
                                  frame.id,
                                  channel,
                                  { time },
                                );
                              }}
                              onCommit={(time) => {
                                const next = readDirector(scene);
                                const target = (
                                  kind === "object"
                                    ? next.objects
                                    : next.cameras
                                ).find((item) => item.id === entity.id)!;
                                target.keyframes = editDirectorChannel(
                                  target.keyframes,
                                  frame.id,
                                  channel,
                                  { time },
                                );
                                onChange(readDirector(next));
                                setKey(null);
                              }}
                              onSelect={() => {
                                setKey({
                                  selection: { kind, id: entity.id },
                                  id: frame.id,
                                  channel,
                                });
                                onTime(frame.time);
                                onSelect({ kind, id: entity.id });
                              }}
                            />
                          ))}
                      </div>
                    </div>
                  ))}
                {kind === "object" &&
                  "motion_clips" in entity &&
                  entity.motion_clips?.map((clip) => (
                    <div key={clip.id} className="director-track-row">
                      <span className="director-track-label">
                        {entity.name} · 动作
                      </span>
                      <div className="director-track-lane">
                        <DirectorTimelineClip
                          clip={clip}
                          baseline={baseline}
                          duration={duration}
                          fps={fps}
                          disabled={disabled || entity.locked === true}
                          onError={setError}
                          validate={(candidate) => {
                            placeDirectorMotionClip(
                              entity.motion_clips ?? [],
                              candidate,
                              duration,
                            );
                          }}
                          onCommit={(candidate) => {
                            const next = readDirector(scene),
                              target = next.objects.find(
                                (item) => item.id === entity.id,
                              )!;
                            target.motion_clips = placeDirectorMotionClip(
                              target.motion_clips ?? [],
                              candidate,
                              duration,
                            );
                            onChange(readDirector(next));
                          }}
                          onSelect={() => {
                            onTime(clip.start!);
                            onSelect({ kind, id: entity.id });
                          }}
                        />
                      </div>
                    </div>
                  ))}
              </div>
            ))}
          </div>
        </div>
      )}
      {frame && key && (
        <div className="flex items-center gap-3">
          <Label>
            关键帧时间
            <Input
              key={`${frame.id}:${frame.time}`}
              type="number"
              min={0}
              max={duration}
              step={1 / fps}
              defaultValue={frame.time}
              disabled={disabled || selected?.locked}
              onBlur={(event) => {
                if (event.target.value.trim())
                  change({
                    time: Math.round(Number(event.target.value) * fps) / fps,
                  });
              }}
            />
          </Label>
          <Label>
            插值
            <Select
              value={frame.easing || "linear"}
              disabled={disabled || selected?.locked}
              onValueChange={(easing) => change({ easing })}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[
                  ["linear", "线性"],
                  ["smooth", "平滑"],
                  ["step", "保持"],
                ].map(([value, label]) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Label>
          <Button
            variant="ghost"
            disabled={disabled || selected?.locked}
            onClick={() => change({ remove: true })}
          >
            删除此通道关键帧
          </Button>
        </div>
      )}
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
