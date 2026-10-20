"use client";

import Image from "next/image";
import { useEffect, useRef, useState, useId, type PointerEvent } from "react";
import { uploadMediaAsset } from "@/api/media";
import {
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import { isProjectID } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";
import { canvasMediaSource } from "./canvas-media-source";
import {
  processCanvasImage,
  dragImageCrop,
  type ImageOperation,
} from "./canvas-image-operations";

type Job = { files: File[]; keys: string[]; assets: API.AssetSummary[] };
export function CanvasImageDialog({
  projectID,
  assetID,
  kind,
  availableSlots,
  onClose,
  onCreated,
}: {
  projectID: string;
  assetID: string;
  kind: ImageOperation["kind"];
  availableSlots: number;
  onClose: () => void;
  onCreated: (assets: API.AssetSummary[]) => boolean | void;
}) {
  const id = useId();
  const [source, setSource] = useState<{
    blob: Blob;
    url: string;
    width: number;
    height: number;
  }>();
  const [values, setValues] = useState({
    x: 0,
    y: 0,
    width: 100,
    height: 100,
    rows: 2,
    columns: 2,
    longEdge: 2048,
  });
  const [error, setError] = useState("");
  const [sourceAttempt, setSourceAttempt] = useState(0);
  const [confirmed, setConfirmed] = useState(false);
  const [pending, setPending] = useState(false);
  const [frozen, setFrozen] = useState(false);
  const [progress, setProgress] = useState(0);
  const stage = useRef<HTMLDivElement>(null);
  const cropDrag = useRef<{
    id: number;
    x: number;
    y: number;
    rect: DOMRect;
    values: typeof values;
    mode: "move" | "resize";
  } | null>(null);
  function beginCrop(
    event: PointerEvent<HTMLButtonElement>,
    mode: "move" | "resize",
  ) {
    if (event.button !== 0 || frozen || pending || !stage.current) return;
    event.preventDefault();
    event.stopPropagation();
    cropDrag.current = {
      id: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      rect: stage.current.getBoundingClientRect(),
      values,
      mode,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  }
  function moveCrop(event: PointerEvent<HTMLButtonElement>) {
    const drag = cropDrag.current;
    if (
      !drag ||
      drag.id !== event.pointerId ||
      !drag.rect.width ||
      !drag.rect.height
    )
      return;
    setValues({
      ...drag.values,
      ...dragImageCrop(
        drag.values,
        ((event.clientX - drag.x) / drag.rect.width) * 100,
        ((event.clientY - drag.y) / drag.rect.height) * 100,
        drag.mode,
      ),
    });
  }
  function endCrop(event: PointerEvent<HTMLButtonElement>, cancel = false) {
    const drag = cropDrag.current;
    if (!drag || drag.id !== event.pointerId) return;
    cropDrag.current = null;
    if (cancel) setValues(drag.values);
    if (event.currentTarget.hasPointerCapture(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId);
  }
  const busy = useRef(false),
    job = useRef<Job | null>(null),
    request = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let url = "";
    canvasMediaSource(projectID, assetID, controller.signal)
      .then(({ blob, asset }) => {
        if (controller.signal.aborted) return;
        url = URL.createObjectURL(blob);
        setSource({
          blob,
          url,
          width: asset.width || 1024,
          height: asset.height || 768,
        });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setError(
            error instanceof RequestError
              ? "无法读取当前项目素材，请重试。"
              : error instanceof Error && error.message
                ? error.message
                : "无法读取原图，请重试。",
          );
      });
    return () => {
      controller.abort();
      request.current?.abort();
      if (url) URL.revokeObjectURL(url);
    };
  }, [projectID, assetID, sourceAttempt]);
  const operation: ImageOperation =
    kind === "crop"
      ? {
          kind,
          x: values.x / 100,
          y: values.y / 100,
          width: values.width / 100,
          height: values.height / 100,
        }
      : kind === "split"
        ? { kind, rows: values.rows, columns: values.columns }
        : { kind, longEdge: values.longEdge };
  const count = kind === "split" ? values.rows * values.columns : 1;
  async function apply() {
    if (busy.current || !source || !confirmed || count > availableSlots) return;
    busy.current = true;
    setPending(true);
    setError("");
    const controller = new AbortController();
    request.current = controller;
    const session = getSessionGeneration(),
      scope = getScopeGeneration();
    const check = () => {
      controller.signal.throwIfAborted();
      if (session !== getSessionGeneration() || scope !== getScopeGeneration())
        throw new Error("登录或项目范围已变化，请重新打开图片工具。");
    };
    try {
      if (!job.current) {
        const files = await processCanvasImage(
          source.blob,
          operation,
          controller.signal,
        );
        check();
        job.current = {
          files,
          keys: files.map(() => crypto.randomUUID()),
          assets: [],
        };
        setFrozen(true);
      }
      const current = job.current;
      for (
        let index = current.assets.length;
        index < current.files.length;
        index++
      ) {
        check();
        const result = await uploadMediaAsset(
          { pid: projectID },
          { local_review_confirmed: true },
          current.files[index],
          {
            signal: controller.signal,
            headers: { "Idempotency-Key": current.keys[index] },
          },
        );
        check();
        if (
          !isProjectID(result.asset?.id) ||
          result.asset?.project_id !== projectID ||
          result.asset.kind !== "image"
        )
          throw new RequestError("invalid_receipt");
        current.assets.push(result.asset);
        setProgress(current.assets.length);
      }
      check();
      if (onCreated(current.assets) === false)
        throw new Error(
          "结果已保存到项目素材库，暂时无法加入画布；可重试添加。",
        );
      onClose();
    } catch (error) {
      if (!controller.signal.aborted)
        setError(
          error instanceof RequestError
            ? "上传尚未确认，请重试原任务。已完成的图片会保留在项目素材中。"
            : error instanceof Error
              ? error.message
              : "图片处理失败，请重试。",
        );
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  const title =
    kind === "crop" ? "裁剪图片" : kind === "split" ? "拆分图片" : "缩放图片";
  const fields: (keyof typeof values)[] =
    kind === "crop"
      ? ["x", "y", "width", "height"]
      : kind === "split"
        ? ["rows", "columns"]
        : ["longEdge"];
  const labels = {
    x: "左侧 (%)",
    y: "顶部 (%)",
    width: "宽度 (%)",
    height: "高度 (%)",
    rows: "行数",
    columns: "列数",
    longEdge: "最长边（像素）",
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy.current) onClose();
      }}
    >
      <DialogContent
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
        showCloseButton={!pending}
      >
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>
          处理结果保存为新的项目素材并添加到画布，原图保留。
          {kind === "resize" && "使用本地插值缩放。"}
        </DialogDescription>
        <div className="relative flex max-h-[40dvh] min-h-40 items-center justify-center overflow-hidden rounded-lg bg-muted">
          {source ? (
            <div ref={stage} className="relative inline-flex max-w-full">
              <Image
                src={source.url}
                alt="处理原图"
                width={source.width}
                height={source.height}
                unoptimized
                className="block h-auto max-h-[40dvh] w-auto max-w-full"
              />
              {kind === "crop" && (
                <div
                  className="absolute border border-foreground bg-foreground/10"
                  style={{
                    left: `${values.x}%`,
                    top: `${values.y}%`,
                    width: `${values.width}%`,
                    height: `${values.height}%`,
                  }}
                >
                  <Button
                    variant="ghost"
                    aria-label="移动裁剪区域"
                    className="absolute inset-0 size-full touch-none rounded-none hover:bg-transparent"
                    disabled={pending || frozen}
                    onPointerDown={(event) => beginCrop(event, "move")}
                    onPointerMove={moveCrop}
                    onPointerUp={(event) => endCrop(event)}
                    onPointerCancel={(event) => endCrop(event, true)}
                    onLostPointerCapture={(event) => endCrop(event, true)}
                  />
                  <Button
                    variant="outline"
                    size="icon-xs"
                    aria-label="调整裁剪范围"
                    className="absolute -right-2 -bottom-2 size-5 cursor-nwse-resize touch-none"
                    disabled={pending || frozen}
                    onPointerDown={(event) => beginCrop(event, "resize")}
                    onPointerMove={moveCrop}
                    onPointerUp={(event) => endCrop(event)}
                    onPointerCancel={(event) => endCrop(event, true)}
                    onLostPointerCapture={(event) => endCrop(event, true)}
                  />
                </div>
              )}
              {kind === "split" && (
                <div
                  className="pointer-events-none absolute inset-0"
                  aria-hidden
                >
                  {Array.from(
                    {
                      length: Math.max(
                        0,
                        Math.min(5, Math.floor(values.columns) - 1),
                      ),
                    },
                    (_, i) => (
                      <span
                        key={`column-${i}`}
                        className="absolute inset-y-0 border-l border-foreground"
                        style={{ left: `${((i + 1) / values.columns) * 100}%` }}
                      />
                    ),
                  )}
                  {Array.from(
                    {
                      length: Math.max(
                        0,
                        Math.min(5, Math.floor(values.rows) - 1),
                      ),
                    },
                    (_, i) => (
                      <span
                        key={`row-${i}`}
                        className="absolute inset-x-0 border-t border-foreground"
                        style={{ top: `${((i + 1) / values.rows) * 100}%` }}
                      />
                    ),
                  )}
                </div>
              )}
            </div>
          ) : error ? (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>原图加载失败</EmptyTitle>
                <EmptyDescription>{error}</EmptyDescription>
              </EmptyHeader>
              <EmptyContent>
                <Button
                  variant="outline"
                  onClick={() => {
                    setError("");
                    setSourceAttempt((attempt) => attempt + 1);
                  }}
                >
                  重新读取原图
                </Button>
              </EmptyContent>
            </Empty>
          ) : (
            <span role="status">正在读取原图…</span>
          )}
        </div>
        <FieldGroup className="grid grid-cols-2 gap-3">
          {fields.map((key) => (
            <Field key={key}>
              <FieldLabel htmlFor={`${id}-${key}`}>{labels[key]}</FieldLabel>
              <Input
                id={`${id}-${key}`}
                type="number"
                min={key === "x" || key === "y" ? 0 : 1}
                max={kind === "crop" ? 100 : kind === "split" ? 6 : 4096}
                disabled={pending || frozen}
                value={values[key]}
                onChange={(event) =>
                  setValues({ ...values, [key]: Number(event.target.value) })
                }
              />
            </Field>
          ))}
        </FieldGroup>
        <Field orientation="horizontal">
          <Checkbox
            id={`${id}-confirm`}
            checked={confirmed}
            onCheckedChange={(value) => setConfirmed(value === true)}
            disabled={pending || frozen}
          />
          <FieldLabel htmlFor={`${id}-confirm`}>
            我已确认原图与处理结果的使用权限
          </FieldLabel>
        </Field>
        {error && source && <FeedbackToast message={error} />}
        {count > availableSlots && (
          <p role="alert" className="text-sm text-destructive">
            结果将超过画布节点上限，请减少拆图数量。
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            关闭
          </Button>
          <Button
            disabled={
              !source || !confirmed || pending || count > availableSlots
            }
            onClick={() => void apply()}
          >
            {pending
              ? `正在保存 ${progress}/${count}`
              : frozen
                ? "重试原任务"
                : `保存并添加 ${count} 张图片`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
