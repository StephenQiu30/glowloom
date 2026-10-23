/* eslint-disable @next/next/no-img-element -- Source media previews for local canvas processing. */
// Attribution and source revision are recorded in /licenses/canvas-sources.md.
// See /licenses/canvas-sources.md.
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Check, X, ZoomIn, ZoomOut } from "lucide-react";
import {
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";

import { useImageEditorViewport } from "./use-image-editor-viewport";
import { readImageMeta } from "./image-meta";

export type CanvasImageCropRect = {
  x: number;
  y: number;
  width: number;
  height: number;
};

type DragMode = "move" | "resize";
type ResizeHandle = "n" | "e" | "s" | "w" | "ne" | "nw" | "se" | "sw";

const handles: ResizeHandle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];
const minSize = 0.06;
const defaultCrop = { x: 0.12, y: 0.12, width: 0.76, height: 0.76 };
export function CanvasNodeCropDialog({
  dataUrl,
  open,
  onClose,
  onConfirm,
  busy = false,
  error,
}: {
  dataUrl: string;
  open: boolean;
  busy?: boolean;
  error?: string;
  onClose: () => void;
  onConfirm: (crop: CanvasImageCropRect) => void;
}) {
  const [crop, setCrop] = useState<CanvasImageCropRect>(defaultCrop);
  const [ratioPreset, setRatioPreset] = useState("free");
  const [fixedRatio, setFixedRatio] = useState<number | null>(null);
  const [image, setImage] = useState<{ width: number; height: number } | null>(
    null,
  );
  const dragAbortRef = useRef<AbortController | null>(null);
  const {
    stageRef: editorStageRef,
    viewportRef: editorViewportRef,
    panHandlers: editorPanHandlers,
    scrollClassName: editorScrollClassName,
    isPanning: editorIsPanning,
    spacePressed: editorSpacePressed,
    contentStyle: editorContentStyle,
    stageStyle: editorStageStyle,
    mediaStyle: editorMediaStyle,
    zoomOut: editorZoomOut,
    canZoomOut: editorCanZoomOut,
    resetZoom: editorResetZoom,
    zoom: editorZoom,
    zoomIn: editorZoomIn,
    canZoomIn: editorCanZoomIn,
  } = useImageEditorViewport(image, open);
  const boxRef = editorStageRef;
  const cropSize = image
    ? {
        width: Math.max(1, Math.round(crop.width * image.width)),
        height: Math.max(1, Math.round(crop.height * image.height)),
      }
    : null;

  useEffect(() => {
    if (open) {
      // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setCrop(defaultCrop);
      setRatioPreset("free");
      setFixedRatio(null);
    }
  }, [dataUrl, open]);

  useEffect(() => {
    if (!open) return;
    let active = true;
    void readImageMeta(dataUrl)
      .then((value) => {
        if (active) setImage(value);
      })
      .catch(() => {
        if (active) setImage(null);
      });
    return () => {
      active = false;
    };
  }, [dataUrl, open]);

  useEffect(() => {
    if (!open) dragAbortRef.current?.abort();
    return () => dragAbortRef.current?.abort();
  }, [open]);

  const startDrag = (
    mode: DragMode,
    event: ReactPointerEvent,
    handle?: ResizeHandle,
  ) => {
    const box = boxRef.current?.getBoundingClientRect();
    if (!box) return;
    event.preventDefault();
    event.stopPropagation();
    dragAbortRef.current?.abort();
    const controller = new AbortController();
    dragAbortRef.current = controller;
    const start = { x: event.clientX, y: event.clientY, crop };
    const move = (event: PointerEvent) => {
      const dx = (event.clientX - start.x) / box.width;
      const dy = (event.clientY - start.y) / box.height;
      setCrop(
        mode === "move"
          ? moveCrop(start.crop, dx, dy)
          : resizeCrop(
              start.crop,
              dx,
              dy,
              handle || "se",
              resolveRatio(ratioPreset, image, fixedRatio),
              box,
            ),
      );
    };
    const stop = () => controller.abort();
    document.addEventListener("pointermove", move, {
      signal: controller.signal,
    });
    document.addEventListener("pointerup", stop, { signal: controller.signal });
    document.addEventListener("pointercancel", stop, {
      signal: controller.signal,
    });
  };

  return (
    <Dialog
      open={open && Boolean(dataUrl)}
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault();
        }}
        onPointerDownOutside={(event) => {
          if (busy) event.preventDefault();
        }}
        className={
          "max-h-[calc(100dvh-2rem)] w-full overflow-y-auto sm:max-w-5xl"
        }
      >
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {busy && <p role="status">正在处理图片…</p>}
        <DialogHeader>
          <DialogTitle>{"裁剪图片"}</DialogTitle>
          <DialogDescription className="sr-only">
            调整裁剪范围，确认后创建新的图片节点，保留原图。
          </DialogDescription>
        </DialogHeader>
        <div>
          <div className="flex flex-col gap-4">
            <div
              ref={editorViewportRef}
              {...editorPanHandlers}
              className={cn(
                "relative h-[min(55dvh,32rem)] min-h-0 min-h-48 rounded-lg bg-black/5",
                editorScrollClassName,
                editorIsPanning
                  ? "cursor-grabbing"
                  : editorSpacePressed && "cursor-grab",
              )}
            >
              <div className="relative" style={editorContentStyle}>
                <div
                  ref={boxRef}
                  className="absolute isolate [transform:translateZ(0)] overflow-hidden rounded-lg bg-black [contain:layout_paint] select-none [backface-visibility:hidden]"
                  style={editorStageStyle}
                >
                  <div
                    className="absolute top-0 left-0 [backface-visibility:hidden]"
                    style={editorMediaStyle}
                  >
                    <img
                      src={dataUrl}
                      alt=""
                      className="block h-full w-full object-contain opacity-90"
                      draggable={false}
                    />
                  </div>
                  <CropMask crop={crop} />
                  <div
                    className="absolute cursor-move border-2 border-white shadow-[0_0_0_1px_var(--border),0_0_28px_transparent]"
                    style={cropStyle(crop)}
                    onPointerDown={(event) => startDrag("move", event)}
                  >
                    <div className="pointer-events-none absolute inset-x-0 top-1/3 border-t border-white/50" />
                    <div className="pointer-events-none absolute inset-x-0 top-2/3 border-t border-white/50" />
                    <div className="pointer-events-none absolute inset-y-0 left-1/3 border-l border-white/50" />
                    <div className="pointer-events-none absolute inset-y-0 left-2/3 border-l border-white/50" />
                    {handles.map((handle) => (
                      <Button
                        variant="ghost"
                        key={handle}
                        type="button"
                        className="absolute size-3 rounded-full border border-black bg-white"
                        style={handleStyle(handle)}
                        onPointerDown={(event) =>
                          startDrag("resize", event, handle)
                        }
                        aria-label={"调整裁剪框"}
                      />
                    ))}
                  </div>
                </div>
              </div>
            </div>

            <div className="flex items-center justify-center gap-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    aria-label={"缩小"}
                    onClick={editorZoomOut}
                    type={"button"}
                    variant={"ghost"}
                    size="icon"
                    disabled={!editorCanZoomOut}
                  >
                    {<ZoomOut data-icon="inline-start" />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top">{"缩小"}</TooltipContent>
              </Tooltip>
              <Button
                variant="ghost"
                type="button"
                className="min-w-14 tabular-nums opacity-70"
                onClick={editorResetZoom}
              >
                {Math.round(editorZoom * 100)}%
              </Button>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    aria-label={"放大"}
                    onClick={editorZoomIn}
                    type={"button"}
                    variant={"ghost"}
                    size="icon"
                    disabled={!editorCanZoomIn}
                  >
                    {<ZoomIn data-icon="inline-start" />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top">{"放大"}</TooltipContent>
              </Tooltip>
              <span className="ml-2 text-xs opacity-55">
                {"滚轮缩放 · 中键或空格+左键拖动画面"}
              </span>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2">
              <div className="flex flex-wrap items-center gap-3 text-sm opacity-80">
                <span>{`裁剪尺寸 ${cropSize ? `${cropSize.width} x ${cropSize.height}` : "未知"}`}</span>
                <span>{`比例 ${cropSize ? formatRatio(cropSize.width, cropSize.height) : "未知"}`}</span>
                {image ? (
                  <span>{`原图 ${image.width} x ${image.height}`}</span>
                ) : null}
              </div>
              <ToggleGroup
                type="single"
                variant="outline"
                className="w-full flex-wrap"
                aria-label="裁剪比例"
                value={String(ratioPreset)}
                size="sm"
                onValueChange={(value) => {
                  if (value)
                    ((value) => {
                      const preset = String(value);
                      setRatioPreset(preset);
                      const currentRatio = image
                        ? (crop.width * image.width) /
                          Math.max(1, crop.height * image.height)
                        : null;
                      const nextFixedRatio =
                        preset === "fixed" ? currentRatio : null;
                      setFixedRatio(nextFixedRatio);
                      const ratio = resolveRatio(preset, image, nextFixedRatio);
                      if (ratio && image)
                        setCrop((current) =>
                          fitCropToRatio(current, ratio, image),
                        );
                    })(value);
                }}
              >
                {[
                  { label: "自由", value: "free" },
                  { label: "固定", value: "fixed" },
                  { label: "原图", value: "original" },
                  ...["1:1", "4:3", "16:9", "9:16"],
                ].map((item) => {
                  const option =
                    typeof item === "object"
                      ? item
                      : { value: item, label: item };
                  return (
                    <ToggleGroupItem
                      key={String(option.value)}
                      value={String(option.value)}
                    >
                      {option.label}
                    </ToggleGroupItem>
                  );
                })}
              </ToggleGroup>
            </div>

            <div className="flex items-center justify-end gap-2">
              <Button
                onClick={() => setCrop(defaultCrop)}
                type={"button"}
                variant={"secondary"}
                size="default"
              >
                {"重置"}
              </Button>
              <Button
                disabled={busy}
                onClick={onClose}
                type={"button"}
                variant={"secondary"}
                size="default"
              >
                {<X data-icon="inline-start" />}
                {"取消"}
              </Button>
              <Button
                disabled={busy || !image}
                onClick={() => onConfirm(crop)}
                type={"button"}
                variant={"default"}
                size="default"
              >
                {<Check data-icon="inline-start" />}
                {"确认裁剪"}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function CropMask({ crop }: { crop: CanvasImageCropRect }) {
  return (
    <>
      <div
        className="absolute inset-x-0 top-0 bg-black/55"
        style={{ height: `${crop.y * 100}%` }}
      />
      <div
        className="absolute inset-x-0 bottom-0 bg-black/55"
        style={{ height: `${(1 - crop.y - crop.height) * 100}%` }}
      />
      <div
        className="absolute bg-black/55"
        style={{
          left: 0,
          top: `${crop.y * 100}%`,
          width: `${crop.x * 100}%`,
          height: `${crop.height * 100}%`,
        }}
      />
      <div
        className="absolute bg-black/55"
        style={{
          right: 0,
          top: `${crop.y * 100}%`,
          width: `${(1 - crop.x - crop.width) * 100}%`,
          height: `${crop.height * 100}%`,
        }}
      />
    </>
  );
}

function moveCrop(
  crop: CanvasImageCropRect,
  dx: number,
  dy: number,
): CanvasImageCropRect {
  return {
    ...crop,
    x: clamp(crop.x + dx, 0, 1 - crop.width),
    y: clamp(crop.y + dy, 0, 1 - crop.height),
  };
}

function resizeCrop(
  crop: CanvasImageCropRect,
  dx: number,
  dy: number,
  handle: ResizeHandle,
  aspectRatio: number | null,
  box: DOMRect,
): CanvasImageCropRect {
  const next = { ...crop };
  if (handle.includes("e")) next.width = crop.width + dx;
  if (handle.includes("s")) next.height = crop.height + dy;
  if (handle.includes("w")) {
    next.x = crop.x + dx;
    next.width = crop.width - dx;
  }
  if (handle.includes("n")) {
    next.y = crop.y + dy;
    next.height = crop.height - dy;
  }
  if (aspectRatio) {
    const normalizedRatio = aspectRatio * (box.height / box.width);
    const horizontalOnly =
      (handle.includes("e") || handle.includes("w")) &&
      !handle.includes("n") &&
      !handle.includes("s");
    const useWidth =
      horizontalOnly ||
      (handle.length > 1 &&
        Math.abs(dx * box.width) >= Math.abs(dy * box.height));
    if (useWidth) next.height = next.width / normalizedRatio;
    else next.width = next.height * normalizedRatio;
    if (handle.includes("w")) next.x = crop.x + crop.width - next.width;
    if (handle.includes("n")) next.y = crop.y + crop.height - next.height;
  }
  if (aspectRatio) {
    const normalizedRatio = aspectRatio * (box.height / box.width);
    const scaleDown = Math.min(
      1,
      1 / Math.max(next.width, 0.001),
      1 / Math.max(next.height, 0.001),
    );
    next.width *= scaleDown;
    next.height *= scaleDown;
    if (next.width < minSize || next.height < minSize) {
      const minimumScale = Math.max(
        minSize / Math.max(next.width, 0.001),
        minSize / Math.max(next.height, 0.001),
      );
      next.width *= minimumScale;
      next.height *= minimumScale;
    }
    next.width = Math.min(next.width, next.height * normalizedRatio);
    next.height = next.width / normalizedRatio;
  } else {
    next.width = clamp(next.width, minSize, 1);
    next.height = clamp(next.height, minSize, 1);
  }
  next.x = clamp(next.x, 0, 1 - next.width);
  next.y = clamp(next.y, 0, 1 - next.height);
  return next;
}

function resolveRatio(
  preset: string,
  image: { width: number; height: number } | null,
  fixedRatio: number | null,
) {
  if (preset === "free" || !image) return null;
  if (preset === "fixed") return fixedRatio;
  if (preset === "original") return image.width / image.height;
  const [width, height] = preset.split(":").map(Number);
  return width > 0 && height > 0 ? width / height : null;
}

function fitCropToRatio(
  crop: CanvasImageCropRect,
  ratio: number,
  image: { width: number; height: number },
): CanvasImageCropRect {
  const normalizedRatio = ratio * (image.height / image.width);
  let width = crop.width;
  let height = width / normalizedRatio;
  if (height > crop.height) {
    height = crop.height;
    width = height * normalizedRatio;
  }
  if (width > 1) {
    width = 1;
    height = width / normalizedRatio;
  }
  if (height > 1) {
    height = 1;
    width = height * normalizedRatio;
  }
  return {
    x: clamp(crop.x + (crop.width - width) / 2, 0, 1 - width),
    y: clamp(crop.y + (crop.height - height) / 2, 0, 1 - height),
    width,
    height,
  };
}

function cropStyle(crop: CanvasImageCropRect) {
  return {
    left: `${crop.x * 100}%`,
    top: `${crop.y * 100}%`,
    width: `${crop.width * 100}%`,
    height: `${crop.height * 100}%`,
  };
}

function handleStyle(handle: ResizeHandle) {
  const top = handle.includes("n")
    ? "-6px"
    : handle.includes("s")
      ? "calc(100% - 6px)"
      : "calc(50% - 6px)";
  const left = handle.includes("w")
    ? "-6px"
    : handle.includes("e")
      ? "calc(100% - 6px)"
      : "calc(50% - 6px)";
  return { top, left, cursor: `${handle}-resize` };
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function formatRatio(width: number, height: number) {
  const divisor = gcd(width, height);
  return `${Math.round(width / divisor)}:${Math.round(height / divisor)}`;
}

function gcd(a: number, b: number): number {
  return b ? gcd(b, a % b) : Math.max(1, a);
}
