"use client";
import { Activity, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import * as api from "@/components/canvas/runtime/api/canvas-document-bff";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import useSWR from "swr";
import { getPersonalProjectWorkspace } from "@/components/canvas/runtime/api/personal-workspace-bff";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  PageError,
  PageLoading,
} from "@/components/canvas/runtime/components/feedback/page-state";
import {
  CanvasIdentityLock,
  useCanvasIdentity,
} from "../hooks/use-canvas-identity";
import dynamic from "next/dynamic";
import { CanvasTopBar } from "../controls/canvas-top-bar";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@/components/ui/popover";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import type { CanvasDocumentIntegration } from "../types/canvas-document-integration";
const CanvasEditor = dynamic(() => import("./canvas-editor"), {
  ssr: false,
  loading: () => <PageLoading label="正在打开画布" />,
});
import {
  fromCanvasGraph,
  toCanvasContentGraph,
} from "../lib/canvas-document-adapter";
import { CanvasMediaContext } from "../lib/canvas-media-context";
import { canvasViewKey } from "../lib/canvas-view-state";
import {
  useCanvasDocument,
  type CanvasScope,
} from "../hooks/use-canvas-document";
import { useCanvasSave } from "../hooks/use-canvas-save";
import { useCanvasGeneration } from "../hooks/use-canvas-generation";
import { useCanvasWorkflow } from "../hooks/use-canvas-workflow";
import { CanvasWorkflowControls } from "../controls/canvas-workflow-controls";
import { CreatePersonalProjectDialog } from "../project/personal-projects";
import { CanvasRecoveryManager } from "./canvas-recovery-manager";
import { CanvasFieldRecovery } from "./canvas-field-recovery";
import { CanvasTextDraftRecovery } from "./canvas-text-draft-recovery";

export function CanvasDocumentPage({
  scope,
  canvasId,
  expectedProjectId,
}: {
  scope: CanvasScope;
  canvasId: string;
  expectedProjectId?: string;
}) {
  return (
    <DocumentLoader
      key={scope + ":" + canvasId + ":" + (expectedProjectId ?? "")}
      scope={scope}
      canvasId={canvasId}
      expectedProjectId={expectedProjectId}
    />
  );
}
function DocumentLoader({
  scope,
  canvasId,
  expectedProjectId,
}: {
  scope: CanvasScope;
  canvasId: string;
  expectedProjectId?: string;
}) {
  const session = useCanvasIdentity(scope);
  const query = useCanvasDocument(
    scope,
    canvasId,
    session.identity,
    expectedProjectId,
  );
  const [loaded, setLoaded] = useState<{
    document: CanvasContract.CanvasVO;
    identity: string;
  } | null>(null);
  if (
    !loaded &&
    query.data &&
    query.qualified &&
    session.enabled &&
    session.identity &&
    (!expectedProjectId || String(query.data.projectId) === expectedProjectId)
  )
    setLoaded({ document: query.data, identity: session.identity });
  if (!loaded && !session.enabled)
    return <CanvasIdentityLock hasDraft={false} />;
  if (!loaded && query.error)
    return (
      <>
        <PageError error={query.error} reset={() => void query.mutate()} />
        <CanvasRecoveryManager />
      </>
    );
  if (
    !loaded &&
    query.data &&
    expectedProjectId &&
    String(query.data.projectId) !== expectedProjectId
  )
    return (
      <PageError
        error={new Error("项目画布关联尚未确认，请重新读取")}
        reset={() => void query.mutate()}
      />
    );
  if (!loaded) return <PageLoading />;
  try {
    fromCanvasGraph(loaded.document.graph);
  } catch (error) {
    return (
      <p role="status" className="p-6">
        {error instanceof Error ? error.message : "无法读取画布"}
      </p>
    );
  }
  return (
    <CanvasDocumentSession
      key={loaded.identity + ":" + canvasId}
      scope={scope}
      canvasId={canvasId}
      identity={loaded.identity}
      enabled={
        session.enabled &&
        query.qualified &&
        (!loaded.document.access?.edit || !!query.data?.access?.edit)
      }
      initialDocument={loaded.document}
      onAccessInvalidated={query.invalidate}
      onRequalify={query.requalify}
      identityEnabled={session.enabled}
      qualifying={query.isLoading}
    />
  );
}
function CanvasDocumentSession({
  scope,
  canvasId,
  identity,
  initialDocument,
  enabled,
  onAccessInvalidated,
  onRequalify,
  identityEnabled,
  qualifying,
}: {
  scope: CanvasScope;
  canvasId: string;
  identity: string;
  initialDocument: CanvasContract.CanvasVO;
  enabled: boolean;
  onAccessInvalidated: () => void;
  onRequalify: () => void;
  identityEnabled: boolean;
  qualifying: boolean;
}) {
  const router = useRouter();
  const save = useCanvasSave({
    scope,
    canvasId,
    identity,
    initialDocument,
    enabled,
    onAccessInvalidated,
  });
  const { state } = save;
  const generation = useCanvasGeneration({
    canvasId,
    identity,
    enabled: enabled && scope === "USER",
    prepare: save.prepareGeneration,
  });
  const [dialog, setDialog] = useState<
    "rename" | "delete" | "conflict" | "discardForm" | null
  >(null);
  const workflow = useCanvasWorkflow({
    canvasId,
    identity,
    enabled: identityEnabled && scope === "USER",
    canStart: enabled,
    prepare: save.prepareGeneration,
  });
  const [discardFormNode, setDiscardFormNode] = useState<string | null>(null);
  const [title, setTitle] = useState(initialDocument.title);
  const [server, setServer] = useState<CanvasContract.CanvasVO | null>(null);
  const [readError, setReadError] = useState("");
  const [creatingProject, setCreatingProject] = useState(false);
  const [projectCreationMounted, setProjectCreationMounted] = useState(false);
  const [bindingPending, setBindingPending] = useState(false);
  const root = state.baseline.projectId
    ? `/projects/${state.baseline.projectId}`
    : "/projects";
  const editorGraph = state.editorGraph ?? state.baseline.graph;
  const editor = useMemo(() => fromCanvasGraph(editorGraph), [editorGraph]);
  const readServer = async () => {
    try {
      setServer(
        unwrap(
          await (
            scope === "LOCAL" ? api.localGetCanvas : api.getCanvasDocument
          )({ canvasId }),
        ),
      );
      setDialog("conflict");
    } catch (error) {
      setReadError(error instanceof Error ? error.message : "读取失败");
    }
  };
  const exportDraft = () => {
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(save.exportDraft(), null, 2)], {
        type: "application/json",
      }),
    );
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "画布草稿-" + canvasId + ".json";
    anchor.click();
    URL.revokeObjectURL(url);
  };
  const clean =
    state.status === "CLEAN" &&
    !bindingPending &&
    !save.restoring &&
    !save.recoveryPending &&
    !save.recoveryBusy &&
    !state.invalidDraft;
  const recoveryIssue = save.recoverySources.length
    ? "有保留输入待逐项处理，未选择的内容会继续保留"
    : save.recoveryChoices.length
      ? "此设备有多份未同步的修改，请选择要恢复的草稿"
      : save.sealedInput && state.status === "READ_ONLY"
        ? "保留输入尚未同步，可导出或在恢复编辑资格后逐项处理"
        : state.status === "CONFLICT"
          ? "画布已在其他页面更新，请选择继续使用的内容"
          : state.status === "FAILED"
            ? state.message || "修改未能同步，请检查后继续"
            : save.connectionStatus === "oversized"
              ? "画布内容超过限制，请保留草稿并调整内容"
              : save.connectionStatus === "blocked"
                ? "画布连接暂时无法恢复，请重试连接"
                : save.recoveryRequired
                  ? "修改暂未确认，请重试同步；当前草稿已保留"
                  : save.localPersistence === "UNAVAILABLE" &&
                      state.status !== "CLEAN"
                    ? "此设备无法保留未同步修改，请保持页面打开并检查浏览器存储"
                    : "";
  useEffect(() => {
    if (readError || recoveryIssue)
      notifyError(
        state.errorCode && recoveryIssue
          ? new ApiError(recoveryIssue, state.errorCode, state.requestId)
          : new Error(readError || recoveryIssue),
        "画布修改需要处理",
      );
  }, [recoveryIssue, state.errorCode, state.requestId, readError]);
  const ownerProject = useSWR(
    enabled && state.baseline.projectId
      ? ["canvas-owner-project", identity, String(state.baseline.projectId)]
      : null,
    async () =>
      unwrap(
        await getPersonalProjectWorkspace({
          projectId: state.baseline.projectId!,
        }),
      ),
    { shouldRetryOnError: false },
  );
  const toolbar: CanvasDocumentIntegration["toolbar"] = (controls) => (
    <CanvasTopBar
      {...controls}
      title={state.baseline.title}
      workflowControl={
        <CanvasWorkflowControls
          workflow={workflow}
          allNodeIds={state.draft.nodes
            .filter((node) => node.type !== "group")
            .map((node) => node.id)}
        />
      }
      canDeleteCanvas={!state.baseline.projectId}
      projectCanvas={!!state.baseline.projectId}
      projectPath={
        state.baseline.projectId
          ? (ownerProject.data?.breadcrumbs
              ?.map((item) => item.title)
              .join(" / ") ??
            (ownerProject.error ? "所属项目暂不可读取" : "正在读取所属项目"))
          : undefined
      }
      canSwitchCanvas={clean}
      onProjectCanvases={
        state.baseline.projectId
          ? () => {
              if (clean) router.push(root);
            }
          : undefined
      }
      onCreatePersonalProject={
        !state.baseline.projectId && (clean || bindingPending)
          ? () => {
              setProjectCreationMounted(true);
              setCreatingProject(true);
            }
          : undefined
      }
      onRenameProject={() => {
        if (clean) setDialog("rename");
      }}
      onDeleteProject={() => {
        if (clean) setDialog("delete");
      }}
      saveControl={
        !enabled ? null : recoveryIssue ? (
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="sm" aria-label="处理未同步修改">
                处理未同步修改
              </Button>
            </PopoverTrigger>
            <PopoverContent
              data-canvas-no-zoom
              align="end"
              className="flex max-h-[70vh] flex-col gap-2 overflow-y-auto"
            >
              <p role="status" className="text-sm">
                {recoveryIssue}
              </p>
              {Object.entries(save.pendingTextDrafts).map(
                ([nodeId, document]) => {
                  const node = state.draft.nodes.find(
                    (value) => value.id === nodeId,
                  );
                  return (
                    <CanvasTextDraftRecovery
                      key={nodeId}
                      nodeId={nodeId}
                      title={node?.title ?? "已删除节点"}
                      document={document}
                      current={node?.textDocument ?? null}
                      busy={
                        !node ||
                        save.recoveryBusy ||
                        save.recoveryPending ||
                        save.restoring ||
                        ["READ_ONLY", "UNKNOWN", "SAVING", "CONFLICT"].includes(
                          state.status,
                        )
                      }
                      apply={save.applyTextDraft}
                    />
                  );
                },
              )}
              {Object.keys(save.pendingTextDrafts).length > 0 && (
                <Button variant="outline" onClick={exportDraft}>
                  导出保留正文与画布
                </Button>
              )}
              {save.recoveryChoices.map((record, index) => (
                <div
                  key={record.scope.epoch + ":" + record.scope.sessionId}
                  className="grid gap-2"
                >
                  <p className="text-sm">
                    草稿 {index + 1} · {record.snapshot.graph.nodes.length}{" "}
                    个节点 · {new Date(record.updatedAt).toLocaleString()}
                  </p>
                  {!record.sealed && (
                    <Button
                      disabled={save.recoveryBusy}
                      onClick={() =>
                        void save
                          .chooseRecovery(record)
                          .catch((error) => notifyError(error, "恢复未完成"))
                      }
                    >
                      恢复此设备草稿
                    </Button>
                  )}
                  {record.sealed && (
                    <>
                      <p className="text-sm">
                        这些输入需要单独处理，已保留可读稿。
                      </p>
                      <Button variant="outline" onClick={exportDraft}>
                        导出未同步输入
                      </Button>
                      <Button
                        variant="outline"
                        disabled={
                          save.recoveryBusy ||
                          !!record.inFlight ||
                          !!record.control
                        }
                        onClick={() =>
                          void save
                            .beginFieldRecovery(record)
                            .catch((error) =>
                              notifyError(error, "逐项处理尚未开始"),
                            )
                        }
                      >
                        逐项选择保留输入
                      </Button>
                      {(record.inFlight || record.control) && (
                        <Button
                          variant="outline"
                          onClick={() =>
                            void save
                              .resolveUnknown()
                              .catch((error) =>
                                notifyError(error, "原结果仍需确认"),
                              )
                          }
                        >
                          查询原同步结果
                        </Button>
                      )}
                    </>
                  )}
                  <Button
                    variant="outline"
                    disabled={save.recoveryBusy}
                    onClick={() =>
                      void save
                        .discardRecovery(record)
                        .catch((error) => notifyError(error, "放弃草稿未完成"))
                    }
                  >
                    放弃此设备草稿
                  </Button>
                </div>
              ))}
              {state.status === "CONFLICT" && (
                <Button
                  variant="outline"
                  disabled={save.recoveryBusy || save.recoveryRequired}
                  onClick={() =>
                    void save
                      .beginFieldRecovery()
                      .catch((error) => notifyError(error, "逐项处理尚未开始"))
                  }
                >
                  逐项选择当前保留输入
                </Button>
              )}
              {save.recoverySources.map((source) => (
                <CanvasFieldRecovery
                  key={source.id}
                  source={source}
                  busy={
                    save.recoveryBusy ||
                    state.status === "UNKNOWN" ||
                    state.status === "SAVING" ||
                    state.status === "READ_ONLY"
                  }
                  apply={save.applyRecoveryFields}
                  discard={save.discardRecoverySource}
                />
              ))}
              {(save.localPersistence === "UNAVAILABLE" ||
                save.recoveryChoices.length > 0) && <CanvasRecoveryManager />}
              {Object.keys(save.pendingForms).length > 0 && (
                <p role="status" className="text-sm">
                  存在未应用参数，可打开对应节点应用，或明确放弃这些输入。
                </p>
              )}
              {Object.keys(save.pendingForms).map((nodeId) => (
                <div key={nodeId} className="grid gap-2">
                  <p className="text-sm">
                    {state.draft.nodes.find((node) => node.id === nodeId)
                      ?.title ?? "已删除节点"}{" "}
                    · 未应用参数
                  </p>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setDiscardFormNode(nodeId);
                      setDialog("discardForm");
                    }}
                  >
                    放弃未应用参数
                  </Button>
                </div>
              ))}
              {["oversized", "blocked"].includes(save.connectionStatus) && (
                <Button variant="outline" onClick={save.recoverConnection}>
                  重试连接
                </Button>
              )}
              {(state.status === "UNKNOWN" || state.status === "READ_ONLY") &&
                save.recoveryRequired && (
                  <>
                    <Button
                      variant="outline"
                      onClick={() => void save.resolveUnknown()}
                    >
                      检查同步结果
                    </Button>
                    <Button
                      variant="outline"
                      disabled={state.status === "READ_ONLY"}
                      onClick={() => void save.retryOriginal()}
                    >
                      重试同步
                    </Button>
                  </>
                )}
              {state.status === "READ_ONLY" && state.baseline.access?.edit && (
                <Button variant="outline" onClick={save.resumeIdentity}>
                  恢复编辑
                </Button>
              )}
              {state.status === "READ_ONLY" && save.sealedInput && (
                <Button variant="outline" onClick={exportDraft}>
                  导出保留输入
                </Button>
              )}
              {state.status === "FAILED" && (
                <Button variant="outline" onClick={save.resumeAfterFix}>
                  修改后继续同步
                </Button>
              )}
              {state.status === "CONFLICT" && (
                <>
                  <Button variant="outline" onClick={() => void readServer()}>
                    查看服务端版本
                  </Button>
                  <Button variant="outline" onClick={exportDraft}>
                    导出当前草稿
                  </Button>
                </>
              )}
            </PopoverContent>
          </Popover>
        ) : (
          <span
            role="status"
            aria-label="画布保存状态"
            className="px-2 py-1 text-xs text-muted-foreground"
          >
            {save.connectionStatus !== "connected"
              ? save.connectionStatus === "connecting"
                ? "正在连接"
                : "连接已断开，等待同步"
              : state.status === "SAVING"
                ? "正在保存"
                : clean
                  ? "已保存"
                  : "修改待同步"}
          </span>
        )
      }
    />
  );
  return (
    <>
      {!enabled &&
        (identityEnabled ? (
          <section
            role="status"
            className="flex flex-col items-center justify-center gap-4 p-6"
          >
            <p>画布访问已暂停，草稿、未应用参数和原命令已保留。</p>
            <CanvasWorkflowControls workflow={workflow} allNodeIds={[]} />
            <Button
              variant="outline"
              disabled={qualifying}
              onClick={onRequalify}
            >
              {qualifying ? "正在验证画布权限" : "重新验证画布权限"}
            </Button>
          </section>
        ) : (
          <CanvasIdentityLock />
        ))}
      <Activity mode={enabled ? "visible" : "hidden"}>
        <CanvasMediaContext
          value={
            scope === "USER" && state.baseline
              ? {
                  canvasId,
                  bindingRevision: state.baseline.projectBindingRevision,
                }
              : undefined
          }
        >
          <CanvasEditor
            key={state.generation}
            documentIntegration={{
              initialState: editor,
              mediaEnabled:
                enabled &&
                scope === "USER" &&
                state.baseline?.access?.read === true,
              mediaContext:
                scope === "USER" && state.baseline
                  ? {
                      canvasId,
                      bindingRevision: state.baseline.projectBindingRevision,
                    }
                  : undefined,
              textBinding: save.textBinding,
              undo: save.undo,
              viewStateKey: canvasViewKey(identity, scope, canvasId),
              projectId: state.baseline?.projectId
                ? String(state.baseline.projectId)
                : undefined,
              pendingForms: save.pendingForms,
              onPendingForm: save.setPendingForm,
              generation: scope === "USER" ? generation : undefined,
              workflow: scope === "USER" ? workflow.integration : undefined,
              toolbar,
              canNavigate: clean,
              frozen:
                bindingPending ||
                save.textRecoveryPending ||
                save.recoveryPending ||
                save.recoveryBusy ||
                save.restoring ||
                state.status === "READ_ONLY" ||
                Boolean(
                  state.inFlight &&
                  ["RESTORE", "DELETE"].includes(state.inFlight.operation),
                ),
              onChange: (value, stable) => {
                try {
                  save.updateDraft(
                    toCanvasContentGraph(value, state.draft),
                    stable,
                  );
                } catch (error) {
                  save.draftError(
                    error instanceof Error ? error.message : "草稿无法保存",
                  );
                }
              },
            }}
          />
        </CanvasMediaContext>
        {projectCreationMounted && (
          <CreatePersonalProjectDialog
            open={creatingProject}
            onOpenChange={setCreatingProject}
            identity={identity}
            source={state.baseline}
            onStarted={() => setBindingPending(true)}
            onAborted={() => setBindingPending(false)}
          />
        )}
        <Dialog
          open={dialog !== null}
          onOpenChange={(open) => {
            if (!open) setDialog(null);
          }}
        >
          <DialogContent className="max-h-[80dvh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>
                {dialog === "rename"
                  ? "重命名画布"
                  : dialog === "delete"
                    ? "删除画布"
                    : dialog === "conflict"
                      ? "选择要继续使用的版本"
                      : dialog === "discardForm"
                        ? "放弃未应用参数？"
                        : "画布操作"}
              </DialogTitle>
              <DialogDescription>
                {dialog === "conflict"
                  ? "当前草稿会保留，采用服务端版本前可先导出草稿。"
                  : dialog === "discardForm"
                    ? "仅放弃此节点尚未应用的参数输入；当前画布的节点、内容和删除操作保持不变。其他节点的未应用参数继续保留。"
                    : "操作将按当前内容和关联版本提交。"}
              </DialogDescription>
            </DialogHeader>
            {dialog === "discardForm" && (
              <div className="flex gap-2">
                <Button variant="outline" onClick={() => setDialog(null)}>
                  继续保留
                </Button>
                <Button
                  onClick={() => {
                    if (discardFormNode)
                      save.setPendingForm(discardFormNode, undefined);
                    setDiscardFormNode(null);
                    setDialog(null);
                  }}
                >
                  确认放弃参数
                </Button>
              </div>
            )}
            {dialog === "rename" && (
              <form
                onSubmit={async (e) => {
                  e.preventDefault();
                  if (await save.runCommand("EDIT", { title })) setDialog(null);
                }}
              >
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="canvas-rename-title">
                      新的画布名称
                    </FieldLabel>
                    <Input
                      id="canvas-rename-title"
                      aria-label="新的画布名称"
                      maxLength={100}
                      value={title}
                      onChange={(e) => setTitle(e.target.value)}
                    />
                  </Field>
                  <Button disabled={!clean}>保存名称</Button>
                </FieldGroup>
              </form>
            )}
            {dialog === "delete" && (
              <Button
                variant="destructive"
                disabled={!clean}
                onClick={async () => {
                  if (await save.runCommand("DELETE")) router.push(root);
                }}
              >
                确认删除
              </Button>
            )}
            {dialog === "conflict" && server && (
              <div className="space-y-3">
                <p>
                  {server.title} · 服务端版本 {String(server.revision)} ·{" "}
                  {server.graph.nodes.length} 个节点
                </p>
                <Button variant="outline" onClick={exportDraft}>
                  导出当前草稿
                </Button>
                <Button
                  onClick={async () => {
                    try {
                      await save.adoptServer();
                      setDialog(null);
                    } catch (error) {
                      notifyError(error, "放弃草稿未完成");
                    }
                  }}
                >
                  放弃当前草稿，采用服务端版本
                </Button>
              </div>
            )}
          </DialogContent>
        </Dialog>
      </Activity>
    </>
  );
}
