"use client";

import { useEffect, useRef, useState, type PointerEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  annotationHistory,
  normalizeAnnotationRect,
  type AnnotationHistory,
} from "./image-annotation";
import { ApiError } from "@/lib/request";
import { getMediaPreview } from "./queries";
import {
  runImageTool,
  type ImageCrop,
  type ImageMark,
  type ImagePoint,
  type ImageToolOperation,
} from "./image-tools";
import type { CanvasNodeData } from "./model";

export function ImageToolsDialog({
  node,
  projectId,
  onClose,
  onApply,
}: {
  node: CanvasNodeData;
  projectId: string;
  onClose: () => void;
  onApply: (files: File[]) => void;
}) {
  const [kind, setKind] = useState<ImageToolOperation["kind"]>("crop");
  const [crop, setCrop] = useState<ImageCrop>({
    x: 0,
    y: 0,
    width: 1,
    height: 1,
  });
  const [rows, setRows] = useState(2),
    [columns, setColumns] = useState(2);
  const [longEdge, setLongEdge] = useState(2048);
  const [algorithm, setAlgorithm] = useState<"nearest" | "bilinear" | "high">(
    "high",
  );
  const [history, setHistory] = useState<AnnotationHistory>(() =>
    annotationHistory.empty(),
  );
  const marks = history.items;
  const [markTool, setMarkTool] = useState<"brush" | "rectangle" | "text">(
    "brush",
  );
  const [points, setPoints] = useState<ImagePoint[]>([]);
  const [color, setColor] = useState("#ff3535"),
    [width, setWidth] = useState(4),
    [label, setLabel] = useState("");
  const [running, setRunning] = useState(false),
    [error, setError] = useState("");
  const active = useRef<AbortController | null>(null);
  const image = useRef<HTMLImageElement>(null);
  const cropGesture = useRef<{
    id: number;
    start: ImagePoint;
    initial: ImageCrop;
  } | null>(null);
  const drawing = useRef<{ id: number; points: ImagePoint[] } | null>(null);
  const frame = useRef<number | null>(null);
  const preview = useQuery({
    queryKey: ["canvas", "image-tool-preview", projectId, node.assetId],
    queryFn: async ({ signal }) => {
      const value = await getMediaPreview(projectId, node.assetId!, signal);
      const url = new URL(value.url);
      if (
        value.asset.id !== node.assetId ||
        value.asset.project_id !== projectId ||
        value.asset.kind !== "image" ||
        !["http:", "https:"].includes(url.protocol) ||
        url.username ||
        url.password
      )
        throw new ApiError(502, "invalid_response");
      return value;
    },
    enabled: Boolean(node.assetId),
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  useEffect(
    () => () => {
      active.current?.abort();
      if (frame.current !== null) cancelAnimationFrame(frame.current);
      if (image.current) image.current.removeAttribute("src");
    },
    [],
  );
  function point(event: PointerEvent<SVGSVGElement>): ImagePoint {
    const bounds = event.currentTarget.getBoundingClientRect();
    return {
      x: Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width)),
      y: Math.max(0, Math.min(1, (event.clientY - bounds.top) / bounds.height)),
    };
  }
  function finishMark(event: PointerEvent<SVGSVGElement>, cancelled = false) {
    const gesture = cropGesture.current;
    if (gesture?.id === event.pointerId) {
      cropGesture.current = null;
      if (event.currentTarget.hasPointerCapture(event.pointerId))
        event.currentTarget.releasePointerCapture(event.pointerId);
      const value = normalizeAnnotationRect(gesture.start, point(event));
      setCrop(
        cancelled || !value.width || !value.height ? gesture.initial : value,
      );
      return;
    }
    const value = drawing.current;
    if (!value || value.id !== event.pointerId) return;
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    drawing.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId);
    if (!cancelled && value.points.length) {
      const rectangle = normalizeAnnotationRect(
        value.points[0],
        value.points.at(-1)!,
      );
      if (markTool === "brush" || (rectangle.width > 0 && rectangle.height > 0))
        setHistory((current) =>
          annotationHistory.push(
            current,
            markTool === "rectangle"
              ? { type: "rectangle", color, size: width, ...rectangle }
              : { type: "brush", color, size: width, points: value.points },
          ),
        );
    }
    setPoints([]);
  }
  async function apply() {
    if (!preview.data || running) return;
    if (Date.parse(preview.data.expires_at) <= Date.now() + 5000) {
      setError("素材授权已过期，请重新读取图片。");
      return;
    }
    const operation: ImageToolOperation =
      kind === "crop"
        ? { kind, crop }
        : kind === "split"
          ? { kind, rows, columns }
          : kind === "resize"
            ? { kind, longEdge, algorithm }
            : { kind, marks };
    const controller = new AbortController();
    active.current = controller;
    setRunning(true);
    setError("");
    try {
      const files = await runImageTool(
        preview.data.url,
        node.title,
        operation,
        controller.signal,
      );
      if (!controller.signal.aborted) onApply(files);
    } catch (caught) {
      if (!controller.signal.aborted)
        setError(caught instanceof Error ? caught.message : "图片处理失败。");
    } finally {
      if (!controller.signal.aborted) setRunning(false);
      active.current = null;
    }
  }
  const aspect =
    preview.data?.asset.width && preview.data.asset.height
      ? preview.data.asset.width / preview.data.asset.height
      : 4 / 3;
  const viewHeight = 1000 / aspect;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          active.current?.abort();
          onClose();
        }
      }}
    >
      <DialogContent className="flex max-h-[92dvh] flex-col overflow-y-auto sm:max-w-4xl [&>*]:shrink-0">
        <DialogHeader>
          <DialogTitle>图片工具 · {node.title}</DialogTitle>
          <DialogDescription>
            裁切、拆分、标注或调整像素尺寸，处理结果作为新素材上传并加入画布。
          </DialogDescription>
        </DialogHeader>
        <Select
          value={kind}
          onValueChange={(value) =>
            setKind(value as ImageToolOperation["kind"])
          }
          disabled={running}
        >
          <SelectTrigger aria-label="图片工具">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="crop">裁切</SelectItem>
              <SelectItem value="split">宫格拆分</SelectItem>
              <SelectItem value="annotate">标注</SelectItem>
              <SelectItem value="resize">像素放大</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        {preview.isPending ? <p role="status">正在读取图片…</p> : null}
        {preview.error || error ? (
          <Alert variant="destructive">
            <AlertTitle>图片工具未完成</AlertTitle>
            <AlertDescription>
              {preview.error?.message || error}
              <Button
                variant="secondary"
                onClick={() => {
                  setError("");
                  void preview.refetch();
                }}
              >
                重新读取图片
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {preview.data ? (
          <div
            className="relative mx-auto w-full max-w-xl overflow-hidden rounded-lg border bg-muted"
            style={{ aspectRatio: aspect }}
          >
            {/* eslint-disable-next-line @next/next/no-img-element -- 授权原图供像素编辑，不进入共享图片缓存。 */}
            <img
              ref={image}
              src={preview.data.url}
              alt={node.title}
              crossOrigin="anonymous"
              className="absolute inset-0 size-full object-contain"
              onError={() => setError("图片无法读取，请重试素材授权。")}
            />
            <svg
              className="absolute inset-0 size-full touch-none"
              viewBox={`0 0 1000 ${viewHeight}`}
              preserveAspectRatio="none"
              role="img"
              aria-label={
                kind === "annotate"
                  ? "标注区域，可使用指针绘制；文字标注可用下方输入框"
                  : "图片处理预览"
              }
              onPointerDown={(event) => {
                if (event.button !== 0 || running) return;
                if (kind === "crop") {
                  event.currentTarget.setPointerCapture(event.pointerId);
                  cropGesture.current = {
                    id: event.pointerId,
                    start: point(event),
                    initial: crop,
                  };
                  return;
                }
                if (kind !== "annotate" || running || marks.length >= 100)
                  return;
                if (markTool === "text") {
                  if (!label.trim()) {
                    setError("先输入文字，再点击图片放置。");
                    return;
                  }
                  setHistory((current) =>
                    annotationHistory.push(current, {
                      type: "text",
                      color,
                      size: width,
                      text: label.trim(),
                      ...point(event),
                    }),
                  );
                  return;
                }
                event.currentTarget.setPointerCapture(event.pointerId);
                drawing.current = {
                  id: event.pointerId,
                  points: [point(event)],
                };
              }}
              onPointerMove={(event) => {
                const gesture = cropGesture.current;
                if (gesture?.id === event.pointerId) {
                  const value = normalizeAnnotationRect(
                    gesture.start,
                    point(event),
                  );
                  if (value.width && value.height) setCrop(value);
                  return;
                }
                const value = drawing.current;
                if (
                  !value ||
                  value.id !== event.pointerId ||
                  value.points.length >= 3000
                )
                  return;
                if (markTool === "rectangle")
                  value.points = [value.points[0], point(event)];
                else value.points.push(point(event));
                if (frame.current === null)
                  frame.current = requestAnimationFrame(() => {
                    frame.current = null;
                    if (drawing.current) setPoints([...drawing.current.points]);
                  });
              }}
              onPointerUp={(event) => finishMark(event)}
              onPointerCancel={(event) => finishMark(event, true)}
            >
              {kind === "crop" ? (
                <rect
                  x={crop.x * 1000}
                  y={crop.y * viewHeight}
                  width={crop.width * 1000}
                  height={crop.height * viewHeight}
                  fill="none"
                  stroke="white"
                  strokeWidth="3"
                  strokeDasharray="10 6"
                />
              ) : null}
              {kind === "split" ? (
                <>
                  {Array.from(
                    { length: Math.max(0, Math.min(4, columns - 1)) },
                    (_, index) => (
                      <line
                        key={`c${index}`}
                        x1={((index + 1) * 1000) / columns}
                        x2={((index + 1) * 1000) / columns}
                        y1="0"
                        y2={viewHeight}
                        stroke="white"
                        strokeWidth="3"
                      />
                    ),
                  )}
                  {Array.from(
                    { length: Math.max(0, Math.min(4, rows - 1)) },
                    (_, index) => (
                      <line
                        key={`r${index}`}
                        x1="0"
                        x2="1000"
                        y1={((index + 1) * viewHeight) / rows}
                        y2={((index + 1) * viewHeight) / rows}
                        stroke="white"
                        strokeWidth="3"
                      />
                    ),
                  )}
                </>
              ) : null}
              {kind === "annotate" ? (
                <>
                  {[
                    ...marks,
                    ...(points.length
                      ? [
                          markTool === "rectangle"
                            ? {
                                type: "rectangle" as const,
                                color,
                                size: width,
                                ...normalizeAnnotationRect(
                                  points[0],
                                  points.at(-1)!,
                                ),
                              }
                            : {
                                type: "brush" as const,
                                points,
                                color,
                                size: width,
                              },
                        ]
                      : []),
                  ].map((mark, index) => (
                    <MarkPreview key={index} mark={mark} height={viewHeight} />
                  ))}
                </>
              ) : null}
            </svg>
          </div>
        ) : null}
        {kind === "crop" ? (
          <div className="flex flex-wrap gap-2">
            <span className="self-center text-xs text-muted-foreground">
              拖拽图片选择区域
            </span>
            {[
              ["全图", aspect],
              ["1:1", 1],
              ["16:9", 16 / 9],
              ["9:16", 9 / 16],
              ["4:3", 4 / 3],
            ].map(([label, ratio]) => (
              <Button
                key={label}
                variant="outline"
                size="sm"
                disabled={running}
                onClick={() => {
                  const width = Math.min(1, Number(ratio) / aspect),
                    height = Math.min(1, aspect / Number(ratio));
                  setCrop({
                    x: (1 - width) / 2,
                    y: (1 - height) / 2,
                    width,
                    height,
                  });
                }}
              >
                {label}
              </Button>
            ))}
          </div>
        ) : null}
        <div className="grid gap-3 sm:grid-cols-2">
          {kind === "crop"
            ? (Object.keys(crop) as Array<keyof ImageCrop>).map((key) => (
                <Field key={key}>
                  <FieldLabel htmlFor={`crop-${key}`}>
                    {
                      {
                        x: "左边距",
                        y: "上边距",
                        width: "宽度",
                        height: "高度",
                      }[key]
                    }
                    （%）
                  </FieldLabel>
                  <Input
                    id={`crop-${key}`}
                    type="number"
                    min="0"
                    max="100"
                    step="1"
                    value={Math.round(crop[key] * 100)}
                    disabled={running}
                    onChange={(event) =>
                      setCrop((value) => ({
                        ...value,
                        [key]: Number(event.target.value) / 100,
                      }))
                    }
                  />
                </Field>
              ))
            : null}
          {kind === "split" ? (
            <>
              <Field>
                <FieldLabel htmlFor="split-rows">行数</FieldLabel>
                <Input
                  id="split-rows"
                  type="number"
                  min="1"
                  max="5"
                  value={rows}
                  onChange={(event) => setRows(Number(event.target.value))}
                  disabled={running}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="split-columns">列数</FieldLabel>
                <Input
                  id="split-columns"
                  type="number"
                  min="1"
                  max="5"
                  value={columns}
                  onChange={(event) => setColumns(Number(event.target.value))}
                  disabled={running}
                />
              </Field>
            </>
          ) : null}
          {kind === "resize" ? (
            <>
              <Field>
                <FieldLabel htmlFor="resize-edge">目标长边（像素）</FieldLabel>
                <Input
                  id="resize-edge"
                  type="number"
                  min="1"
                  max="4096"
                  value={longEdge}
                  onChange={(event) => setLongEdge(Number(event.target.value))}
                  disabled={running}
                />
              </Field>
              <Field>
                <FieldLabel>插值算法</FieldLabel>
                <Select
                  value={algorithm}
                  onValueChange={(value) =>
                    setAlgorithm(value as typeof algorithm)
                  }
                  disabled={running}
                >
                  <SelectTrigger aria-label="插值算法">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="high">高质量插值</SelectItem>
                      <SelectItem value="bilinear">双线性插值</SelectItem>
                      <SelectItem value="nearest">最近邻像素</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            </>
          ) : null}
          {kind === "annotate" ? (
            <>
              <Field>
                <FieldLabel>标注方式</FieldLabel>
                <Select
                  value={markTool}
                  disabled={running}
                  onValueChange={(value: typeof markTool) => setMarkTool(value)}
                >
                  <SelectTrigger aria-label="标注方式">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="brush">画笔</SelectItem>
                    <SelectItem value="rectangle">矩形</SelectItem>
                    <SelectItem value="text">文字</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="mark-label">文字标注</FieldLabel>
                <Input
                  id="mark-label"
                  value={label}
                  maxLength={200}
                  onChange={(event) => setLabel(event.target.value)}
                  disabled={running}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="mark-color">标注颜色</FieldLabel>
                <Input
                  id="mark-color"
                  type="color"
                  value={color}
                  onChange={(event) => setColor(event.target.value)}
                  disabled={running}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="mark-width">
                  {markTool === "text" ? "文字字号" : "线条粗细"}
                </FieldLabel>
                <Input
                  id="mark-width"
                  type="number"
                  min="1"
                  max={markTool === "text" ? 96 : 80}
                  value={width}
                  onChange={(event) => setWidth(Number(event.target.value))}
                  disabled={running}
                />
              </Field>
              <div className="flex items-end gap-2">
                <Button
                  variant="secondary"
                  onClick={() =>
                    setHistory((value) => annotationHistory.undo(value))
                  }
                  disabled={running || !marks.length}
                >
                  撤销标注
                </Button>
                <Button
                  variant="secondary"
                  disabled={running || !history.redo.length}
                  onClick={() =>
                    setHistory((value) => annotationHistory.redo(value))
                  }
                >
                  重做标注
                </Button>
                <Button
                  variant="ghost"
                  onClick={() => {
                    setHistory(annotationHistory.empty());
                    setLabel("");
                  }}
                  disabled={running}
                >
                  清空标注
                </Button>
              </div>
            </>
          ) : null}
        </div>
        {kind === "resize" ? (
          <p className="text-sm text-muted-foreground">
            像素插值保留原始图像内容，最大长边4096像素。
          </p>
        ) : null}
        <DialogFooter>
          <Button
            variant="ghost"
            onClick={() => {
              active.current?.abort();
              onClose();
            }}
          >
            取消
          </Button>
          <Button
            onClick={() => void apply()}
            disabled={running || !preview.data || Boolean(preview.error)}
          >
            {running ? "正在处理…" : "处理并继续保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MarkPreview({ mark, height }: { mark: ImageMark; height: number }) {
  if (mark.type === "rectangle")
    return (
      <rect
        x={mark.x * 1000}
        y={mark.y * height}
        width={mark.width * 1000}
        height={mark.height * height}
        fill="none"
        stroke={mark.color}
        strokeWidth={mark.size}
      />
    );
  if (mark.type === "text")
    return (
      <text
        x={mark.x * 1000}
        y={mark.y * height}
        dominantBaseline="hanging"
        fill={mark.color}
        fontSize={mark.size}
      >
        {mark.text}
      </text>
    );
  if (mark.points.length === 1)
    return (
      <circle
        cx={mark.points[0].x * 1000}
        cy={mark.points[0].y * height}
        r={mark.size / 2}
        fill={mark.color}
      />
    );
  return (
    <polyline
      points={mark.points
        .map((point) => `${point.x * 1000},${point.y * height}`)
        .join(" ")}
      fill="none"
      stroke={mark.color}
      strokeWidth={mark.size}
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  );
}
