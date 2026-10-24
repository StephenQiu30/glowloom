"use client";
import { useEffect, useState, useSyncExternalStore } from "react";
import { toast } from "sonner";
import { CanvasYjsLiveSession } from "./yjs-live-session";
import { CanvasParticipants } from "./canvas-participants";
import { YjsPresence } from "./yjs-presence";
import { SpaceSwitcher } from "@/components/spaces/space-context";
import { CanvasProviders } from "./editor";
import CanvasPage from "./editor/canvas-editor";
import { CanvasTopBar } from "./controls/canvas-top-bar";
import { CanvasMediaContext } from "./lib/canvas-media-context";
import { canvasViewKey } from "./view-state";
import { getCSRFToken } from "@/lib/request";
import { ensureSessionFresh } from "@/components/auth/session-renewal";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import type { RecoveryField } from "@glowloom/canvas-collab/recovery";

export function YjsCanvasWorkspace(props: {
  canvasID: string;
  projectID: string;
  actorID: string;
  scopeID: string;
}) {
  const [client] = useState(
    () =>
      new CanvasYjsLiveSession({
        ...props,
        csrf: getCSRFToken,
        renew: async (signal) => {
          const session = await ensureSessionFresh(signal, true);
          if (session.actor.id !== props.actorID)
            throw new Error("画布账号已变更");
        },
      }),
  );
  useEffect(() => {
    client.start();
    return () => client.stop();
  }, [client]);
  return (
    <CanvasProviders>
      <CollaborativeEditor client={client} {...props} />
    </CanvasProviders>
  );
}
function CollaborativeEditor({
  client,
  canvasID,
  projectID,
  actorID,
  scopeID,
}: {
  client: CanvasYjsLiveSession;
  canvasID: string;
  projectID: string;
  actorID: string;
  scopeID: string;
}) {
  const view = useSyncExternalStore(
    client.subscribe,
    client.getSnapshot,
    client.getSnapshot,
  );
  const [rename, setRename] = useState<string | null>(null);
  const [review, setReview] = useState<{
    sourceId: string;
    fields: RecoveryField[];
    selected: Set<string>;
    commitSeq: string;
  } | null>(null);
  useEffect(() => {
    if (!view.dirty) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [view.dirty]);
  if (!view.document || !view.dto)
    return (
      <div role="status" className="grid h-dvh place-items-center">
        {view.message || "正在连接画布"}
        <SpaceSwitcher />
        <Button onClick={client.retry}>重新连接</Button>
      </div>
    );
  return (
    <CanvasMediaContext.Provider
      value={{
        canvasId: canvasID,
        projectId: projectID,
        bindingRevision: view.dto.revision!,
      }}
    >
      <div className="canvas-workspace relative h-dvh">
        <CanvasPage
          documentIntegration={{
            projectId: projectID,
            initialState: view.document,
            viewStateKey: canvasViewKey(actorID, scopeID, projectID, canvasID),
            mediaContext: {
              canvasId: canvasID,
              projectId: projectID,
              bindingRevision: view.dto.revision!,
            },
            mediaEnabled: true,
            canNavigate: !view.dirty,
            frozen: !view.writable || view.migration,
            admit: client.canAccept,
            onChange: client.change,
            textBinding: client.textBinding,
            onCursor: client.setCursor,
            presenceOverlay: <YjsPresence client={client} cursors />,
            undo: {
              canUndo: view.canUndo,
              canRedo: view.canRedo,
              undo: () => client.history("undo"),
              redo: () => client.history("redo"),
            },
            toolbar: (controls) => (
              <CanvasTopBar
                {...controls}
                title={view.dto!.name ?? "画布"}
                canDeleteCanvas={false}
                canSwitchCanvas={!view.dirty}
                onDeleteProject={() => {}}
                onRenameProject={() => setRename(view.dto?.name ?? "画布")}
                participantsControl={
                  <CanvasParticipants
                    client={client}
                    actorID={actorID}
                    scopeID={scopeID}
                    connected={view.connected}
                  />
                }
                saveControl={
                  <div className="flex items-center gap-2">
                    <span role="status" className="text-xs">
                      {view.phase}
                    </span>
                    {view.migration && (
                      <Button size="sm" onClick={client.initialize}>
                        启用多人协作
                      </Button>
                    )}
                    {view.recovery.map((source) => (
                      <Button
                        key={source.sourceId}
                        size="sm"
                        onClick={() =>
                          setReview({
                            ...source,
                            selected: new Set(),
                            commitSeq: client.commitSeq,
                          })
                        }
                      >
                        查看待恢复输入
                      </Button>
                    ))}
                  </div>
                }
              />
            ),
          }}
        />
        {view.message && (
          <div
            role="alert"
            className="absolute bottom-16 left-1/2 z-50 flex max-w-xl -translate-x-1/2 items-center gap-3 rounded-lg bg-popover p-3 shadow"
          >
            <span>{view.message}</span>
            <Button variant="secondary" onClick={client.retry}>
              重新连接并查证
            </Button>
          </div>
        )}
        <Dialog
          open={rename !== null}
          onOpenChange={(open) => {
            if (!open) setRename(null);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>重命名画布</DialogTitle>
              <DialogDescription>
                名称更改会自动保存并同步给协作者。
              </DialogDescription>
            </DialogHeader>
            <Label>
              画布名称
              <Input
                value={rename ?? ""}
                maxLength={128}
                onChange={(event) => setRename(event.target.value)}
              />
            </Label>
            <DialogFooter>
              <Button
                disabled={!view.writable || !rename?.trim()}
                onClick={() => {
                  try {
                    client.rename(rename ?? "");
                    setRename(null);
                  } catch (error) {
                    toast.error(
                      error instanceof Error ? error.message : "无法修改名称",
                    );
                  }
                }}
              >
                确认名称
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
        <Dialog
          open={!!review}
          onOpenChange={(open) => {
            if (!open) setReview(null);
          }}
        >
          <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-3xl">
            <DialogHeader>
              <DialogTitle>核对并恢复本页输入</DialogTitle>
              <DialogDescription>
                只重新应用勾选内容。服务器发生变化后需要重新核对；未处理的输入仅保留在此页面。
              </DialogDescription>
            </DialogHeader>
            {review?.fields.map((field) => (
              <Label
                key={field.key}
                className="block space-y-2 rounded border p-3"
              >
                <span className="flex gap-2">
                  <Checkbox
                    checked={review.selected.has(field.key)}
                    onCheckedChange={(checked) => {
                      setReview((current) => {
                        if (!current) return null;
                        const selected = new Set(current.selected);
                        if (checked === true) selected.add(field.key);
                        else selected.delete(field.key);
                        return { ...current, selected };
                      });
                    }}
                  />
                  {field.field === "textDocument"
                    ? "正文"
                    : field.field === "title"
                      ? "名称"
                      : field.field === "entity"
                        ? "节点"
                        : "编辑内容"}
                </span>
                {field.copyRequired && (
                  <p className="text-sm text-muted-foreground">
                    原节点已删除或无法原位恢复。勾选后将把所选输入恢复为新节点，不恢复原连线。
                  </p>
                )}
                <div className="grid grid-cols-2 gap-3 text-xs">
                  <div>
                    当前内容
                    <pre className="break-all whitespace-pre-wrap">
                      {recoveryText(field.current.value)}
                    </pre>
                  </div>
                  <div>
                    本页输入
                    <pre className="break-all whitespace-pre-wrap">
                      {recoveryText(field.after.value)}
                    </pre>
                  </div>
                </div>
              </Label>
            ))}
            <DialogFooter>
              <Button
                variant="secondary"
                onClick={() => {
                  if (review && window.confirm("放弃这份本页输入？"))
                    void client
                      .discardRecovery(review.sourceId)
                      .then(() => setReview(null));
                }}
              >
                放弃这份输入
              </Button>
              <Button
                disabled={!review?.selected.size}
                onClick={() => {
                  if (!review) return;
                  void client
                    .applyRecovery(
                      review.sourceId,
                      review.fields
                        .filter((field) => review.selected.has(field.key))
                        .map((field) => ({
                          ...field,
                          copyNode: field.copyRequired,
                        })),
                      review.commitSeq,
                    )
                    .then(() => setReview(null))
                    .catch((cause) => {
                      setReview(null);
                      toast.error(
                        cause instanceof Error ? cause.message : "恢复失败",
                      );
                    });
                }}
              >
                重新应用所选内容
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </CanvasMediaContext.Provider>
  );
}

function recoveryText(value: unknown): string {
  if (value === undefined) return "内容已不存在";
  if (typeof value === "string") return value;
  if (
    value &&
    typeof value === "object" &&
    "schemaVersion" in value &&
    "content" in value
  ) {
    const text = (node: unknown): string => {
      if (!node || typeof node !== "object") return "";
      const part = node as {
        text?: string;
        type?: string;
        content?: unknown[];
      };
      return (
        part.text ??
        part.content?.map(text).join(part.type === "doc" ? "\n" : "") ??
        ""
      );
    };
    return text((value as { content: unknown }).content) || "（空白正文）";
  }
  return JSON.stringify(value, null, 2);
}
