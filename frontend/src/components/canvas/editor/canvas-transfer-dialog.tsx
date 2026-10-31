"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import { toast } from "sonner";
import { createCanvas, getCanvas, listCanvases } from "@/api/canvases";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RequestError } from "@/lib/request";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import type { CanvasClipboard } from "../lib/canvas-clipboard";
import {
  beginCanvasTransfer,
  markCanvasTransferApplied,
  canvasTransferOwnerKey,
  clearCanvasTransfer,
  pendingCanvasTransfer,
  requireTransferReceipt,
  requireTransferSource,
  subscribeCanvasTransfer,
} from "../lib/canvas-transfer-state";

type Props = {
  client: CanvasYjsLiveSession;
  canvasID: string;
  projectID: string;
  actorID: string;
  scopeID: string;
};

export function CanvasTransferDialog({
  selection,
  onClose,
  ...props
}: Props & { selection: CanvasClipboard; onClose: () => void }) {
  const session = useSession();
  const owner = { ...props, sessionID: session.session_id };
  const identity = canvasTransferOwnerKey(owner);
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [creation, setCreation] = useState<{
    key: string;
    name: string;
  } | null>(null);
  const creating = useRef(false);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const targets = useSWR(
    ["canvas-transfer-targets", identity, props.canvasID],
    async () => {
      const result = await listCanvases({ pid: props.projectID });
      if (identity !== canvasTransferOwnerKey(owner))
        throw new Error("当前身份已变化");
      return (result.items ?? []).filter(
        (item) =>
          item.id !== props.canvasID && item.project_id === props.projectID,
      );
    },
    { shouldRetryOnError: false },
  );
  const choose = async (targetID: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      if (props.client.getSnapshot().dirty) await props.client.waitForSaved();
      const [source, target] = await Promise.all([
        getCanvas({ id: props.canvasID }),
        getCanvas({ id: targetID }),
      ]);
      if (!active.current || identity !== canvasTransferOwnerKey(owner)) return;
      if (
        !source.permissions?.read ||
        source.project_id !== props.projectID ||
        selection.nodes.some((n) => !source.nodes?.some((s) => s.id === n.id))
      )
        throw new Error("来源已变化，请重新选择内容");
      beginCanvasTransfer(owner, props.canvasID, target, selection);
      router.push(
        `/canvas?project_id=${encodeURIComponent(props.projectID)}&canvas_id=${encodeURIComponent(targetID)}`,
      );
      onClose();
    } catch (cause) {
      if (active.current)
        setError(cause instanceof Error ? cause.message : "目标画布不可用");
    } finally {
      if (active.current) setBusy(false);
    }
  };
  const createTarget = async () => {
    if (creating.current || busy || (!creation && !name.trim())) return;
    creating.current = true;
    setBusy(true);
    setError("");
    const intent = creation ?? { key: crypto.randomUUID(), name: name.trim() };
    setCreation(intent);
    try {
      if (props.client.getSnapshot().dirty) await props.client.waitForSaved();
      const created = await createCanvas(
        { pid: props.projectID },
        { name: intent.name, scope: {} },
        { headers: { "Idempotency-Key": intent.key } },
      );
      if (!created.id || created.project_id !== props.projectID)
        throw new Error("创建回执不完整，请核验原操作");
      const [source, target] = await Promise.all([
        getCanvas({ id: props.canvasID }),
        getCanvas({ id: created.id }),
      ]);
      if (!active.current || identity !== canvasTransferOwnerKey(owner)) return;
      if (
        !source.permissions?.read ||
        source.project_id !== props.projectID ||
        selection.nodes.some((n) => !source.nodes?.some((s) => s.id === n.id))
      )
        throw new Error("来源已变化，请重新选择内容");
      beginCanvasTransfer(owner, props.canvasID, target, selection);
      router.push(
        `/canvas?project_id=${encodeURIComponent(props.projectID)}&canvas_id=${encodeURIComponent(created.id)}`,
      );
      onClose();
    } catch (cause) {
      if (active.current) {
        setError(
          cause instanceof Error ? cause.message : "创建尚未确认，请核验原操作",
        );
        if (
          cause instanceof RequestError &&
          [400, 401, 403, 404, 422].includes(cause.status)
        )
          setCreation(null);
      }
    } finally {
      creating.current = false;
      if (active.current) setBusy(false);
    }
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy && !creation) onClose();
      }}
    >
      <DialogContent
        onMouseDown={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
        data-canvas-shortcuts-ignore
      >
        <DialogHeader>
          <DialogTitle>保存到其他画布</DialogTitle>
          <DialogDescription>
            选择当前项目中的画布，将打开目标画布以确认保存{" "}
            {selection.nodes.length} 个节点。原画布保留。
          </DialogDescription>
        </DialogHeader>
        {targets.isLoading && <p role="status">正在读取画布…</p>}
        {targets.error && (
          <p role="alert">
            读取失败，请重试。
            <Button variant="outline" onClick={() => void targets.mutate()}>
              重试
            </Button>
          </p>
        )}
        {targets.data?.length === 0 && <p>当前项目没有其他画布。</p>}
        <ul className="max-h-80 space-y-2 overflow-auto">
          {targets.data?.map((target) => (
            <li key={target.id}>
              <Button
                variant="outline"
                className="w-full justify-start"
                disabled={busy || !target.permissions?.write}
                onClick={() => void choose(target.id!)}
              >
                {target.name || "未命名画布"}
                {!target.permissions?.write ? " · 无编辑权限" : ""}
              </Button>
            </li>
          ))}
        </ul>
        <div className="space-y-2 border-t pt-3">
          <Label htmlFor="canvas-transfer-target-name">新目标画布名称</Label>
          <Input
            id="canvas-transfer-target-name"
            value={name}
            disabled={busy || !!creation}
            maxLength={120}
            onChange={(event) => setName(event.target.value)}
          />
          <Button
            variant="outline"
            disabled={busy || (!creation && !name.trim())}
            onClick={() => void createTarget()}
          >
            {creation ? "核验创建结果" : "新建并打开"}
          </Button>
          {creation && (
            <p className="text-sm text-muted-foreground">
              保留原创建请求核验结果，不会重复创建。
            </p>
          )}
        </div>
        {busy && <p role="status">正在核验来源与目标…</p>}
        {error && <p role="alert">{error}</p>}
      </DialogContent>
    </Dialog>
  );
}

/** The destination's existing live session owns submission, unknown recovery, and undo. */
export function CanvasTransferReceiver(props: Props) {
  const session = useSession();
  const owner = { ...props, sessionID: session.session_id };
  const intent = useSyncExternalStore(
    subscribeCanvasTransfer,
    () => pendingCanvasTransfer(owner, props.canvasID),
    () => null,
  );
  const view = useSyncExternalStore(
    props.client.subscribe,
    props.client.getSnapshot,
    props.client.getSnapshot,
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const execute = async () => {
    if (!intent || running.current) return;
    running.current = true;
    setBusy(true);
    setError("");
    try {
      const source = await getCanvas({ id: intent.sourceID });
      requireTransferSource(intent, owner, source);
      if (!intent.applied) {
        const current = props.client.getSnapshot();
        if (
          !current.document ||
          !current.writable ||
          current.migration ||
          current.dirty
        )
          throw new Error("请先完成目标画布的现有编辑，再保存选区");
        const next = {
          ...current.document,
          nodes: [...current.document.nodes, ...intent.copy.nodes],
          connections: [
            ...current.document.connections,
            ...intent.copy.connections,
          ],
        };
        if (!props.client.canAccept(next))
          throw new Error("目标画布当前不可编辑");
        if (markCanvasTransferApplied(intent)) props.client.change(next, true);
      }
      await props.client.waitForSaved(() => {
        if (pendingCanvasTransfer(owner)?.id !== intent.id)
          throw new Error("当前身份或保存意图已变化");
      });
      const saved = await getCanvas({ id: props.canvasID });
      requireTransferReceipt(intent, owner, saved);
      clearCanvasTransfer(intent);
      toast.success("所选内容已保存到当前画布");
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "保存尚未确认，请继续核验原操作",
      );
    } finally {
      running.current = false;
      setBusy(false);
    }
  };
  // User explicitly confirms in the target canvas; retries retain the same identities.
  if (!intent) return null;
  return (
    <section
      aria-label="跨画布保存"
      className="absolute top-16 left-1/2 z-50 w-96 -translate-x-1/2 space-y-2 rounded-xl border bg-background p-4 shadow-lg"
      onMouseDown={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
      data-canvas-shortcuts-ignore
    >
      <p>将 {intent.copy.nodes.length} 个节点保存到当前画布</p>
      <p className="text-sm text-muted-foreground">
        原画布保留。保存后可以撤销整次副本。
      </p>
      {error && <p role="alert">{error}</p>}
      <div className="flex gap-2">
        <Button
          disabled={busy || !view.writable}
          onClick={() => void execute()}
        >
          {busy
            ? "正在核验保存…"
            : intent.applied
              ? "核验原保存"
              : "保存所选内容"}
        </Button>
        {!intent.applied && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => clearCanvasTransfer(intent)}
          >
            取消
          </Button>
        )}
      </div>
    </section>
  );
}
