"use client";
import { useEffect, useId, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
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
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { LocalUploadPreview } from "@/components/canvas/local-upload-preview";
import type { LibraryIdentity } from "./library-model";
import { useLibraryUpload } from "./use-library-upload";
import { formatBytes } from "./library-items";

const accept =
  ".jpg,.jpeg,.png,.webp,.gif,.mp4,.mov,.webm,.mp3,.wav,.m4a,.glb,.gltf,.txt,.docx";
export function LibraryUploadDialog({
  identity,
  onBusyChange,
  onClose,
  onCloseAutoFocus,
  onUploaded,
}: {
  identity: LibraryIdentity;
  onBusyChange: (busy: boolean) => void;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
  onUploaded: () => void | Promise<void>;
}) {
  const uploader = useLibraryUpload(identity, onUploaded),
    [files, setFiles] = useState<File[]>([]),
    [reviewed, setReviewed] = useState(false),
    [originals, setOriginals] = useState<Record<string, File>>({});
  const fileId = useId(),
    reviewId = useId(),
    recoverFocus = useRef(false),
    root = useRef<HTMLDivElement>(null);
  const unresolved = uploader.entries.filter(
    (entry) => entry.status !== "confirmed",
  );
  useEffect(() => {
    onBusyChange(uploader.locked);
    return () => onBusyChange(false);
  }, [uploader.locked, onBusyChange]);
  useEffect(() => {
    const current = document.activeElement;
    if (
      (uploader.storageError || unresolved.length) &&
      !uploader.busy &&
      current instanceof HTMLElement &&
      ((root.current?.contains(current) && current.matches(":disabled")) ||
        (recoverFocus.current &&
          (current === document.body || current === root.current)))
    ) {
      const available =
        root.current?.querySelector<HTMLButtonElement>(
          "button[data-upload-recovery]:not(:disabled),[data-upload-recovery] button:not(:disabled)",
        ) ??
        root.current?.querySelector<HTMLInputElement>(
          "[data-upload-recovery] input:not(:disabled)",
        );
      available?.focus();
      recoverFocus.current = false;
    }
  }, [uploader.storageError, uploader.busy, unresolved.length]);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !uploader.locked) onClose();
      }}
    >
      <DialogContent
        data-library-dialog
        onCloseAutoFocus={onCloseAutoFocus}
        ref={root}
        showCloseButton={false}
        className="flex max-h-[92dvh] flex-col overflow-y-auto sm:max-w-3xl"
        onEscapeKeyDown={(event) => {
          if (uploader.locked) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (uploader.locked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>上传素材原件</DialogTitle>
          <DialogDescription>
            {identity.scope.kind === "personal"
              ? "上传到当前账号的个人素材库。"
              : "上传到当前项目的素材库。"}{" "}
            每批最多25个文件、总计700MiB，同时最多发送4个文件。
          </DialogDescription>
        </DialogHeader>
        {uploader.storageError && (
          <Alert variant="destructive">
            <AlertTitle>原上传存储尚未恢复</AlertTitle>
            <AlertDescription>
              <p>{uploader.storageError}</p>
              <Button
                data-upload-recovery
                variant="outline"
                disabled={uploader.busy}
                onClick={() => void uploader.restoreStorage()}
              >
                恢复上传意图存储
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {uploader.error && (
          <p role="alert" className="text-sm text-destructive">
            {uploader.error}
          </p>
        )}
        {!unresolved.length && !uploader.storageError && (
          <fieldset disabled={uploader.busy || !uploader.ready}>
            <FieldGroup data-upload-recovery>
              <Field>
                <FieldLabel htmlFor={fileId}>本地素材原件</FieldLabel>
                <Input
                  id={fileId}
                  type="file"
                  multiple
                  accept={accept}
                  onChange={(event) => {
                    setFiles(Array.from(event.currentTarget.files ?? []));
                    setReviewed(false);
                  }}
                />
                <FieldDescription>
                  图片20MiB、视频500MiB、音频100MiB、GLB/glTF模型64MiB、TXT/DOCX文档20MiB。GIF
                  保留完整动画，缩略图显示首帧；服务端核验全部帧，最多1000帧、累计解码像素1.28亿。glTF
                  2.0须为自包含JSON，资源仅使用嵌入data URI；GLB资源可在包内。
                </FieldDescription>
              </Field>
              <div className="grid gap-3 sm:grid-cols-2">
                {files.map((file, index) => (
                  <div
                    key={`${index}:${file.name}`}
                    className="min-w-0 space-y-2 rounded-lg border p-3"
                  >
                    <p className="text-sm break-words">
                      {file.name} · {formatBytes(file.size)}
                    </p>
                    {!/^.*\.(txt|docx)$/i.test(file.name) && (
                      <LocalUploadPreview file={file} />
                    )}
                  </div>
                ))}
              </div>
              <Field orientation="horizontal">
                <Checkbox
                  id={reviewId}
                  checked={reviewed}
                  onCheckedChange={(checked) => setReviewed(checked === true)}
                />
                <FieldLabel htmlFor={reviewId}>
                  我已检查全部文件内容，拥有使用权限，并确认不含需要授权的真人素材。
                </FieldLabel>
              </Field>
            </FieldGroup>
          </fieldset>
        )}
        {uploader.entries.length > 0 && (
          <div className="space-y-3" aria-label="本批真实上传结果">
            {uploader.entries.map((entry) => {
              const progress =
                entry.total && entry.total > 0
                  ? Math.min(100, (entry.loaded / entry.total) * 100)
                  : undefined;
              return (
                <div
                  key={entry.intent.key}
                  className="min-w-0 space-y-2 rounded-lg border p-3"
                >
                  <p className="font-medium break-words">
                    {entry.intent.fileName} ·{" "}
                    {formatBytes(entry.intent.byteSize)}
                  </p>
                  <p className="text-sm">
                    {
                      {
                        pending: "原意图已保存，等待发送",
                        uploading:
                          progress === 100
                            ? "文件已发送，等待服务端核验和保存"
                            : "正在发送文件",
                        unknown: "服务端结果尚未确认",
                        rejected: "此请求已被服务端确定拒绝",
                        confirmed: "正式素材已确认",
                      }[entry.status]
                    }
                  </p>
                  {entry.status === "uploading" && progress !== undefined && (
                    <Progress
                      value={progress}
                      aria-label={`${entry.intent.fileName} 已发送字节进度`}
                    />
                  )}
                  <p className="text-xs break-all text-muted-foreground">
                    原键 {entry.intent.key} · 原件 SHA-256 {entry.intent.sha256}
                  </p>
                  {entry.asset && (
                    <p className="text-sm break-all">
                      素材 {entry.asset.id} · 实际{" "}
                      {formatBytes(entry.asset.byte_size)} ·{" "}
                      {entry.asset.mime_type}
                    </p>
                  )}
                  {entry.error && (
                    <p role="alert" className="text-sm text-destructive">
                      {entry.error}
                    </p>
                  )}
                  {!uploader.busy &&
                    entry.status === "unknown" &&
                    !uploader.storageError && (
                      <FieldGroup data-upload-recovery>
                        <Field>
                          <FieldLabel htmlFor={`original-${entry.intent.key}`}>
                            重新选择确切原文件 {entry.intent.fileName}
                          </FieldLabel>
                          <Input
                            id={`original-${entry.intent.key}`}
                            type="file"
                            accept={accept}
                            onChange={(event) => {
                              const file = event.currentTarget.files?.[0];
                              if (file)
                                setOriginals((old) => ({
                                  ...old,
                                  [entry.intent.key]: file,
                                }));
                            }}
                          />
                        </Field>
                        <Button
                          variant="outline"
                          disabled={!originals[entry.intent.key]}
                          onClick={() => {
                            recoverFocus.current = true;
                            void uploader.replay(
                              entry.intent.key,
                              originals[entry.intent.key] ?? null,
                            );
                          }}
                        >
                          人工使用原键和确切原文件核验
                        </Button>
                      </FieldGroup>
                    )}
                  {!uploader.busy && entry.status === "rejected" && (
                    <Button
                      data-upload-recovery
                      variant="outline"
                      onClick={() =>
                        void uploader.discardRejected(entry.intent.key)
                      }
                    >
                      读取当前身份并释放已拒绝的原意图
                    </Button>
                  )}
                </div>
              );
            })}
          </div>
        )}
        <DialogFooter className="flex-wrap">
          {uploader.busy ? (
            <Button variant="outline" onClick={uploader.stopSending}>
              停止继续发送并保留未确认原键
            </Button>
          ) : (
            !unresolved.length &&
            !uploader.storageError && (
              <Button
                disabled={!uploader.ready || !reviewed || !files.length}
                onClick={() => {
                  recoverFocus.current = true;
                  void uploader.start(files);
                }}
              >
                确认上传到{identity.scope.kind === "personal" ? "个人" : "项目"}
                素材库
              </Button>
            )
          )}
          <Button
            variant="outline"
            disabled={uploader.locked}
            onClick={onClose}
          >
            关闭上传
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
