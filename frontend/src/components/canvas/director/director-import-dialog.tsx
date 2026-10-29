"use client";

import { useEffect, useId, useRef, useState } from "react";
import { uploadMediaAsset } from "@/api/media";
import { mediaError } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RequestError } from "@/lib/request";
import {
  DIRECTOR_MODEL_ACCEPT,
  type DirectorModelImport,
} from "./director-model-import";

export type DirectorImportDialogProps = {
  projectID: string;
  disabled?: boolean;
  /** Throw when the opening project/session/document epoch is no longer writable. */
  guard: () => void;
  /** Stage the uploaded asset for placement; retry receives the same asset without uploading again. */
  onAdd: (asset: API.AssetSummary) => void | Promise<void>;
  onClose: () => void;
};

export function DirectorImportDialog(props: DirectorImportDialogProps) {
  const [model, setModel] = useState<DirectorModelImport | null>(null);
  const [busy, setBusy] = useState(false);
  const [reviewed, setReviewed] = useState(false);
  const [error, setError] = useState("");
  const [asset, setAsset] = useState<API.AssetSummary | null>(null);
  const [unconfirmed, setUnconfirmed] = useState(false);
  const [progress, setProgress] = useState<number | null>(null);
  const initialProject = useRef(props.projectID);
  const current = useRef(props);
  const active = useRef(false);
  const alive = useRef(true);
  const request = useRef<AbortController | null>(null);
  const key = useRef(crypto.randomUUID());
  const field = useId();
  useEffect(() => {
    current.current = props;
  });
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      request.current?.abort();
    };
  }, []);

  function guard() {
    if (
      !alive.current ||
      current.current.disabled ||
      current.current.projectID !== initialProject.current ||
      !isProjectID(initialProject.current)
    )
      throw new Error("项目或编辑权限已变化，请重新打开导入窗口");
    current.current.guard();
  }

  async function choose(file?: File) {
    if (!file || active.current || unconfirmed || asset) return;
    active.current = true;
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setModel(null);
    setReviewed(false);
    setError("");
    setProgress(null);
    try {
      guard();
      const { convertDirectorModel } = await import("./director-model-import");
      controller.signal.throwIfAborted();
      const converted = await convertDirectorModel(file, controller.signal);
      controller.signal.throwIfAborted();
      guard();
      key.current = crypto.randomUUID();
      setModel(converted);
    } catch (cause) {
      if (alive.current)
        setError(
          controller.signal.aborted
            ? "模型转换已中止"
            : cause instanceof Error
              ? cause.message
              : "模型转换失败",
        );
    } finally {
      active.current = false;
      request.current = null;
      if (alive.current) setBusy(false);
    }
  }

  async function publish() {
    if (!model || !reviewed || active.current) return;
    active.current = true;
    const controller = new AbortController();
    request.current = controller;
    setBusy(true);
    setError("");
    let accepted = asset;
    let sent = false;
    try {
      guard();
      if (!accepted) {
        sent = true;
        const receipt = await uploadMediaAsset(
          { pid: initialProject.current },
          { local_review_confirmed: true },
          model.file,
          {
            signal: controller.signal,
            headers: { "Idempotency-Key": key.current },
            onUploadProgress: ({
              loaded,
              total,
            }: {
              loaded: number;
              total?: number;
            }) => {
              if (alive.current && !controller.signal.aborted && total)
                setProgress(Math.min(100, Math.round((loaded / total) * 100)));
            },
          },
        );
        if (
          !isProjectID(receipt.asset?.id) ||
          receipt.asset?.project_id !== initialProject.current ||
          receipt.asset?.kind !== "model"
        )
          throw new Error("模型上传回执与项目或素材类型不匹配");
        accepted = receipt.asset;
        if (alive.current) setAsset(accepted);
      }
      controller.signal.throwIfAborted();
      guard();
      await current.current.onAdd(accepted);
      if (alive.current) current.current.onClose();
    } catch (cause) {
      if (alive.current) {
        setUnconfirmed(
          !!accepted ||
            (sent &&
              !(
                cause instanceof RequestError &&
                cause.status >= 400 &&
                cause.status < 500
              )),
        );
        setError(
          accepted
            ? `模型已上传，尚未进入放置。${cause instanceof Error ? cause.message : "请重试。"}`
            : controller.signal.aborted
              ? "上传已中止，服务可能已接收文件。请重试原上传确认结果。"
              : cause instanceof RequestError
                ? mediaError(cause)
                : cause instanceof Error
                  ? cause.message
                  : "模型导入失败",
        );
      }
    } finally {
      active.current = false;
      request.current = null;
      if (alive.current) setBusy(false);
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !active.current) props.onClose();
      }}
    >
      <DialogContent
        data-canvas-no-zoom
        className="sm:max-w-xl"
        showCloseButton={!busy}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <DialogHeader>
          <DialogTitle>导入模型</DialogTitle>
          <DialogDescription>
            支持自包含 GLB、glTF、FBX 和 OBJ，文件及转换结果均不超过 64 MiB。FBX
            按厘米转换为米。上传后点击地面放置，取消放置会保留已上传的资产。
          </DialogDescription>
        </DialogHeader>
        <Label htmlFor={`${field}-file`}>选择模型文件</Label>
        <Input
          id={`${field}-file`}
          type="file"
          accept={DIRECTOR_MODEL_ACCEPT}
          disabled={props.disabled || busy || unconfirmed || !!asset}
          onChange={(event) => {
            void choose(event.currentTarget.files?.[0]);
            event.currentTarget.value = "";
          }}
        />
        {model && (
          <p role="status" className="text-sm text-muted-foreground">
            {model.file.name} · {model.meshes} 个网格 · {model.triangles}{" "}
            个三角面 · {model.animations.length} 个动画
          </p>
        )}
        <Label className="flex items-center gap-2">
          <Checkbox
            checked={reviewed}
            disabled={
              !model || props.disabled || busy || unconfirmed || !!asset
            }
            onCheckedChange={(value) => setReviewed(value === true)}
          />
          我有使用权限，且无待授权真人内容。
        </Label>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {busy && (
          <p role="status">
            {!model
              ? "正在转换模型…"
              : progress === null
                ? "正在准备放置…"
                : progress < 100
                  ? `正在上传 ${progress}%`
                  : "正在确认上传结果…"}
          </p>
        )}
        {unconfirmed && !busy && (
          <p className="text-sm text-muted-foreground">
            {asset
              ? "重试会使用已上传的模型。"
              : "结果待确认，重试将使用原上传请求。"}
          </p>
        )}
        <div className="flex justify-end gap-2">
          {busy ? (
            <Button variant="outline" onClick={() => request.current?.abort()}>
              中止
            </Button>
          ) : (
            <Button variant="outline" onClick={props.onClose}>
              关闭
            </Button>
          )}
          <Button
            disabled={props.disabled || busy || !model || !reviewed}
            onClick={() => void publish()}
          >
            {asset ? "重试放置" : unconfirmed ? "重试原上传" : "上传并放置"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
