"use client";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { toast } from "@/components/feedback/notification";
import dynamic from "next/dynamic";
import { createCanvasAssistantAdoption } from "./assistant/canvas-assistant-adoption";
import { createCanvasDomainConnector } from "./lib/canvas-domain-connection";
import { createCanvasAudioConnector } from "./lib/canvas-audio-connection";
import { previewCanvasAudioConnection } from "@/components/canvas-editor/lib/canvas-audio-connection-preview.ts";
import { useCanvasAudioModelCatalog } from "./hooks/use-canvas-audio-model-catalog";
import { useCanvasAudioSubjectCapabilities } from "./hooks/use-canvas-audio-subject-capabilities";
import { canvasAudioCatalogAcceptsSubjects } from "@/components/canvas-editor/lib/canvas-audio-connection-capability.ts";
import type { CanvasNodeData } from "@/components/canvas-editor/types/canvas.ts";
import type { CanvasClipboard } from "@/components/canvas-editor/lib/canvas-clipboard.ts";
import { CanvasYjsLiveSession } from "./yjs-live-session";
import { CanvasGenerationPanel } from "@/components/canvas-editor/nodes/canvas-generation-panel.tsx";
import { CanvasOperationStatus } from "@/components/canvas-editor/nodes/canvas-operation-status.tsx";
import { useCanvasOperationTasks } from "./hooks/use-canvas-operation-tasks";
import {
  applyCanvasOperationResult,
  canvasOperationTarget,
} from "./lib/canvas-operation-result";
import type { Task } from "@/components/generation/contracts";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Skeleton } from "@/components/ui/skeleton";
import { ContentState } from "@/components/layout/content-state";
import { CanvasParticipants } from "@/components/canvas-editor/canvas-participants.tsx";
import { SpaceSwitcher } from "@/components/spaces/space-context";
import { YjsPresence } from "@/components/canvas-editor/yjs-presence.tsx";
import {
  CanvasConnectionState,
  canvasConnectionMessage,
} from "@/components/canvas-editor/connection-state.tsx";
import { CanvasProviders } from "./editor";
import CanvasPage from "@/components/canvas-editor/editor/canvas-editor.tsx";
import { CanvasTopBar } from "@/components/canvas-editor/controls/canvas-top-bar.tsx";
import { CanvasMediaContext } from "@/components/canvas-editor/lib/canvas-media-context.ts";
import { canvasViewKey } from "@/components/canvas-editor/view-state.ts";
import { canvasSubjectSources } from "./lib/canvas-subject-source";
import { getCSRFToken } from "@/lib/request";
import { useIsMobile } from "@/hooks/use-mobile";
import { ensureSessionFresh } from "@/components/auth/session-renewal";
import { CanvasAccountControls } from "./canvas-account-controls";
import { Button } from "@/components/ui/button";
import { CanvasLocalDrafts } from "@/components/canvas-editor/controls/canvas-local-drafts.tsx";
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
import { getProject } from "@/api/projects";
import {
  isProjectID,
  requireProject,
  type Project,
} from "@/components/projects/project";
import { ProjectRenameDialog } from "@/components/projects/project-rename-dialog";
import { ProjectMediaPreview } from "@/components/media/project-media";

const ReferenceAssetsPanel = dynamic(
  () =>
    import("@/app/assets/_components/assets-page").then(
      (module) => module.ReferenceAssetsPanel,
    ),
  { loading: () => <ContentState state="loading" title="正在读取参考素材" /> },
);

const CanvasTransferDialog = dynamic(() =>
  import("@/components/canvas-editor/editor/canvas-transfer-dialog.tsx").then(
    (m) => m.CanvasTransferDialog,
  ),
);
const CanvasTransferReceiver = dynamic(() =>
  import("@/components/canvas-editor/editor/canvas-transfer-dialog.tsx").then(
    (m) => m.CanvasTransferReceiver,
  ),
);

const CanvasTimelineWorkspace = dynamic(
  () =>
    import("@/components/canvas-editor/timeline/canvas-timeline-workspace.tsx").then(
      (module) => module.CanvasTimelineWorkspace,
    ),
  {
    loading: () => (
      <Skeleton className="size-full" aria-label="正在打开时间线…" />
    ),
  },
);
const CanvasSubjectNode = dynamic(
  () =>
    import("@/components/canvas-editor/subject/canvas-subject-node.tsx").then(
      (module) => module.CanvasSubjectNode,
    ),
  {
    loading: () => (
      <Skeleton className="size-full" aria-label="正在打开主体…" />
    ),
  },
);

const CanvasCinemaKitNode = dynamic(
  () =>
    import("@/components/canvas-editor/cinema-kit/canvas-cinema-kit.tsx").then(
      (m) => m.CanvasCinemaKitNode,
    ),
  {
    loading: () => (
      <Skeleton className="size-full" aria-label="正在打开导演工具箱" />
    ),
  },
);
const CanvasDirectorWorkspace = dynamic(
  () =>
    import("@/components/canvas-editor/director/canvas-director-workspace.tsx").then(
      (module) => module.CanvasDirectorWorkspace,
    ),
  {
    loading: () => (
      <Skeleton className="size-full" aria-label="正在打开导演台…" />
    ),
  },
);
const CanvasAssistantPanel = dynamic(
  () =>
    import("@/components/canvas-editor/assistant/canvas-assistant-panel.tsx").then(
      (module) => module.CanvasAssistantPanel,
    ),
  {
    loading: () => (
      <Skeleton
        className="absolute top-4 right-4 h-12 w-72"
        aria-label="正在打开助手"
      />
    ),
  },
);
const CanvasConversationPanel = dynamic(() =>
  import("./assistant/canvas-conversation-panel").then(
    (module) => module.CanvasConversationPanel,
  ),
);

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
  const operations = useCanvasOperationTasks({
    projectID,
    canvasID,
    identity: `${actorID}/${scopeID}`,
  });
  const { register: registerTask, complete: completeTask } = operations;
  const operationTargets = useRef(new Map<string, string>());
  const registerOperation = useCallback(
    (nodeID: string, operationID: string) => {
      if (!operationTargets.current.has(operationID))
        operationTargets.current.set(
          operationID,
          canvasOperationTarget(client, nodeID),
        );
      registerTask(nodeID, operationID);
    },
    [client, registerTask],
  );
  const adoption = useMemo(
    () => createCanvasAssistantAdoption(client, projectID),
    [client, projectID],
  );
  const domainConnector = useMemo(
    () => createCanvasDomainConnector(client, projectID, scopeID),
    [client, projectID, scopeID],
  );
  useEffect(() => {
    domainConnector.start();
    return () => domainConnector.dispose();
  }, [domainConnector]);
  const { catalog: audioCatalog } = useCanvasAudioModelCatalog({
    projectID,
    scopeID,
  });
  const { prepare: prepareAudioSubjects, pinnedSubjects } =
    useCanvasAudioSubjectCapabilities(
      { projectID, scopeID },
      canvasAudioCatalogAcceptsSubjects(audioCatalog, { projectID, scopeID }),
    );
  const audioConnector = useMemo(
    () => createCanvasAudioConnector(client, projectID, scopeID),
    [client, projectID, scopeID],
  );
  useEffect(() => {
    audioConnector.start();
    return () => audioConnector.dispose();
  }, [audioConnector]);
  const audioConnections = useMemo(
    () => ({
      prepare: prepareAudioSubjects,
      preview: (sources: CanvasNodeData[], target?: CanvasNodeData) =>
        previewCanvasAudioConnection(
          audioCatalog,
          { projectID, scopeID },
          sources,
          target,
          pinnedSubjects,
        ),
      connect: audioConnector.connect,
    }),
    [
      audioCatalog,
      audioConnector,
      prepareAudioSubjects,
      pinnedSubjects,
      projectID,
      scopeID,
    ],
  );
  const isMobile = useIsMobile();
  const [legacyOpen, setLegacyOpen] = useState(false);
  const [conversation, setConversation] = useState<
    "unopened" | "open" | "closed"
  >("unopened");
  const conversationTrigger = useRef<HTMLElement | null>(null);
  function openConversation() {
    if (document.activeElement instanceof HTMLElement)
      conversationTrigger.current = document.activeElement;
    setLegacyOpen(false);
    setConversation("open");
  }
  function closeConversation() {
    setConversation("closed");
    requestAnimationFrame(() => conversationTrigger.current?.focus());
  }
  const [transfer, setTransfer] = useState<CanvasClipboard | null>(null);
  const [rename, setRename] = useState<string | null>(null);
  const [renameProject, setRenameProject] = useState<Project | null>(null);
  const renameRequest = useRef<AbortController | null>(null);
  useEffect(() => () => renameRequest.current?.abort(), []);
  const openRename = async () => {
    if (!view.writable || view.dirty || renameRequest.current) return;
    const controller = new AbortController();
    renameRequest.current = controller;
    try {
      const project = requireProject(
        await getProject({ pid: projectID }, { signal: controller.signal }),
      );
      if (controller.signal.aborted) return;
      if (project.id !== projectID) throw new Error("画布所属项目不匹配");
      // The main canvas name belongs to the project's lifecycle and revision.
      // Sending it as a Yjs title edit is rejected and seals the local input.
      if (project.canvas_id === canvasID) setRenameProject(project);
      else setRename(client.getSnapshot().dto?.name ?? "画布");
    } catch (cause) {
      if (!controller.signal.aborted)
        toast.error(
          cause instanceof Error ? cause.message : "无法读取画布名称",
        );
    } finally {
      if (renameRequest.current === controller) renameRequest.current = null;
    }
  };
  useEffect(() => {
    if (!view.unsafeToLeave) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [view.unsafeToLeave]);
  const completeOperation = useCallback(
    async (nodeID: string, operationID: string, task: Task) => {
      const outcome = await applyCanvasOperationResult(
        client,
        projectID,
        canvasID,
        nodeID,
        task,
        operationTargets.current.get(operationID),
      );
      if (outcome === "superseded")
        toast.info("节点内容已变化，生成结果已保留在历史中");
      operationTargets.current.delete(operationID);
      completeTask(nodeID, operationID);
    },
    [client, projectID, canvasID, completeTask],
  );
  const { document: graph, dto } = view;
  if (!graph || !dto)
    return (
      <CanvasConnectionState
        message={view.message}
        onRetry={client.retry}
        scopeControl={<SpaceSwitcher compact />}
      />
    );
  const ordinaryCanvas = dto.kind !== "short_drama";
  const conversationOpen = conversation === "open";
  return (
    <CanvasMediaContext.Provider
      value={{
        canvasId: canvasID,
        projectId: projectID,
        bindingRevision: dto.revision!,
      }}
    >
      <div className="canvas-workspace relative h-dvh">
        <CanvasTransferReceiver
          client={client}
          canvasID={canvasID}
          projectID={projectID}
          actorID={actorID}
          scopeID={scopeID}
        />
        {transfer && (
          <CanvasTransferDialog
            client={client}
            canvasID={canvasID}
            projectID={projectID}
            actorID={actorID}
            scopeID={scopeID}
            selection={transfer}
            onClose={() => setTransfer(null)}
          />
        )}
        <div className="size-full" inert={conversationOpen && isMobile}>
          <CanvasPage
            documentIntegration={{
              projectId: projectID,
              getDocumentEpoch: client.getDocumentEpoch,
              initialState: graph,
              viewStateKey: canvasViewKey(
                actorID,
                scopeID,
                projectID,
                canvasID,
              ),
              mediaContext: {
                canvasId: canvasID,
                projectId: projectID,
                bindingRevision: dto.revision!,
              },
              mediaEnabled: true,
              renderReferenceAssets: (refreshKey) => (
                <ReferenceAssetsPanel refreshKey={refreshKey} />
              ),
              assistantOpen: ordinaryCanvas ? conversationOpen : undefined,
              onOpenAssistant: ordinaryCanvas
                ? openConversation
                : () => setLegacyOpen(true),
              onTransferSelection: setTransfer,
              canNavigate: !view.unsafeToLeave,
              frozen: !view.writable || view.migration,
              admit: client.canAccept,
              admitConnectionGraph: client.canAcceptConnectionGraph,
              resolveConnectionMedia: client.connectionMediaNode,
              connectDomain: domainConnector.connect,
              audioConnections,
              onChange: client.change,
              textBinding: client.textBinding,
              renderToolNode: (nodeID, type) =>
                type === "storyboard" ? (
                  <ArchivedStoryboardPreview
                    projectID={projectID}
                    storyboard={
                      graph.nodes.find((node) => node.id === nodeID)
                        ?.serviceData?.config?.storyboard
                    }
                  />
                ) : type === "timeline" ? (
                  <CanvasTimelineWorkspace
                    client={client}
                    nodeID={nodeID}
                    canvasID={canvasID}
                    projectID={projectID}
                    compact
                  />
                ) : type === "director" ? (
                  <CanvasDirectorWorkspace
                    client={client}
                    nodeID={nodeID}
                    canvasID={canvasID}
                    projectID={projectID}
                    compact
                  />
                ) : type === "cinema_studio" ? (
                  <CanvasCinemaKitNode client={client} nodeID={nodeID} />
                ) : type === "subject" ? (
                  <CanvasSubjectNode
                    projectID={projectID}
                    client={client}
                    nodeID={nodeID}
                    actorID={actorID}
                    scopeID={scopeID}
                  />
                ) : operations.tasks[nodeID] &&
                  ["text", "image", "video", "audio"].includes(type) ? (
                  <CanvasOperationStatus
                    projectID={projectID}
                    canvasID={canvasID}
                    nodeID={nodeID}
                    operationID={operations.tasks[nodeID]}
                    identity={`${actorID}/${scopeID}`}
                    frozen={!view.writable || view.migration}
                    onStopped={operations.refresh}
                    onCompleted={completeOperation}
                  />
                ) : null,
              renderGenerationPanel: (nodeID, onSubmitted) => (
                <CanvasGenerationPanel
                  client={client}
                  nodeID={nodeID}
                  canvasID={canvasID}
                  projectID={projectID}
                  onOperationReady={registerOperation}
                  onSubmitted={onSubmitted}
                />
              ),
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
                  canvasID={canvasID}
                  projectID={projectID}
                  title={dto.name ?? "画布"}
                  canDeleteCanvas={false}
                  canRenameCanvas={view.writable && !view.dirty}
                  canSwitchCanvas={!view.unsafeToLeave}
                  renderAccount={
                    ordinaryCanvas
                      ? (compact) => (
                          <CanvasAccountControls
                            compact={compact}
                            scopeID={scopeID}
                            canNavigate={!view.unsafeToLeave}
                            onHome={controls.onHome}
                          />
                        )
                      : undefined
                  }
                  presenceConnected={view.connected}
                  onDeleteProject={() => {}}
                  onRenameProject={() => void openRename()}
                  participantsControl={
                    <CanvasParticipants
                      client={client}
                      actorID={actorID}
                      scopeID={scopeID}
                      connected={view.connected}
                      showSelf={false}
                    />
                  }
                  saveControl={
                    <div className="flex items-center gap-2">
                      <span role="status" className="text-xs max-sm:sr-only">
                        {view.phase}
                      </span>
                      {view.migration && (
                        <FeedbackToast
                          message="等待具有编辑权限的成员初始化画布"
                          tone="info"
                        />
                      )}
                      {(view.recovery.length > 0 ||
                        view.recoveryRemaining > 0) && (
                        <CanvasLocalDrafts
                          sources={view.recovery}
                          otherSessions={view.recoveryRemaining}
                          writable={
                            view.writable && view.connected && !view.dirty
                          }
                          commitSeq={client.commitSeq}
                          apply={(id, fields, seq) =>
                            client.applyRecovery(id, fields, seq)
                          }
                          discard={client.discardRecovery}
                        />
                      )}
                    </div>
                  }
                />
              ),
            }}
          />
        </div>
        {conversation !== "unopened" && (
          <CanvasConversationPanel
            canvasID={canvasID}
            projectID={projectID}
            scopeID={scopeID}
            actorID={actorID}
            writable={view.writable && !view.migration}
            open={conversationOpen}
            onClose={closeConversation}
            onOpenLegacy={() => {
              setConversation("closed");
              setLegacyOpen(true);
            }}
          />
        )}
        {legacyOpen && (
          <CanvasAssistantPanel
            canvasID={canvasID}
            projectID={projectID}
            onClose={() => {
              setLegacyOpen(false);
              if (conversation !== "unopened") setConversation("open");
            }}
            onAdopt={adoption.adoptMedia}
            onAdoptText={adoption.adoptText}
            referenceSources={graph.nodes.flatMap((node) => {
              const assetID = node.documentData?.fileReferences?.find(
                (ref) => ref.purpose === "CONTENT",
              )?.fileId;
              return assetID && ["image", "video", "audio"].includes(node.type)
                ? [
                    {
                      nodeID: node.id,
                      title: node.title,
                      kind: node.type as "image" | "video" | "audio",
                      assetID,
                    },
                  ]
                : [];
            })}
            subjectSources={canvasSubjectSources(graph.nodes, canvasID)}
          />
        )}
        <FeedbackToast
          message={view.message ? canvasConnectionMessage(view.message) : ""}
          action={{ label: "重新连接", onClick: client.retry }}
        />
        <Dialog
          open={rename !== null}
          onOpenChange={(open) => {
            if (!open) setRename(null);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>重命名画布</DialogTitle>
              <DialogDescription>名称会同步给协作者。</DialogDescription>
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
        {renameProject && (
          <ProjectRenameDialog
            project={renameProject}
            open
            onOpenChange={(open) => {
              if (!open) {
                setRenameProject(null);
                client.retry();
              }
            }}
          />
        )}
      </div>
    </CanvasMediaContext.Provider>
  );
}

function ArchivedStoryboardPreview({
  projectID,
  storyboard,
}: {
  projectID: string;
  storyboard?: API.Storyboard;
}) {
  const shots = storyboard?.shots ?? [];
  const adopted = shots.find((shot) =>
    isProjectID(shot.adopted?.asset_id),
  )?.adopted;
  return (
    <div className="flex size-full min-h-0 flex-col justify-center gap-2 overflow-hidden p-2">
      {adopted?.asset_id ? (
        <div className="relative min-h-0 flex-1 overflow-hidden rounded-md bg-muted">
          <ProjectMediaPreview
            projectID={projectID}
            id={adopted.asset_id}
            kind="video"
          />
        </div>
      ) : null}
      <span className="text-center text-sm text-muted-foreground">
        历史故事板 · {shots.length} 个镜头
      </span>
    </div>
  );
}
