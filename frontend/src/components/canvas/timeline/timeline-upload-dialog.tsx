"use client";

import { useEffect, useId, useRef, useState } from "react";
import { uploadMediaAsset } from "@/api/media";
import { mediaError, validateUpload } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { Progress } from "@/components/ui/progress";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Upload } from "lucide-react";

type UploadState =
  "waiting" | "queued" | "uploading" | "archiving" | "ready" | "failed";
type Item = {
  id: string;
  file: File;
  kind: "image" | "video" | "audio" | null;
  state: UploadState;
  progress: number | null;
  error: string;
  validation: string;
  confirmed: boolean;
  asset?: API.AssetSummary;
};
export type TimelineUploadDialogProps = {
  projectID: string;
  initialFiles?: readonly File[];
  onUploaded: (
    asset: API.AssetSummary,
    context: { signal: AbortSignal },
  ) => void | Promise<void>;
  onClose: () => void;
  disabled?: boolean;
  /** Rechecks the captured node/epoch immediately before every mutation. */
  isCurrent?: () => boolean;
};
const maximumFiles = 50;
const limitMessage = "每批最多选择50个文件，本次选择未添加。";
const extensions = {
  image: ["png", "jpg", "jpeg", "webp"],
  video: ["mp4", "webm", "mov"],
  audio: ["mp3", "wav", "m4a", "ogg", "flac"],
};
function itemFor(file: File): Item {
  const extension = file.name.split(".").at(-1)?.toLowerCase() ?? "";
  const kind =
    (Object.keys(extensions) as Array<keyof typeof extensions>).find((key) =>
      extensions[key].includes(extension),
    ) ?? null;
  const validation = kind
    ? validateUpload(file)
    : "时间线仅支持图片、视频和音频文件。";
  return {
    id: crypto.randomUUID(),
    file,
    kind,
    state: validation ? "failed" : "waiting",
    progress: null,
    error: validation,
    validation,
    confirmed: false,
  };
}
export function TimelineUploadDialog(props: TimelineUploadDialogProps) {
  return <TimelineUploadSession key={props.projectID} {...props} />;
}
function TimelineUploadSession({
  projectID,
  initialFiles = [],
  onUploaded,
  onClose,
  disabled = false,
  isCurrent,
}: TimelineUploadDialogProps) {
  const [items, setItems] = useState<Item[]>(() =>
    initialFiles.length > maximumFiles ? [] : initialFiles.map(itemFor),
  );
  const [error, setError] = useState(
    initialFiles.length > maximumFiles ? limitMessage : "",
  );
  const [confirmed, setConfirmed] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const entries = useRef(items);
  const requests = useRef(new Map<string, AbortController>());
  const mounted = useRef(false);
  const closing = useRef(false);
  const latest = useRef({ onUploaded, disabled, isCurrent });
  const fieldID = useId();
  useEffect(() => {
    latest.current = { onUploaded, disabled, isCurrent };
    if (disabled || isCurrent?.() === false)
      for (const request of requests.current.values()) request.abort();
  }, [onUploaded, disabled, isCurrent]);
  useEffect(() => {
    mounted.current = true;
    const active = requests.current;
    return () => {
      mounted.current = false;
      for (const request of active.values()) request.abort();
    };
  }, []);
  function canWrite() {
    return (
      mounted.current &&
      !closing.current &&
      !latest.current.disabled &&
      latest.current.isCurrent?.() !== false
    );
  }
  function update(id: string, patch: Partial<Item>) {
    entries.current = entries.current.map((item) =>
      item.id === id ? { ...item, ...patch } : item,
    );
    if (mounted.current) setItems(entries.current);
  }
  function choose(files: File[]) {
    if (!canWrite() || !files.length) return;
    if (
      files.length > maximumFiles ||
      entries.current.length + files.length > maximumFiles
    ) {
      setError(limitMessage);
      return;
    }
    entries.current = [...entries.current, ...files.map(itemFor)];
    setItems(entries.current);
    setError("");
    setConfirmed(false);
  }
  async function run(item: Item, controller: AbortController) {
    let accepted = item.asset;
    try {
      if (!canWrite() || !item.confirmed)
        throw new Error("当前时间线不可写入。");
      if (!accepted) {
        const receipt = await uploadMediaAsset(
          { pid: projectID },
          { local_review_confirmed: item.confirmed },
          item.file,
          {
            signal: controller.signal,
            headers: { "Idempotency-Key": item.id },
            onUploadProgress: (event: { loaded: number; total?: number }) => {
              if (!controller.signal.aborted && event.total)
                update(item.id, {
                  progress: Math.min(
                    100,
                    Math.round((event.loaded / event.total) * 100),
                  ),
                });
            },
          },
        );
        if (
          !isProjectID(receipt.asset?.id) ||
          receipt.asset?.project_id !== projectID ||
          receipt.asset?.kind !== item.kind
        )
          throw new RequestError("invalid_receipt");
        accepted = receipt.asset;
        update(item.id, { asset: accepted, progress: 100 });
      }
      if (controller.signal.aborted || !canWrite())
        throw new Error("当前时间线不可写入。");
      update(item.id, { state: "archiving" });
      await latest.current.onUploaded(accepted, { signal: controller.signal });
      if (controller.signal.aborted || !canWrite())
        throw new Error("当前时间线不可写入。");
      update(item.id, { state: "ready", error: "" });
    } catch (cause) {
      update(item.id, {
        state: "failed",
        error: accepted
          ? "文件已上传，归档尚未完成，请重试归档。"
          : controller.signal.aborted
            ? "上传已中止，结果待确认，请重试原上传。"
            : mediaError(cause),
      });
    } finally {
      requests.current.delete(item.id);
      pump();
    }
  }
  function pump() {
    if (!canWrite()) {
      if (
        mounted.current &&
        entries.current.some((item) => item.state === "queued")
      ) {
        entries.current = entries.current.map((item) =>
          item.state === "queued"
            ? {
                ...item,
                state: "failed",
                error: "当前时间线不可写入，请恢复后重试。",
              }
            : item,
        );
        setItems(entries.current);
      }
      return;
    }
    while (requests.current.size < 2) {
      const next = entries.current.find((item) => item.state === "queued");
      if (!next) break;
      const controller = new AbortController();
      requests.current.set(next.id, controller);
      update(next.id, {
        state: next.asset ? "archiving" : "uploading",
        error: "",
        progress: next.asset ? 100 : null,
      });
      void run(next, controller);
    }
  }
  function start() {
    if (!confirmed || !canWrite()) return;
    entries.current = entries.current.map((item) =>
      item.state === "waiting"
        ? { ...item, confirmed: true, state: "queued" }
        : item,
    );
    setItems(entries.current);
    pump();
  }
  function retry(item: Item) {
    if (
      !canWrite() ||
      item.validation ||
      !item.confirmed ||
      item.state !== "failed"
    )
      return;
    update(item.id, { state: "queued", error: "" });
    pump();
  }
  const active = items.some((item) =>
    ["uploading", "archiving", "queued"].includes(item.state),
  );
  const ready = items.filter((item) => item.state === "ready").length;
  const failed = items.filter((item) => item.state === "failed").length;
  const blocked = disabled || isCurrent?.() === false;
  function close() {
    closing.current = true;
    for (const request of requests.current.values()) request.abort();
    onClose();
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        className="sm:max-w-[560px]"
        data-canvas-shortcuts-ignore
        onPointerDown={(event) => event.stopPropagation()}
      >
        <DialogTitle>导入素材</DialogTitle>
        <DialogDescription>
          每批最多50个 · 图片≤20MB · 音频≤100MB · 视频≤500MB/60秒
        </DialogDescription>
        <Input
          ref={input}
          className="sr-only"
          type="file"
          multiple
          accept={Object.values(extensions)
            .flat()
            .map((extension) => `.${extension}`)
            .join(",")}
          aria-label="选择素材文件"
          disabled={blocked}
          onChange={(event) => {
            choose(Array.from(event.currentTarget.files ?? []));
            event.currentTarget.value = "";
          }}
        />
        <Button
          type="button"
          variant="outline"
          disabled={blocked}
          aria-label="选择或拖入素材"
          aria-disabled={blocked}
          className="flex min-h-24 cursor-pointer items-center justify-center gap-2 rounded-lg border border-dashed p-4 text-sm text-muted-foreground"
          onClick={() => {
            if (!blocked) input.current?.click();
          }}
          onKeyDown={(event) => {
            if (!blocked && (event.key === "Enter" || event.key === " ")) {
              event.preventDefault();
              input.current?.click();
            }
          }}
          onDragOver={(event) => event.preventDefault()}
          onDrop={(event) => {
            event.preventDefault();
            event.stopPropagation();
            choose(Array.from(event.dataTransfer.files));
          }}
        >
          <Upload className="size-4" aria-hidden="true" />
          选择文件或拖至此处
        </Button>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {items.length > 0 && (
          <>
            <p role="status" className="text-xs text-muted-foreground">
              共{items.length}个 · 已就绪{ready}个 · 失败{failed}个
            </p>
            <ul
              aria-label="素材导入结果"
              className="max-h-72 space-y-3 overflow-y-auto"
            >
              {items.map((item) => (
                <li
                  key={item.id}
                  className="space-y-1.5 rounded-lg bg-muted/40 p-3"
                >
                  <div className="flex items-center justify-between gap-3">
                    <span
                      className="min-w-0 truncate text-sm"
                      title={item.file.name}
                    >
                      {item.file.name}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {
                        {
                          waiting: "待上传",
                          queued: "等待上传",
                          uploading:
                            item.progress === 100 ? "等待服务确认" : "上传中",
                          archiving: "归档中",
                          ready: "已就绪",
                          failed: "失败",
                        }[item.state]
                      }
                    </span>
                  </div>
                  {item.state === "uploading" && (
                    <Progress
                      aria-label={`${item.file.name} 上传进度`}
                      value={item.progress}
                    />
                  )}
                  {item.error && (
                    <p className="text-xs text-destructive">{item.error}</p>
                  )}
                  {item.state === "failed" &&
                    !item.validation &&
                    item.confirmed && (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={blocked}
                        aria-label={`${item.asset ? "重试归档" : "重试"} ${item.file.name}`}
                        onClick={() => retry(item)}
                      >
                        {item.asset ? "重试归档" : "重试"}
                      </Button>
                    )}
                </li>
              ))}
            </ul>
          </>
        )}
        {items.some((item) => item.state === "waiting") && (
          <div className="flex items-start gap-2">
            <Checkbox
              id={`${fieldID}-confirmation`}
              checked={confirmed}
              disabled={blocked}
              onCheckedChange={(value) => setConfirmed(value === true)}
            />
            <Label
              htmlFor={`${fieldID}-confirmation`}
              className="text-sm leading-5"
            >
              我有使用权限，且无待授权真人内容。
            </Label>
          </div>
        )}
        {blocked && (
          <p role="status" className="text-xs text-muted-foreground">
            当前时间线不可写入。
          </p>
        )}
        <DialogFooter>
          <Button variant="secondary" onClick={close}>
            {active
              ? "中止并关闭"
              : ready === items.length && ready > 0
                ? "完成"
                : "关闭"}
          </Button>
          <Button
            disabled={
              blocked ||
              !confirmed ||
              !items.some((item) => item.state === "waiting")
            }
            onClick={start}
          >
            开始上传
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
