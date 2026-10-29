"use client";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { createAssetCanvas } from "@/api/canvases";
import { isProjectID } from "@/components/projects/project";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";

/** The creation tile opens a new project and its main canvas. */
export function AssetCreateProject({
  kind,
  template,
  onClose,
  onLock,
}: {
  kind: "canvas" | "short_drama";
  template?: { key: string; name: string };
  onClose: () => void;
  onLock: (locked: boolean) => void;
}) {
  const router = useRouter();
  const [operation] = useState(() => ({
    key: crypto.randomUUID(),
    body: { kind, template_key: template?.key },
  }));
  const [pending, setPending] = useState(true);
  const [uncertain, setUncertain] = useState(true);
  const [error, setError] = useState("");
  const busy = useRef(false),
    flight = useRef<AbortController | null>(null);
  async function create() {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setUncertain(true);
    setError("");
    onLock(true);
    const controller = new AbortController();
    flight.current = controller;
    try {
      const result = await createAssetCanvas(operation.body, {
        signal: controller.signal,
        headers: { "Idempotency-Key": operation.key },
      });
      if (
        !isProjectID(result.id) ||
        !isProjectID(result.project_id) ||
        typeof result.name !== "string" ||
        !result.name.trim() ||
        typeof result.revision !== "string" ||
        !/^[1-9]\d*$/.test(result.revision) ||
        !Number.isSafeInteger(result.project_revision) ||
        result.project_revision! < 1 ||
        result.kind !== kind ||
        result.collection !== "current" ||
        (result.template_key ?? "") !== (template?.key ?? "") ||
        result.is_template !== false
      )
        throw new RequestError("invalid_receipt");
      if (controller.signal.aborted) return;
      onLock(false);
      onClose();
      router.push(
        `/canvas?project_id=${result.project_id}&canvas_id=${result.id}`,
      );
    } catch (reason) {
      if (controller.signal.aborted) return;
      if (
        reason instanceof RequestError &&
        reason.status >= 400 &&
        reason.status < 500 &&
        reason.code !== "invalid_receipt"
      ) {
        setUncertain(false);
        onLock(false);
        setError(readErrorMessage(reason));
      } else setError("创建结果尚未确认，请重试原操作。");
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  const autoCreate = useEffectEvent(() => void create());
  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (active) autoCreate();
    });
    return () => {
      active = false;
      flight.current?.abort();
      onLock(false);
    };
  }, [onLock]);
  const frozen = pending || uncertain;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !frozen) onClose();
      }}
    >
      <DialogContent showCloseButton={!frozen}>
        <DialogTitle>
          {template
            ? `使用模板 · ${template.name}`
            : kind === "short_drama"
              ? "新建短剧画布"
              : "新建画布"}
        </DialogTitle>
        <DialogDescription>
          在当前空间创建项目与主画布，完成后打开编辑器。
        </DialogDescription>
        {pending ? (
          <ContentState state="loading" title="正在创建画布" />
        ) : (
          <p
            role="status"
            className="text-sm break-words text-muted-foreground"
          >
            {error}
          </p>
        )}
        <DialogFooter>
          {!frozen && (
            <Button variant="ghost" onClick={onClose}>
              关闭
            </Button>
          )}
          {!pending && uncertain && (
            <Button onClick={() => void create()}>重试原操作</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
