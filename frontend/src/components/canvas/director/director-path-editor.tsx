"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  actorPathFrames,
  customCameraPathFrames,
  type CustomCameraMotion,
  cameraMotionFrames,
  mergeDirectorPath,
  CAMERA_MOTIONS,
  type CameraMotion,
} from "./director-paths";
import {
  sampleDirectorCamera,
  sampleDirectorTransform,
} from "./director-animation";
import {
  readDirector,
  type Director,
  type DirectorSelection,
} from "./director-model";
export function DirectorPathEditor({
  scene,
  selection,
  time,
  disabled,
  onPreview,
  onApply,
  onClose,
}: {
  scene: Director;
  selection: DirectorSelection;
  time: number;
  disabled: boolean;
  onPreview: (scene: Director, time: number) => void;
  onApply: (scene: Director) => boolean | undefined;
  onClose: () => void;
}) {
  const [cameraPoints, setCameraPoints] = useState<{ x: number; z: number }[]>(
    [],
  );
  const [targetPoints, setTargetPoints] = useState<{ x: number; z: number }[]>(
    [],
  );
  const [route, setRoute] = useState<"camera" | "target">("target");
  const [cameraMode, setCameraMode] = useState<"preset" | CustomCameraMotion>(
    "preset",
  );
  const useTarget =
    selection.kind === "camera" &&
    (cameraMode === "fixed-camera" ||
      (cameraMode === "free" && route === "target"));
  const points = useTarget ? targetPoints : cameraPoints;
  const setPoints = useTarget ? setTargetPoints : setCameraPoints;
  const [start, setStart] = useState(time),
    [duration, setDuration] = useState(4),
    [facing, setFacing] = useState(true),
    [replace, setReplace] = useState(false),
    [kind, setKind] = useState<CameraMotion>("push_in"),
    [error, setError] = useState("");
  function build() {
    const next = readDirector(scene),
      entity = (selection.kind === "object" ? next.objects : next.cameras).find(
        (item) => item.id === selection.id,
      );
    if (!entity || entity.locked || disabled)
      throw new Error("对象当前不可编辑");
    const frames =
      selection.kind === "camera"
        ? cameraMode === "preset"
          ? cameraMotionFrames(
              {
                ...next.cameras.find((item) => item.id === selection.id)!,
                ...sampleDirectorCamera(
                  next.cameras.find((item) => item.id === selection.id)!,
                  start,
                ),
              },
              kind,
              start,
              duration,
            )
          : customCameraPathFrames(
              {
                ...next.cameras.find((item) => item.id === selection.id)!,
                ...sampleDirectorCamera(
                  next.cameras.find((item) => item.id === selection.id)!,
                  start,
                ),
              },
              cameraMode,
              cameraPoints,
              targetPoints,
              start,
              duration,
            )
        : actorPathFrames(
            sampleDirectorTransform(entity.transform, entity.keyframes, start),
            points,
            start,
            duration,
            facing,
          );
    entity.keyframes = mergeDirectorPath(entity.keyframes, frames, replace);
    if (selection.kind === "camera") {
      const camera = next.cameras.find((item) => item.id === selection.id)!;
      camera.look_at_mode = "coordinates";
      delete camera.look_at_object_id;
      delete camera.follow_object_id;
    }
    const shot = next.shots.find((item) => item.id === next.active_shot_id)!;
    shot.duration = Math.max(shot.duration, start + duration);
    return readDirector(next);
  }
  function run(apply: boolean) {
    try {
      const next = build();
      setError("");
      if (apply) {
        if (onApply(next)) onClose();
      } else onPreview(next, start);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "路径无效");
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="sm:left-8 sm:max-w-sm sm:translate-x-0"
        data-canvas-no-zoom
      >
        <DialogTitle>
          {selection.kind === "camera" ? "添加运镜" : "绘制移动路径"}
        </DialogTitle>
        <DialogDescription>
          预览后确认，将关键帧写入当前动态工作区。
        </DialogDescription>
        {selection.kind === "camera" && (
          <>
            <Select
              value={cameraMode}
              onValueChange={(value) =>
                setCameraMode(value as typeof cameraMode)
              }
            >
              <SelectTrigger aria-label="运镜方式">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="preset">预设运镜</SelectItem>
                <SelectItem value="fixed-target">
                  自定义 · 固定注视点
                </SelectItem>
                <SelectItem value="fixed-camera">自定义 · 固定机位</SelectItem>
                <SelectItem value="free">自定义 · 自由运镜</SelectItem>
              </SelectContent>
            </Select>
            {cameraMode === "free" && (
              <div className="flex gap-2">
                <Button
                  variant={route === "target" ? "secondary" : "outline"}
                  onClick={() => setRoute("target")}
                >
                  1. 注视点路径
                </Button>
                <Button
                  variant={route === "camera" ? "secondary" : "outline"}
                  disabled={targetPoints.length < 2}
                  onClick={() => setRoute("camera")}
                >
                  2. 机位路径
                </Button>
              </div>
            )}
          </>
        )}
        {selection.kind === "camera" && cameraMode === "preset" ? (
          <Select
            value={kind}
            onValueChange={(value) => setKind(value as CameraMotion)}
          >
            <SelectTrigger aria-label="运镜预设">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CAMERA_MOTIONS.map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <>
            <p className="text-xs text-muted-foreground">
              {useTarget
                ? "注视点"
                : selection.kind === "camera"
                  ? "机位"
                  : "对象"}
              俯视路径：点击添加节点，每格 1 米；起点对齐当前位置。
            </p>
            <svg
              viewBox="0 0 300 200"
              className="w-full rounded border bg-muted/30"
              role="button"
              tabIndex={0}
              aria-label="绘制地面路径"
              onKeyDown={(event) => {
                if (event.key === "Enter")
                  setPoints([...points, { x: points.length, z: 0 }]);
              }}
              onClick={(event) => {
                const box = event.currentTarget.getBoundingClientRect();
                setPoints([
                  ...points,
                  {
                    x:
                      (((event.clientX - box.left) / box.width) * 300 - 150) /
                      25,
                    z:
                      (((event.clientY - box.top) / box.height) * 200 - 100) /
                      25,
                  },
                ]);
              }}
            >
              {Array.from({ length: 13 }, (_, i) => (
                <line
                  key={`x${i}`}
                  x1={i * 25}
                  x2={i * 25}
                  y1={0}
                  y2={200}
                  stroke="currentColor"
                  opacity=".1"
                />
              ))}
              {Array.from({ length: 9 }, (_, i) => (
                <line
                  key={`z${i}`}
                  x1={0}
                  x2={300}
                  y1={i * 25}
                  y2={i * 25}
                  stroke="currentColor"
                  opacity=".1"
                />
              ))}
              <polyline
                points={points
                  .map((p) => `${p.x * 25 + 150},${p.z * 25 + 100}`)
                  .join(" ")}
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
              />
              {points.map((p, i) => (
                <circle
                  key={i}
                  cx={p.x * 25 + 150}
                  cy={p.z * 25 + 100}
                  r={3}
                  fill="currentColor"
                />
              ))}
            </svg>
            <div className="flex gap-2">
              <Button
                variant="outline"
                onClick={() => setPoints(points.slice(0, -1))}
              >
                撤销节点
              </Button>
              <Button variant="ghost" onClick={() => setPoints([])}>
                清空
              </Button>
            </div>
            {selection.kind === "object" && (
              <Label>
                <Checkbox
                  checked={facing}
                  onCheckedChange={(value) => setFacing(value === true)}
                />
                沿路径转向
              </Label>
            )}
          </>
        )}
        <Label>
          开始时间（秒）
          <Input
            type="number"
            min={0}
            max={3600}
            value={start}
            onChange={(event) => setStart(Number(event.target.value))}
          />
        </Label>
        <Label>
          持续时间（秒）
          <Input
            type="number"
            min={0.1}
            max={3600}
            step={0.1}
            value={duration}
            onChange={(event) => setDuration(Number(event.target.value))}
          />
        </Label>
        <Label>
          <Checkbox
            checked={replace}
            onCheckedChange={(value) => setReplace(value === true)}
          />
          替换此范围内的相关关键帧
        </Label>
        {error && (
          <p role="alert" className="text-destructive">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button
            variant="outline"
            disabled={disabled}
            onClick={() => run(false)}
          >
            预览
          </Button>
          <Button disabled={disabled} onClick={() => run(true)}>
            确认
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
