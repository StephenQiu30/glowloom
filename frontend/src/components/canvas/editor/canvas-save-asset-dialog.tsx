"use client";

import { useEffect, useRef, useState } from "react";
import { useSWRConfig } from "swr";
import { toast } from "sonner";
import {
  getAssetCommand,
  saveMyAsset,
} from "@/components/canvas/runtime/api/assetBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AssetLibrarySelect,
  assetLibraryTarget,
  useAssetLibrary,
} from "@/components/canvas/runtime/components/assets/library/asset-library-select";
import { canvasAssetRequest } from "../lib/canvas-asset-request";
import type { CanvasNodeData } from "../types/canvas";

export function CanvasSaveAssetDialog({
  node,
  projectId,
  accountId,
  onClose,
  onSaved,
}: {
  node: CanvasNodeData;
  projectId?: string;
  accountId: string;
  onClose: () => void;
  onSaved: (scope: string) => void;
}) {
  const [title, setTitle] = useState(
    (node.title || node.metadata?.prompt || "画布素材").slice(0, 100),
  );
  const project = useAssetLibrary(projectId);
  const [chosenTarget, setChosenTarget] = useState<string>();
  const target =
    chosenTarget ?? (project.data?.enabled ? projectId! : "GENERAL");
  const targetUnresolved =
    !chosenTarget && !!projectId && (project.isLoading || !!project.error);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<CanvasContract.AssetSaveRequest>();
  const [canRetry, setCanRetry] = useState(false);
  const writing = useRef(false);
  const abort = useRef<AbortController | null>(null);
  const { mutate } = useSWRConfig();
  useEffect(() => () => abort.current?.abort(), []);

  async function submit(recover = false) {
    if (writing.current || (!pending && targetUnresolved)) return;
    let body: CanvasContract.AssetSaveRequest;
    try {
      body = pending ?? {
        ...canvasAssetRequest(node, title, crypto.randomUUID()),
        ...assetLibraryTarget(target),
      };
    } catch (error) {
      notifyError(error, "无法保存素材");
      return;
    }
    writing.current = true;
    setBusy(true);
    const controller = new AbortController();
    abort.current = controller;
    try {
      const result = unwrap(
        await (recover
          ? getAssetCommand(
              { commandId: body.commandId },
              { signal: controller.signal },
            )
          : saveMyAsset(body, { signal: controller.signal })),
      );
      controller.signal.throwIfAborted();
      if (!result.id || result.deleted) throw new Error("素材保存回执无效");
      // A cache read failure must not turn a committed save into another write.
      void mutate(
        (key) =>
          Array.isArray(key) && key[0] === "my-assets" && key[1] === accountId,
      ).catch(() => undefined);
      toast.success("已加入我的资产");
      onSaved(
        body.libraryScope === "PROJECT" ? String(body.projectId) : "GENERAL",
      );
      onClose();
    } catch (error) {
      if (controller.signal.aborted) return;
      const uncertain =
        recover ||
        !(error instanceof ApiError) ||
        Math.floor(error.code / 100) >= 500;
      setPending(uncertain ? body : undefined);
      setCanRetry(recover);
      notifyError(
        error,
        uncertain ? "保存结果待确认，请核对原请求" : "素材保存失败",
      );
    } finally {
      writing.current = false;
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy && !pending) onClose();
      }}
    >
      <DialogContent>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void submit(!!pending);
          }}
        >
          <DialogHeader>
            <DialogTitle>加入我的资产</DialogTitle>
            <DialogDescription>
              为素材命名，保存后可在个人参考素材中使用。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup className="my-4">
            <Field>
              <FieldLabel>保存到参考素材</FieldLabel>
              <AssetLibrarySelect
                value={target}
                onChange={setChosenTarget}
                disabled={busy || !!pending || project.isLoading}
              />
            </Field>
            <Field data-disabled={busy || !!pending}>
              <FieldLabel htmlFor="canvas-asset-title">素材名称</FieldLabel>
              <Input
                id="canvas-asset-title"
                autoFocus
                maxLength={100}
                value={title}
                disabled={busy || !!pending}
                onChange={(event) => setTitle(event.target.value)}
              />
            </Field>
          </FieldGroup>
          {project.error && !chosenTarget && !pending && (
            <p role="alert" className="mb-4 text-sm text-destructive">
              无法读取当前项目的参考素材，请重试或明确选择保存位置。
              <Button
                type="button"
                variant="link"
                onClick={() => void project.mutate()}
              >
                重试
              </Button>
            </p>
          )}
          {pending && (
            <p role="status" className="mb-4 text-sm text-muted-foreground">
              保存结果尚未确认，名称已保留。请先核对保存结果。
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={busy || !!pending}
              onClick={onClose}
            >
              取消
            </Button>
            {pending && canRetry && (
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => void submit()}
              >
                按原请求重试
              </Button>
            )}
            <Button
              type="submit"
              disabled={busy || !title.trim() || (!pending && targetUnresolved)}
            >
              {busy ? "处理中…" : pending ? "核对保存结果" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
