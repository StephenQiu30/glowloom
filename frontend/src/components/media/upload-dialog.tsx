"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldDescription,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useEffect, useRef, useState, useId } from "react";
import { Upload, X } from "lucide-react";
import { LoadingState } from "@/components/layout/loading-state";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { uploadPersonalMediaAsset } from "@/api/media";
import { mediaError, validateUpload } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import { uploadMediaAsset } from "@/api/media";
export function UploadDialog({
  onClose,
  onUploaded,
  projectID,
  initialFile,
  destinationLabel,
}: {
  onClose: () => void;
  onUploaded: (asset?: API.AssetSummary) => void | Promise<void>;
  projectID?: string;
  initialFile?: File;
  destinationLabel?: string;
}) {
  const formFieldId = useId();

  const [file, setFile] = useState<File | null>(initialFile ?? null);
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [done, setDone] = useState(false);
  const [unconfirmed, setUnconfirmed] = useState(false);
  const [progress, setProgress] = useState<number | null>(null);
  const key = useRef(crypto.randomUUID());
  const [uploaded, setUploaded] = useState<API.AssetSummary | null>(null);
  const busy = useRef(false);
  const request = useRef<AbortController | null>(null);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => () => request.current?.abort(), []);
  function choose(f?: File) {
    if (pending || unconfirmed) return;
    if (!f) return;
    setError(validateUpload(f));
    setFile(f);
    setUploaded(null);
    setConfirmed(false);
    setProgress(null);
    setDone(false);
    key.current = crypto.randomUUID();
  }
  async function upload() {
    if (!file || busy.current || !confirmed || validateUpload(file)) return;
    busy.current = true;
    setPending(true);
    setProgress(null);
    setError("");
    const controller = new AbortController();
    request.current = controller;
    let accepted = uploaded;
    try {
      const sendOptions = {
        headers: { "Idempotency-Key": key.current },
        signal: controller.signal,
        onUploadProgress: (event: { loaded: number; total?: number }) => {
          if (!controller.signal.aborted && event.total)
            setProgress(
              Math.min(100, Math.round((event.loaded / event.total) * 100)),
            );
        },
      };
      if (!accepted) {
        const r = projectID
          ? await uploadMediaAsset(
              { pid: projectID },
              { local_review_confirmed: true },
              file,
              sendOptions,
            )
          : await uploadPersonalMediaAsset(
              { local_review_confirmed: true },
              file,
              sendOptions,
            );
        if (
          !isProjectID(r.asset?.id) ||
          (projectID &&
            (!r.asset ||
              !("project_id" in r.asset) ||
              r.asset.project_id !== projectID))
        )
          throw new RequestError("invalid_receipt");
        accepted = r.asset!;
        setUploaded(accepted);
      }
      await onUploaded(accepted);
      setDone(true);
      setUnconfirmed(false);
    } catch (e) {
      setUnconfirmed(
        !(e instanceof RequestError && e.status >= 400 && e.status < 500),
      );
      setError(
        controller.signal.aborted
          ? "上传已中止，但服务器可能已接收文件。请重试原上传确认结果。"
          : accepted
            ? `文件已上传，但归档尚未完成。${mediaError(e)}`
            : mediaError(e),
      );
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent>
        <DialogTitle>{done ? "素材上传成功" : "上传素材"}</DialogTitle>
        <DialogDescription>
          {done
            ? destinationLabel
              ? `文件已保存到「${destinationLabel}」。`
              : "文件已保存到当前参考素材目录。"
            : "图片和文档最多 20 MB，音频 100 MB，视频 500 MB（60 秒），GLB 模型 64 MB。"}
        </DialogDescription>
        {!done && (
          <FieldGroup className="min-w-0">
            <div className="sr-only">
              <FieldLabel htmlFor={`${formFieldId}-file`}>
                选择素材文件
              </FieldLabel>
              <Input
                id={`${formFieldId}-file`}
                ref={input}
                type="file"
                tabIndex={-1}
                aria-label="选择素材文件"
                disabled={pending || unconfirmed}
                onChange={(e) => choose(e.target.files?.[0])}
              />
            </div>
            <Button
              variant="secondary"
              className="h-auto min-h-40 flex-col gap-3 p-4 break-all whitespace-normal"
              disabled={pending || unconfirmed}
              onClick={() => input.current?.click()}
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault();
                if (e.dataTransfer.files.length > 1) {
                  setError("请每次上传一个文件。");
                  return;
                }
                choose(e.dataTransfer.files[0]);
              }}
            >
              <Upload data-icon="inline-start" />
              {file ? file.name : "拖入文件，或点击选择"}
            </Button>
            {file && (
              <FieldDescription>
                {(file.size / 1024 / 1024).toFixed(2)} MB
              </FieldDescription>
            )}
            <Field
              data-disabled={pending || unconfirmed}
              orientation="horizontal"
            >
              <Checkbox
                id={`${formFieldId}-field-1`}
                checked={confirmed}
                disabled={pending || unconfirmed}
                onCheckedChange={(v) => setConfirmed(v === true)}
              />
              <FieldLabel
                className="min-w-0 whitespace-normal"
                htmlFor={`${formFieldId}-field-1`}
              >
                我已确认内容与使用权限，文件不包含需要另行授权的真人内容。
              </FieldLabel>
            </Field>
            {pending && (
              <LoadingState>
                {progress === null
                  ? "正在准备上传…"
                  : progress < 100
                    ? `正在上传 ${progress}%`
                    : "上传完成，正在等待服务确认…"}
              </LoadingState>
            )}
            {error && <FeedbackToast message={error} />}
            {unconfirmed && (
              <FieldDescription>
                保留当前文件和原请求编号以安全重试。关闭后，请先刷新资产列表确认结果。
              </FieldDescription>
            )}
          </FieldGroup>
        )}
        <DialogFooter>
          {pending ? (
            <Button variant="outline" onClick={() => request.current?.abort()}>
              <X />
              中止上传
            </Button>
          ) : (
            <Button variant="outline" onClick={onClose}>
              {done ? "完成" : "关闭"}
            </Button>
          )}
          {!done && (
            <Button
              disabled={
                !file ||
                !confirmed ||
                !!(file && validateUpload(file)) ||
                pending
              }
              onClick={() => void upload()}
            >
              {uploaded ? "重试归档" : unconfirmed ? "重试原上传" : "上传"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
