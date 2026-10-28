"use client";

import { useEffect, useRef, useState } from "react";
import { useSWRConfig } from "swr";
import { toast } from "sonner";
import {
  createAssetLibrary,
  getAssetLibraryCommand,
  getAssetMoveCommand,
  moveMyAssets,
} from "@/components/canvas/runtime/api/assetBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AssetLibrarySelect, assetLibraryTarget } from "./asset-library-select";

type Pending =
  | { kind: "create"; body: CanvasContract.AssetLibraryCreateRequest }
  | { kind: "move"; body: CanvasContract.AssetMoveRequest };

export function AssetLibraryCommandDialog({
  assets,
  initialProject,
  onClose,
  onDone,
}: {
  assets?: CanvasContract.AssetVO[];
  initialProject?: CanvasContract.AssetLibraryVO;
  onClose: () => void;
  onDone: (scope: string) => void;
}) {
  const [target, setTarget] = useState(
    assets
      ? "GENERAL"
      : initialProject?.projectId
        ? String(initialProject.projectId)
        : "",
  );
  const [project, setProject] = useState(initialProject);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<Pending>();
  const [queried, setQueried] = useState(false);
  const writing = useRef(false);
  const abort = useRef<AbortController | null>(null);
  const { mutate } = useSWRConfig();
  useEffect(() => () => abort.current?.abort(), []);
  async function submit(recover = false) {
    if (writing.current || !target || (!assets && !project)) return;
    const command: Pending =
      pending ??
      (assets
        ? {
            kind: "move",
            body: {
              commandId: crypto.randomUUID(),
              ...assetLibraryTarget(target),
              assets: assets.map((asset) => ({
                assetId: asset.id!,
                expectedRevision: asset.revision!,
              })),
            },
          }
        : {
            kind: "create",
            body: {
              commandId: crypto.randomUUID(),
              projectId: project!.projectId!,
              expectedRevision: project!.revision!,
            },
          });
    writing.current = true;
    setBusy(true);
    const controller = new AbortController();
    abort.current = controller;
    try {
      if (command.kind === "create")
        unwrap(
          await (recover
            ? getAssetLibraryCommand(
                { commandId: command.body.commandId },
                { signal: controller.signal },
              )
            : createAssetLibrary(command.body, { signal: controller.signal })),
        );
      else
        unwrap(
          await (recover
            ? getAssetMoveCommand(
                { commandId: command.body.commandId },
                { signal: controller.signal },
              )
            : moveMyAssets(command.body, { signal: controller.signal })),
        );
      controller.signal.throwIfAborted();
      void mutate(
        (key) =>
          Array.isArray(key) &&
          ["my-assets", "asset-libraries", "asset-library"].includes(key[0]),
      ).catch(() => undefined);
      toast.success(assets ? "已移动素材" : "项目参考素材目录已就绪");
      onDone(target);
      onClose();
    } catch (error) {
      if (controller.signal.aborted) return;
      const unknown =
        recover ||
        !(error instanceof ApiError) ||
        Math.floor(error.code / 100) >= 500;
      setPending(unknown ? command : undefined);
      setQueried(recover);
      notifyError(
        error,
        unknown ? "结果待确认，请核对原请求" : "操作未完成，请重新选择后重试",
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
        <DialogHeader>
          <DialogTitle>{assets ? "移动素材" : "新建项目参考素材目录"}</DialogTitle>
          <DialogDescription>
            {assets
              ? `将 ${assets.length} 个素材移动到所选参考素材，已保存的画布引用保持有效。`
              : "选择已有项目，每个项目一个参考素材，名称跟随项目。"}
          </DialogDescription>
        </DialogHeader>
        <AssetLibrarySelect
          value={target}
          includeGeneral={!!assets}
          enabledOnly={!!assets}
          disabled={busy || !!pending}
          label={assets ? "移动到参考素材" : "选择已有项目"}
          onChange={(value, item) => {
            setTarget(value);
            setProject(item);
          }}
        />
        {pending && (
          <p role="status" className="text-sm text-muted-foreground">
            正在核对原请求，目标和选中素材已保留。
          </p>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            disabled={busy || !!pending}
            onClick={onClose}
          >
            取消
          </Button>
          {pending && queried && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void submit()}
            >
              按原请求重试
            </Button>
          )}
          <Button
            disabled={busy || !target || (!assets && !project?.revision)}
            onClick={() => void submit(!!pending)}
          >
            {busy
              ? "处理中…"
              : pending
                ? "核对结果"
                : assets
                  ? "确认移动"
                  : project?.enabled
                    ? "进入参考素材"
                    : "创建参考素材"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
