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
import { previewCanvasAudioConnection } from "./lib/canvas-audio-connection-preview";
import { useCanvasAudioModelCatalog } from "./hooks/use-canvas-audio-model-catalog";
import { useCanvasAudioSubjectCapabilities } from "./hooks/use-canvas-audio-subject-capabilities";
import { canvasAudioCatalogAcceptsSubjects } from "./lib/canvas-audio-connection-capability";
import type { CanvasNodeData } from "./types/canvas";
import type { CanvasClipboard } from "./lib/canvas-clipboard";
import { CanvasYjsLiveSession } from "./yjs-live-session";
import { CanvasGenerationPanel } from "./nodes/canvas-generation-panel";
import { CanvasOperationStatus } from "./nodes/canvas-operation-status";
import { useCanvasOperationTasks } from "./hooks/use-canvas-operation-tasks";
import {
  applyCanvasOperationResult,
  canvasOperationTarget,
} from "./lib/canvas-operation-result";
import type { Task } from "@/components/generation/contracts";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Skeleton } from "@/components/ui/skeleton";
import { CanvasParticipants } from "./canvas-participants";
import { SpaceSwitcher } from "@/components/spaces/space-context";
import { YjsPresence } from "./yjs-presence";
import {
  CanvasConnectionState,
  canvasConnectionMessage,
} from "./connection-state";
import { CanvasProviders } from "./editor";
import CanvasPage from "./editor/canvas-editor";
import { CanvasTopBar } from "./controls/canvas-top-bar";
import { CanvasMediaContext } from "./lib/canvas-media-context";
import { canvasViewKey } from "./view-state";
import { canvasSubjectSources } from "./lib/canvas-subject-source";
import { getCSRFToken } from "@/lib/request";
import { ensureSessionFresh } from "@/components/auth/session-renewal";
import { Button } from "@/components/ui/button";
import { CanvasLocalDrafts } from "./controls/canvas-local-drafts";
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
import { requireProject, type Project } from "@/components/projects/project";
import { ProjectRenameDialog } from "@/components/projects/project-rename-dialog";

const CanvasTransferDialog = dynamic(() =>
  import("./editor/canvas-transfer-dialog").then((m) => m.CanvasTransferDialog),
);
const CanvasTransferReceiver = dynamic(() =>
  import("./editor/canvas-transfer-dialog").then(
    (m) => m.CanvasTransferReceiver,
  ),
);

const CanvasTimelineWorkspace = dynamic(
  () =>
    import("./timeline/canvas-timeline-workspace").then(
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
    import("./subject/canvas-subject-node").then(
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
    import("./cinema-kit/canvas-cinema-kit").then((m) => m.CanvasCinemaKitNode),
  {
    loading: () => (
      <Skeleton className="size-full" aria-label="正在打开导演工具箱" />
    ),
  },
);
const CanvasDirectorWorkspace = dynamic(
  () =>
    import("./director/canvas-director-workspace").then(
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
    import("./assistant/canvas-assistant-panel").then(
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
  const [assistantOpen, setAssistantOpen] = useState(false);
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
  if (!view.document || !view.dto)
    return (
      <CanvasConnectionState
        message={view.message}
        onRetry={client.retry}
        scopeControl={<SpaceSwitcher compact />}
      />
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
        <CanvasPage
          documentIntegration={{
            projectId: projectID,
            getDocumentEpoch: client.getDocumentEpoch,
            initialState: view.document,
            viewStateKey: canvasViewKey(actorID, scopeID, projectID, canvasID),
            mediaContext: {
              canvasId: canvasID,
              projectId: projectID,
              bindingRevision: view.dto.revision!,
            },
            mediaEnabled: true,
            onOpenAssistant: () => setAssistantOpen(true),
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
              type === "timeline" ? (
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
                title={view.dto!.name ?? "画布"}
                canDeleteCanvas={false}
                canRenameCanvas={view.writable && !view.dirty}
                canSwitchCanvas={!view.unsafeToLeave}
                onDeleteProject={() => {}}
                onRenameProject={() => void openRename()}
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
        {assistantOpen && (
          <CanvasAssistantPanel
            canvasID={canvasID}
            projectID={projectID}
            onClose={() => setAssistantOpen(false)}
            onAdopt={adoption.adoptMedia}
            onAdoptText={adoption.adoptText}
            referenceSources={view.document.nodes.flatMap((node) => {
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
            subjectSources={canvasSubjectSources(view.document.nodes, canvasID)}
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
