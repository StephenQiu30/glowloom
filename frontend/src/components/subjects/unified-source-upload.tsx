"use client";
import { useEffect, useId, useRef, useState } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { UploadDropzone } from "@/components/media/upload-dropzone";
import { Progress } from "@/components/ui/progress";
import { Spinner } from "@/components/ui/spinner";
import { uploadSubjectSource } from "@/api/subjects";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { validateUpload } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";
import {
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import { subjectError, type SelectedSubjectMedia } from "./unified-contracts";
import { subjectSourceKind } from "./unified-source-contracts";
const extensions = {
  image: ".png,.jpg,.jpeg,.webp",
  video: ".mp4,.webm,.mov",
  audio: ".mp3,.wav,.m4a,.ogg,.flac",
};
export function UnifiedSubjectSourceUpload({
  open = true,
  allowedKinds,
  initialFile,
  onClose,
  onUploaded,
  onSuspend,
  onReturnFocus,
}: {
  open?: boolean;
  allowedKinds: readonly ("image" | "video" | "audio")[];
  initialFile?: File;
  onClose: () => void;
  onUploaded: (media: SelectedSubjectMedia) => string | void;
  onSuspend?: () => void;
  onReturnFocus?: () => void;
}) {
  const id = useId();
  const [file, setFile] = useState<File | null>(initialFile ?? null);
  const [reviewed, setReviewed] = useState(false);
  const [pending, setPending] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [progress, setProgress] = useState<number | null>(null);
  const [error, setError] = useState("");
  const [accepted, setAccepted] = useState<SelectedSubjectMedia | null>(null);
  const key = useRef(crypto.randomUUID());
  const busy = useRef(false);
  const request = useRef<AbortController | null>(null);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => () => request.current?.abort(), []);
  function validate(value: File | null) {
    if (!value) return "请选择文件。";
    const kind = subjectSourceKind(value);
    if (!kind || !allowedKinds.includes(kind))
      return allowedKinds.length === 1 && allowedKinds[0] === "audio"
        ? "请选择音频文件。"
        : allowedKinds.length === 1
          ? "补充参考仅支持图片。"
          : "请选择图片或视频文件。";
    return validateUpload(value);
  }
  function choose(value?: File) {
    if (!value || pending || uncertain) return;
    setFile(value);
    setReviewed(false);
    setAccepted(null);
    setError(validate(value));
    setProgress(null);
    key.current = crypto.randomUUID();
  }
  async function send() {
    if (!file || !reviewed || validate(file) || busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    setProgress(null);
    const controller = new AbortController();
    request.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    try {
      let value = accepted;
      if (!value) {
        const result = await uploadSubjectSource(
          { local_review_confirmed: true },
          file,
          {
            headers: { "Idempotency-Key": key.current },
            signal: controller.signal,
            onUploadProgress: (event) => {
              if (!controller.signal.aborted && event.total)
                setProgress(
                  Math.min(100, Math.round((event.loaded / event.total) * 100)),
                );
            },
          },
        );
        const asset = result.asset;
        if (
          !isProjectID(asset?.id) ||
          !Number.isSafeInteger(asset?.revision) ||
          asset!.revision! < 1 ||
          !allowedKinds.includes(asset!.kind as SelectedSubjectMedia["kind"]) ||
          !asset?.mime_type ||
          typeof asset.file_name !== "string"
        )
          throw new RequestError("invalid_receipt");
        value = {
          asset_id: asset.id!,
          asset_revision: asset.revision!,
          kind: asset.kind as SelectedSubjectMedia["kind"],
          mime_type: asset.mime_type,
          file_name: asset.file_name,
          library_item_id: asset.id!,
          library_source: "subject",
        };
      }
      if (
        controller.signal.aborted ||
        scope !== getScopeGeneration() ||
        session !== getSessionGeneration()
      )
        return;
      setAccepted(value);
      const issue = onUploaded(value);
      setUncertain(false);
      if (issue) setError(issue);
    } catch (reason) {
      if (!controller.signal.aborted) {
        setError(
          reason instanceof RequestError && reason.status === 403
            ? "当前空间没有上传权限。"
            : subjectError(reason),
        );
        setUncertain(
          !(
            reason instanceof RequestError &&
            reason.status >= 400 &&
            reason.status < 500
          ),
        );
      }
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  const invalid = file ? validate(file) : "";
  const audio = allowedKinds.length === 1 && allowedKinds[0] === "audio";
  const locked = pending || uncertain;
  function close() {
    if ((pending || uncertain) && onSuspend) onSuspend();
    else onClose();
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value) close();
      }}
    >
      <DialogContent
        className="unified-subject-source w-[calc(100vw-32px)] sm:max-w-[560px]"
        aria-describedby={`${id}-limits`}
        onCloseAutoFocus={(event) => {
          if (!open && onReturnFocus) {
            event.preventDefault();
            onReturnFocus();
          }
        }}
      >
        <DialogTitle>{audio ? "上传音色" : "上传参考"}</DialogTitle>
        <DialogDescription id={`${id}-limits`}>
          {audio
            ? "音频 ≤100 MB"
            : allowedKinds.length === 1
              ? "图片 ≤20 MB"
              : "图片 ≤20 MB · 视频 ≤500 MB / 60秒"}
        </DialogDescription>
        <FieldGroup className="gap-4">
          <Input
            ref={input}
            id={`${id}-file`}
            type="file"
            tabIndex={-1}
            className="sr-only"
            accept={allowedKinds.map((k) => extensions[k]).join(",")}
            aria-label={audio ? "选择音色文件" : "选择主体参考文件"}
            disabled={locked}
            onChange={(event) => choose(event.target.files?.[0])}
          />
          <UploadDropzone
            file={file}
            disabled={locked}
            describedBy={`${id}-limits`}
            onBrowse={() => input.current?.click()}
            onChoose={choose}
            onError={setError}
          />
          <Field orientation="horizontal" data-disabled={locked || undefined}>
            <Checkbox
              id={`${id}-review`}
              checked={reviewed}
              disabled={locked}
              onCheckedChange={(value) => setReviewed(value === true)}
            />
            <FieldLabel htmlFor={`${id}-review`}>
              我有使用权限，且无待授权真人内容。
            </FieldLabel>
          </Field>
        </FieldGroup>
        {pending ? (
          <div className="flex flex-col gap-2">
            <p role="status" className="text-sm text-muted-foreground">
              {progress === null
                ? "正在准备上传…"
                : progress < 100
                  ? `正在上传 ${progress}%`
                  : "正在处理文件…"}
            </p>
            {progress !== null ? (
              <Progress value={progress} aria-label="上传进度" />
            ) : null}
          </div>
        ) : uncertain ? (
          <p role="status" className="text-sm text-muted-foreground">
            结果待确认，重试不会重复上传。
          </p>
        ) : error || invalid ? (
          <Alert variant="destructive">
            <AlertDescription>{error || invalid}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="secondary"
            className="h-11 min-w-24"
            onClick={close}
          >
            {pending || uncertain ? "返回设置" : "取消"}
          </Button>
          <Button
            className="h-11 min-w-28"
            disabled={pending || !file || !reviewed || !!invalid}
            onClick={() => void send()}
          >
            {pending ? <Spinner data-icon="inline-start" /> : null}
            {pending
              ? "正在上传…"
              : uncertain
                ? "重试原上传"
                : accepted
                  ? "添加已上传文件"
                  : "上传"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
