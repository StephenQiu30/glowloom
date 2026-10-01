"use client";
import { useEffect, useRef, useState, type PointerEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { getMediaPreview, type MediaAsset } from "./queries";
import {
  drawMaskStroke,
  editMaskFile,
  maskDimensions,
  renderMaskPreview,
} from "./image-mask";
import {
  MediaUploadDialog,
  type CanvasMediaUpload,
} from "./media-upload-dialog";
import type { CanvasNodeData } from "./model";

export function ImageMaskDialog({
  node,
  projectId,
  remainingSlots,
  upload,
  onDraft,
  onBusyChange,
  onClose,
}: {
  node: CanvasNodeData;
  projectId: string;
  remainingSlots: number;
  upload: CanvasMediaUpload;
  onDraft: (mask: MediaAsset, prompt: string) => Promise<boolean>;
  onBusyChange: (value: boolean) => void;
  onClose: () => void;
}) {
  const [mode, setMode] = useState<"paint" | "erase">("paint"),
    [size, setSize] = useState(100),
    [prompt, setPrompt] = useState("");
  const [error, setError] = useState(""),
    [running, setRunning] = useState(false),
    [file, setFile] = useState<File>();
  const [dimensions, setDimensions] = useState<{
    width: number;
    height: number;
  }>();
  const mask = useRef<HTMLCanvasElement>(null),
    previewCanvas = useRef<HTMLCanvasElement>(null),
    image = useRef<HTMLImageElement>(null);
  const drawing = useRef<{ id: number; last: { x: number; y: number } } | null>(
      null,
    ),
    frame = useRef<number | null>(null),
    mounted = useRef(true);
  const preview = useQuery({
    queryKey: ["canvas", "mask-preview", projectId, node.assetId],
    queryFn: async ({ signal }) => {
      const value = await getMediaPreview(projectId, node.assetId!, signal),
        url = new URL(value.url);
      if (
        value.asset.id !== node.assetId ||
        value.asset.project_id !== projectId ||
        value.asset.kind !== "image" ||
        !["https:", "http:"].includes(url.protocol) ||
        url.username ||
        url.password ||
        !Number.isFinite(Date.parse(value.expires_at)) ||
        Date.parse(value.expires_at) <= Date.now() + 5000
      )
        throw new Error("图片预览身份或授权无效，请重新读取。");
      return value;
    },
    enabled: Boolean(node.assetId),
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  useEffect(() => {
    mounted.current = true;
    const currentImage = image.current;
    return () => {
      mounted.current = false;
      if (frame.current !== null) cancelAnimationFrame(frame.current);
      currentImage?.removeAttribute("src");
    };
  }, []);
  function point(event: PointerEvent<HTMLCanvasElement>) {
    const bounds = event.currentTarget.getBoundingClientRect();
    return {
      x: Math.max(
        0,
        Math.min(
          event.currentTarget.width,
          ((event.clientX - bounds.left) / Math.max(1, bounds.width)) *
            event.currentTarget.width,
        ),
      ),
      y: Math.max(
        0,
        Math.min(
          event.currentTarget.height,
          ((event.clientY - bounds.top) / Math.max(1, bounds.height)) *
            event.currentTarget.height,
        ),
      ),
    };
  }
  function draw(event: PointerEvent<HTMLCanvasElement>) {
    const current = drawing.current,
      context = mask.current?.getContext("2d");
    if (!current || current.id !== event.pointerId || !context) return;
    const next = point(event);
    context.lineCap = "round";
    context.lineJoin = "round";
    context.lineWidth = size;
    context.globalCompositeOperation =
      mode === "paint" ? "source-over" : "destination-out";
    context.strokeStyle = "#000";
    context.fillStyle = "#000";
    drawMaskStroke(context, current.last, next, size);
    current.last = next;
    if (frame.current === null)
      frame.current = requestAnimationFrame(() => {
        frame.current = null;
        if (mask.current && previewCanvas.current)
          renderMaskPreview(mask.current, previewCanvas.current);
      });
  }
  function stop(event: PointerEvent<HTMLCanvasElement>) {
    if (drawing.current?.id !== event.pointerId) return;
    drawing.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId);
  }
  async function submit() {
    if (!mask.current || running) return;
    if (!prompt.trim()) {
      setError("请输入修改要求。");
      return;
    }
    if (
      !preview.data ||
      Date.parse(preview.data.expires_at) <= Date.now() + 5000
    ) {
      setError("素材授权已过期，请重新读取图片。");
      return;
    }
    setRunning(true);
    setError("");
    try {
      const next = await editMaskFile(mask.current, node.title);
      if (mounted.current) setFile(next);
    } catch (failure) {
      if (mounted.current)
        setError(failure instanceof Error ? failure.message : "蒙版创建失败。");
    } finally {
      if (mounted.current) setRunning(false);
    }
  }
  if (file)
    return (
      <MediaUploadDialog
        open
        onOpenChange={(value) => {
          if (!value) onClose();
        }}
        initialFiles={[file]}
        remainingSlots={Math.max(0, remainingSlots - 1)}
        maximumFiles={1}
        upload={upload}
        onBusyChange={onBusyChange}
        onImported={async (assets) => {
          const asset = assets[0];
          if (
            assets.length !== 1 ||
            asset.kind !== "image" ||
            asset.project_id !== projectId ||
            asset.width !== dimensions?.width ||
            asset.height !== dimensions?.height
          )
            throw new Error("蒙版正式素材尺寸或项目与原图不一致。");
          if (!(await onDraft(asset, prompt.trim())))
            throw new Error("蒙版已上传，生成草稿尚未确认保存，请重试。");
        }}
      />
    );
  return (
    <Dialog
      open
      onOpenChange={(value) => {
        if (!value && !running) onClose();
      }}
    >
      <DialogContent className="flex max-h-[92dvh] flex-col overflow-y-auto sm:max-w-3xl [&>*]:shrink-0">
        <DialogHeader>
          <DialogTitle>局部重绘 · {node.title}</DialogTitle>
          <DialogDescription>
            涂抹需要修改的区域，审核并上传蒙版后创建图片编辑草稿。选择模型后确认原图与蒙版的用途，再查看报价。
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant={mode === "paint" ? "default" : "outline"}
            onClick={() => setMode("paint")}
          >
            画笔
          </Button>
          <Button
            variant={mode === "erase" ? "default" : "outline"}
            onClick={() => setMode("erase")}
          >
            擦除
          </Button>
          <Input
            className="w-24"
            aria-label="蒙版笔刷大小（原图像素）"
            type="number"
            min={8}
            max={160}
            step={2}
            value={size}
            onChange={(event) =>
              setSize(
                Math.max(8, Math.min(160, Number(event.target.value) || 8)),
              )
            }
          />
          <span className="text-xs text-muted-foreground">原图像素</span>
          <Button
            variant="ghost"
            onClick={() => {
              if (mask.current)
                mask.current
                  .getContext("2d")
                  ?.clearRect(0, 0, mask.current.width, mask.current.height);
              if (previewCanvas.current)
                previewCanvas.current
                  .getContext("2d")
                  ?.clearRect(
                    0,
                    0,
                    previewCanvas.current.width,
                    previewCanvas.current.height,
                  );
              setError("");
            }}
          >
            清空蒙版
          </Button>
        </div>
        {preview.data ? (
          <div
            className="relative mx-auto w-full max-w-xl overflow-hidden rounded-lg border bg-muted"
            style={{
              aspectRatio: dimensions
                ? dimensions.width / dimensions.height
                : 4 / 3,
            }}
          >
            {/* eslint-disable-next-line @next/next/no-img-element -- 私有授权原图直接用于蒙版编辑。 */}
            <img
              ref={image}
              src={preview.data.url}
              alt={node.title}
              crossOrigin="anonymous"
              className="block w-full"
              onError={() => {
                setDimensions(undefined);
                setError("原图读取失败，请重新读取素材。");
              }}
              onLoad={(event) => {
                try {
                  const width = event.currentTarget.naturalWidth,
                    height = event.currentTarget.naturalHeight;
                  maskDimensions(width, height);
                  if (
                    preview.data.asset.width !== width ||
                    preview.data.asset.height !== height
                  )
                    throw new Error("原图像素与正式素材不一致。");
                  setDimensions({ width, height });
                } catch (failure) {
                  setDimensions(undefined);
                  setError(
                    failure instanceof Error
                      ? failure.message
                      : "原图尺寸无效。",
                  );
                }
              }}
            />
            {dimensions ? (
              <>
                <canvas
                  ref={mask}
                  width={dimensions.width}
                  height={dimensions.height}
                  className="hidden"
                />
                <canvas
                  ref={previewCanvas}
                  width={dimensions.width}
                  height={dimensions.height}
                  aria-label="局部重绘蒙版绘制区域"
                  className="absolute inset-0 size-full cursor-crosshair touch-none"
                  onPointerDown={(event) => {
                    if (event.button !== 0 || running) return;
                    event.preventDefault();
                    event.currentTarget.setPointerCapture(event.pointerId);
                    drawing.current = {
                      id: event.pointerId,
                      last: point(event),
                    };
                    draw(event);
                  }}
                  onPointerMove={draw}
                  onPointerUp={stop}
                  onPointerCancel={stop}
                />
              </>
            ) : null}
          </div>
        ) : (
          <p role="status">正在读取原图…</p>
        )}
        {error || preview.error ? (
          <p role="alert" className="text-sm text-destructive">
            {error || preview.error?.message}
            <Button
              variant="ghost"
              onClick={() => {
                setError("");
                void preview.refetch();
              }}
            >
              重新读取
            </Button>
          </p>
        ) : null}
        <Field>
          <FieldLabel htmlFor="mask-prompt">局部修改要求</FieldLabel>
          <Textarea
            id="mask-prompt"
            maxLength={10000}
            value={prompt}
            disabled={running}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder="例如：将手中的杯子替换成透明玻璃杯，保留人物与背景。"
          />
        </Field>
        <DialogFooter>
          <Button variant="outline" disabled={running} onClick={onClose}>
            关闭
          </Button>
          <Button
            disabled={running || !dimensions || remainingSlots < 2}
            onClick={() => void submit()}
          >
            {running ? "创建蒙版中…" : "审核蒙版并创建编辑草稿"}
          </Button>
        </DialogFooter>
        {remainingSlots < 2 ? (
          <p role="alert">画布需要至少两个剩余节点位置，用于蒙版与编辑草稿。</p>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
