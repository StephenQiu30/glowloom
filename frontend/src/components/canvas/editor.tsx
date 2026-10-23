"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { Provider } from "react-redux";
import { useTheme } from "next-themes";
import { toast } from "sonner";
import { makeAppStore } from "./runtime/store/app-store";
import { AppFeedbackProvider } from "./runtime/components/feedback/app-feedback-provider";
import CanvasPage from "./editor/canvas-editor";
import { CanvasTopBar } from "./controls/canvas-top-bar";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";
import { serviceGraphOperations } from "./service-graph";
import { Button } from "@/components/ui/button";
import { renameCanvas } from "@/api/canvases";
import type { Operation } from "./service-document";
import { getSession } from "@/api/identity";
import { publishCanvasMedia, type CanvasMediaReceipt } from "./publish-media";
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
  viewStateKey: string;
  commit: (
    operations: Operation[],
    revision: string,
  ) => Promise<{ document: CanvasEditorState; revision: string }>;
  undo: {
    canUndo: boolean;
    canRedo: boolean;
    undo: () => void;
    redo: () => void;
  };
};
function Editor({ service }: { service: ProductionCanvasService }) {
  const [state, setState] = useState(service.document);
  const [name, setName] = useState(service.name);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const draft = useRef(service.document);
  const accepted = useRef(service.document);
  const pending = useRef(false);
  const baseRevision = useRef(service.revision);
  const receipts = useRef(new Map<string, CanvasMediaReceipt>());
  const upload = useRef<AbortController | null>(null);
  useEffect(() => () => upload.current?.abort(), []);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!pending.current) {
      accepted.current = service.document;
      baseRevision.current = service.revision;
      setState(service.document);
      draft.current = service.document;
    }
  }, [service.document, service.revision]);
  const change = useCallback((next: CanvasEditorState) => {
    draft.current = next;
    try {
      const changed = serviceGraphOperations(accepted.current, next).length > 0;
      pending.current = changed;
      setDirty(changed);
      setError("");
    } catch {
      pending.current = true;
      setDirty(true);
      setError("");
    }
  }, []);
  async function save() {
    if (saving || !service.writable) return;
    setSaving(true);
    setError("");
    try {
      const abort = new AbortController();
      upload.current = abort;
      const published = await publishCanvasMedia(
        draft.current,
        service.projectID,
        receipts.current,
        abort.signal,
      );
      draft.current = published;
      const operations = serviceGraphOperations(accepted.current, published);
      if (operations.length) {
        const saved = await service.commit(operations, baseRevision.current);
        accepted.current = saved.document;
        baseRevision.current = saved.revision;
        draft.current = saved.document;
        setState(saved.document);
      }
      pending.current = false;
      setDirty(false);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "保存未完成，修改仍在本页",
      );
    } finally {
      setSaving(false);
    }
  }
  useEffect(() => {
    if (!dirty) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [dirty]);
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
            initialState: state,
            viewStateKey: service.viewStateKey,
            mediaContext: {
              canvasId: service.canvasID,
              bindingRevision: service.revision,
              projectId: service.projectID,
            },
            mediaEnabled: true,
            canNavigate: !dirty && !saving,
            frozen: !service.writable || saving,
            onChange: change,
            undo: dirty ? undefined : service.undo,
            toolbar: (controls) => (
              <CanvasTopBar
                {...controls}
                title={name}
                canDeleteCanvas={false}
                canSwitchCanvas={!dirty && !saving}
                onDeleteProject={() => {}}
                onRenameProject={() => {
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
                      {saving
                        ? "正在保存"
                        : dirty
                          ? "有未保存修改"
                          : service.status}
                    </span>
                    <Button
                      size="sm"
                      disabled={!dirty || saving || !service.writable}
                      onClick={() => void save()}
                    >
                      保存
                    </Button>
                  </div>
                }
              />
            ),
          }}
        />
        {error && (
          <div
            role="alert"
            className="text-destructive-foreground absolute top-16 right-4 z-50 max-w-sm rounded-lg bg-destructive p-3 text-sm"
          >
            {error}
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
