"use client";
import { useEffect, useState, useSyncExternalStore } from "react";
import { Provider } from "react-redux";
import { useTheme } from "next-themes";
import { toast } from "sonner";
import { makeAppStore } from "./runtime/store/app-store";
import { AppFeedbackProvider } from "./runtime/components/feedback/app-feedback-provider";
import CanvasPage from "./editor/canvas-editor";
import { CanvasTopBar } from "./controls/canvas-top-bar";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";
import { CanvasRecoveryDialog } from "./recovery-dialog";
import {
  CanvasAutosaveSession,
  type CanvasRecoveryReview,
} from "./autosave-session";
import { Button } from "@/components/ui/button";
import { renameCanvas } from "@/api/canvases";
import type { Operation } from "./service-document";
import { getSession } from "@/api/identity";
import { revokeSession, useOptionalSession } from "@/components/auth/session";
import { CanvasAccountContext } from "./runtime/components/account/account-provider";
import { CanvasMediaContext } from "./lib/canvas-media-context";

export type ProductionCanvasService = {
  document: CanvasEditorState;
  canvasID: string;
  projectID: string;
  revision: string;
  name: string;
  status: string;
  writable: boolean;
  canSubmit?: boolean;
  viewStateKey: string;
  commit: (
    operations: Operation[],
    revision: string,
    strictBase?: boolean,
  ) => Promise<{ document: CanvasEditorState; revision: string }>;
  undo: {
    canUndo: boolean;
    canRedo: boolean;
    undo: () => void;
    redo: () => void;
  };
};
function Editor({ service }: { service: ProductionCanvasService }) {
  const [review, setReview] = useState<CanvasRecoveryReview | null>(null);
  const [name, setName] = useState(service.name);
  const [queue] = useState(() => new CanvasAutosaveSession(service));
  const saving = useSyncExternalStore(
    queue.subscribe,
    queue.getSnapshot,
    queue.getSnapshot,
  );
  useEffect(() => {
    queue.activate();
    return () => queue.dispose();
  }, [queue]);
  useEffect(() => {
    queue.receive(
      service.document,
      service.revision,
      service.writable,
      service.canSubmit ?? service.writable,
    );
  }, [
    queue,
    service.document,
    service.revision,
    service.writable,
    service.canSubmit,
  ]);
  useEffect(() => {
    if (!saving.dirty) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [saving.dirty]);
  const labels = {
    saved: "已保存",
    editing: "正在编辑，尚未保存",
    waiting: "有未保存修改",
    saving: "等待保存确认",
    uploading: "素材待处理",
    recovery: "输入已保留，需处理",
  };
  return (
    <CanvasMediaContext.Provider
      value={{
        canvasId: service.canvasID,
        bindingRevision: service.revision,
        projectId: service.projectID,
      }}
    >
      <div className="canvas-workspace relative h-full">
        <CanvasPage
          documentIntegration={{
            projectId: service.projectID,
            initialState: saving.replacement,
            viewStateKey: service.viewStateKey,
            mediaContext: {
              canvasId: service.canvasID,
              bindingRevision: service.revision,
              projectId: service.projectID,
            },
            mediaEnabled: true,
            canNavigate: !saving.dirty,
            frozen: !saving.writable,
            admit: (next) => {
              if (queue.canAccept(next)) return true;
              toast.error("暂时无法接纳这次编辑，请等待已有修改保存后再试。", {
                id: "canvas-admission",
              });
              return false;
            },
            onChange: (next, stable) => queue.edit(next, stable),
            undo: saving.dirty ? undefined : service.undo,
            toolbar: (controls) => (
              <CanvasTopBar
                {...controls}
                title={name}
                canDeleteCanvas={false}
                canSwitchCanvas={!saving.dirty}
                onDeleteProject={() => {}}
                onRenameProject={() => {
                  if (!saving.writable) return;
                  const next = window.prompt("画布名称", name)?.trim();
                  if (!next || next === name) return;
                  void renameCanvas({ id: service.canvasID }, { name: next })
                    .then((doc) => setName(doc.name ?? next))
                    .catch((cause) =>
                      toast.error(
                        cause instanceof Error ? cause.message : "重命名失败",
                      ),
                    );
                }}
                saveControl={
                  <div className="flex items-center gap-2">
                    <span role="status" className="text-xs">
                      {labels[saving.phase]}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {service.status}
                    </span>
                    {saving.phase === "uploading" && saving.error && (
                      <Button size="sm" onClick={() => queue.retry()}>
                        重试素材
                      </Button>
                    )}
                    {saving.phase === "recovery" && (
                      <Button
                        size="sm"
                        onClick={() => {
                          try {
                            setReview(queue.reviewRecovery());
                          } catch (cause) {
                            toast.error(
                              cause instanceof Error
                                ? cause.message
                                : "无法重新应用，请保留本页输入",
                            );
                          }
                        }}
                      >
                        查看差异
                      </Button>
                    )}
                    {saving.phase === "recovery" && (
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          if (
                            window.confirm(
                              "放弃本页未确认输入，并采用服务器已接受的内容？",
                            )
                          )
                            queue.useAccepted();
                        }}
                      >
                        采用服务器版本
                      </Button>
                    )}
                  </div>
                }
              />
            ),
          }}
        />
        {review && (
          <CanvasRecoveryDialog
            review={review}
            onClose={() => setReview(null)}
            onApply={() => {
              try {
                queue.applyRecovery(review);
                setReview(null);
              } catch (cause) {
                setReview(null);
                toast.error(
                  cause instanceof Error ? cause.message : "恢复未完成",
                );
              }
            }}
          />
        )}
        {saving.error && (
          <div
            role="alert"
            className="text-destructive-foreground absolute top-16 right-4 z-50 max-w-sm rounded-lg bg-destructive p-3 text-sm"
          >
            {saving.error}
          </div>
        )}
      </div>
    </CanvasMediaContext.Provider>
  );
}
export function CanvasEditor({
  service,
}: {
  service: ProductionCanvasService;
}) {
  const { resolvedTheme } = useTheme();
  const session = useOptionalSession();
  const [store] = useState(() =>
    makeAppStore(resolvedTheme === "dark" ? "dark" : "light"),
  );
  return (
    <Provider store={store}>
      <CanvasAccountContext.Provider
        value={{
          account: session
            ? { id: session.actor.id, userName: session.actor.display_name }
            : null,
          ready: !!session,
          refresh: async () => {
            await getSession();
            return undefined;
          },
          adopt: () => {
            throw new Error("请通过当前项目登录入口更新会话");
          },
          leave: async () => {
            if (session) await revokeSession(session);
          },
          signOut: async () => {
            if (session) await revokeSession(session);
          },
          signingOut: false,
        }}
      >
        <AppFeedbackProvider>
          <Editor service={service} />
        </AppFeedbackProvider>
      </CanvasAccountContext.Provider>
    </Provider>
  );
}
