import { cn } from "@/lib/utils";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { useId } from "react";
import { Input } from "@/components/ui/input";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Grid2x2,
  ListRestart,
  PanelTop,
  Redo2,
  Rows3,
  Trash2,
  Undo2,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";

import { useImageEditorViewport } from "@/components/canvas/hooks/use-image-editor-viewport";
import type { ImageSplitParams } from "@/components/canvas/lib/canvas-image-data";
import { readImageMeta } from "@/components/canvas/runtime/lib/media/image-utils";

export type CanvasImageSplitParams = ImageSplitParams;

const defaultParams: CanvasImageSplitParams = {
  rows: 2,
  columns: 2,
  horizontalLines: [0.5],
  verticalLines: [0.5],
};
const maxGridSize = 12;
type ActiveLine = { axis: "horizontal" | "vertical"; index: number } | null;

export function CanvasNodeSplitDialog({
  dataUrl,
  open,
  onClose,
  onConfirm,
}: {
  dataUrl: string;
  open: boolean;
  onClose: () => void;
  onConfirm: (params: CanvasImageSplitParams) => void;
}) {
  const [params, setParams] = useState(defaultParams);
  const [image, setImage] = useState<{ width: number; height: number } | null>(
    null,
  );
  const [active, setActive] = useState<ActiveLine>(null);
  const historyRef = useRef<CanvasImageSplitParams[]>([]);
  const redoRef = useRef<CanvasImageSplitParams[]>([]);
  const dragAbortRef = useRef<AbortController | null>(null);
  const [historySize, setHistorySize] = useState(0);
  const [redoSize, setRedoSize] = useState(0);
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
  const previewRef = editorStageRef;
  const horizontalLines = params.horizontalLines || [];
  const verticalLines = params.verticalLines || [];
  const rows = horizontalLines.length + 1;
  const columns = verticalLines.length + 1;
  const total = rows * columns;
  const pieceSize = image
    ? {
        width: Math.max(1, Math.floor(image.width / columns)),
        height: Math.max(1, Math.floor(image.height / rows)),
      }
    : null;

  useEffect(() => {
    if (!open) return;
    // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setParams(defaultParams);
    setActive(null);
    setImage(null);
    historyRef.current = [];
    redoRef.current = [];
    setHistorySize(0);
    setRedoSize(0);
  }, [dataUrl, open]);

  useEffect(() => {
    if (!open) return;
    void readImageMeta(dataUrl).then(setImage);
  }, [dataUrl, open]);

  useEffect(() => {
    if (!open) dragAbortRef.current?.abort();
    return () => dragAbortRef.current?.abort();
  }, [open]);

  const update = (key: "rows" | "columns", value: string | number | null) => {
    const count = clampGrid(value ?? params[key]);
    pushHistory(historyRef, redoRef, params, setHistorySize, setRedoSize);
    setActive(null);
    setParams((current) => ({
      ...current,
      [key]: count,
      [key === "rows" ? "horizontalLines" : "verticalLines"]:
        buildGridLines(count),
    }));
  };
  const addLine = (axis: "horizontal" | "vertical") => {
    pushHistory(historyRef, redoRef, params, setHistorySize, setRedoSize);
    const key = axis === "horizontal" ? "horizontalLines" : "verticalLines";
    const spot = findLineSpot(params[key] || []);
    const lines = [...(params[key] || []), spot].sort((a, b) => a - b);
    setActive({ axis, index: lines.indexOf(spot) });
    setParams({
      ...params,
      [key]: lines,
      rows: axis === "horizontal" ? lines.length + 1 : params.rows,
      columns: axis === "vertical" ? lines.length + 1 : params.columns,
    });
  };
  const deleteLine = useCallback(() => {
    if (!active) return;
    pushHistory(historyRef, redoRef, params, setHistorySize, setRedoSize);
    setParams((current) => {
      const key =
        active.axis === "horizontal" ? "horizontalLines" : "verticalLines";
      const lines = (current[key] || []).filter(
        (_, index) => index !== active.index,
      );
      return {
        ...current,
        [key]: lines,
        rows: active.axis === "horizontal" ? lines.length + 1 : current.rows,
        columns:
          active.axis === "vertical" ? lines.length + 1 : current.columns,
      };
    });
    setActive(null);
  }, [active, params]);
  const startDrag = (
    axis: "horizontal" | "vertical",
    index: number,
    event: ReactPointerEvent,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    setActive({ axis, index });
    const box = previewRef.current?.getBoundingClientRect();
    if (!box) return;
    pushHistory(historyRef, redoRef, params, setHistorySize, setRedoSize);
    dragAbortRef.current?.abort();
    const controller = new AbortController();
    dragAbortRef.current = controller;
    const move = (moveEvent: PointerEvent) =>
      setLine(
        axis,
        index,
        axis === "horizontal"
          ? (moveEvent.clientY - box.top) / box.height
          : (moveEvent.clientX - box.left) / box.width,
      );
    const stop = () => controller.abort();
    window.addEventListener("pointermove", move, { signal: controller.signal });
    window.addEventListener("pointerup", stop, { signal: controller.signal });
    window.addEventListener("pointercancel", stop, {
      signal: controller.signal,
    });
  };
  const setLine = (
    axis: "horizontal" | "vertical",
    index: number,
    value: number,
  ) => {
    setParams((current) => {
      const key = axis === "horizontal" ? "horizontalLines" : "verticalLines";
      const lines = [...(current[key] || [])];
      lines[index] = clampLine(
        value,
        lines[index - 1] ?? 0,
        lines[index + 1] ?? 1,
      );
      return { ...current, [key]: lines };
    });
  };
  const resetLines = () => {
    pushHistory(historyRef, redoRef, params, setHistorySize, setRedoSize);
    setActive(null);
    setParams((current) => ({
      ...current,
      horizontalLines: buildGridLines(current.rows),
      verticalLines: buildGridLines(current.columns),
    }));
  };
  const undoSplit = useCallback(() => {
    const previous = historyRef.current.pop();
    if (!previous) return;
    redoRef.current.push(cloneSplitParams(params));
    setParams(previous);
    setActive(null);
    setHistorySize(historyRef.current.length);
    setRedoSize(redoRef.current.length);
  }, [params]);
  const redoSplit = useCallback(() => {
    const next = redoRef.current.pop();
    if (!next) return;
    historyRef.current.push(cloneSplitParams(params));
    setParams(next);
    setActive(null);
    setHistorySize(historyRef.current.length);
    setRedoSize(redoRef.current.length);
  }, [params]);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      if (target?.closest("input,textarea,[contenteditable='true']")) return;
      const key = event.key.toLowerCase();
      const isUndo =
        (event.metaKey || event.ctrlKey) &&
        !event.altKey &&
        !event.shiftKey &&
        key === "z";
      const isRedo =
        (event.metaKey || event.ctrlKey) &&
        !event.altKey &&
        ((event.shiftKey && key === "z") || (!event.shiftKey && key === "y"));
      const isDelete = event.key === "Delete" || event.key === "Backspace";
      if (!isUndo && !isRedo && !isDelete) return;
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
      if (isDelete) deleteLine();
      else if (isRedo) redoSplit();
      else undoSplit();
    };
    window.addEventListener("keydown", handleKeyDown, true);
    return () => window.removeEventListener("keydown", handleKeyDown, true);
  }, [deleteLine, open, redoSplit, undoSplit]);
  const confirmParams = {
    ...params,
    horizontalLines,
    verticalLines,
    rows,
    columns,
  };

  return (
    <Dialog
      open={open && Boolean(dataUrl)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className={"dialog-image-editor max-h-dialog w-full overflow-y-auto"}
      >
        <DialogHeader>
          <DialogTitle className="sr-only">拆分图片</DialogTitle>
          <DialogDescription className="sr-only">
            设置切分行列，预览并确认图片切分结果。
          </DialogDescription>
        </DialogHeader>
        <div>
          <div className="flex flex-col gap-5" data-canvas-no-zoom>
            <div>
              <h2 className="text-xl font-semibold">{"切分图片"}</h2>
              <p className="mt-1 text-sm opacity-60">{`生成 ${total} 个图片子节点，并按原图网格排列到画布右侧`}</p>
              <p className="mt-2 text-xs leading-5 opacity-55">
                {
                  "滚轮缩放 · 中键或空格+左键拖动画面 · Delete 删除选中线 · Ctrl/Cmd+Z 撤回 · Ctrl/Cmd+Shift+Z 重做"
                }
              </p>
            </div>
            <div className="md:grid-cols-asset-form xl:grid-cols-asset-form 2xl:grid-cols-asset-form grid gap-6">
              <Card>
                <CardHeader className="sr-only">
                  <CardTitle>切分预览</CardTitle>
                </CardHeader>
                <CardContent>
                  <div
                    ref={editorViewportRef}
                    {...editorPanHandlers}
                    className={cn(
                      "bg-media-backdrop/5 relative isolate h-media-editor min-h-0 rounded-lg",
                      editorScrollClassName,
                      editorIsPanning
                        ? "cursor-grabbing"
                        : editorSpacePressed
                          ? "cursor-grab"
                          : "",
                    )}
                  >
                    <div className="relative" style={editorContentStyle}>
                      <div
                        ref={previewRef}
                        className="bg-media-backdrop absolute isolate [transform:translateZ(0)] overflow-hidden rounded-lg [contain:layout_paint] [backface-visibility:hidden]"
                        style={editorStageStyle}
                      >
                        <div
                          className="absolute top-0 left-0 [backface-visibility:hidden]"
                          style={editorMediaStyle}
                        >
                          <PreviewImage
                            src={dataUrl}
                            alt=""
                            className="block h-full w-full object-contain"
                            draggable={false}
                          />
                        </div>
                        <SplitGrid
                          horizontalLines={horizontalLines}
                          verticalLines={verticalLines}
                          active={active}
                          onPointerDown={startDrag}
                        />
                      </div>
                    </div>
                  </div>
                  <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-sm">
                    <div className="flex flex-wrap items-center gap-1">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            aria-label={"撤回切图调整"}
                            onClick={undoSplit}
                            type={"button"}
                            variant={"ghost"}
                            size="icon"
                            disabled={!historySize}
                          >
                            {<Undo2 data-icon="inline-start" />}
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent side="top">
                          {"撤回切图调整 (Ctrl/Cmd+Z)"}
                        </TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            aria-label={"重做切图调整"}
                            onClick={redoSplit}
                            type={"button"}
                            variant={"ghost"}
                            size="icon"
                            disabled={!redoSize}
                          >
                            {<Redo2 data-icon="inline-start" />}
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent side="top">
                          {"重做切图调整 (Ctrl/Cmd+Shift+Z)"}
                        </TooltipContent>
                      </Tooltip>
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
                    </div>
                    <span className="font-semibold">
                      {image ? `${image.width} x ${image.height} px` : "读取中"}
                    </span>
                  </div>
                </CardContent>
              </Card>
              <div className="flex flex-col gap-5 py-2">
                <NumberField
                  label={"行数"}
                  value={rows}
                  onChange={(value) => update("rows", value)}
                />
                <NumberField
                  label={"列数"}
                  value={columns}
                  onChange={(value) => update("columns", value)}
                />
                <div className="grid grid-cols-2 gap-2">
                  <Button
                    onClick={() => addLine("horizontal")}
                    type={"button"}
                    variant={"secondary"}
                    size="default"
                  >
                    {<Rows3 data-icon="inline-start" />}
                    {"横向线"}
                  </Button>
                  <Button
                    onClick={() => addLine("vertical")}
                    type={"button"}
                    variant={"secondary"}
                    size="default"
                  >
                    {<PanelTop data-icon="inline-start" />}
                    {"纵向线"}
                  </Button>
                  <Button
                    onClick={deleteLine}
                    type={"button"}
                    variant={"secondary"}
                    size="default"
                    disabled={!active}
                  >
                    {<Trash2 data-icon="inline-start" />}
                    {"删除线"}
                  </Button>
                  <Button
                    onClick={resetLines}
                    type={"button"}
                    variant={"secondary"}
                    size="default"
                  >
                    {<ListRestart data-icon="inline-start" />}
                    {"重置线"}
                  </Button>
                </div>
                <Card>
                  <CardHeader className="sr-only">
                    <CardTitle>切片信息</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <div className="flex items-center justify-between">
                      <span className="opacity-60">{"切片数量"}</span>
                      <span className="font-semibold">{`${total} 个`}</span>
                    </div>
                    <div className="mt-2 flex items-center justify-between">
                      <span className="opacity-60">{"平均约"}</span>
                      <span className="font-semibold">
                        {pieceSize
                          ? `${pieceSize.width} x ${pieceSize.height}`
                          : "未知"}
                      </span>
                    </div>
                  </CardContent>
                </Card>
                <Button
                  onClick={() => onConfirm(confirmParams)}
                  type={"button"}
                  variant={"default"}
                  size="lg"
                  className={"w-full"}
                >
                  {<Grid2x2 data-icon="inline-start" />}
                  {"生成子节点"}
                </Button>
              </div>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function NumberField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (value: string | number | null) => void;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        className="w-full"
        min={1}
        max={maxGridSize}
        type="number"
        value={value ?? ""}
        onChange={(event) =>
          onChange(
            event.target.value === "" ? null : Number(event.target.value),
          )
        }
      />
    </Field>
  );
}

function SplitGrid({
  horizontalLines,
  verticalLines,
  active,
  onPointerDown,
}: {
  horizontalLines: number[];
  verticalLines: number[];
  active: ActiveLine;
  onPointerDown: (
    axis: "horizontal" | "vertical",
    index: number,
    event: ReactPointerEvent,
  ) => void;
}) {
  return (
    <div className="pointer-events-none absolute inset-0">
      {verticalLines.map((line, index) => (
        <div
          key={`column-${index}`}
          className="pointer-events-auto absolute inset-y-0 -ml-2 w-4 cursor-ew-resize"
          style={{ left: `${line * 100}%` }}
          onPointerDown={(event) => onPointerDown("vertical", index, event)}
        >
          <div
            className={cn(
              "absolute top-0 left-1/2 h-full border-l shadow-[0_0_0_1px_var(--canvas-grid-outline)]",
              active?.axis === "vertical" && active.index === index
                ? "border-warning"
                : "border-media-foreground/90",
            )}
          />
        </div>
      ))}
      {horizontalLines.map((line, index) => (
        <div
          key={`row-${index}`}
          className="pointer-events-auto absolute inset-x-0 -mt-2 h-4 cursor-ns-resize"
          style={{ top: `${line * 100}%` }}
          onPointerDown={(event) => onPointerDown("horizontal", index, event)}
        >
          <div
            className={cn(
              "absolute top-1/2 left-0 w-full border-t shadow-[0_0_0_1px_var(--canvas-grid-outline)]",
              active?.axis === "horizontal" && active.index === index
                ? "border-warning"
                : "border-media-foreground/90",
            )}
          />
        </div>
      ))}
    </div>
  );
}

function buildGridLines(count: number) {
  return Array.from(
    { length: Math.max(1, count) - 1 },
    (_, index) => (index + 1) / count,
  );
}

function findLineSpot(lines: number[]) {
  const cuts = [0, ...lines, 1].sort((a, b) => a - b);
  let spot = 0.5;
  let max = 0;
  for (let index = 0; index < cuts.length - 1; index += 1) {
    const gap = cuts[index + 1] - cuts[index];
    if (gap > max) {
      max = gap;
      spot = cuts[index] + gap / 2;
    }
  }
  return spot;
}

function clampLine(value: number, min: number, max: number) {
  return Math.min(max - 0.01, Math.max(min + 0.01, value));
}

function clampGrid(value: string | number) {
  const numberValue = Number(value);
  return Math.min(
    maxGridSize,
    Math.max(1, Math.round(Number.isFinite(numberValue) ? numberValue : 1)),
  );
}

function cloneSplitParams(params: CanvasImageSplitParams) {
  return {
    ...params,
    horizontalLines: [...(params.horizontalLines || [])],
    verticalLines: [...(params.verticalLines || [])],
  };
}

function pushHistory(
  historyRef: { current: CanvasImageSplitParams[] },
  redoRef: { current: CanvasImageSplitParams[] },
  params: CanvasImageSplitParams,
  setHistorySize: (size: number) => void,
  setRedoSize: (size: number) => void,
) {
  historyRef.current.push(cloneSplitParams(params));
  if (historyRef.current.length > 50) historyRef.current.shift();
  redoRef.current = [];
  setHistorySize(historyRef.current.length);
  setRedoSize(0);
}
