"use client";
// Video crop interaction adapted from BeefTV 1ae25027 canvas-video-crop-dialog.tsx.
// MIT; source pointer document listeners are replaced by capture and bounded rAF updates.
import { useEffect, useRef, useState, type PointerEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { CanvasNodeData } from "./model";
import { getMediaPreview } from "./queries";
import { useReleaseMediaSource } from "./nodes/media-lifecycle";
import type { TimelineProject } from "./timeline";
import { captureVideoFrame, createVideoClipTimeline } from "./video-tools";
import {
  moveVideoCrop,
  resizeVideoCrop,
  normalizeVideoCropForEncoding,
  type VideoCropHandle,
  type VideoCropRect,
} from "./video-crop-geometry";
const handles: VideoCropHandle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];
export function VideoToolsDialog({
  node,
  projectId,
  remainingSlots,
  onClose,
  onFrame,
  onTimeline,
}: {
  node: CanvasNodeData;
  projectId: string;
  remainingSlots: number;
  onClose: () => void;
  onFrame: (files: File[]) => void;
  onTimeline: (value: TimelineProject) => Promise<boolean>;
}) {
  const preview = useQuery({
    queryKey: ["canvas", "video-tools", projectId, node.assetId],
    enabled: Boolean(node.assetId),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    queryFn: async ({ signal }) => {
      const value = await getMediaPreview(projectId, node.assetId!, signal),
        url = new URL(value.url);
      if (
        value.asset.id !== node.assetId ||
        value.asset.project_id !== projectId ||
        value.asset.kind !== "video" ||
        !["https:", "http:"].includes(url.protocol) ||
        url.username ||
        url.password ||
        !Number.isFinite(Date.parse(value.expires_at)) ||
        Date.parse(value.expires_at) <= Date.now() + 5000
      )
        throw new Error("视频身份或预览授权无效。");
      return value;
    },
  });
  const video = useRef<HTMLVideoElement>(null),
    active = useRef<AbortController | null>(null),
    dragging = useRef<{
      pointer: number;
      point: { x: number; y: number };
      original: VideoCropRect;
      handle?: VideoCropHandle;
      next: VideoCropRect;
    } | null>(null),
    frame = useRef<number | null>(null);
  const [crop, setCrop] = useState<VideoCropRect>(),
    [cropping, setCropping] = useState(false),
    [startMs, setStartMs] = useState(0),
    [endMs, setEndMs] = useState(0),
    [aspectRatio, setAspect] = useState<TimelineProject["aspectRatio"]>("16:9"),
    [running, setRunning] = useState(false),
    [error, setError] = useState("");
  useReleaseMediaSource(video, preview.data?.url ?? "");
  useEffect(
    () => () => {
      active.current?.abort();
      if (frame.current !== null) cancelAnimationFrame(frame.current);
    },
    [],
  );
  const source = preview.data?.asset;
  function validLease() {
    if (
      !preview.data ||
      Date.parse(preview.data.expires_at) <= Date.now() + 5000
    )
      throw new Error("视频授权已过期，请重新读取素材。");
  }
  async function capture() {
    if (!video.current || running) return;
    const controller = new AbortController();
    active.current = controller;
    setRunning(true);
    setError("");
    try {
      validLease();
      const file = await captureVideoFrame(
        video.current,
        cropping ? (crop ?? null) : null,
        node.title,
        controller.signal,
      );
      if (!controller.signal.aborted) onFrame([file]);
    } catch (failure) {
      if (!controller.signal.aborted)
        setError(failure instanceof Error ? failure.message : "截帧失败。");
    } finally {
      if (!controller.signal.aborted) setRunning(false);
      active.current = null;
    }
  }
  async function save() {
    if (!source || running) return;
    setRunning(true);
    setError("");
    try {
      validLease();
      const value = createVideoClipTimeline(node, source, projectId, {
        startMs,
        endMs,
        aspectRatio,
        crop: cropping ? (crop ?? null) : null,
      });
      if (await onTimeline(value)) onClose();
      else setError("时间线尚未确认保存，请修复画布错误后重试。");
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : "剪辑草稿保存失败。",
      );
    } finally {
      setRunning(false);
    }
  }
  function begin(event: PointerEvent<HTMLElement>, handle?: VideoCropHandle) {
    if (!crop || !cropping || running || event.button !== 0) return;
    try {
      normalizeVideoCropForEncoding(crop, {
        width: source?.width ?? 0,
        height: source?.height ?? 0,
      });
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "裁切选区无效。");
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    dragging.current = {
      pointer: event.pointerId,
      point: { x: event.clientX, y: event.clientY },
      original: crop,
      next: crop,
      handle,
    };
  }
  function move(event: PointerEvent<HTMLDivElement>) {
    const drag = dragging.current,
      bounds = event.currentTarget.getBoundingClientRect();
    if (
      !drag ||
      drag.pointer !== event.pointerId ||
      !source?.width ||
      !source.height
    )
      return;
    const dx = ((event.clientX - drag.point.x) * source.width) / bounds.width,
      dy = ((event.clientY - drag.point.y) * source.height) / bounds.height;
    drag.next = drag.handle
      ? resizeVideoCrop(
          drag.original,
          { width: source.width, height: source.height },
          drag.handle,
          dx,
          dy,
        )
      : moveVideoCrop(
          drag.original,
          { width: source.width, height: source.height },
          dx,
          dy,
        );
    if (frame.current === null)
      frame.current = requestAnimationFrame(() => {
        frame.current = null;
        if (dragging.current) setCrop(dragging.current.next);
      });
  }
  function stop(event: PointerEvent<HTMLDivElement>, cancelled = false) {
    const drag = dragging.current;
    if (!drag || drag.pointer !== event.pointerId) return;
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    dragging.current = null;
    setCrop(cancelled ? drag.original : drag.next);
    if (event.currentTarget.hasPointerCapture(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId);
  }
  return (
    <Dialog
      open
      onOpenChange={(value) => {
        if (!value) {
          active.current?.abort();
          onClose();
        }
      }}
    >
      <DialogContent className="flex max-h-[92dvh] flex-col overflow-y-auto sm:max-w-3xl [&>*]:shrink-0">
        <DialogHeader>
          <DialogTitle>视频工具 · {node.title}</DialogTitle>
          <DialogDescription>
            截取浏览器已解码的当前帧，或保存剪辑/空间裁切时间线，再由正式导出任务生成视频文件。
          </DialogDescription>
        </DialogHeader>
        {preview.data ? (
          <div
            className="relative mx-auto w-full max-w-xl overflow-hidden rounded-lg border bg-black"
            style={{
              aspectRatio:
                source?.width && source.height
                  ? source.width / source.height
                  : 16 / 9,
            }}
            onPointerMove={move}
            onPointerUp={(event) => stop(event)}
            onPointerCancel={(event) => stop(event, true)}
          >
            <video
              ref={video}
              src={preview.data.url}
              crossOrigin="anonymous"
              aria-label="视频工具原件"
              controls
              playsInline
              preload="metadata"
              className="block h-full w-full"
              onError={() => setError("视频原件无法播放。")}
              onLoadedMetadata={(event) => {
                const element = event.currentTarget;
                if (
                  source?.width !== element.videoWidth ||
                  source.height !== element.videoHeight ||
                  !source.duration_ms ||
                  !Number.isFinite(element.duration) ||
                  Math.abs(element.duration * 1000 - source.duration_ms) > 1000
                ) {
                  setError("视频解码尺寸/时长与正式素材不一致，不能创建剪辑。");
                  return;
                }
                setCrop({
                  x: 0,
                  y: 0,
                  width: element.videoWidth,
                  height: element.videoHeight,
                });
                setEndMs(source.duration_ms);
              }}
            />
            {cropping && crop && source?.width && source.height ? (
              <div
                className="absolute inset-0 touch-none"
                onPointerDown={(event) => begin(event)}
              >
                <div
                  className="absolute cursor-move border border-white shadow-[0_0_0_9999px_rgba(0,0,0,.45)]"
                  style={{
                    left: `${(crop.x / source.width) * 100}%`,
                    top: `${(crop.y / source.height) * 100}%`,
                    width: `${(crop.width / source.width) * 100}%`,
                    height: `${(crop.height / source.height) * 100}%`,
                  }}
                >
                  <span className="absolute top-1 left-1 bg-black/70 px-1 text-xs text-white">
                    {crop.width} × {crop.height}
                  </span>
                  {handles.map((handle) => (
                    <button
                      key={handle}
                      type="button"
                      aria-label={`调整视频裁切 ${handle}`}
                      className="absolute size-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white bg-black"
                      style={{
                        left: handle.includes("w")
                          ? "0%"
                          : handle.includes("e")
                            ? "100%"
                            : "50%",
                        top: handle.includes("n")
                          ? "0%"
                          : handle.includes("s")
                            ? "100%"
                            : "50%",
                      }}
                      onPointerDown={(event) => {
                        event.stopPropagation();
                        begin(event, handle);
                      }}
                      onKeyDown={(event) => {
                        const direction = {
                          ArrowLeft: [-1, 0],
                          ArrowRight: [1, 0],
                          ArrowUp: [0, -1],
                          ArrowDown: [0, 1],
                        }[event.key];
                        if (direction && !running) {
                          event.preventDefault();
                          try {
                            setCrop(
                              resizeVideoCrop(
                                crop,
                                {
                                  width: source.width!,
                                  height: source.height!,
                                },
                                handle,
                                direction[0] * (event.shiftKey ? 10 : 1),
                                direction[1] * (event.shiftKey ? 10 : 1),
                              ),
                            );
                          } catch {
                            setError("裁切选区无效，请先修正像素值。");
                          }
                        }
                      }}
                    />
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        ) : (
          <p role="status">正在读取视频…</p>
        )}
        {preview.error || error ? (
          <p role="alert" className="text-sm text-destructive">
            {error || preview.error?.message}
            <Button
              variant="ghost"
              onClick={() => {
                setError("");
                void preview.refetch();
              }}
            >
              重新读取视频
            </Button>
          </p>
        ) : null}
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={cropping}
            disabled={running || !crop}
            onCheckedChange={(value) => setCropping(value === true)}
          />
          使用空间裁切选区
        </label>
        {cropping && crop ? (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {(["x", "y", "width", "height"] as const).map((key) => (
              <Field key={key}>
                <FieldLabel htmlFor={`video-crop-${key}`}>
                  {
                    { x: "左边距", y: "上边距", width: "宽度", height: "高度" }[
                      key
                    ]
                  }
                  （像素）
                </FieldLabel>
                <Input
                  id={`video-crop-${key}`}
                  type="number"
                  min={key === "x" || key === "y" ? 0 : 2}
                  max={32768}
                  step={2}
                  value={crop[key]}
                  disabled={running}
                  onChange={(event) =>
                    setCrop({ ...crop, [key]: Number(event.target.value) })
                  }
                />
              </Field>
            ))}
          </div>
        ) : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="video-trim-start">剪辑起点（毫秒）</FieldLabel>
            <Input
              id="video-trim-start"
              type="number"
              min={0}
              max={source?.duration_ms}
              value={startMs}
              disabled={running}
              onChange={(event) => setStartMs(Number(event.target.value))}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="video-trim-end">剪辑终点（毫秒）</FieldLabel>
            <Input
              id="video-trim-end"
              type="number"
              min={100}
              max={source?.duration_ms}
              value={endMs}
              disabled={running}
              onChange={(event) => setEndMs(Number(event.target.value))}
            />
          </Field>
        </div>
        <Select
          value={aspectRatio}
          onValueChange={(value) =>
            setAspect(value as TimelineProject["aspectRatio"])
          }
          disabled={running}
        >
          <SelectTrigger aria-label="导出画幅">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {["16:9", "9:16", "1:1"].map((ratio) => (
              <SelectItem key={ratio} value={ratio}>
                {ratio}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          裁切编码时按偶数像素对齐。按所选画幅适配画面；视频文件需要时间线中创建实际导出任务。
        </p>
        <DialogFooter>
          <Button variant="outline" disabled={running} onClick={onClose}>
            关闭
          </Button>
          <Button
            variant="outline"
            disabled={running || !crop || remainingSlots < 1}
            onClick={() => void capture()}
          >
            截取当前帧
          </Button>
          <Button
            disabled={running || !crop || remainingSlots < 1}
            onClick={() => void save()}
          >
            {running ? "处理中…" : "保存剪辑与裁切时间线"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
