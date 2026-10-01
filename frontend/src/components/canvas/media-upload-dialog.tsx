"use client";
import { useEffect, useId, useRef, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { ApiError } from "@/lib/request";
import { LocalUploadPreview } from "./local-upload-preview";
import { validateCanvasMediaFiles } from "./media-import";
import type { MediaAsset } from "./queries";
export type CanvasMediaUpload = (
  file: File,
  key: string,
  options: {
    signal: AbortSignal;
    onProgress: (progress: { loaded: number; total?: number }) => void;
  },
) => Promise<MediaAsset>;
export type MediaUploadDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialFiles?: readonly File[];
  remainingSlots: number;
  maximumFiles?: number;
  upload: CanvasMediaUpload;
  onImported: (assets: MediaAsset[]) => Promise<void>;
  onBusyChange?: (busy: boolean) => void;
  disabled?: boolean;
  readOnly?: boolean;
  target?: "canvas" | "folder-cover";
};
type Entry = {
  file: File;
  key: string;
  status: "pending" | "uploading" | "ready" | "failed" | "cancelled";
  progress?: number;
  error?: string;
  asset?: MediaAsset;
};
function entriesFor(files: readonly File[]): Entry[] {
  return files.map((file) => ({
    file,
    key: crypto.randomUUID(),
    status: "pending",
  }));
}
function safeError(error: unknown, fallback: string) {
  return error instanceof ApiError
    ? `${error.message}${error.requestId ? ` 请求编号：${error.requestId}` : ""}`
    : fallback;
}

/** 父级在项目变更时卸载；每次显式打开建立新的、只存在于本页的上传批次。 */
export function MediaUploadDialog(props: MediaUploadDialogProps) {
  return props.open ? <MediaUploadSession {...props} /> : null;
}
function MediaUploadSession({
  initialFiles = [],
  remainingSlots,
  maximumFiles = 20,
  upload,
  onImported,
  onOpenChange,
  onBusyChange,
  disabled = false,
  readOnly = false,
  target = "canvas",
}: MediaUploadDialogProps) {
  const [entries, setEntries] = useState(() => entriesFor(initialFiles));
  const [confirmed, setConfirmed] = useState(false);
  const [phase, setPhase] = useState<"idle" | "uploading" | "saving">("idle");
  const [saveError, setSaveError] = useState<string | null>(null);
  const mounted = useRef(false);
  const active = useRef<AbortController | null>(null);
  const inFlight = useRef(false);
  const saveBatch = useRef<MediaAsset[] | null>(null);
  const notifyBusy = useRef(onBusyChange);
  const filesId = useId();
  const reviewId = useId();
  const busy = phase !== "idle";
  const blocked = disabled || readOnly;
  const folderCover = target === "folder-cover";
  const validation = validateCanvasMediaFiles(
    entries.map((entry) => entry.file),
    remainingSlots,
    maximumFiles,
    folderCover,
  );
  const successful = entries.flatMap((entry) =>
    entry.asset ? [entry.asset] : [],
  );
  const failed = entries.some(
    (entry) => entry.status === "failed" || entry.status === "cancelled",
  );
  const attempted = entries.some((entry) => entry.status !== "pending");
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      active.current?.abort();
      notifyBusy.current?.(false);
    };
  }, []);
  useEffect(() => {
    notifyBusy.current = onBusyChange;
    onBusyChange?.(busy);
  }, [busy, onBusyChange]);
  useEffect(() => {
    if (!busy) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [busy]);
  const requestClose = () => {
    if (!inFlight.current) onOpenChange(false);
  };
  const finish = () => {
    active.current = null;
    inFlight.current = false;
    if (mounted.current) setPhase("idle");
  };
  const save = async (assets: MediaAsset[]) => {
    if (!mounted.current || !assets.length) return;
    setPhase("saving");
    setSaveError(null);
    // 保存结果不确定时必须重放原批次，不能再上传文件改变 AddNodes 请求。
    saveBatch.current ??= assets;
    try {
      await onImported(saveBatch.current);
      if (mounted.current) onOpenChange(false);
    } catch (error) {
      if (mounted.current)
        setSaveError(
          safeError(
            error,
            folderCover
              ? "图片已上传，但封面草稿尚未确认更新。请用原批次重试选择，素材已保留。"
              : "素材已上传，但画布尚未确认保存。请重试加入画布，或稍后从媒体库添加。",
          ),
        );
    } finally {
      finish();
    }
  };
  const saveSuccessful = () => {
    if (
      inFlight.current ||
      blocked ||
      !successful.length ||
      (!saveBatch.current && successful.length > remainingSlots)
    )
      return;
    inFlight.current = true;
    void save(successful);
  };
  const runUploads = async () => {
    if (
      inFlight.current ||
      blocked ||
      saveBatch.current ||
      !confirmed ||
      !validation.valid
    )
      return;
    inFlight.current = true;
    const controller = new AbortController();
    active.current = controller;
    setPhase("uploading");
    setSaveError(null);
    let current = [...entries];
    const update = (key: string, patch: Partial<Entry>) => {
      current = current.map((entry) =>
        entry.key === key ? { ...entry, ...patch } : entry,
      );
      if (mounted.current) setEntries(current);
    };
    for (const entry of current) {
      if (entry.status === "ready") continue;
      if (controller.signal.aborted || !mounted.current) break;
      update(entry.key, {
        status: "uploading",
        progress: undefined,
        error: undefined,
      });
      try {
        const asset = await upload(entry.file, entry.key, {
          signal: controller.signal,
          onProgress: ({ loaded, total }) => {
            if (mounted.current && !controller.signal.aborted)
              update(entry.key, {
                progress:
                  total &&
                  Number.isFinite(total) &&
                  total > 0 &&
                  Number.isFinite(loaded)
                    ? Math.max(
                        0,
                        Math.min(100, Math.round((loaded / total) * 100)),
                      )
                    : undefined,
              });
          },
        });
        if (controller.signal.aborted || !mounted.current) break;
        update(entry.key, { status: "ready", progress: 100, asset });
      } catch (error) {
        if (!mounted.current || controller.signal.aborted) break;
        update(entry.key, {
          status: "failed",
          error: safeError(error, "上传结果尚未确认，请使用原请求重试。"),
        });
      }
    }
    if (!mounted.current) return;
    if (controller.signal.aborted) {
      current = current.map((entry) =>
        entry.status === "ready" || entry.status === "failed"
          ? entry
          : { ...entry, status: "cancelled", error: undefined },
      );
      setEntries(current);
    }
    if (current.every((entry) => entry.status === "ready"))
      await save(current.map((entry) => entry.asset!));
    else finish();
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) requestClose();
      }}
    >
      <DialogContent
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl"
        showCloseButton={false}
        onEscapeKeyDown={(event) => {
          if (inFlight.current) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (inFlight.current) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {folderCover ? "上传目录封面" : "上传媒体到画布"}
          </DialogTitle>
          <DialogDescription>
            {folderCover ? (
              "上传一张图片到选中的来源项目。完成后选择正式素材作为封面草稿，再明确保存目录封面。"
            ) : (
              <>
                每批最多 {maximumFiles} 个文件、总计 700 MiB；当前可添加{" "}
                {Math.max(0, remainingSlots)} 个节点。上传后将保存正式素材引用。
              </>
            )}
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={filesId}>选择本地媒体文件</FieldLabel>
            <Input
              id={filesId}
              type="file"
              multiple={maximumFiles > 1}
              accept={
                folderCover
                  ? ".jpg,.jpeg,.png,.webp"
                  : ".jpg,.jpeg,.png,.webp,.mp4,.mov,.webm,.mp3,.wav,.m4a,.glb"
              }
              disabled={busy || blocked || successful.length > 0}
              onChange={(event) => {
                if (inFlight.current || successful.length) return;
                setEntries(entriesFor(Array.from(event.target.files ?? [])));
                setConfirmed(false);
                setSaveError(null);
                event.target.value = "";
              }}
            />
            <FieldDescription>
              {folderCover ? (
                "图片 JPG/PNG/WebP ≤20 MiB，每边 ≤8192、总像素 ≤4000 万。服务端将检查真实格式和尺寸。"
              ) : (
                <>
                  图片 JPG/PNG/WebP ≤20 MiB；视频 MP4/MOV 与静音 VP8/VP9 WebM
                  ≤500 MiB、≤60 秒；WebM 将实际转换为 MP4。音频 MP3/WAV/M4A ≤100
                  MiB。图片和视频每边 ≤8192、总像素 ≤4000 万。 GLB v2 ≤64
                  MiB，资源须在包内或嵌入 data
                  URI。服务端将检查真实格式、尺寸和时长。
                </>
              )}
            </FieldDescription>
            <FieldError
              errors={validation.errors.map((message) => ({ message }))}
            />
          </Field>
          <Field orientation="horizontal">
            <Checkbox
              id={reviewId}
              checked={confirmed}
              onCheckedChange={(value) => setConfirmed(value === true)}
              disabled={busy || blocked}
            />
            <FieldContent>
              <FieldLabel htmlFor={reviewId}>
                我已检查内容，拥有使用权，且不含需要授权的真人素材
              </FieldLabel>
              <FieldDescription>
                这是本地人工确认，不代表已通过外部自动审核。需要真人授权的素材请等待授权流程。
              </FieldDescription>
            </FieldContent>
          </Field>
        </FieldGroup>
        {entries.length > 0 && (
          <ul className="flex flex-col gap-3" aria-label="上传文件列表">
            {entries.map((entry) => (
              <li key={entry.key} className="flex flex-col gap-1">
                <p className="truncate text-sm font-medium">
                  {entry.file.name}
                </p>
                <LocalUploadPreview file={entry.file} />
                <Progress
                  value={entry.progress ?? null}
                  aria-label={`${entry.file.name} 上传进度`}
                  aria-valuenow={entry.progress}
                  aria-valuemin={0}
                  aria-valuemax={100}
                />
                <p className="text-xs text-muted-foreground" role="status">
                  {entry.status === "pending"
                    ? "等待上传"
                    : entry.status === "uploading"
                      ? `${entry.progress === undefined ? "正在上传" : `正在上传 ${entry.progress}%`}，完成后由服务端校验`
                      : entry.status === "ready"
                        ? folderCover
                          ? "已上传，等待选择为目录封面"
                          : "已上传，等待加入画布"
                        : entry.status === "cancelled"
                          ? "已取消，可用原请求重试"
                          : entry.error}
                </p>
              </li>
            ))}
          </ul>
        )}
        {successful.length > 0 && phase !== "saving" && (
          <Alert>
            <AlertTitle>已成功上传的素材已保留</AlertTitle>
            <AlertDescription>
              {folderCover ? (
                "正式图片已进入来源项目媒体库。选用失败时请重试原批次，无需重新上传。关闭后仍可在正式图片列表选择。"
              ) : (
                <>
                  {saveError
                    ? "请重试原画布保存批次。素材已进入媒体库，无需重新上传。"
                    : failed
                      ? "请重试未成功的文件，或选择将成功的素材加入画布。"
                      : "素材已进入媒体库，画布保存失败时可以直接重试加入。"}{" "}
                  关闭后仍可从媒体库添加。
                </>
              )}
            </AlertDescription>
          </Alert>
        )}
        {saveError && (
          <Alert variant="destructive">
            <AlertTitle>
              {folderCover ? "封面草稿未确认更新" : "画布未确认保存"}
            </AlertTitle>
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        )}
        {blocked && (
          <Alert>
            <AlertTitle>
              {folderCover ? "当前项目不可上传" : "当前画布不可修改"}
            </AlertTitle>
            <AlertDescription>
              {folderCover
                ? "请选择当前可编辑的来源项目后再上传封面。"
                : "请退出只读状态后再上传和添加素材。"}
            </AlertDescription>
          </Alert>
        )}
        {phase === "saving" && (
          <p role="status" className="text-sm text-muted-foreground">
            {folderCover
              ? "正在选择目录封面，请等待结果。"
              : "正在加入画布，请等待保存结果。"}
          </p>
        )}
        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={requestClose}>
            关闭上传窗口
          </Button>
          {phase === "uploading" ? (
            <Button variant="outline" onClick={() => active.current?.abort()}>
              取消上传
            </Button>
          ) : (
            <>
              {successful.length > 0 && (failed || saveError) && (
                <Button
                  variant="outline"
                  disabled={
                    busy ||
                    blocked ||
                    (!saveError && successful.length > remainingSlots)
                  }
                  onClick={saveSuccessful}
                >
                  {folderCover
                    ? "重试选择目录封面"
                    : saveError
                      ? "重试加入画布"
                      : `将成功的 ${successful.length} 个加入画布`}
                </Button>
              )}
              {(!attempted || failed) && !saveError && (
                <Button
                  disabled={busy || blocked || !confirmed || !validation.valid}
                  onClick={() => void runUploads()}
                >
                  {attempted ? "重试未成功文件" : "开始上传"}
                </Button>
              )}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
