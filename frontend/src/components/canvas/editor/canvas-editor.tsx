"use client";
import { canvasConnectionHandles } from "../constants/canvas-input-ports";
import { isCanvasShortcut } from "../lib/canvas-keyboard";
import { removeCanvasReferences } from "../lib/canvas-reference-state";
import type { CanvasDocumentIntegration } from "../types/canvas-document-integration";
import { useCanvasViewState } from "../hooks/use-canvas-view-state";

import type { RichTextDocument } from "@/components/canvas/runtime/components/editor/types/rich-text";

import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useRouter } from "next/navigation";
import { useIsMobile } from "@/hooks/use-mobile";
import { workspaceRoutes } from "@/components/canvas/runtime/components/layout/lib/workspace-routes";
import type {
  Dispatch,
  SetStateAction,
  DragEvent as ReactDragEvent,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";

import { useCanvasFilePreviews } from "../hooks/use-canvas-file-previews";
import { useCanvasMediaSize } from "../hooks/use-canvas-media-size";
import { CanvasDocumentNodePanel } from "../nodes/canvas-document-node-panel";
import {
  canvasMediaState,
  matchesFrozenNode,
  canDisplayCanvasGeneration,
} from "../lib/canvas-generation-projection";
import { repeatCanvasMediaGeneration } from "../lib/canvas-generation-repeat";
import { CanvasMediaStatus } from "../nodes/canvas-media-status";
import { CanvasConnectionDialog } from "../connections/canvas-connection-dialog";
import { toCanvasGraph } from "../lib/canvas-document-adapter";
import {
  replaceCanvasNodeAsset,
  removeCanvasNodeAsset,
} from "../lib/canvas-asset-replacement";
import { isFixedCanvasMedia } from "../lib/canvas-media-source";
import {
  canvasConnectionDirection,
  connectCanvasDocument,
  supportsCanvasConnection,
  updateCanvasDocumentNode,
  validateCanvasConnection,
} from "../lib/canvas-document-connections";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import {
  AssetPickerModal,
  type InsertAssetPayload,
} from "@/components/canvas/runtime/components/assets/picker/asset-picker-modal";
import { downloadCanvasOriginal } from "../lib/canvas-original-download";
import { useTransferAbort } from "@/components/canvas/runtime/components/assets/hooks/use-transfer-abort";
import { CanvasConfigComposer } from "@/components/canvas/parameters/canvas-config-composer";
import { CanvasConfigNodePanel } from "@/components/canvas/nodes/canvas-config-node-panel";
import {
  ActiveConnectionPath,
  ConnectionPath,
} from "@/components/canvas/connections/canvas-connections";
import { CanvasNodeContextMenu } from "@/components/canvas/controls/canvas-context-menu";
import {
  ConnectionCreateMenu,
  NodeCreateMenu,
  type PendingConnectionCreate,
} from "@/components/canvas/controls/canvas-create-menus";
import { CanvasEmptyState } from "@/components/canvas/editor/canvas-empty-state";
import { Minimap } from "@/components/canvas/controls/canvas-mini-map";
import { CanvasNode } from "@/components/canvas/nodes/canvas-node";
import {
  CanvasNodeAngleDialog,
  type CanvasImageAngleParams,
} from "@/components/canvas/nodes/canvas-node-angle-dialog";
import {
  CanvasNodeCropDialog,
  type CanvasImageCropRect,
} from "@/components/canvas/nodes/canvas-node-crop-dialog";
import {
  buildNodeGenerationContext,
  buildNodeGenerationInputs,
  buildNodeResponseMessages,
  hydrateNodeGenerationContext,
  type NodeGenerationInput,
} from "@/components/canvas/lib/canvas-node-generation";
import {
  CanvasNodeHoverToolbar,
  CanvasNodeInfoModal,
} from "@/components/canvas/nodes/canvas-node-hover-toolbar";
import {
  CanvasNodeMaskEditDialog,
  type CanvasImageMaskEditPayload,
} from "@/components/canvas/nodes/canvas-node-mask-edit-dialog";
import {
  CanvasNodePromptPanel,
  type CanvasNodeGenerationMode,
} from "@/components/canvas/nodes/canvas-node-prompt-panel";
import {
  CanvasNodeSplitDialog,
  type CanvasImageSplitParams,
} from "@/components/canvas/nodes/canvas-node-split-dialog";
import {
  CanvasNodeUpscaleDialog,
  type CanvasImageUpscaleParams,
} from "@/components/canvas/nodes/canvas-node-upscale-dialog";
import { CanvasRefreshShell } from "@/components/canvas/editor/canvas-refresh-shell";
import { CanvasSelectionToolbar } from "@/components/canvas/controls/canvas-selection-toolbar";
import { CanvasSidePanel } from "@/components/canvas/editor/canvas-side-panel";
import { CanvasToolbar } from "@/components/canvas/controls/canvas-toolbar";
import {
  buildCanvasSpatialIndex,
  canvasNodeBounds,
} from "@/components/canvas/lib/canvas-spatial-index";
import { CanvasTopBar } from "@/components/canvas/controls/canvas-top-bar";
import { CanvasZoomControls } from "@/components/canvas/controls/canvas-zoom-controls";
import { InfiniteCanvas } from "@/components/canvas/editor/infinite-canvas";
import { registerBuiltinNodes } from "@/components/canvas/nodes/builtin-nodes";
import { useAppFeedback } from "@/components/canvas/runtime/components/feedback/app-feedback-provider";
import { useProjectActionActive } from "../project/project-action-provider";
import {
  NODE_DEFAULT_SIZE,
  getNodeSpec,
} from "@/components/canvas/constants/canvas";
import {
  canvasThemes,
  type CanvasBackgroundMode,
} from "@/components/canvas/runtime/lib/theme";
import {
  buildAngleLabel,
  buildAnglePrompt,
  buildGenerationConfig,
  findRetrySourceNode,
  generationReferenceUrls,
  getGenerationCount,
  getInputSummary,
  hasResumableVideoTask,
  isGenerationCanceled,
  resolveMetadataReferences,
  sourceNodeReferenceImages,
} from "@/components/canvas/lib/canvas-generation-helpers";
import {
  cropDataUrl,
  splitDataUrl,
  upscaleDataUrl,
} from "@/components/canvas/lib/canvas-image-data";
import {
  applyNodeConfigPatch,
  audioMetadata,
  buildAudioGenerationMetadata,
  buildImageGenerationMetadata,
  createCanvasNode,
  imageMetadata,
  videoMetadata,
} from "@/components/canvas/lib/canvas-node-factory";
import {
  applyGroupSelection,
  applyUngroupSelection,
  canGroupSelectedNodes,
  canUngroupSelectedNodes,
  collectGroupMemberNodes,
  findContainingGroupId,
  findGroupDropTarget,
  getConnectionTargetAnchor,
  getGroupWrapRect,
  normalizeConnection,
  snapNodesIntoGroup,
} from "@/components/canvas/lib/canvas-node-geometry";
import {
  fitNodeSize,
  nodeSizeFromRatio,
} from "@/components/canvas/lib/canvas-node-size";
import {
  buildNodeMentionReferences,
  buildConnectedNodeMap,
  getGroupResourceNodes,
  isCanvasReferenceNode,
  type CanvasResourceReference,
} from "@/components/canvas/lib/canvas-resource-references";
import {
  captureVideoFrame,
  type VideoFramePosition,
} from "@/components/canvas/lib/canvas-video-frame";
import {
  canvasPoint,
  fitCanvasNodes,
  zoomAt,
} from "@/components/canvas/lib/canvas-viewport";
import {
  getNodeDefinition,
  isBuiltinNodeType as isBuiltinType,
  useNodeRegistryVersion,
} from "@/components/canvas/lib/node-registry";
import { imageReferenceLabel } from "@/components/canvas/runtime/lib/generation/image-reference-prompt";
import { canvasConnectionGeometry } from "../lib/canvas-connection-geometry";
import {
  CanvasNodeDragSession,
  applyCanvasDragPositions,
} from "../lib/canvas-node-drag";
import { useCanvasOperations } from "@/components/canvas/hooks/use-canvas-operations";
import { usePluginHost } from "@/components/canvas/hooks/use-plugin-host";
import {
  requestAudioGeneration,
  storeGeneratedAudio,
} from "@/components/canvas/runtime/lib/generation/audio";
import {
  requestEdit,
  requestGeneration,
  requestImageQuestion,
} from "@/components/canvas/runtime/lib/generation/image";
import {
  createVideoGenerationTask,
  isVideoTaskFailed,
  storeGeneratedVideo,
  waitForVideoGenerationTask,
} from "@/components/canvas/runtime/lib/generation/video";
import type { UploadedFile } from "@/components/canvas/runtime/lib/media/file-storage";
import { uploadImage } from "@/components/canvas/runtime/lib/media/image-storage";

import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { CanvasSaveAssetDialog } from "./canvas-save-asset-dialog";
import { canvasAssetRequest } from "../lib/canvas-asset-request";
import {
  defaultConfig,
  useConfigStore,
  useEffectiveConfig,
} from "@/components/canvas/runtime/store/use-config-store";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import {
  CanvasNodeType,
  type CanvasAssistantSession,
  type CanvasConnection,
  type CanvasNodeData,
  type CanvasNodeImage,
  type CanvasNodeText,
  type CanvasNodeTypeId,
  type ConnectionHandle,
  type ContextMenuState,
  type Position,
  type SelectionBox,
} from "@/components/canvas/types/canvas";
import type {
  ReferenceAudio,
  ReferenceVideo,
} from "@/components/canvas/runtime/types/media";
import { canvasId as nanoid } from "@/components/canvas/lib/canvas-id";

// Register built-in nodes in the shared registry once when the module loads.
registerBuiltinNodes();

type CanvasClipboard = {
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
};

type ConnectionDropTarget = {
  nodeId: string | null;
  isNearNode: boolean;
};

type CanvasHistoryEntry = Pick<CanvasClipboard, "nodes" | "connections"> & {
  chatSessions: CanvasAssistantSession[];
  activeChatId: string | null;
};

type CanvasGenerationRequest = {
  targetNodeId: string;
  originNodeId: string;
  runningNodeId: string;
  controller: AbortController;
};

const VIDEO_NODE_MAX_WIDTH = 420;
const VIDEO_NODE_MAX_HEIGHT = 420;
// Stable empty reference array prevents `... || []` from invalidating CanvasNode's React.memo on every render.
const EMPTY_REFERENCES: CanvasResourceReference[] = [];
const CONNECTION_HANDLE_HIT_RADIUS = 40;
const CONNECTION_NODE_HIT_PADDING = 32;
const NODE_STATUS_IDLE = "idle" as const;
const NODE_STATUS_LOADING = "loading" as const;
const NODE_STATUS_SUCCESS = "success" as const;
const NODE_STATUS_ERROR = "error" as const;

function applyGeneratedVideo(
  item: CanvasNodeData,
  video: UploadedFile,
  extra: CanvasNodeData["metadata"] = {},
): CanvasNodeData {
  const videoSize = fitNodeSize(
    video.width || item.width,
    video.height || item.height,
    VIDEO_NODE_MAX_WIDTH,
    VIDEO_NODE_MAX_HEIGHT,
  );
  return {
    ...item,
    width: videoSize.width,
    height: videoSize.height,
    position: {
      x: item.position.x + item.width / 2 - videoSize.width / 2,
      y: item.position.y + item.height / 2 - videoSize.height / 2,
    },
    metadata: { ...item.metadata, ...videoMetadata(video), ...extra },
  };
}

const subscribeToMount = () => () => {};

export default function CanvasPage({
  project,
  documentIntegration,
}: {
  project?: CanvasContract.ProjectVO;
  documentIntegration?: CanvasDocumentIntegration;
}) {
  const mounted = useSyncExternalStore(
    subscribeToMount,
    () => true,
    () => false,
  );

  if (!mounted) return <CanvasRefreshShell />;

  return (
    <InfiniteCanvasPage
      project={project}
      documentIntegration={documentIntegration}
    />
  );
}

function InfiniteCanvasPage({
  project,
  documentIntegration,
}: {
  project?: CanvasContract.ProjectVO;
  documentIntegration?: CanvasDocumentIntegration;
}) {
  const isMobile = useIsMobile();
  const [initialProject] = useState(() => {
    if (documentIntegration)
      return {
        ...documentIntegration.initialState,
        chatSessions: [],
        activeChatId: null,
      };
    if (!project) throw new Error("缺少画布数据");
    return {
      ...project.graph,
      activeChatId: project.graph.activeChatId ?? null,
    } as Pick<
      import("@/components/canvas/types/canvas-project").CanvasProject,
      | "nodes"
      | "connections"
      | "chatSessions"
      | "activeChatId"
      | "backgroundMode"
      | "showImageInfo"
      | "viewport"
    >;
  });
  const projectActionActive = useProjectActionActive();
  const { message, modal } = useAppFeedback();

  // Subscribe to the registry version so plugin registration changes rerender the canvas.
  const nodeRegistryVersion = useNodeRegistryVersion((state) => state.version);
  const router = useRouter();
  const containerRef = useRef<HTMLDivElement>(null);
  const clipboardRef = useRef<CanvasClipboard | null>(null);
  const historyRef = useRef<{
    past: CanvasHistoryEntry[];
    future: CanvasHistoryEntry[];
  }>({ past: [], future: [] });
  const lastHistoryRef = useRef<CanvasHistoryEntry | null>({
    nodes: initialProject.nodes,
    connections: initialProject.connections,
    chatSessions: initialProject.chatSessions,
    activeChatId: initialProject.activeChatId,
  });
  const historyCommitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );

  const applyingHistoryRef = useRef(false);
  const historyPausedRef = useRef(false);
  const nodeDraggingRef = useRef(false);
  const nodeDragSessionRef = useRef<CanvasNodeDragSession | null>(null);
  const releasedDragSessionRef = useRef<CanvasNodeDragSession | null>(null);

  const effectiveConfig = useEffectiveConfig();
  const isAiConfigReady = useConfigStore((state) => state.isAiConfigReady);
  const { account } = useAccount();
  const [assetSave, setAssetSave] = useState<{
    node: CanvasNodeData;
    accountId: string;
  } | null>(null);
  const [assetsRevision, setAssetsRevision] = useState(0);
  const [savedAssetScope, setSavedAssetScope] = useState<string>();
  const currentProject = project;
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const [nodes, setNodesState] = useState<CanvasNodeData[]>(
    initialProject.nodes,
  );
  const [connections, setConnectionsState] = useState<CanvasConnection[]>(
    initialProject.connections,
  );
  const integrationRef = useRef(documentIntegration);
  integrationRef.current = documentIntegration;
  const nodesStateRef = useRef(nodes);
  const connectionsStateRef = useRef(connections);
  nodesStateRef.current = nodes;
  connectionsStateRef.current = connections;
  const admitGraph = useCallback(
    (nextNodes: CanvasNodeData[], nextConnections: CanvasConnection[]) => {
      const integration = integrationRef.current;
      if (!integration) return true;
      if (integration.frozen) return false;
      return (
        integration.admit?.({
          ...integration.initialState,
          nodes: nextNodes,
          connections: nextConnections,
        }) ?? true
      );
    },
    [],
  );
  const setNodes: Dispatch<SetStateAction<CanvasNodeData[]>> = useCallback(
    (next) => {
      const value =
        typeof next === "function" ? next(nodesStateRef.current) : next;
      if (!admitGraph(value, connectionsStateRef.current)) return;
      nodesStateRef.current = value;
      setNodesState(value);
    },
    [admitGraph],
  );
  const setConnections: Dispatch<SetStateAction<CanvasConnection[]>> =
    useCallback(
      (next) => {
        const value =
          typeof next === "function" ? next(connectionsStateRef.current) : next;
        if (!admitGraph(nodesStateRef.current, value)) return;
        connectionsStateRef.current = value;
        setConnectionsState(value);
      },
      [admitGraph],
    );
  const [chatSessions, setChatSessions] = useState<CanvasAssistantSession[]>(
    initialProject.chatSessions,
  );
  const [activeChatId, setActiveChatId] = useState<string | null>(
    initialProject.activeChatId,
  );
  const [size, setSize] = useState({ width: 0, height: 0 });
  const { view, updateView, setViewport, previewViewport } = useCanvasViewState(
    {
      key: documentIntegration?.viewStateKey,
      initial: initialProject,
      size,
    },
  );
  const {
    viewport,
    backgroundMode,
    showImageInfo,
    showConnections,
    snapToGrid,
    miniMap: isMiniMapOpen,
  } = view;
  const setBackgroundMode = (backgroundMode: CanvasBackgroundMode) =>
    updateView({ backgroundMode });
  const setShowImageInfo = (showImageInfo: boolean) =>
    updateView({ showImageInfo });
  const setShowConnections = (showConnections: boolean) =>
    updateView({ showConnections });
  const setSnapToGrid = (snapToGrid: boolean) => updateView({ snapToGrid });
  const setIsMiniMapOpen = (miniMap: boolean) => updateView({ miniMap });
  const [canvasTool, setCanvasTool] = useState<"select" | "pan">("select");
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(
    new Set(),
  );
  const [selectedConnectionId, setSelectedConnectionId] = useState<
    string | null
  >(null);
  const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
  const [connectingParams, setConnectingParams] =
    useState<ConnectionHandle | null>(null);
  const [connectionTargetNodeId, setConnectionTargetNodeId] = useState<
    string | null
  >(null);
  const [documentConnection, setDocumentConnection] =
    useState<CanvasContract.CanvasDocumentConnectionDTO | null>(null);
  const [pendingConnectionCreate, setPendingConnectionCreate] =
    useState<PendingConnectionCreate | null>(null);
  const [mouseWorld, setMouseWorld] = useState<Position>({ x: 0, y: 0 });
  const [selectionBox, setSelectionBox] = useState<SelectionBox | null>(null);
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [nodeCreatePosition, setNodeCreatePosition] = useState<Position | null>(
    null,
  );
  const [runningNodeId, setRunningNodeId] = useState<string | null>(null);
  const snapToGridRef = useRef(false);
  snapToGridRef.current = snapToGrid;
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false);
  const [assetPickerOpen, setAssetPickerOpen] = useState(false);
  const [assetUploadOpen, setAssetUploadOpen] = useState(false);
  const [assetTarget, setAssetTarget] = useState<
    Pick<CanvasNodeData, "id" | "type"> | undefined
  >();
  const [sourceFile, setSourceFile] = useState<File>();
  const [sourceFiles, setSourceFiles] = useState<File[]>([]);
  const downloadAbort = useTransferAbort();
  const projectLoaded = true;
  const [toolbarNodeId, setToolbarNodeId] = useState<string | null>(null);
  const [nodeImageSettingsOpen, setNodeImageSettingsOpen] = useState(false);
  const [dialogNodeId, setDialogNodeId] = useState<string | null>(null);
  const [infoNodeId, setInfoNodeId] = useState<string | null>(null);
  const [cropNodeId, setCropNodeId] = useState<string | null>(null);
  const [maskEditNodeId, setMaskEditNodeId] = useState<string | null>(null);
  const [splitNodeId, setSplitNodeId] = useState<string | null>(null);
  const [upscaleNodeId, setUpscaleNodeId] = useState<string | null>(null);
  const [superResolveNodeId, setSuperResolveNodeId] = useState<string | null>(
    null,
  );
  const [angleNodeId, setAngleNodeId] = useState<string | null>(null);
  const [previewNodeId, setPreviewNodeId] = useState<string | null>(null);
  const [previewImageId, setPreviewImageId] = useState<string | null>(null);
  const [historyState, setHistoryState] = useState({
    canUndo: false,
    canRedo: false,
  });
  const [expandedBatchNodeIds, setExpandedBatchNodeIds] = useState<Set<string>>(
    new Set(),
  );
  const [isNodeDragging, setIsNodeDragging] = useState(false);
  const [composing, setComposing] = useState(false);
  const composingElement = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    // Removing the active editor can omit compositionend. A remote deletion
    // already preserves its draft; the detached DOM must not freeze new edits.
    if (composing && composingElement.current?.isConnected === false) {
      composingElement.current = null;
      setComposing(false);
    }
  }, [composing, nodes, dialogNodeId]);
  const [pointerActive, setPointerActive] = useState(false);
  const measureMedia = useCanvasMediaSize(
    nodes,
    !documentIntegration?.frozen &&
      !isNodeDragging &&
      !pointerActive &&
      !composing,
    setNodes,
  );
  useEffect(() => {
    const settled = () => setPointerActive(false);
    window.addEventListener("pointerup", settled);
    window.addEventListener("mouseup", settled);
    window.addEventListener("pointercancel", settled);
    window.addEventListener("blur", settled);
    return () => {
      window.removeEventListener("pointerup", settled);
      window.removeEventListener("mouseup", settled);
      window.removeEventListener("pointercancel", settled);
      window.removeEventListener("blur", settled);
    };
  }, []);

  useEffect(() => {
    const runs = documentIntegration?.generation?.runs;
    if (!runs) return;
    setNodes((previous) => {
      let changed = false;
      const projected = previous.map((node): CanvasNodeData => {
        const run = runs.find((value) => value.nodeId === node.id);
        const current = !!run && matchesFrozenNode(node, run);
        const output =
          canDisplayCanvasGeneration(node, run) &&
          run &&
          run.state === "SUCCEEDED" &&
          run.previewUrl &&
          run.outputs?.length;
        const status =
          current && run.pending
            ? "loading"
            : current && run.state === "FAILED"
              ? "error"
              : output
                ? "success"
                : "idle";
        const old = node.metadata?.generationOutput;
        if (!current && !old && !output) return node;
        const content = output
          ? run.previewUrl
          : old
            ? undefined
            : node.metadata?.content;
        if (
          node.metadata?.status === status &&
          node.metadata?.content === content
        )
          return node;
        changed = true;
        return {
          ...node,
          metadata: {
            ...node.metadata,
            status,
            content,
            generationOutput: output
              ? { commandId: run.commandId!, files: run.outputs! }
              : undefined,
          },
        };
      });
      return changed ? projected : previous;
    });
  }, [documentIntegration?.generation?.runs, nodes, setNodes]);
  useCanvasFilePreviews(
    nodes,
    !!documentIntegration &&
      (documentIntegration.mediaEnabled ?? !documentIntegration.frozen),
    setNodesState,
    documentIntegration?.mediaContext,
  );
  const savedViewport = initialProject.viewport;
  const incomingDocument = documentIntegration?.initialState;
  const receivedDocument = useRef(incomingDocument);
  const applyingRemote = useRef<typeof incomingDocument>(undefined);
  useLayoutEffect(() => {
    if (!incomingDocument || receivedDocument.current === incomingDocument)
      return;
    receivedDocument.current = incomingDocument;
    applyingRemote.current = incomingDocument;
    if (historyCommitTimerRef.current)
      clearTimeout(historyCommitTimerRef.current);
    historyCommitTimerRef.current = null;
    // Shared graph undo belongs to the collaboration document; these entries serve local canvas sessions.
    historyRef.current = { past: [], future: [] };
    lastHistoryRef.current = {
      nodes: incomingDocument.nodes,
      connections: incomingDocument.connections,
      chatSessions,
      activeChatId,
    };
    setHistoryState({ canUndo: false, canRedo: false });
    nodesStateRef.current = incomingDocument.nodes;
    connectionsStateRef.current = incomingDocument.connections;
    setNodesState(incomingDocument.nodes);
    setConnectionsState(incomingDocument.connections);
  }, [incomingDocument, chatSessions, activeChatId]);
  useEffect(() => {
    if (applyingRemote.current) {
      if (
        nodes === applyingRemote.current.nodes &&
        connections === applyingRemote.current.connections
      )
        applyingRemote.current = undefined;
      return;
    }
    if (isNodeDragging) return;
    integrationRef.current?.onChange(
      {
        nodes,
        connections,
        backgroundMode: initialProject.backgroundMode,
        showImageInfo: initialProject.showImageInfo,
        viewport: savedViewport,
      },
      !composing && !isNodeDragging && !pointerActive,
    );
  }, [
    nodes,
    connections,
    initialProject.backgroundMode,
    initialProject.showImageInfo,
    savedViewport,
    composing,
    isNodeDragging,
    pointerActive,
  ]);
  const [isNodeResizing, setIsNodeResizing] = useState(false);
  const [dropTargetGroupId, setDropTargetGroupId] = useState<string | null>(
    null,
  );
  const [referencePickerNodeId, setReferencePickerNodeId] = useState<
    string | null
  >(null);

  const nodesRef = useRef(nodes);
  const connectionsRef = useRef(connections);
  const selectedNodeIdsRef = useRef(selectedNodeIds);
  const viewportRef = useRef(viewport);
  const focusAnimRef = useRef<number | null>(null);
  const generateNodeRef = useRef<
    | ((
        nodeId: string,
        mode: CanvasNodeGenerationMode,
        prompt: string,
      ) => Promise<void>)
    | null
  >(null);
  const connectingParamsRef = useRef(connectingParams);
  const connectionTargetNodeIdRef = useRef(connectionTargetNodeId);
  const selectionBoxRef = useRef(selectionBox);
  const pendingConnectionCreateRef = useRef(pendingConnectionCreate);
  const generationRequestsRef = useRef(
    new Map<string, CanvasGenerationRequest>(),
  );
  const videoPollIdsRef = useRef(new Set<string>());

  const createHistoryEntry = useCallback(
    (): CanvasHistoryEntry => ({
      nodes: nodesRef.current,
      connections: connectionsRef.current,
      chatSessions,
      activeChatId,
    }),
    [activeChatId, chatSessions],
  );

  const startGenerationRequest = useCallback(
    (
      targetNodeId: string,
      originNodeId: string,
      runningId = originNodeId,
      controller = new AbortController(),
    ) => {
      const previous = generationRequestsRef.current.get(targetNodeId);
      if (previous?.controller !== controller) previous?.controller.abort();
      generationRequestsRef.current.set(targetNodeId, {
        targetNodeId,
        originNodeId,
        runningNodeId: runningId,
        controller,
      });
      return controller;
    },
    [],
  );

  const finishGenerationRequest = useCallback(
    (targetNodeId: string, controller: AbortController) => {
      const request = generationRequestsRef.current.get(targetNodeId);
      if (request?.controller === controller)
        generationRequestsRef.current.delete(targetNodeId);
    },
    [],
  );

  const completeVideoNodeTask = useCallback(
    async (
      nodeId: string,
      config: Parameters<typeof buildGenerationConfig>[0],
      prompt: string,
      images: Parameters<typeof createVideoGenerationTask>[2],
      signal: AbortSignal,
      extra: CanvasNodeData["metadata"] = {},
      videos: ReferenceVideo[] = [],
      audios: ReferenceAudio[] = [],
    ) => {
      const task = await createVideoGenerationTask(config, prompt, images, {
        signal,
        videos,
        audios,
      });
      setNodes((prev) =>
        prev.map((item) =>
          item.id === nodeId
            ? {
                ...item,
                metadata: {
                  ...item.metadata,
                  videoTaskId: task.id,
                  videoTaskProvider: "agent",
                  model: config.model,
                },
              }
            : item,
        ),
      );
      const video = await storeGeneratedVideo(
        await waitForVideoGenerationTask(config, task, { signal }),
      );
      setNodes((prev) =>
        prev.map((item) =>
          item.id === nodeId
            ? applyGeneratedVideo(item, video, {
                prompt,
                model: config.model,
                ...extra,
              })
            : item,
        ),
      );
    },
    [setNodes],
  );

  const pollVideoNodeTask = useCallback(
    async (node: CanvasNodeData, silent = false) => {
      const taskId = node.metadata?.videoTaskId;
      if (
        !taskId ||
        node.metadata?.content ||
        generationRequestsRef.current.has(node.id) ||
        videoPollIdsRef.current.has(node.id)
      )
        return;
      videoPollIdsRef.current.add(node.id);
      let controller: AbortController | undefined;
      try {
        const generationConfig = buildGenerationConfig(
          effectiveConfig,
          node,
          "video",
        );
        if (!isAiConfigReady(generationConfig, generationConfig.model)) {
          if (silent) {
            setNodes((prev) =>
              prev.map((item) =>
                item.id === node.id
                  ? {
                      ...item,
                      metadata: {
                        ...item.metadata,
                        status: NODE_STATUS_ERROR,
                        errorDetails: "生成服务尚未接入",
                      },
                    }
                  : item,
              ),
            );
            return;
          }
          message.warning("生成服务尚未接入");
          return;
        }
        setRunningNodeId(node.id);
        setNodes((prev) =>
          prev.map((item) =>
            item.id === node.id
              ? {
                  ...item,
                  metadata: {
                    ...item.metadata,
                    status: NODE_STATUS_LOADING,
                    errorDetails: undefined,
                  },
                }
              : item,
          ),
        );
        controller = startGenerationRequest(node.id, node.id, node.id);
        const video = await storeGeneratedVideo(
          await waitForVideoGenerationTask(
            generationConfig,
            { id: taskId, provider: "agent", model: generationConfig.model },
            { signal: controller.signal },
          ),
        );
        setNodes((prev) =>
          prev.map((item) =>
            item.id === node.id
              ? applyGeneratedVideo(item, video, {
                  prompt: item.metadata?.prompt,
                  model: generationConfig.model,
                  size: generationConfig.size,
                  seconds: generationConfig.videoSeconds,
                  vquality: generationConfig.vquality,
                  generateAudio: generationConfig.videoGenerateAudio,
                  watermark: generationConfig.videoWatermark,
                  videoMode: generationConfig.videoMode,
                })
              : item,
          ),
        );
      } catch (error) {
        if (isGenerationCanceled(error)) return;
        const errorDetails =
          error instanceof Error ? error.message : "生成失败";
        message.error(errorDetails);
        setNodes((prev) =>
          prev.map((item) =>
            item.id === node.id
              ? {
                  ...item,
                  metadata: {
                    ...item.metadata,
                    status: item.metadata?.content
                      ? NODE_STATUS_SUCCESS
                      : NODE_STATUS_ERROR,
                    errorDetails: item.metadata?.content
                      ? undefined
                      : errorDetails,
                    ...(isVideoTaskFailed(error)
                      ? { videoTaskId: undefined }
                      : {}),
                  },
                }
              : item,
          ),
        );
      } finally {
        videoPollIdsRef.current.delete(node.id);
        if (controller) {
          finishGenerationRequest(node.id, controller);
          setRunningNodeId((current) => (current === node.id ? null : current));
        }
      }
    },
    [
      effectiveConfig,
      finishGenerationRequest,
      isAiConfigReady,
      message,
      setNodes,
      startGenerationRequest,
    ],
  );

  const stopGenerationByRunningId = useCallback(
    (runningId: string) => {
      const affectedNodeIds = new Set<string>();
      generationRequestsRef.current.forEach((request) => {
        if (request.runningNodeId !== runningId) return;
        request.controller.abort();
        generationRequestsRef.current.delete(request.targetNodeId);
        affectedNodeIds.add(request.targetNodeId);
        affectedNodeIds.add(request.originNodeId);
      });
      setRunningNodeId((current) => (current === runningId ? null : current));
      if (!affectedNodeIds.size) return;
      setNodes((prev) =>
        prev.map((node) =>
          affectedNodeIds.has(node.id) &&
          node.metadata?.status === NODE_STATUS_LOADING
            ? {
                ...node,
                metadata: {
                  ...node.metadata,
                  status: NODE_STATUS_IDLE,
                  errorDetails: undefined,
                  images: node.metadata.images?.map((image) =>
                    image.status === NODE_STATUS_LOADING
                      ? {
                          ...image,
                          status: NODE_STATUS_ERROR,
                          errorDetails: "请求已取消",
                        }
                      : image,
                  ),
                  texts: node.metadata.texts?.map((text) =>
                    text.status === NODE_STATUS_LOADING
                      ? {
                          ...text,
                          status: NODE_STATUS_ERROR,
                          errorDetails: "请求已取消",
                        }
                      : text,
                  ),
                },
              }
            : node,
        ),
      );
    },
    [setNodes],
  );

  const confirmStopGeneration = useCallback(
    (nodeId: string) => {
      modal.confirm({
        title: "停止生成？",
        content: "当前生成请求会被中断，已经生成完成的内容会保留。",
        okText: "停止",
        cancelText: "继续生成",
        okButtonProps: { danger: true },
        onOk: () => stopGenerationByRunningId(nodeId),
      });
    },
    [modal, stopGenerationByRunningId],
  );

  useEffect(() => {
    if (!projectLoaded) return;
    nodesRef.current
      .filter(hasResumableVideoTask)
      .forEach((node) => void pollVideoNodeTask(node, true));
    // Resume once after the current canvas is restored, not on later config identity changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectLoaded]);

  useEffect(() => {
    if (
      !projectLoaded ||
      !!documentIntegration?.undo ||
      applyingHistoryRef.current ||
      historyPausedRef.current
    )
      return;
    const next = createHistoryEntry();
    const previous = lastHistoryRef.current;
    if (
      previous?.nodes === next.nodes &&
      previous.connections === next.connections &&
      previous.chatSessions === next.chatSessions &&
      previous.activeChatId === next.activeChatId
    )
      return;

    if (historyCommitTimerRef.current)
      clearTimeout(historyCommitTimerRef.current);
    historyCommitTimerRef.current = setTimeout(() => {
      const current = createHistoryEntry();
      const last = lastHistoryRef.current;
      if (!last) return;
      historyRef.current.past = [...historyRef.current.past.slice(-49), last];
      historyRef.current.future = [];
      setHistoryState({ canUndo: true, canRedo: false });
      lastHistoryRef.current = current;
      historyCommitTimerRef.current = null;
    }, 180);

    return () => {
      if (historyCommitTimerRef.current) {
        clearTimeout(historyCommitTimerRef.current);
        historyCommitTimerRef.current = null;
      }
    };
  }, [
    activeChatId,
    chatSessions,
    connections,
    createHistoryEntry,
    nodes,
    projectLoaded,
    documentIntegration?.undo,
  ]);

  useEffect(() => {
    if (!dialogNodeId) setNodeImageSettingsOpen(false);
  }, [dialogNodeId]);

  useLayoutEffect(() => {
    nodesRef.current = nodes;
    connectionsRef.current = connections;
    selectedNodeIdsRef.current = selectedNodeIds;
    viewportRef.current = viewport;
    connectingParamsRef.current = connectingParams;
    connectionTargetNodeIdRef.current = connectionTargetNodeId;
    pendingConnectionCreateRef.current = pendingConnectionCreate;
  }, [
    nodes,
    connections,
    selectedNodeIds,
    viewport,
    connectingParams,
    connectionTargetNodeId,
    pendingConnectionCreate,
  ]);

  useLayoutEffect(() => {
    selectionBoxRef.current = selectionBox;
  }, [selectionBox]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const updateSize = () => {
      const rect = el.getBoundingClientRect();
      setSize({ width: rect.width, height: rect.height });
    };

    updateSize();
    const resizeObserver = new ResizeObserver(updateSize);
    resizeObserver.observe(el);
    return () => resizeObserver.disconnect();
  }, [projectLoaded]);

  const screenToCanvas = useCallback((clientX: number, clientY: number) => {
    const rect = containerRef.current?.getBoundingClientRect();
    const currentViewport = viewportRef.current;
    const localX = clientX - (rect?.left || 0);
    const localY = clientY - (rect?.top || 0);

    return canvasPoint({ x: localX, y: localY }, currentViewport);
  }, []);

  const getCanvasCenter = useCallback(() => {
    const rect = containerRef.current?.getBoundingClientRect();
    return screenToCanvas(
      (rect?.left || 0) + (rect?.width || size.width) / 2,
      (rect?.top || 0) + (rect?.height || size.height) / 2,
    );
  }, [screenToCanvas, size.height, size.width]);

  const setConnecting = useCallback((next: ConnectionHandle | null) => {
    connectingParamsRef.current = next;
    setConnectingParams(next);
    if (!next) {
      connectionTargetNodeIdRef.current = null;
      setConnectionTargetNodeId(null);
    }
  }, []);

  const keepNodeToolbar = useCallback(
    (nodeId: string) => {
      if (
        nodeDraggingRef.current ||
        nodeImageSettingsOpen ||
        !selectedNodeIdsRef.current.has(nodeId)
      )
        return;
      setToolbarNodeId(nodeId);
    },
    [nodeImageSettingsOpen],
  );

  const hideNodeToolbar = useCallback(() => {}, []);

  const connectDocumentNodes = useCallback(
    (fromNodeId: string, toNodeId: string, currentNodes = nodesRef.current) => {
      if (integrationRef.current?.frozen) return false;
      try {
        const result = connectCanvasDocument(
          {
            nodes: currentNodes,
            connections: connectionsRef.current,
            backgroundMode,
            showImageInfo,
            viewport: viewportRef.current,
          },
          fromNodeId,
          toNodeId,
        );
        nodesRef.current = result.nodes;
        connectionsRef.current = result.connections;
        setNodes(result.nodes);
        setConnections(result.connections);
        return true;
      } catch (error) {
        notifyError(error, "无法关联节点");
        return false;
      }
    },
    [backgroundMode, setConnections, setNodes, showImageInfo],
  );

  const connectNodes = useCallback(
    (current: ConnectionHandle, targetNodeId: string) => {
      if (current.nodeId === targetNodeId) return;
      if (integrationRef.current) {
        const { fromNodeId, toNodeId } = canvasConnectionDirection(
          current,
          targetNodeId,
        );
        connectDocumentNodes(fromNodeId, toNodeId);
        setContextMenu(null);
        return;
      }
      const connection = normalizeConnection(
        current.nodeId,
        targetNodeId,
        nodesRef.current,
        current.handleType,
      );
      if (!connection) {
        message.warning("配置节点之间不能连接");
        return;
      }
      const { fromNodeId, toNodeId } = connection;
      const exists = connectionsRef.current.some(
        (conn) => conn.fromNodeId === fromNodeId && conn.toNodeId === toNodeId,
      );
      if (!exists) {
        setConnections((prev) => [
          ...prev,
          { id: crypto.randomUUID(), fromNodeId, toNodeId },
        ]);
      }
      setContextMenu(null);
    },
    [connectDocumentNodes, message, setConnections],
  );

  const createConnectedNode = useCallback(
    (
      type:
        | CanvasNodeType.Image
        | CanvasNodeType.Text
        | CanvasNodeType.Config
        | CanvasNodeType.Video
        | CanvasNodeType.Audio,
      pending: PendingConnectionCreate,
    ) => {
      const metadata =
        type === CanvasNodeType.Config
          ? {
              model: effectiveConfig.imageModel || effectiveConfig.model,
              size: effectiveConfig.size,
              count: getGenerationCount(
                effectiveConfig.canvasImageCount || effectiveConfig.count,
              ),
            }
          : undefined;
      const newNode = createCanvasNode(type, pending.position, metadata);
      const connection = integrationRef.current
        ? canvasConnectionDirection(pending.connection, newNode.id)
        : normalizeConnection(
            pending.connection.nodeId,
            newNode.id,
            [...nodesRef.current, newNode],
            pending.connection.handleType,
          );
      if (!connection) {
        message.warning("配置节点之间不能连接");
        return;
      }
      if (integrationRef.current) {
        if (
          !connectDocumentNodes(connection.fromNodeId, connection.toNodeId, [
            ...nodesRef.current,
            newNode,
          ])
        ) {
          setPendingConnectionCreate(null);
          setConnecting(null);
          return;
        }
      } else {
        setNodes((prev) => [...prev, newNode]);
        setConnections((prev) => [...prev, { id: nanoid(), ...connection }]);
      }
      setSelectedNodeIds(new Set([newNode.id]));
      setSelectedConnectionId(null);
      if (type !== CanvasNodeType.Text && type !== CanvasNodeType.Audio)
        setDialogNodeId(newNode.id);
      setPendingConnectionCreate(null);
      setConnecting(null);
    },
    [
      effectiveConfig.imageModel,
      effectiveConfig.model,
      effectiveConfig.size,
      effectiveConfig.canvasImageCount,
      effectiveConfig.count,
      setConnecting,
      message,
      connectDocumentNodes,
      setNodes,
      setConnections,
    ],
  );

  const cancelPendingConnectionCreate = useCallback(() => {
    setPendingConnectionCreate(null);
    setConnecting(null);
  }, [setConnecting]);

  const getConnectionDropTarget = useCallback(
    (
      clientX: number,
      clientY: number,
      current: ConnectionHandle,
    ): ConnectionDropTarget => {
      const world = screenToCanvas(clientX, clientY);
      const scale = Math.max(viewportRef.current.k, 0.05);
      const padding = CONNECTION_NODE_HIT_PADDING / scale;
      const handleRadius = CONNECTION_HANDLE_HIT_RADIUS / scale;
      let isNearNode = false;
      let bestNodeId: string | null = null;
      let bestPriority = Number.POSITIVE_INFINITY;

      [...nodesRef.current].reverse().forEach((node) => {
        const anchor = getConnectionTargetAnchor(node, current);
        const dx = world.x - anchor.x;
        const dy = world.y - anchor.y;
        const hitsHandle = dx * dx + dy * dy <= handleRadius * handleRadius;
        const hitsInside =
          world.x >= node.position.x &&
          world.x <= node.position.x + node.width &&
          world.y >= node.position.y &&
          world.y <= node.position.y + node.height;
        const hitsExpanded =
          world.x >= node.position.x - padding &&
          world.x <= node.position.x + node.width + padding &&
          world.y >= node.position.y - padding &&
          world.y <= node.position.y + node.height + padding;

        if (!hitsHandle && !hitsInside && !hitsExpanded) return;
        isNearNode = true;
        if (
          node.id === current.nodeId ||
          (!integrationRef.current &&
            !normalizeConnection(
              current.nodeId,
              node.id,
              nodesRef.current,
              current.handleType,
            ))
        )
          return;

        const priority = hitsInside ? 0 : hitsHandle ? 1 : 2;
        if (priority < bestPriority) {
          bestNodeId = node.id;
          bestPriority = priority;
        }
      });

      return { nodeId: bestNodeId, isNearNode };
    },
    [screenToCanvas],
  );

  const viewBounds = useMemo(() => {
    const padding = 280;
    const rect = containerRef.current?.getBoundingClientRect();
    const width = rect?.width || size.width;
    const height = rect?.height || size.height;
    const left = -viewport.x / viewport.k - padding;
    const top = -viewport.y / viewport.k - padding;
    return {
      left,
      top,
      right: left + width / viewport.k + padding * 2,
      bottom: top + height / viewport.k + padding * 2,
    };
  }, [size.height, size.width, viewport.k, viewport.x, viewport.y]);

  const nodeById = useMemo(
    () => new Map(nodes.map((node) => [node.id, node])),
    [nodes],
  );
  const spatialIndex = useMemo(
    () =>
      buildCanvasSpatialIndex(
        nodes.map((node) => ({
          id: node.id,
          bounds: canvasNodeBounds(node),
          value: node,
        })),
      ),
    [nodes],
  );
  const nodeOrder = useMemo(
    () => new Map(nodes.map((node, index) => [node.id, index])),
    [nodes],
  );
  const visibleNodes = useMemo(() => {
    const visible = new Map(
      spatialIndex.query(viewBounds).map((node) => [node.id, node]),
    );
    // Keep active editors and expanded batches mounted outside their root bounds.
    const pinnedIds = new Set([...selectedNodeIds, ...expandedBatchNodeIds]);
    if (isNodeDragging)
      nodeDragSessionRef.current?.movedIds.forEach((id) => pinnedIds.add(id));
    if (dialogNodeId) pinnedIds.add(dialogNodeId);
    pinnedIds.forEach((id) => {
      const node = nodeById.get(id);
      if (node) visible.set(id, node);
    });
    return [...visible.values()].sort(
      (a, b) => (nodeOrder.get(a.id) ?? 0) - (nodeOrder.get(b.id) ?? 0),
    );
  }, [
    spatialIndex,
    viewBounds,
    selectedNodeIds,
    expandedBatchNodeIds,
    dialogNodeId,
    nodeById,
    nodeOrder,
    isNodeDragging,
  ]);

  // Connections are culled like nodes; a cubic curve stays inside its control-point hull, so endpoint bounds widened by the curvature is a safe test.
  const visibleConnections = useMemo(
    () =>
      connections.flatMap((connection) => {
        const from = nodeById.get(connection.fromNodeId);
        const to = nodeById.get(connection.toNodeId);
        if (!from || !to) return [];
        const startX = from.position.x + from.width;
        const startY = from.position.y + from.height / 2;
        const endX = to.position.x;
        const endY = to.position.y + to.height / 2;
        const { bounds } = canvasConnectionGeometry(
          { x: startX, y: startY },
          { x: endX, y: endY },
        );
        const dragging =
          isNodeDragging &&
          (nodeDragSessionRef.current?.movedIds.has(from.id) ||
            nodeDragSessionRef.current?.movedIds.has(to.id));
        const inView =
          bounds.right >= viewBounds.left &&
          bounds.left <= viewBounds.right &&
          bounds.bottom >= viewBounds.top &&
          bounds.top <= viewBounds.bottom;
        return dragging || inView ? [{ connection, from, to }] : [];
      }),
    [connections, nodeById, viewBounds, isNodeDragging],
  );
  useLayoutEffect(() => {
    releasedDragSessionRef.current?.settle(nodes, connections);
    releasedDragSessionRef.current = null;
    nodeDragSessionRef.current?.refresh(nodes, connections);
  }, [nodes, connections, isNodeDragging, selectedNodeIds, showConnections]);
  useEffect(() => () => nodeDragSessionRef.current?.cancel(), []);
  // The toolbar follows a single selected node selected by click, creation, marquee, or keyboard.
  // It stays hidden for multi-selection and while isNodeDragging is true.
  const singleSelectedNodeId =
    selectedNodeIds.size === 1 ? Array.from(selectedNodeIds)[0] : null;
  const toolbarNode =
    (toolbarNodeId ? nodeById.get(toolbarNodeId) || null : null) ||
    (singleSelectedNodeId ? nodeById.get(singleSelectedNodeId) || null : null);
  const infoNode = infoNodeId ? nodeById.get(infoNodeId) || null : null;
  const cropNode = cropNodeId ? nodeById.get(cropNodeId) || null : null;
  const maskEditNode = maskEditNodeId
    ? nodeById.get(maskEditNodeId) || null
    : null;
  const splitNode = splitNodeId ? nodeById.get(splitNodeId) || null : null;
  const upscaleNode = upscaleNodeId
    ? nodeById.get(upscaleNodeId) || null
    : null;
  const superResolveNode = superResolveNodeId
    ? nodeById.get(superResolveNodeId) || null
    : null;
  const angleNode = angleNodeId ? nodeById.get(angleNodeId) || null : null;
  const contextMenuNode =
    contextMenu?.type === "node"
      ? nodeById.get(contextMenu.nodeId) || null
      : null;
  const previewNode = previewNodeId
    ? nodeById.get(previewNodeId) || null
    : null;
  const previewContent = previewImageId
    ? previewNode?.metadata?.images?.find(
        (image) => image.id === previewImageId,
      )?.content
    : previewNode?.metadata?.content;
  const hasMultipleSelectedNodes = selectedNodeIds.size > 1;
  const selectedNodes = useMemo(
    () => nodes.filter((node) => selectedNodeIds.has(node.id)),
    [nodes, selectedNodeIds],
  );
  const canGroupSelection = canGroupSelectedNodes(selectedNodeIds, nodes);
  const canUngroupSelection = canUngroupSelectedNodes(selectedNodeIds, nodes);
  const activeNodeId = hasMultipleSelectedNodes
    ? null
    : hoveredNodeId ||
      (selectedNodeIds.size === 1 ? Array.from(selectedNodeIds)[0] : null);
  const groupChildCountById = useMemo(() => {
    const map = new Map<string, number>();
    nodes.forEach((node) => {
      const groupId = node.metadata?.groupId;
      if (groupId) map.set(groupId, (map.get(groupId) || 0) + 1);
    });
    return map;
  }, [nodes]);
  const relatedHighlight = useMemo(() => {
    const nodeIds = new Set<string>();
    const connectionIds = new Set<string>();

    if (!activeNodeId) return { nodeIds, connectionIds };

    const addNode = (nodeId: string) => {
      nodeIds.add(nodeId);
      if (nodeById.get(nodeId)?.type === CanvasNodeType.Group)
        nodes.forEach(
          (node) => node.metadata?.groupId === nodeId && nodeIds.add(node.id),
        );
    };
    addNode(activeNodeId);
    connections.forEach((connection) => {
      if (
        connection.fromNodeId !== activeNodeId &&
        connection.toNodeId !== activeNodeId
      )
        return;
      connectionIds.add(connection.id);
      addNode(connection.fromNodeId);
      addNode(connection.toNodeId);
    });

    return { nodeIds, connectionIds };
  }, [activeNodeId, connections, nodeById, nodes]);

  const configInputsById = useMemo(() => {
    const map = new Map<string, NodeGenerationInput[]>();
    nodes.forEach((node) => {
      if (node.type !== CanvasNodeType.Config) return;
      map.set(node.id, buildNodeGenerationInputs(node.id, nodes, connections));
    });
    return map;
  }, [connections, nodes]);
  const mentionReferencesByNodeId = useMemo(() => {
    const map = new Map<
      string,
      ReturnType<typeof buildNodeMentionReferences>
    >();
    nodes.forEach((node) =>
      map.set(node.id, buildNodeMentionReferences(node, nodes, connections)),
    );
    return map;
  }, [connections, nodes]);
  const connectedNodesByNodeId = useMemo(
    () => buildConnectedNodeMap(nodes, connections),
    [connections, nodes],
  );
  const referenceConnectedNodeIds = useMemo(
    () =>
      new Set(
        [
          referencePickerNodeId,
          ...(referencePickerNodeId
            ? connectedNodesByNodeId
                .get(referencePickerNodeId)
                ?.flatMap((node) =>
                  node.type === CanvasNodeType.Group
                    ? [
                        node.id,
                        ...getGroupResourceNodes(node.id, nodes).map(
                          (child) => child.id,
                        ),
                      ]
                    : [node.id],
                ) || []
            : []),
        ].filter((id): id is string => Boolean(id)),
      ),
    [connectedNodesByNodeId, nodes, referencePickerNodeId],
  );
  const applyCanvasOps = useCanvasOperations({
    nodesRef,
    connectionsRef,
    selectedNodeIdsRef,
    viewportRef,
    generateNodeRef,
    setNodes,
    setConnections,
    setSelectedNodeIds,
    setSelectedConnectionId,
    setViewport,
    setContextMenu,
  });

  const { pluginHost, renderPluginPanel, buildNodeToolbarItems } =
    usePluginHost({
      effectiveConfig,
      isAiConfigReady,
      theme,
      nodesRef,
      connectionsRef,
      viewportRef,
      setNodes,
      setDialogNodeId,
      applyCanvasOps,
    });
  const createNode = useCallback(
    (type: CanvasNodeTypeId, position?: Position) => {
      const targetPosition = position || getCanvasCenter();
      const configMetadata =
        type === CanvasNodeType.Config
          ? {
              model: effectiveConfig.imageModel || effectiveConfig.model,
              size: effectiveConfig.size,
              count: getGenerationCount(
                effectiveConfig.canvasImageCount || effectiveConfig.count,
              ),
            }
          : undefined;
      const newNode = createCanvasNode(type, targetPosition, configMetadata);
      if (!position)
        newNode.position = {
          x: targetPosition.x - newNode.width / 2,
          y: targetPosition.y - newNode.height / 2,
        };

      setNodes((prev) => [...prev, newNode]);
      setSelectedNodeIds(new Set([newNode.id]));
      setSelectedConnectionId(null);
      const definition = getNodeDefinition(type);
      // Display-only plugin nodes with hidePanel do not open a panel; custom Panels require autoOpenPanel on creation.
      // Plugin nodes declaring useBuiltinPanel open the built-in generation panel on creation, like image nodes.
      // Built-in image, video, and config nodes retain their existing open-on-create behavior.
      const wantsPanel = definition?.hidePanel
        ? false
        : definition?.Panel
          ? Boolean(definition.autoOpenPanel)
          : definition?.useBuiltinPanel
            ? true
            : isBuiltinType(type) &&
              type !== CanvasNodeType.Text &&
              type !== CanvasNodeType.Audio &&
              type !== CanvasNodeType.Group;
      if (wantsPanel) setDialogNodeId(newNode.id);
    },
    [
      effectiveConfig.canvasImageCount,
      effectiveConfig.count,
      effectiveConfig.imageModel,
      effectiveConfig.model,
      effectiveConfig.size,
      getCanvasCenter,
      setNodes,
    ],
  );

  const deleteNodes = useCallback(
    (ids: Set<string>) => {
      if (!ids.size) return;
      const allIds = new Set(ids);
      const next = removeCanvasReferences(
        nodesRef.current,
        connectionsRef.current,
        { nodeIds: allIds },
      );
      nodesRef.current = next.nodes;
      connectionsRef.current = next.connections;
      setNodes(next.nodes);
      setConnections(next.connections);
      setSelectedNodeIds(new Set());
      setSelectedConnectionId(null);
      setHoveredNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setToolbarNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setDialogNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setInfoNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setCropNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setMaskEditNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setAngleNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setPreviewNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setRunningNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setReferencePickerNodeId((current) =>
        current && allIds.has(current) ? null : current,
      );
      setExpandedBatchNodeIds(
        (current) =>
          new Set([...current].filter((nodeId) => !allIds.has(nodeId))),
      );
      setContextMenu((current) =>
        current?.type === "node" && allIds.has(current.nodeId) ? null : current,
      );
    },
    [setConnections, setNodes],
  );

  const groupSelection = useCallback(() => {
    const selectedIds = selectedNodeIdsRef.current;
    const members = collectGroupMemberNodes(selectedIds, nodesRef.current);
    if (members.length < 2) return;
    const rect = getGroupWrapRect(members);
    const created = createCanvasNode(CanvasNodeType.Group, {
      x: rect.x + rect.width / 2,
      y: rect.y + rect.height / 2,
    });
    const result = applyGroupSelection(
      selectedIds,
      nodesRef.current,
      connectionsRef.current,
      {
        ...created,
        position: { x: rect.x, y: rect.y },
        width: rect.width,
        height: rect.height,
      },
    );
    if (!result) return;
    setNodes(result.nodes);
    setConnections(result.connections);
    setSelectedNodeIds(new Set(result.selectedIds));
    setSelectedConnectionId(null);
    setToolbarNodeId(result.selectedIds[0] || null);
    setDialogNodeId(null);
    setContextMenu(null);
  }, [setConnections, setNodes]);

  const ungroupSelection = useCallback(
    (ids?: Set<string>) => {
      const result = applyUngroupSelection(
        ids || selectedNodeIdsRef.current,
        nodesRef.current,
        connectionsRef.current,
      );
      if (!result) return;
      setNodes(result.nodes);
      setConnections(result.connections);
      setSelectedNodeIds(new Set(result.selectedIds));
      setSelectedConnectionId(null);
      setToolbarNodeId(
        result.selectedIds.length === 1 ? result.selectedIds[0] : null,
      );
      setDialogNodeId(null);
      setContextMenu(null);
    },
    [setConnections, setNodes],
  );

  const deleteConnection = useCallback(
    (connectionId: string) => {
      const next = removeCanvasReferences(
        nodesRef.current,
        connectionsRef.current,
        { connectionIds: new Set([connectionId]) },
      );
      nodesRef.current = next.nodes;
      connectionsRef.current = next.connections;
      setNodes(next.nodes);
      setConnections(next.connections);
      setSelectedConnectionId((current) =>
        current === connectionId ? null : current,
      );
      setContextMenu((current) =>
        current?.type === "connection" && current.connectionId === connectionId
          ? null
          : current,
      );
    },
    [setConnections, setNodes],
  );

  const disconnectNodeReference = useCallback(
    (fromNodeId: string, toNodeId: string) => {
      const next = removeCanvasReferences(
        nodesRef.current,
        connectionsRef.current,
        { source: { fromNodeId, toNodeId } },
      );
      nodesRef.current = next.nodes;
      connectionsRef.current = next.connections;
      setNodes(next.nodes);
      setConnections(next.connections);
    },
    [setConnections, setNodes],
  );

  const startNodeReferenceSelection = useCallback((nodeId: string) => {
    setReferencePickerNodeId(nodeId);
    setSelectedNodeIds(new Set([nodeId]));
    setSelectedConnectionId(null);
    setDialogNodeId(null);
  }, []);

  const exitNodeReferenceSelection = useCallback(() => {
    if (!referencePickerNodeId) return;
    setSelectedNodeIds(new Set([referencePickerNodeId]));
    setDialogNodeId(referencePickerNodeId);
    setReferencePickerNodeId(null);
  }, [referencePickerNodeId]);

  const selectNodeReference = useCallback(
    (fromNodeId: string) => {
      if (!referencePickerNodeId || referenceConnectedNodeIds.has(fromNodeId))
        return;
      const source = nodesRef.current.find((node) => node.id === fromNodeId);
      if (integrationRef.current) {
        connectDocumentNodes(fromNodeId, referencePickerNodeId);
        return;
      }
      if (!source || !isCanvasReferenceNode(source, nodesRef.current)) return;
      setConnections((prev) => [
        ...prev,
        { id: nanoid(), fromNodeId, toNodeId: referencePickerNodeId },
      ]);
    },
    [
      connectDocumentNodes,
      referenceConnectedNodeIds,
      referencePickerNodeId,
      setConnections,
    ],
  );

  useEffect(() => {
    if (!referencePickerNodeId) return;
    const exit = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !isCanvasShortcut(event)) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      exitNodeReferenceSelection();
    };
    window.addEventListener("keydown", exit);
    return () => window.removeEventListener("keydown", exit);
  }, [exitNodeReferenceSelection, referencePickerNodeId]);

  const deselectCanvas = useCallback(() => {
    cancelPendingConnectionCreate();
    setSelectedNodeIds(new Set());
    setSelectedConnectionId(null);
    setContextMenu(null);
    setSelectionBox(null);
    setHoveredNodeId(null);
    setToolbarNodeId(null);
    setDialogNodeId(null);
  }, [cancelPendingConnectionCreate]);

  const clearCanvas = useCallback(() => {
    setNodes([]);
    setConnections([]);
    setInfoNodeId(null);
    setCropNodeId(null);
    setMaskEditNodeId(null);
    setAngleNodeId(null);
    setPreviewNodeId(null);
    setRunningNodeId(null);
    deselectCanvas();
    setClearConfirmOpen(false);
  }, [deselectCanvas, setConnections, setNodes]);

  const duplicateNode = useCallback(
    (nodeId: string) => {
      const source = nodesRef.current.find((node) => node.id === nodeId);
      if (!source) return;

      const id = crypto.randomUUID();
      const next: CanvasNodeData = {
        ...source,
        id,
        title: `${source.title} Copy`,
        position: { x: source.position.x + 36, y: source.position.y + 36 },
      };

      setNodes((prev) => [...prev, next]);
      setSelectedNodeIds(new Set([id]));
      setSelectedConnectionId(null);
      if (next.type !== CanvasNodeType.Group) setDialogNodeId(id);
    },
    [setNodes],
  );

  const copySelectedNodes = useCallback(() => {
    const selectedIds = selectedNodeIdsRef.current;
    if (!selectedIds.size) return;

    const copiedNodes = nodesRef.current
      .filter((node) => selectedIds.has(node.id))
      .map((node) => ({
        ...node,
        position: { ...node.position },
        metadata: node.metadata ? { ...node.metadata } : undefined,
      }));

    if (!copiedNodes.length) return;

    clipboardRef.current = {
      nodes: copiedNodes,
      connections: connectionsRef.current
        .filter(
          (connection) =>
            selectedIds.has(connection.fromNodeId) &&
            selectedIds.has(connection.toNodeId),
        )
        .map((connection) => ({ ...connection })),
    };
  }, []);

  const pasteCopiedNodes = useCallback(() => {
    const clipboard = clipboardRef.current;
    if (!clipboard?.nodes.length) return false;

    const center = getCanvasCenter();
    const bounds = clipboard.nodes.reduce(
      (acc, node) => ({
        left: Math.min(acc.left, node.position.x),
        top: Math.min(acc.top, node.position.y),
        right: Math.max(acc.right, node.position.x + node.width),
        bottom: Math.max(acc.bottom, node.position.y + node.height),
      }),
      { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity },
    );
    const dx = center.x - (bounds.left + bounds.right) / 2;
    const dy = center.y - (bounds.top + bounds.bottom) / 2;
    const idMap = new Map<string, string>();
    const nextNodes = clipboard.nodes.map((node) => {
      const id = crypto.randomUUID();
      idMap.set(node.id, id);
      return {
        ...node,
        id,
        title: node.title.endsWith(" Copy") ? node.title : `${node.title} Copy`,
        position: {
          x: node.position.x + dx,
          y: node.position.y + dy,
        },
        metadata: node.metadata ? { ...node.metadata } : undefined,
      };
    });

    const pastedNodes = nextNodes.map((node) => {
      const groupId = node.metadata?.groupId;
      if (!groupId) return node;
      return {
        ...node,
        metadata: { ...node.metadata, groupId: idMap.get(groupId) },
      };
    });

    const nextConnections = clipboard.connections.flatMap((connection) => {
      const fromNodeId = idMap.get(connection.fromNodeId);
      const toNodeId = idMap.get(connection.toNodeId);
      if (!fromNodeId || !toNodeId) return [];
      return [
        {
          ...connection,
          id: crypto.randomUUID(),
          fromNodeId,
          toNodeId,
        },
      ];
    });

    setNodes((prev) => [...prev, ...pastedNodes]);
    setConnections((prev) => [...prev, ...nextConnections]);
    setSelectedNodeIds(new Set(pastedNodes.map((node) => node.id)));
    setSelectedConnectionId(null);
    setContextMenu(null);
    setDialogNodeId(
      pastedNodes[0]?.type === CanvasNodeType.Group
        ? null
        : pastedNodes[0]?.id || null,
    );
    return true;
  }, [getCanvasCenter, setConnections, setNodes]);

  const resetViewport = useCallback(() => {
    if (focusAnimRef.current) cancelAnimationFrame(focusAnimRef.current);
    setViewport(fitCanvasNodes(nodesRef.current, size));
    setContextMenu(null);
  }, [size, setViewport]);

  const focusNode = useCallback(
    (nodeId: string) => {
      const node = nodesRef.current.find((item) => item.id === nodeId);
      if (!node) return;
      const worldX = node.position.x + node.width / 2;
      const worldY = node.position.y + node.height / 2;
      const k = Math.min(
        Math.max(
          Math.min(
            (size.width * 0.6) / node.width,
            (size.height * 0.6) / node.height,
          ),
          0.05,
        ),
        1,
      );
      const target = {
        x: size.width / 2 - worldX * k,
        y: size.height / 2 - worldY * k,
        k,
      };
      setSelectedNodeIds(new Set([nodeId]));
      setSelectedConnectionId(null);
      setContextMenu(null);

      if (focusAnimRef.current) cancelAnimationFrame(focusAnimRef.current);
      const start = { ...viewportRef.current };
      const duration = 450;
      const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3);
      let startTime: number | null = null;
      const step = (now: number) => {
        if (startTime === null) startTime = now;
        const progress = Math.min((now - startTime) / duration, 1);
        const t = easeOutCubic(progress);
        setViewport({
          x: start.x + (target.x - start.x) * t,
          y: start.y + (target.y - start.y) * t,
          k: start.k + (target.k - start.k) * t,
        });
        focusAnimRef.current =
          progress < 1 ? requestAnimationFrame(step) : null;
      };
      focusAnimRef.current = requestAnimationFrame(step);
    },
    [size.height, size.width, setViewport],
  );

  useEffect(
    () => () =>
      void (focusAnimRef.current && cancelAnimationFrame(focusAnimRef.current)),
    [],
  );

  const setZoomScale = useCallback(
    (scale: number) => {
      if (focusAnimRef.current) cancelAnimationFrame(focusAnimRef.current);
      setViewport((prev) =>
        zoomAt(prev, { x: size.width / 2, y: size.height / 2 }, scale),
      );
      setContextMenu(null);
    },
    [size.height, size.width, setViewport],
  );

  const applyHistory = useCallback(
    (entry: CanvasHistoryEntry) => {
      if (historyCommitTimerRef.current) {
        clearTimeout(historyCommitTimerRef.current);
        historyCommitTimerRef.current = null;
      }
      applyingHistoryRef.current = true;
      setNodes(entry.nodes);
      setConnections(entry.connections);
      setChatSessions(entry.chatSessions);
      setActiveChatId(entry.activeChatId);
      setSelectedNodeIds(new Set());
      setSelectedConnectionId(null);
      setContextMenu(null);
      setTimeout(() => {
        lastHistoryRef.current = entry;
        applyingHistoryRef.current = false;
        setHistoryState({
          canUndo: historyRef.current.past.length > 0,
          canRedo: historyRef.current.future.length > 0,
        });
      });
    },
    [setConnections, setNodes],
  );

  const undoCanvas = useCallback(() => {
    if (integrationRef.current?.undo) {
      integrationRef.current.undo.undo();
      return;
    }
    const previous = historyRef.current.past.pop();
    const current = lastHistoryRef.current;
    if (!previous || !current) return;
    historyRef.current.future.push(current);
    applyHistory(previous);
  }, [applyHistory]);

  const redoCanvas = useCallback(() => {
    if (integrationRef.current?.undo) {
      integrationRef.current.undo.redo();
      return;
    }
    const next = historyRef.current.future.pop();
    const current = lastHistoryRef.current;
    if (!next || !current) return;
    historyRef.current.past.push(current);
    applyHistory(next);
  }, [applyHistory]);

  const draftChanged =
    nodes !== initialProject.nodes ||
    connections !== initialProject.connections ||
    chatSessions !== initialProject.chatSessions ||
    activeChatId !== initialProject.activeChatId ||
    backgroundMode !== initialProject.backgroundMode ||
    showImageInfo !== initialProject.showImageInfo ||
    viewport !== initialProject.viewport;
  useEffect(() => {
    if (!draftChanged || documentIntegration) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [draftChanged, documentIntegration]);
  const navigate = (href: string) => {
    if (documentIntegration) {
      if (documentIntegration.canNavigate) router.push(href);
      else message.error("请先保存画布或处理待确认的命令");
      return;
    }
    if (projectActionActive) {
      message.error("请先查询当前项目操作结果");
      return;
    }
    if (draftChanged)
      modal.confirm({
        title: "离开画布预览？",
        content: "节点编辑尚未保存，离开后会丢失。",
        okText: "确认离开",
        onOk: () => router.push(href),
      });
    else router.push(href);
  };
  const createAndOpenProject = () =>
    navigate(`${workspaceRoutes.projects.href}?mode=new`);
  const deleteCurrentProject = () => {
    message.info("请从项目空间打开已保存的画布");
  };
  const exportCurrentProject = () => message.info("项目导出尚未开放");

  const handleCanvasMouseDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      setContextMenu(null);
      setNodeCreatePosition(null);
      setHoveredNodeId(null);
      setToolbarNodeId(null);
      setDialogNodeId(null);
      if (pendingConnectionCreateRef.current) cancelPendingConnectionCreate();
      if (event.button !== 0) return;

      const world = screenToCanvas(event.clientX, event.clientY);
      const nextSelectionBox = {
        startWorldX: world.x,
        startWorldY: world.y,
        currentWorldX: world.x,
        currentWorldY: world.y,
        additive: event.shiftKey,
        initialSelectedNodeIds: event.shiftKey
          ? Array.from(selectedNodeIdsRef.current)
          : [],
      };
      selectionBoxRef.current = nextSelectionBox;
      setSelectionBox(nextSelectionBox);
      if (!event.shiftKey) {
        setSelectedNodeIds(new Set());
      }

      setSelectedConnectionId(null);
    },
    [cancelPendingConnectionCreate, screenToCanvas],
  );

  // Selection-only logic shared by the bubbling drag entry point and outer capture handler.
  // Returns the single target ID after the click, or null for multi-selection or deselection, to sync the toolbar.
  const selectNodeByEvent = useCallback(
    (
      event: Pick<ReactMouseEvent, "shiftKey" | "metaKey" | "ctrlKey">,
      nodeId: string,
    ) => {
      const nextSelected = new Set(selectedNodeIdsRef.current);
      if (event.shiftKey || event.metaKey || event.ctrlKey) {
        if (nextSelected.has(nodeId)) nextSelected.delete(nodeId);
        else nextSelected.add(nodeId);
      } else if (!nextSelected.has(nodeId)) {
        nextSelected.clear();
        nextSelected.add(nodeId);
      }
      setSelectedNodeIds(nextSelected);
      const soloId =
        nextSelected.size === 1 && nextSelected.has(nodeId) ? nodeId : null;
      setToolbarNodeId(soloId);
      return { nextSelected, soloId };
    },
    [],
  );

  // Capture-phase selection lets any inner element, including textarea or iframe, select the node and show its toolbar.
  // It only selects; body onMouseDown still starts dragging, so text selection inside editors does not drag the node.
  // Cache the capture result for the following bubbling drag handler to avoid applying shift-selection twice.
  const pendingSelectionRef = useRef<Set<string> | null>(null);
  const handleNodeSelectCapture = useCallback(
    (event: ReactMouseEvent, nodeId: string) => {
      if (event.button !== 0) return;
      setContextMenu(null);
      setHoveredNodeId(null);
      setSelectedConnectionId(null);
      const { nextSelected } = selectNodeByEvent(event, nodeId);
      pendingSelectionRef.current = nextSelected;
    },
    [selectNodeByEvent],
  );

  const handleNodeMouseDown = useCallback(
    (event: ReactMouseEvent, nodeId: string) => {
      event.stopPropagation();
      if (
        event.button !== 0 ||
        integrationRef.current?.frozen ||
        !containerRef.current
      )
        return;
      // Capture already selected the node; this only starts dragging, with a fallback selection if capture did not run.
      const currentNodes = nodesRef.current;
      const nextSelected =
        pendingSelectionRef.current ??
        selectNodeByEvent(event, nodeId).nextSelected;
      pendingSelectionRef.current = null;
      const dragIds = new Set(nextSelected);
      currentNodes.forEach((node) => {
        if (!nextSelected.has(node.id)) return;
        if (node.type === CanvasNodeType.Group) {
          currentNodes.forEach((child) => {
            if (child.metadata?.groupId === node.id) dragIds.add(child.id);
          });
        }
      });
      nodeDragSessionRef.current?.cancel();
      nodeDragSessionRef.current = new CanvasNodeDragSession({
        container: containerRef.current,
        nodes: currentNodes,
        connections: connectionsRef.current,
        movedIds: dragIds,
        start: { x: event.clientX, y: event.clientY },
        scale: viewportRef.current.k,
        snap: snapToGridRef.current,
        onDropTarget: setDropTargetGroupId,
      });
      historyPausedRef.current = true;
      nodeDraggingRef.current = true;
      setIsNodeDragging(true);
    },
    [selectNodeByEvent],
  );

  const finishNodeDrag = useCallback(
    (clientX?: number, clientY?: number) => {
      const session = nodeDragSessionRef.current;
      if (!session) return;
      const positions = session.finish(
        clientX != null && clientY != null
          ? { x: clientX, y: clientY }
          : undefined,
        viewportRef.current.k,
        snapToGridRef.current,
      );
      const wasClick = !positions && session.initialPositions.size === 1;
      const clickedNodeId = session.initialPositions.keys().next().value;
      const movedIds = session.movedIds;
      nodeDragSessionRef.current = null;
      releasedDragSessionRef.current = positions ? session : null;

      historyPausedRef.current = false;
      nodeDraggingRef.current = false;
      setIsNodeDragging(false);
      setDropTargetGroupId(null);
      if (positions) {
        setNodes((prev) => {
          const moved = applyCanvasDragPositions(prev, positions);
          const targetGroup = findGroupDropTarget(movedIds, moved);
          if (targetGroup)
            return snapNodesIntoGroup(movedIds, moved, targetGroup);
          return moved.map((node) => {
            if (!movedIds.has(node.id) || node.type === CanvasNodeType.Group)
              return node;
            const groupId = findContainingGroupId(node, moved);
            if (node.metadata?.groupId === groupId) return node;
            return { ...node, metadata: { ...node.metadata, groupId } };
          });
        });
      }

      if (wasClick && clickedNodeId) {
        const clickedNode = nodesRef.current.find(
          (node) => node.id === clickedNodeId,
        );
        const clickedDefinition = clickedNode
          ? getNodeDefinition(clickedNode.type)
          : undefined;
        if (clickedDefinition?.hidePanel) {
          // Clicking a display-only plugin node selects it without opening a lower panel.
          setDialogNodeId((current) =>
            current === clickedNodeId ? current : null,
          );
        } else if (clickedNode?.type !== CanvasNodeType.Group) {
          setDialogNodeId(clickedNodeId);
        }
      }
    },
    [setNodes],
  );

  useLayoutEffect(() => {
    if (!documentIntegration?.frozen || !nodeDragSessionRef.current) return;
    nodeDragSessionRef.current.cancel();
    nodeDragSessionRef.current = null;
    historyPausedRef.current = false;
    nodeDraggingRef.current = false;
    setIsNodeDragging(false);
    setDropTargetGroupId(null);
  }, [documentIntegration?.frozen]);

  const handleGlobalMouseMove = useCallback(
    (event: MouseEvent) => {
      if (nodeDragSessionRef.current) {
        nodeDragSessionRef.current.move(
          { x: event.clientX, y: event.clientY },
          viewportRef.current.k,
          snapToGridRef.current,
        );
        return;
      }

      if (connectingParamsRef.current && !pendingConnectionCreateRef.current) {
        const dropTarget = getConnectionDropTarget(
          event.clientX,
          event.clientY,
          connectingParamsRef.current,
        );
        connectionTargetNodeIdRef.current = dropTarget.nodeId;
        setConnectionTargetNodeId(dropTarget.nodeId);
        setMouseWorld(screenToCanvas(event.clientX, event.clientY));
      }
    },
    [getConnectionDropTarget, screenToCanvas],
  );

  const handleGlobalPointerMove = useCallback(
    (event: PointerEvent) => {
      const currentSelection = selectionBoxRef.current;
      if (!currentSelection) return;

      if (event.buttons === 0) {
        selectionBoxRef.current = null;
        setSelectionBox(null);
        return;
      }

      const world = screenToCanvas(event.clientX, event.clientY);
      const rectX = Math.min(currentSelection.startWorldX, world.x);
      const rectY = Math.min(currentSelection.startWorldY, world.y);
      const rectW = Math.abs(world.x - currentSelection.startWorldX);
      const rectH = Math.abs(world.y - currentSelection.startWorldY);
      const nextSelected = new Set<string>(
        currentSelection.additive
          ? currentSelection.initialSelectedNodeIds
          : [],
      );

      nodesRef.current.forEach((node) => {
        const intersects =
          rectX < node.position.x + node.width &&
          rectX + rectW > node.position.x &&
          rectY < node.position.y + node.height &&
          rectY + rectH > node.position.y;

        if (intersects) nextSelected.add(node.id);
      });

      const nextSelectionBox = {
        ...currentSelection,
        currentWorldX: world.x,
        currentWorldY: world.y,
      };
      selectionBoxRef.current = nextSelectionBox;
      setSelectionBox(nextSelectionBox);
      setSelectedNodeIds(nextSelected);
    },
    [screenToCanvas],
  );

  const handleGlobalMouseUp = useCallback(
    (event: MouseEvent) => {
      finishNodeDrag(event.clientX, event.clientY);

      selectionBoxRef.current = null;
      setSelectionBox(null);

      if (pendingConnectionCreateRef.current) return;

      const currentConnection = connectingParamsRef.current;
      if (currentConnection) {
        const dropTarget = getConnectionDropTarget(
          event.clientX,
          event.clientY,
          currentConnection,
        );
        if (dropTarget.nodeId) {
          connectNodes(currentConnection, dropTarget.nodeId);
          setConnecting(null);
        } else if (dropTarget.isNearNode) {
          setConnecting(null);
        } else {
          setMouseWorld(screenToCanvas(event.clientX, event.clientY));
          setPendingConnectionCreate({
            connection: currentConnection,
            position: screenToCanvas(event.clientX, event.clientY),
          });
        }
      }
    },
    [
      connectNodes,
      finishNodeDrag,
      getConnectionDropTarget,
      screenToCanvas,
      setConnecting,
    ],
  );

  useEffect(() => {
    const handlePointerUp = (event: PointerEvent) =>
      finishNodeDrag(event.clientX, event.clientY);
    const cancelNodeDrag = () => finishNodeDrag();
    window.addEventListener("mousemove", handleGlobalMouseMove);
    window.addEventListener("mouseup", handleGlobalMouseUp);
    window.addEventListener("pointerup", handlePointerUp);
    window.addEventListener("pointercancel", cancelNodeDrag);
    window.addEventListener("blur", cancelNodeDrag);
    window.addEventListener("pointermove", handleGlobalPointerMove);
    return () => {
      window.removeEventListener("mousemove", handleGlobalMouseMove);
      window.removeEventListener("mouseup", handleGlobalMouseUp);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", cancelNodeDrag);
      window.removeEventListener("blur", cancelNodeDrag);
      window.removeEventListener("pointermove", handleGlobalPointerMove);
    };
  }, [
    finishNodeDrag,
    handleGlobalMouseMove,
    handleGlobalMouseUp,
    handleGlobalPointerMove,
  ]);

  const createTextNodeFromClipboard = useCallback(
    (text: string) => {
      const trimmed = text.trim();
      if (!trimmed) return false;

      const node = {
        ...createCanvasNode(CanvasNodeType.Text, getCanvasCenter(), {
          content: trimmed,
          status: NODE_STATUS_SUCCESS,
        }),
        title: trimmed.slice(0, 32) || "剪切板文本",
      };

      setNodes((prev) => [...prev, node]);
      setSelectedNodeIds(new Set([node.id]));
      setSelectedConnectionId(null);
      setContextMenu(null);
      setDialogNodeId(node.id);
      return true;
    },
    [getCanvasCenter, setNodes],
  );

  const pasteSystemClipboard = useCallback(async () => {
    if (!navigator.clipboard) return;

    const items = await navigator.clipboard.read();
    const imageItem = items.find((item) =>
      item.types.some((type) => type.startsWith("image/")),
    );
    if (imageItem) {
      message.info("请通过素材上传图片");
      return;
    }

    const text = await navigator.clipboard.readText();
    if (createTextNodeFromClipboard(text))
      message.success("已从剪切板添加文本");
  }, [createTextNodeFromClipboard, message]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (integrationRef.current?.frozen) return;
      if (!isCanvasShortcut(event)) return;

      const key = event.key.toLowerCase();
      const isModifierShortcut = event.metaKey || event.ctrlKey;
      if (
        !isModifierShortcut &&
        !event.altKey &&
        !event.shiftKey &&
        (key === "v" || key === "h")
      ) {
        event.preventDefault();
        setCanvasTool(key === "v" ? "select" : "pan");
        return;
      }

      if (
        isModifierShortcut &&
        key === "c" &&
        window.getSelection()?.toString()
      )
        return;
      if (isModifierShortcut && ["0", "=", "+", "-"].includes(key)) {
        event.preventDefault();
        if (key === "0") resetViewport();
        else
          setZoomScale(viewportRef.current.k * (key === "-" ? 1 / 1.1 : 1.1));
        return;
      }

      if (
        key === "tab" &&
        !isModifierShortcut &&
        !event.altKey &&
        !event.shiftKey &&
        event.target === containerRef.current
      ) {
        const rect = containerRef.current?.getBoundingClientRect();
        if (rect) {
          event.preventDefault();
          setNodeCreatePosition(
            screenToCanvas(
              rect.left + rect.width / 2,
              rect.top + rect.height / 2,
            ),
          );
        }
        return;
      }

      if (isModifierShortcut && !event.altKey && key === "z") {
        event.preventDefault();
        if (event.shiftKey) redoCanvas();
        else undoCanvas();
        return;
      }

      if (isModifierShortcut && !event.altKey && key === "y") {
        event.preventDefault();
        redoCanvas();
        return;
      }

      if (isModifierShortcut && !event.altKey && key === "a") {
        event.preventDefault();
        setSelectedNodeIds(new Set(nodesRef.current.map((node) => node.id)));
        setSelectedConnectionId(null);
        setContextMenu(null);
        setSelectionBox(null);
        return;
      }

      if (isModifierShortcut && !event.altKey && key === "g") {
        if (event.shiftKey) {
          if (
            canUngroupSelectedNodes(
              selectedNodeIdsRef.current,
              nodesRef.current,
            )
          ) {
            event.preventDefault();
            ungroupSelection();
          }
          return;
        }
        if (
          canGroupSelectedNodes(selectedNodeIdsRef.current, nodesRef.current)
        ) {
          event.preventDefault();
          groupSelection();
        }
        return;
      }

      if (isModifierShortcut && !event.altKey && key === "c") {
        event.preventDefault();
        copySelectedNodes();
        return;
      }

      if (isModifierShortcut && !event.altKey && key === "v") {
        event.preventDefault();
        if (!pasteCopiedNodes()) void pasteSystemClipboard();
        return;
      }

      if (event.key === "Delete" || event.key === "Backspace") {
        event.preventDefault();
        if (selectedNodeIdsRef.current.size) {
          deleteNodes(new Set(selectedNodeIdsRef.current));
        } else if (selectedConnectionId) {
          deleteConnection(selectedConnectionId);
        }
      }

      if (event.key === "Escape") {
        setSelectedNodeIds(new Set());
        setSelectedConnectionId(null);
        setContextMenu(null);
        setNodeCreatePosition(null);
        setSelectionBox(null);
        setConnecting(null);
        setHoveredNodeId(null);
        setToolbarNodeId(null);
        setDialogNodeId(null);
        setInfoNodeId(null);
        setCropNodeId(null);
        setMaskEditNodeId(null);
        setPendingConnectionCreate(null);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [
    copySelectedNodes,
    deleteConnection,
    deleteNodes,
    groupSelection,
    pasteCopiedNodes,
    pasteSystemClipboard,
    redoCanvas,
    resetViewport,
    setZoomScale,
    selectedConnectionId,
    screenToCanvas,
    setConnecting,
    undoCanvas,
    ungroupSelection,
  ]);

  const handleConnectStart = useCallback(
    (
      event: ReactMouseEvent,
      nodeId: string,
      handleType: "source" | "target",
    ) => {
      event.stopPropagation();
      setMouseWorld(screenToCanvas(event.clientX, event.clientY));
      setConnecting({ nodeId, handleType });
      connectionTargetNodeIdRef.current = null;
      setConnectionTargetNodeId(null);
      setSelectedConnectionId(null);
    },
    [screenToCanvas, setConnecting],
  );

  const handleNodeResize = useCallback(
    (nodeId: string, width: number, height: number, position?: Position) => {
      setNodes((prev) =>
        prev.map((node) =>
          node.id === nodeId
            ? { ...node, width, height, position: position || node.position }
            : node,
        ),
      );
    },
    [setNodes],
  );

  const handleNodeResizeStart = useCallback(() => {
    setIsNodeResizing(true);
  }, []);
  const handleNodeResizeEnd = useCallback(() => setIsNodeResizing(false), []);

  const toggleNodeFreeResize = useCallback(
    (nodeId: string) => {
      setNodes((prev) =>
        prev.map((node) => {
          if (node.id !== nodeId) return node;
          const freeResize = !node.metadata?.freeResize;
          if (freeResize || node.type !== CanvasNodeType.Image)
            return { ...node, metadata: { ...node.metadata, freeResize } };
          const ratio =
            (node.metadata?.naturalWidth || node.width) /
            (node.metadata?.naturalHeight || node.height || 1);
          const height = node.width / ratio;
          return {
            ...node,
            height,
            position: {
              x: node.position.x,
              y: node.position.y + node.height / 2 - height / 2,
            },
            metadata: { ...node.metadata, freeResize },
          };
        }),
      );
    },
    [setNodes],
  );

  const handleNodeContentChange = useCallback(
    (nodeId: string, content: string, document?: RichTextDocument) => {
      setNodes((prev) =>
        prev.map((node) =>
          node.id === nodeId
            ? {
                ...node,
                metadata: {
                  ...node.metadata,
                  content,
                  richText: document,
                  texts: node.metadata?.texts?.map((text) =>
                    text.id ===
                    (node.metadata?.primaryTextId ??
                      node.metadata?.texts?.[0]?.id)
                      ? { ...text, content, richText: document }
                      : text,
                  ),
                },
              }
            : node,
        ),
      );
    },
    [setNodes],
  );

  const handleNodeTitleChange = useCallback(
    (nodeId: string, title: string) => {
      setNodes((prev) =>
        prev.map((node) => (node.id === nodeId ? { ...node, title } : node)),
      );
    },
    [setNodes],
  );

  const toggleBatchExpanded = useCallback((nodeId: string) => {
    setExpandedBatchNodeIds((current) => {
      const next = new Set(current);
      if (next.has(nodeId)) next.delete(nodeId);
      else next.add(nodeId);
      return next;
    });
  }, []);

  const setBatchPrimary = useCallback(
    (nodeId: string, itemId: string) => {
      setNodes((prev) =>
        prev.map((node) => {
          if (node.id !== nodeId) return node;
          if (node.type === CanvasNodeType.Text) {
            const text = node.metadata?.texts?.find(
              (item) => item.id === itemId,
            );
            return text?.content
              ? {
                  ...node,
                  metadata: {
                    ...node.metadata,
                    content: text.content,
                    richText: text.richText,
                    primaryTextId: text.id,
                  },
                }
              : node;
          }
          const image = node.metadata?.images?.find(
            (item) => item.id === itemId,
          );
          if (!image?.content) return node;
          const edge = Math.max(node.width, node.height);
          const size = node.metadata?.freeResize
            ? { width: node.width, height: node.height }
            : fitNodeSize(image.naturalWidth, image.naturalHeight, edge, edge);
          return {
            ...node,
            position: {
              x: node.position.x + node.width / 2 - size.width / 2,
              y: node.position.y + node.height / 2 - size.height / 2,
            },
            ...size,
            metadata: {
              ...node.metadata,
              content: image.content,
              storageKey: image.storageKey,
              naturalWidth: image.naturalWidth,
              naturalHeight: image.naturalHeight,
              bytes: image.bytes,
              mimeType: image.mimeType,
              primaryImageId: image.id,
            },
          };
        }),
      );
    },
    [setNodes],
  );

  const duplicateBatchImage = useCallback(
    (node: CanvasNodeData, imageId: string) => {
      const image = node.metadata?.images?.find((item) => item.id === imageId);
      if (!image?.content) return;
      const id = nanoid();
      const edge = Math.max(node.width, node.height);
      const size = fitNodeSize(
        image.naturalWidth,
        image.naturalHeight,
        edge,
        edge,
      );
      const copy: CanvasNodeData = {
        id,
        type: CanvasNodeType.Image,
        title: node.title,
        position: {
          x: node.position.x + node.width * 2 + 96,
          y: node.position.y + node.height / 2 - size.height / 2,
        },
        ...size,
        metadata: {
          content: image.content,
          storageKey: image.storageKey,
          naturalWidth: image.naturalWidth,
          naturalHeight: image.naturalHeight,
          bytes: image.bytes,
          mimeType: image.mimeType,
          status: NODE_STATUS_SUCCESS,
          prompt: node.metadata?.prompt,
          generationType: node.metadata?.generationType,
          model: node.metadata?.model,
          size: node.metadata?.size,
          quality: node.metadata?.quality,
          background: node.metadata?.background,
          references: node.metadata?.references,
        },
      };
      setNodes((prev) => [...prev, copy]);
      setSelectedNodeIds(new Set([id]));
      setSelectedConnectionId(null);
      setDialogNodeId(id);
    },
    [setNodes],
  );

  const handleNodePromptChange = useCallback(
    (nodeId: string, prompt: string) => {
      setNodes((prev) =>
        prev.map((node) =>
          node.id === nodeId
            ? { ...node, metadata: { ...node.metadata, prompt } }
            : node,
        ),
      );
    },
    [setNodes],
  );

  const handleConfigNodeChange = useCallback(
    (nodeId: string, patch: Partial<CanvasNodeData["metadata"]>) => {
      setNodes((prev) =>
        prev.map((node) =>
          node.id === nodeId ? applyNodeConfigPatch(node, patch) : node,
        ),
      );
    },
    [setNodes],
  );

  const downloadNodeImage = useCallback(
    (node: CanvasNodeData) => {
      void downloadCanvasOriginal(node, undefined, downloadAbort.signal).catch(
        (cause) => notifyError(cause, "素材下载尚未发起"),
      );
    },
    [downloadAbort],
  );

  const downloadBatchImage = useCallback(
    (node: CanvasNodeData, imageId: string) => {
      const image = node.metadata?.images?.find((item) => item.id === imageId);
      if (!image?.content) return;
      void downloadCanvasOriginal(node, image.id, downloadAbort.signal).catch(
        (cause) => notifyError(cause, "素材下载尚未发起"),
      );
    },
    [downloadAbort],
  );

  const captureVideoNodeFrame = useCallback(
    async (nodeId: string, position: VideoFramePosition) => {
      setContextMenu(null);
      const node = nodesRef.current.find((item) => item.id === nodeId);
      const video = Array.from(
        containerRef.current!.querySelectorAll<HTMLVideoElement>(
          "video[data-canvas-video]",
        ),
      ).find((item) => item.dataset.canvasVideo === nodeId);
      if (
        node?.type !== CanvasNodeType.Video ||
        !node.metadata?.content ||
        !video
      )
        return message.error("无法截取该画面，请重试");
      try {
        const image = await uploadImage(
          await captureVideoFrame(
            node.metadata.content,
            position,
            video.currentTime,
          ),
        );
        const size = fitNodeSize(
          image.width,
          image.height,
          VIDEO_NODE_MAX_WIDTH,
          VIDEO_NODE_MAX_HEIGHT,
        );
        const id = nanoid();
        const x = node.position.x + node.width + 96;
        let y = node.position.y + node.height / 2 - size.height / 2;
        while (
          nodesRef.current.some(
            (item) =>
              item.id !== node.id &&
              item.position.x < x + size.width &&
              item.position.x + item.width > x &&
              item.position.y < y + size.height &&
              item.position.y + item.height > y,
          )
        )
          y += size.height + 24;
        const child: CanvasNodeData = {
          id,
          type: CanvasNodeType.Image,
          title: `${node.title || "视频"} ${position === "first" ? "首帧" : position === "last" ? "尾帧" : "当前帧"}`,
          position: { x, y },
          ...size,
          metadata: imageMetadata(image),
        };
        setNodes((prev) => [...prev, child]);
        setConnections((prev) => [
          ...prev,
          { id: nanoid(), fromNodeId: node.id, toNodeId: id },
        ]);
        setSelectedNodeIds(new Set([id]));
        setSelectedConnectionId(null);
        setDialogNodeId(id);
        message.success("已生成图片节点");
      } catch {
        message.error("无法截取该画面，请重试");
      }
    },
    [message, setConnections, setNodes],
  );

  const saveNodeAsset = (node: CanvasNodeData) => {
    try {
      if (!account) throw new Error("请先登录后保存素材");
      canvasAssetRequest(node, "画布素材", "validation");
      setAssetSave({ node, accountId: String(account.id) });
    } catch (error) {
      notifyError(error, "无法保存素材");
    }
  };

  const createImageReversePromptNodes = useCallback(
    (node: CanvasNodeData) => {
      if (node.type !== CanvasNodeType.Image || !node.metadata?.content) {
        message.warning("图片节点为空，无法反推提示词");
        return;
      }

      const gap = 96;
      const textSpec = NODE_DEFAULT_SIZE[CanvasNodeType.Text];
      const configSpec = NODE_DEFAULT_SIZE[CanvasNodeType.Config];
      const centerY = node.position.y + node.height / 2;
      const textNode = {
        ...createCanvasNode(
          CanvasNodeType.Text,
          {
            x: node.position.x + node.width + gap + textSpec.width / 2,
            y: centerY,
          },
          {
            content:
              "请根据参考图片反推一段适合用于 AI 生图的提示词。\n\n要求：\n1. 只输出提示词正文，不要解释。\n2. 覆盖主体、构图、风格、光线、色彩、材质、镜头和氛围。\n3. 尽量写成可直接用于生图模型的完整提示词。",
            prompt:
              "请根据参考图片反推一段适合用于 AI 生图的提示词。\n\n要求：\n1. 只输出提示词正文，不要解释。\n2. 覆盖主体、构图、风格、光线、色彩、材质、镜头和氛围。\n3. 尽量写成可直接用于生图模型的完整提示词。",
            status: NODE_STATUS_SUCCESS,
            fontSize: 14,
          },
        ),
        title: "反推提示词",
      };
      const configNode = {
        ...createCanvasNode(
          CanvasNodeType.Config,
          {
            x:
              textNode.position.x + textNode.width + gap + configSpec.width / 2,
            y: centerY,
          },
          {
            generationMode: "text",
            model:
              effectiveConfig.textModel ||
              effectiveConfig.model ||
              defaultConfig.textModel,
            count: 1,
            composerContent: `参考图片：@[node:${node.id}]\n任务说明：@[node:${textNode.id}]`,
          },
        ),
        title: "反推提示词配置",
      };

      setNodes((prev) => [...prev, textNode, configNode]);
      setConnections((prev) => [
        ...prev,
        { id: nanoid(), fromNodeId: node.id, toNodeId: configNode.id },
        { id: nanoid(), fromNodeId: textNode.id, toNodeId: configNode.id },
      ]);
      setSelectedNodeIds(new Set([configNode.id]));
      setSelectedConnectionId(null);
      setDialogNodeId(configNode.id);
      setContextMenu(null);
    },
    [
      effectiveConfig.model,
      effectiveConfig.textModel,
      message,
      setConnections,
      setNodes,
    ],
  );

  const cropImageNode = useCallback(
    async (node: CanvasNodeData, crop: CanvasImageCropRect) => {
      if (!node.metadata?.content) return;
      const cropped = await cropDataUrl(node.metadata.content, crop);
      const image = await uploadImage(cropped);
      const width = Math.min(node.width, Math.max(220, image.width));
      const childId = nanoid();
      const child: CanvasNodeData = {
        id: childId,
        type: CanvasNodeType.Image,
        title: "Cropped Image",
        position: { x: node.position.x + node.width + 96, y: node.position.y },
        width,
        height: width * (image.height / image.width),
        metadata: {
          ...imageMetadata(image),
          prompt: node.metadata?.prompt,
        },
      };
      setNodes((prev) => [...prev, child]);
      setConnections((prev) => [
        ...prev,
        { id: nanoid(), fromNodeId: node.id, toNodeId: childId },
      ]);
      setSelectedNodeIds(new Set([childId]));
      setDialogNodeId(childId);
      setCropNodeId(null);
    },
    [setConnections, setNodes],
  );

  const splitImageNode = useCallback(
    async (node: CanvasNodeData, params: CanvasImageSplitParams) => {
      if (!node.metadata?.content) return;
      setSplitNodeId(null);
      const pieces = await splitDataUrl(node.metadata.content, params);
      const gap = 16;
      const cellWidth = node.width / params.columns;
      const cellHeight = node.height / params.rows;
      const startX = node.position.x + node.width + 96;
      const startY = node.position.y;
      const childNodes = await Promise.all(
        pieces.map(async (piece) => {
          const image = await uploadImage(piece.dataUrl);
          const id = nanoid();
          return {
            id,
            type: CanvasNodeType.Image,
            title: `${node.title || "图片"} ${piece.row + 1}-${piece.column + 1}`,
            position: {
              x: startX + piece.column * (cellWidth + gap),
              y: startY + piece.row * (cellHeight + gap),
            },
            width: cellWidth,
            height: cellHeight,
            metadata: {
              ...imageMetadata(image),
              prompt: node.metadata?.prompt,
            },
          } satisfies CanvasNodeData;
        }),
      );
      setNodes((prev) => [...prev, ...childNodes]);
      setConnections((prev) => [
        ...prev,
        ...childNodes.map((child) => ({
          id: nanoid(),
          fromNodeId: node.id,
          toNodeId: child.id,
        })),
      ]);
      setSelectedNodeIds(new Set(childNodes.map((child) => child.id)));
      setSelectedConnectionId(null);
      setDialogNodeId(null);
      message.success(`已切分为 ${childNodes.length} 个子节点`);
    },
    [message, setConnections, setNodes],
  );

  const maskEditImageNode = useCallback(
    async (node: CanvasNodeData, payload: CanvasImageMaskEditPayload) => {
      if (integrationRef.current) {
        message.warning("独立画布当前支持内容保存，执行功能待接入");
        return;
      }
      if (!node.metadata?.content) return;
      const generationConfig = {
        ...buildGenerationConfig(effectiveConfig, node, "image"),
        count: "1",
        size: node.metadata?.size || "auto",
      };
      if (
        payload.generate &&
        !isAiConfigReady(generationConfig, generationConfig.model)
      ) {
        message.warning("生成服务尚未接入");
        return;
      }
      const userPrompt = payload.prompt.trim();
      const prompt = `参考${imageReferenceLabel(0)}为原图，${imageReferenceLabel(1)}是在原图上用蓝色半透明标注出的待修改区域。请只修改蓝色标注覆盖的区域，其余区域与原图保持完全一致，输出与原图相同尺寸的完整图片，并且结果中不要保留任何蓝色标注。修改要求：${userPrompt}`;
      setMaskEditNodeId(null);
      const maskImage = await uploadImage(payload.maskDataUrl);
      const maskNodeId = nanoid();
      const childId = nanoid();
      const source = {
        id: node.id,
        name: `${node.title || node.id}.png`,
        type: node.metadata.mimeType || "image/png",
        dataUrl: node.metadata.content,
        storageKey: node.metadata.storageKey,
      };
      const maskSource = {
        id: maskNodeId,
        name: "mask.png",
        type: maskImage.mimeType || "image/png",
        dataUrl: maskImage.url,
        storageKey: maskImage.storageKey,
      };
      const references = [source, maskSource];
      const generationMetadata = buildImageGenerationMetadata(
        "edit",
        generationConfig,
        1,
        references,
      );
      const childMetadata = payload.generate
        ? { prompt, status: NODE_STATUS_LOADING, ...generationMetadata }
        : { prompt };
      setNodes((prev) => [
        ...prev,
        {
          id: maskNodeId,
          type: CanvasNodeType.Image,
          title: "遮罩标注",
          position: {
            x: node.position.x,
            y: node.position.y + node.height + 96,
          },
          width: node.width,
          height: node.height,
          metadata: imageMetadata(maskImage),
        },
        {
          id: childId,
          type: CanvasNodeType.Image,
          title: userPrompt.slice(0, 32) || "局部编辑结果",
          position: {
            x: node.position.x + node.width + 96,
            y: node.position.y,
          },
          width: node.width,
          height: node.height,
          metadata: childMetadata,
        },
      ]);
      setConnections((prev) => [
        ...prev,
        { id: nanoid(), fromNodeId: node.id, toNodeId: childId },
        { id: nanoid(), fromNodeId: maskNodeId, toNodeId: childId },
      ]);
      setSelectedNodeIds(new Set([childId]));
      setSelectedConnectionId(null);
      setDialogNodeId(childId);
      if (!payload.generate) return;
      setRunningNodeId(childId);
      const controller = startGenerationRequest(childId, node.id, childId);
      try {
        const image = await requestEdit(generationConfig, prompt, references, {
          signal: controller.signal,
        }).then((items) => items[0]);
        const uploaded = await uploadImage(image.dataUrl, {
          signal: controller.signal,
        });
        const size = fitNodeSize(
          uploaded.width,
          uploaded.height,
          node.width,
          node.height,
        );
        setNodes((prev) =>
          prev.map((item) =>
            item.id === childId
              ? {
                  ...item,
                  width: size.width,
                  height: size.height,
                  metadata: {
                    ...item.metadata,
                    ...imageMetadata(uploaded),
                    prompt,
                    ...generationMetadata,
                  },
                }
              : item,
          ),
        );
      } catch (error) {
        if (isGenerationCanceled(error)) return;
        const errorDetails =
          error instanceof Error ? error.message : "局部修改失败";
        message.error(errorDetails);
        setNodes((prev) =>
          prev.map((item) =>
            item.id === childId
              ? {
                  ...item,
                  metadata: {
                    ...item.metadata,
                    status: NODE_STATUS_ERROR,
                    errorDetails,
                  },
                }
              : item,
          ),
        );
      } finally {
        finishGenerationRequest(childId, controller);
        setRunningNodeId(null);
      }
    },
    [
      effectiveConfig,
      finishGenerationRequest,
      isAiConfigReady,
      message,
      setConnections,
      setNodes,
      startGenerationRequest,
    ],
  );

  const upscaleImageNode = useCallback(
    async (node: CanvasNodeData, params: CanvasImageUpscaleParams) => {
      if (!node.metadata?.content) return;
      setUpscaleNodeId(null);
      const upscaled = await upscaleDataUrl(node.metadata.content, params);
      const image = await uploadImage(upscaled);
      const size = fitNodeSize(image.width, image.height);
      const childId = nanoid();
      const child: CanvasNodeData = {
        id: childId,
        type: CanvasNodeType.Image,
        title: "Upscaled Image",
        position: { x: node.position.x + node.width + 96, y: node.position.y },
        width: size.width,
        height: size.height,
        metadata: {
          ...imageMetadata(image),
          prompt: node.metadata?.prompt,
        },
      };
      setNodes((prev) => [...prev, child]);
      setConnections((prev) => [
        ...prev,
        { id: nanoid(), fromNodeId: node.id, toNodeId: childId },
      ]);
      setSelectedNodeIds(new Set([childId]));
      setDialogNodeId(childId);
    },
    [setConnections, setNodes],
  );

  const generateAngleNode = useCallback(
    async (node: CanvasNodeData, params: CanvasImageAngleParams) => {
      if (integrationRef.current) {
        message.warning("独立画布当前支持内容保存，执行功能待接入");
        return;
      }
      if (!node.metadata?.content) return;
      const generationConfig = {
        ...buildGenerationConfig(effectiveConfig, node, "image"),
        count: "1",
      };
      if (!isAiConfigReady(generationConfig, generationConfig.model)) {
        message.warning("生成服务尚未接入");
        return;
      }
      const childId = nanoid();
      const imageConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
      const title = buildAngleLabel(params);
      const prompt = buildAnglePrompt(params);
      const generationMetadata = buildImageGenerationMetadata(
        "edit",
        generationConfig,
        1,
        [
          {
            id: node.id,
            name: `${node.title || node.id}.png`,
            type: node.metadata.mimeType || "image/png",
            dataUrl: node.metadata.content,
            storageKey: node.metadata.storageKey,
          },
        ],
      );
      setAngleNodeId(null);
      setRunningNodeId(childId);
      setNodes((prev) => [
        ...prev,
        {
          id: childId,
          type: CanvasNodeType.Image,
          title,
          position: {
            x: node.position.x + node.width + 96,
            y: node.position.y,
          },
          width: imageConfig.width,
          height: imageConfig.height,
          metadata: {
            prompt,
            status: NODE_STATUS_LOADING,
            ...generationMetadata,
          },
        },
      ]);
      setConnections((prev) => [
        ...prev,
        { id: nanoid(), fromNodeId: node.id, toNodeId: childId },
      ]);
      setSelectedNodeIds(new Set([childId]));
      setDialogNodeId(childId);
      const controller = startGenerationRequest(childId, node.id, childId);
      try {
        const image = await requestEdit(
          generationConfig,
          prompt,
          [
            {
              id: node.id,
              name: `${node.title || node.id}.png`,
              type: node.metadata.mimeType || "image/png",
              dataUrl: node.metadata.content,
              storageKey: node.metadata.storageKey,
            },
          ],
          {
            signal: controller.signal,
          },
        ).then((items) => items[0]);
        const uploaded = await uploadImage(image.dataUrl, {
          signal: controller.signal,
        });
        const size = fitNodeSize(
          uploaded.width,
          uploaded.height,
          imageConfig.width,
          imageConfig.height,
        );
        setNodes((prev) =>
          prev.map((item) =>
            item.id === childId
              ? {
                  ...item,
                  width: size.width,
                  height: size.height,
                  metadata: {
                    ...item.metadata,
                    ...imageMetadata(uploaded),
                    prompt,
                    ...generationMetadata,
                  },
                }
              : item,
          ),
        );
      } catch (error) {
        if (isGenerationCanceled(error)) return;
        const errorDetails =
          error instanceof Error ? error.message : "生成失败";
        message.error(errorDetails);
        setNodes((prev) =>
          prev.map((item) =>
            item.id === childId
              ? {
                  ...item,
                  metadata: {
                    ...item.metadata,
                    status: NODE_STATUS_ERROR,
                    errorDetails,
                  },
                }
              : item,
          ),
        );
      } finally {
        finishGenerationRequest(childId, controller);
        setRunningNodeId(null);
      }
    },
    [
      effectiveConfig,
      finishGenerationRequest,
      isAiConfigReady,
      message,
      setConnections,
      setNodes,
      startGenerationRequest,
    ],
  );

  const handleFontSizeChange = useCallback(
    (nodeId: string, fontSize: number) => {
      setNodes((prev) =>
        prev.map((node) =>
          node.id === nodeId
            ? { ...node, metadata: { ...node.metadata, fontSize } }
            : node,
        ),
      );
    },
    [setNodes],
  );

  const handleUploadRequest = useCallback((node?: CanvasNodeData) => {
    setAssetUploadOpen(false);
    setAssetTarget(node ? { id: node.id, type: node.type } : undefined);
    setSourceFile(undefined);
    setSourceFiles([]);
    setAssetPickerOpen(true);
  }, []);

  const handleDrop = useCallback((event: ReactDragEvent<HTMLDivElement>) => {
    event.preventDefault();
    if (integrationRef.current?.frozen) return;
    const files = Array.from(event.dataTransfer.files);
    if (!files.length) return;
    setAssetTarget(undefined);
    if (files.length > 1 && files.some((file) => /\.zip$/i.test(file.name))) {
      notifyError(new Error("ZIP导入每次请选择一个压缩包"), "素材尚未导入");
      return;
    }
    setSourceFile(files.length === 1 ? files[0] : undefined);
    setSourceFiles(files.length > 1 ? files : []);
    setAssetPickerOpen(true);
  }, []);

  const preventCanvasContextMenu = useCallback((event: ReactMouseEvent) => {
    if ((event.target as HTMLElement).closest("[data-node-id]")) return;
    event.preventDefault();
    setContextMenu(null);
  }, []);

  const documentGenerationRequests = useRef(new Set<string>());
  const generateDocumentNode = useCallback(
    async (nodeId: string) => {
      const integration = integrationRef.current;
      if (
        !integration?.generation ||
        integration.frozen ||
        documentGenerationRequests.current.has(nodeId)
      )
        return;
      documentGenerationRequests.current.add(nodeId);
      try {
        if (integration.generation.ready === false)
          throw new Error("正在读取生成记录，请稍后重试");
        const run = integration.generation.runs.find(
          (item) => item.nodeId === nodeId,
        );
        const next = repeatCanvasMediaGeneration(
          nodesRef.current,
          connectionsRef.current,
          nodeId,
          run,
        );
        if (next.nodeId !== nodeId) {
          nodesRef.current = next.nodes;
          connectionsRef.current = next.connections;
          setNodes(next.nodes);
          setConnections(next.connections);
          setSelectedNodeIds(new Set([next.nodeId]));
          setDialogNodeId(null);
        }
        integration.onChange(
          {
            nodes: next.nodes,
            connections: next.connections,
            backgroundMode,
            showImageInfo,
            viewport: savedViewport,
          },
          true,
        );
        await integration.generation.generate(next.nodeId);
      } catch (error) {
        notifyError(error, "画布生成未完成");
      } finally {
        documentGenerationRequests.current.delete(nodeId);
      }
    },
    [backgroundMode, showImageInfo, savedViewport, setNodes, setConnections],
  );

  const handleGenerateNode = useCallback(
    async (nodeId: string, mode: CanvasNodeGenerationMode, prompt: string) => {
      if (integrationRef.current) {
        const source = nodesRef.current.find((node) => node.id === nodeId);
        if (
          !source ||
          !["text", "image", "video"].includes(source.type) ||
          source.type !== mode
        ) {
          message.info("请从对应的文字、图片或视频节点生成");
          return;
        }
        await generateDocumentNode(nodeId);
        return;
      }
      const sourceNode = nodesRef.current.find((node) => node.id === nodeId);
      const generationConfig = buildGenerationConfig(
        effectiveConfig,
        sourceNode,
        mode,
      );
      if (!isAiConfigReady(generationConfig, generationConfig.model)) {
        message.warning("生成服务尚未接入");
        return;
      }

      // useBuiltinPanel.writeBackToSelf reuses built-in generation while writing the result back to the plugin node.
      // Image mode currently supports display-only nodes such as panoramas, with a useBuiltinPanel.promptPrefix.
      const builtinPanel = sourceNode
        ? getNodeDefinition(sourceNode.type)?.useBuiltinPanel
        : undefined;
      if (
        sourceNode &&
        builtinPanel?.writeBackToSelf &&
        builtinPanel.mode === "image"
      ) {
        const scene = prompt.trim();
        if (!scene) return;
        setRunningNodeId(nodeId);
        const controller = startGenerationRequest(nodeId, nodeId, nodeId);
        setNodes((prev) =>
          prev.map((node) =>
            node.id === nodeId
              ? {
                  ...node,
                  metadata: {
                    ...node.metadata,
                    prompt: scene,
                    status: NODE_STATUS_LOADING,
                    errorDetails: undefined,
                  },
                }
              : node,
          ),
        );
        try {
          const fullPrompt = (builtinPanel.promptPrefix || "") + scene;
          const context = await hydrateNodeGenerationContext(
            buildNodeGenerationContext(
              nodeId,
              nodesRef.current,
              connectionsRef.current,
              fullPrompt,
            ),
          );
          const refs = context.referenceImages;
          const image = refs.length
            ? await requestEdit(
                { ...generationConfig, count: "1" },
                context.prompt,
                refs,
                { signal: controller.signal },
              ).then((items) => items[0])
            : await requestGeneration(
                { ...generationConfig, count: "1" },
                context.prompt,
                { signal: controller.signal },
              ).then((items) => items[0]);
          const uploaded = await uploadImage(image.dataUrl, {
            signal: controller.signal,
          });
          setNodes((prev) =>
            prev.map((node) =>
              node.id === nodeId
                ? {
                    ...node,
                    metadata: {
                      ...node.metadata,
                      ...imageMetadata(uploaded),
                      prompt: scene,
                      model: generationConfig.model,
                      status: NODE_STATUS_SUCCESS,
                      errorDetails: undefined,
                    },
                  }
                : node,
            ),
          );
          setDialogNodeId(null);
        } catch (error) {
          if (!isGenerationCanceled(error)) {
            const errorDetails =
              error instanceof Error ? error.message : "生成失败";
            message.error(errorDetails);
            setNodes((prev) =>
              prev.map((node) =>
                node.id === nodeId
                  ? {
                      ...node,
                      metadata: {
                        ...node.metadata,
                        status: NODE_STATUS_ERROR,
                        errorDetails,
                      },
                    }
                  : node,
              ),
            );
          }
        } finally {
          finishGenerationRequest(nodeId, controller);
        }
        return;
      }

      setRunningNodeId(nodeId);
      const runController = startGenerationRequest(nodeId, nodeId, nodeId);
      const sourceTextContent =
        sourceNode?.type === CanvasNodeType.Text
          ? sourceNode.metadata?.content?.trim() || ""
          : "";
      const editingTextNode = mode === "text" && Boolean(sourceTextContent);
      const generationContext = await hydrateNodeGenerationContext(
        buildNodeGenerationContext(
          nodeId,
          nodesRef.current,
          connectionsRef.current,
          editingTextNode
            ? `请根据要求修改以下文本。\n\n原文：\n${sourceTextContent}\n\n修改要求：\n${prompt}`
            : prompt,
        ),
      );
      const effectivePrompt = generationContext.prompt.trim();
      if (runController.signal.aborted) {
        finishGenerationRequest(nodeId, runController);
        setRunningNodeId(null);
        return;
      }
      const markSourceStatus =
        sourceNode?.type !== CanvasNodeType.Image && !editingTextNode;
      if (!effectivePrompt && (mode === "text" || mode === "audio")) {
        finishGenerationRequest(nodeId, runController);
        setRunningNodeId(null);
        return;
      }
      let pendingChildIds: string[] = [];
      if (markSourceStatus)
        setNodes((prev) =>
          prev.map((node) =>
            node.id === nodeId
              ? {
                  ...node,
                  metadata: {
                    ...node.metadata,
                    ...(node.type === CanvasNodeType.Config ? {} : { prompt }),
                    status: NODE_STATUS_LOADING,
                    errorDetails: undefined,
                  },
                }
              : node,
          ),
        );

      try {
        if (mode === "image") {
          const count = getGenerationCount(generationConfig.count);
          const isConfigNode = sourceNode?.type === CanvasNodeType.Config;
          const isImageNode = sourceNode?.type === CanvasNodeType.Image;
          const isEmptyImageNode =
            isImageNode && !sourceNode?.metadata?.content;
          const sourceReference =
            isImageNode && sourceNode?.metadata?.content
              ? [
                  {
                    id: sourceNode.id,
                    name: `${sourceNode.title || sourceNode.id}.png`,
                    type: sourceNode.metadata.mimeType || "image/png",
                    dataUrl: sourceNode.metadata.content,
                    storageKey: sourceNode.metadata.storageKey,
                  },
                ]
              : [];
          const referenceImages = [
            ...new Map(
              [...sourceReference, ...generationContext.referenceImages].map(
                (image) => [image.id, image],
              ),
            ).values(),
          ];
          const generationType = referenceImages.length
            ? ("edit" as const)
            : ("generation" as const);
          const generationMetadata = buildImageGenerationMetadata(
            generationType,
            generationConfig,
            count,
            referenceImages,
          );
          const parentConfig =
            NODE_DEFAULT_SIZE[
              isConfigNode
                ? CanvasNodeType.Config
                : isImageNode
                  ? CanvasNodeType.Image
                  : CanvasNodeType.Text
            ];
          const imageConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
          const parentPosition = sourceNode?.position || { x: 0, y: 0 };
          const rootId = isEmptyImageNode ? nodeId : nanoid();
          const imageIds = Array.from({ length: count }, () => nanoid());
          pendingChildIds = [rootId];
          const rootNode: CanvasNodeData = {
            id: rootId,
            type: CanvasNodeType.Image,
            title: effectivePrompt.slice(0, 32) || "Generated Image",
            position: {
              x: isEmptyImageNode
                ? parentPosition.x
                : parentPosition.x + parentConfig.width + 96,
              y:
                parentPosition.y +
                parentConfig.height / 2 -
                imageConfig.height / 2,
            },
            width: isEmptyImageNode
              ? sourceNode?.width || imageConfig.width
              : imageConfig.width,
            height: isEmptyImageNode
              ? sourceNode?.height || imageConfig.height
              : imageConfig.height,
            metadata: {
              prompt: effectivePrompt,
              status: NODE_STATUS_LOADING,
              images: imageIds.map((id) => ({
                id,
                status: NODE_STATUS_LOADING,
                content: "",
                naturalWidth: 0,
                naturalHeight: 0,
                bytes: 0,
                mimeType: "",
              })),
              ...generationMetadata,
            },
          };

          setNodes((prev) => [
            ...prev.map((node) =>
              node.id === nodeId
                ? isConfigNode
                  ? {
                      ...node,
                      metadata: {
                        ...node.metadata,
                        status: NODE_STATUS_LOADING,
                        errorDetails: undefined,
                      },
                    }
                  : isEmptyImageNode
                    ? {
                        ...node,
                        position: rootNode.position,
                        width: rootNode.width,
                        height: rootNode.height,
                        title: rootNode.title,
                        metadata: {
                          ...node.metadata,
                          ...rootNode.metadata,
                          errorDetails: undefined,
                        },
                      }
                    : isImageNode
                      ? {
                          ...node,
                          metadata: {
                            ...node.metadata,
                            status: NODE_STATUS_SUCCESS,
                            errorDetails: undefined,
                          },
                        }
                      : {
                          ...node,
                          type: CanvasNodeType.Text,
                          title: prompt.slice(0, 32) || "Prompt",
                          width: parentConfig.width,
                          height: parentConfig.height,
                          metadata: {
                            ...node.metadata,
                            content: prompt,
                            prompt,
                            status: NODE_STATUS_SUCCESS,
                            fontSize: 14,
                            errorDetails: undefined,
                          },
                        }
                : node,
            ),
            ...(isEmptyImageNode ? [] : [rootNode]),
          ]);
          if (!isEmptyImageNode)
            setConnections((prev) => [
              ...prev,
              { id: nanoid(), fromNodeId: nodeId, toNodeId: rootId },
            ]);
          setSelectedNodeIds(new Set([nodeId]));
          setSelectedConnectionId(null);
          setDialogNodeId(nodeId);

          const controller =
            rootId === nodeId
              ? runController
              : startGenerationRequest(rootId, nodeId, nodeId, runController);
          let hasSuccess = false;
          let hasFailure = false;
          let firstError = "";
          await Promise.all(
            imageIds.map(async (imageId) => {
              try {
                const image = referenceImages.length
                  ? await requestEdit(
                      { ...generationConfig, count: "1" },
                      effectivePrompt,
                      referenceImages,
                      { signal: controller.signal },
                    ).then((items) => items[0])
                  : await requestGeneration(
                      { ...generationConfig, count: "1" },
                      effectivePrompt,
                      { signal: controller.signal },
                    ).then((items) => items[0]);
                const uploaded = await uploadImage(image.dataUrl, {
                  signal: controller.signal,
                });
                const imageSize = fitNodeSize(
                  uploaded.width,
                  uploaded.height,
                  imageConfig.width,
                  imageConfig.height,
                );
                const item: CanvasNodeImage = {
                  id: imageId,
                  status: NODE_STATUS_SUCCESS,
                  content: uploaded.url,
                  storageKey: uploaded.storageKey,
                  naturalWidth: uploaded.width,
                  naturalHeight: uploaded.height,
                  bytes: uploaded.bytes,
                  mimeType: uploaded.mimeType,
                };
                setNodes((prev) =>
                  prev.map((node) => {
                    if (node.id !== rootId) return node;
                    const images =
                      node.metadata?.images?.map((image) =>
                        image.id === imageId ? item : image,
                      ) || [];
                    if (node.metadata?.primaryImageId)
                      return {
                        ...node,
                        metadata: { ...node.metadata, images },
                      };
                    const center = {
                      x: node.position.x + node.width / 2,
                      y: node.position.y + node.height / 2,
                    };
                    return {
                      ...node,
                      position: {
                        x: center.x - imageSize.width / 2,
                        y: center.y - imageSize.height / 2,
                      },
                      ...imageSize,
                      metadata: {
                        ...node.metadata,
                        content: item.content,
                        storageKey: item.storageKey,
                        naturalWidth: item.naturalWidth,
                        naturalHeight: item.naturalHeight,
                        bytes: item.bytes,
                        mimeType: item.mimeType,
                        images,
                        primaryImageId: imageId,
                      },
                    };
                  }),
                );
                hasSuccess = true;
                if (isConfigNode)
                  setNodes((prev) =>
                    prev.map((node) =>
                      node.id === nodeId
                        ? {
                            ...node,
                            metadata: {
                              ...node.metadata,
                              status: NODE_STATUS_SUCCESS,
                              errorDetails: undefined,
                            },
                          }
                        : node,
                    ),
                  );
                return true;
              } catch (error) {
                if (isGenerationCanceled(error)) return false;
                const errorDetails =
                  error instanceof Error ? error.message : "生成失败";
                if (!firstError) firstError = errorDetails;
                hasFailure = true;
                setNodes((prev) =>
                  prev.map((node) =>
                    node.id === rootId
                      ? {
                          ...node,
                          metadata: {
                            ...node.metadata,
                            images: node.metadata?.images?.map((image) =>
                              image.id === imageId
                                ? {
                                    ...image,
                                    status: NODE_STATUS_ERROR,
                                    errorDetails,
                                  }
                                : image,
                            ),
                          },
                        }
                      : node,
                  ),
                );
              }
              return false;
            }),
          );
          if (rootId !== nodeId) finishGenerationRequest(rootId, controller);
          if (controller.signal.aborted) {
            setNodes((prev) =>
              prev.map((node) =>
                node.id === nodeId &&
                isConfigNode &&
                node.metadata?.status === NODE_STATUS_LOADING
                  ? {
                      ...node,
                      metadata: {
                        ...node.metadata,
                        status: NODE_STATUS_IDLE,
                        errorDetails: undefined,
                      },
                    }
                  : node,
              ),
            );
            return;
          }
          if (hasFailure) {
            message.error(
              hasSuccess ? "部分图片生成失败" : firstError || "生成失败",
            );
          }
          setNodes((prev) =>
            prev.map((node) =>
              node.id === nodeId && isConfigNode
                ? {
                    ...node,
                    metadata: {
                      ...node.metadata,
                      status: hasSuccess
                        ? NODE_STATUS_SUCCESS
                        : NODE_STATUS_ERROR,
                      errorDetails: hasSuccess ? undefined : "生成失败",
                    },
                  }
                : node.id === rootId
                  ? {
                      ...node,
                      metadata: {
                        ...node.metadata,
                        status: hasSuccess
                          ? NODE_STATUS_SUCCESS
                          : NODE_STATUS_ERROR,
                        errorDetails: hasSuccess
                          ? undefined
                          : "全部图片生成失败",
                      },
                    }
                  : node,
            ),
          );
          return;
        }

        if (mode === "video") {
          const spec =
            nodeSizeFromRatio(
              generationConfig.size,
              NODE_DEFAULT_SIZE[CanvasNodeType.Video].width,
              NODE_DEFAULT_SIZE[CanvasNodeType.Video].height,
            ) || NODE_DEFAULT_SIZE[CanvasNodeType.Video];
          const isEmptyVideoNode =
            sourceNode?.type === CanvasNodeType.Video &&
            !sourceNode.metadata?.content;
          const videoId = isEmptyVideoNode ? nodeId : nanoid();
          const parent = sourceNode?.position || { x: 0, y: 0 };
          const videoNode: CanvasNodeData = {
            id: videoId,
            type: CanvasNodeType.Video,
            title: effectivePrompt.slice(0, 32) || "Generated Video",
            position: isEmptyVideoNode
              ? sourceNode.position
              : {
                  x: parent.x + (sourceNode?.width || spec.width) + 96,
                  y: parent.y,
                },
            width: isEmptyVideoNode ? sourceNode.width : spec.width,
            height: isEmptyVideoNode ? sourceNode.height : spec.height,
            metadata: {
              prompt: effectivePrompt,
              status: NODE_STATUS_LOADING,
              model: generationConfig.model,
              size: generationConfig.size,
              seconds: generationConfig.videoSeconds,
              vquality: generationConfig.vquality,
              generateAudio: generationConfig.videoGenerateAudio,
              watermark: generationConfig.videoWatermark,
              videoMode: generationConfig.videoMode,
              references: generationReferenceUrls(generationContext),
            },
          };
          pendingChildIds = [videoId];
          setNodes((prev) =>
            isEmptyVideoNode
              ? prev.map((node) =>
                  node.id === nodeId ? { ...node, ...videoNode } : node,
                )
              : [
                  ...prev.map((node) =>
                    node.id === nodeId
                      ? {
                          ...node,
                          metadata: {
                            ...node.metadata,
                            status: NODE_STATUS_SUCCESS,
                          },
                        }
                      : node,
                  ),
                  videoNode,
                ],
          );
          if (!isEmptyVideoNode)
            setConnections((prev) => [
              ...prev,
              { id: nanoid(), fromNodeId: nodeId, toNodeId: videoId },
            ]);
          const controller = startGenerationRequest(
            videoId,
            nodeId,
            nodeId,
            runController,
          );
          try {
            await completeVideoNodeTask(
              videoId,
              generationConfig,
              effectivePrompt,
              generationContext.referenceImages,
              controller.signal,
              {
                size: generationConfig.size,
                seconds: generationConfig.videoSeconds,
                vquality: generationConfig.vquality,
                generateAudio: generationConfig.videoGenerateAudio,
                watermark: generationConfig.videoWatermark,
                videoMode: generationConfig.videoMode,
                references: generationReferenceUrls(generationContext),
              },
              generationContext.referenceVideos,
              generationContext.referenceAudios,
            );
          } finally {
            finishGenerationRequest(videoId, controller);
          }
          return;
        }

        if (mode === "audio") {
          const spec = NODE_DEFAULT_SIZE[CanvasNodeType.Audio];
          const isEmptyAudioNode =
            sourceNode?.type === CanvasNodeType.Audio &&
            !sourceNode.metadata?.content;
          const audioId = isEmptyAudioNode ? nodeId : nanoid();
          const parent = sourceNode?.position || { x: 0, y: 0 };
          const audioNode: CanvasNodeData = {
            id: audioId,
            type: CanvasNodeType.Audio,
            title: effectivePrompt.slice(0, 32) || "Generated Audio",
            position: isEmptyAudioNode
              ? sourceNode.position
              : {
                  x: parent.x + (sourceNode?.width || spec.width) + 96,
                  y:
                    parent.y +
                    ((sourceNode?.height || spec.height) - spec.height) / 2,
                },
            width: isEmptyAudioNode ? sourceNode.width : spec.width,
            height: isEmptyAudioNode ? sourceNode.height : spec.height,
            metadata: {
              prompt: effectivePrompt,
              status: NODE_STATUS_LOADING,
              ...buildAudioGenerationMetadata(generationConfig),
            },
          };
          pendingChildIds = [audioId];
          setNodes((prev) =>
            isEmptyAudioNode
              ? prev.map((node) =>
                  node.id === nodeId ? { ...node, ...audioNode } : node,
                )
              : [
                  ...prev.map((node) =>
                    node.id === nodeId
                      ? {
                          ...node,
                          metadata: {
                            ...node.metadata,
                            status: NODE_STATUS_SUCCESS,
                          },
                        }
                      : node,
                  ),
                  audioNode,
                ],
          );
          if (!isEmptyAudioNode)
            setConnections((prev) => [
              ...prev,
              { id: nanoid(), fromNodeId: nodeId, toNodeId: audioId },
            ]);
          const controller = startGenerationRequest(
            audioId,
            nodeId,
            nodeId,
            runController,
          );
          try {
            const audio = await storeGeneratedAudio(
              await requestAudioGeneration(generationConfig, effectivePrompt, {
                signal: controller.signal,
              }),
              generationConfig.audioFormat,
            );
            setNodes((prev) =>
              prev.map((node) =>
                node.id === audioId
                  ? {
                      ...node,
                      metadata: {
                        ...node.metadata,
                        ...audioMetadata(audio),
                        prompt: effectivePrompt,
                        ...buildAudioGenerationMetadata(generationConfig),
                      },
                    }
                  : node,
              ),
            );
          } finally {
            finishGenerationRequest(audioId, controller);
          }
          return;
        }

        const isConfigNode = sourceNode?.type === CanvasNodeType.Config;
        const textCount = getGenerationCount(
          String(sourceNode?.metadata?.textCount || 1),
        );
        const parentConfig =
          NODE_DEFAULT_SIZE[
            isConfigNode ? CanvasNodeType.Config : CanvasNodeType.Text
          ];
        const textConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Text];
        const parentPosition = sourceNode?.position || { x: 0, y: 0 };
        const isEmptyTextNode =
          sourceNode?.type === CanvasNodeType.Text && !sourceTextContent;
        const rootId = isEmptyTextNode ? nodeId : nanoid();
        const textIds = Array.from({ length: textCount }, () => nanoid());
        const rootNode: CanvasNodeData = {
          id: rootId,
          type: CanvasNodeType.Text,
          title: effectivePrompt.slice(0, 32) || "Generated Text",
          position: isEmptyTextNode
            ? sourceNode.position
            : {
                x: parentPosition.x + parentConfig.width + 96,
                y:
                  parentPosition.y +
                  parentConfig.height / 2 -
                  textConfig.height / 2,
              },
          width: isEmptyTextNode ? sourceNode.width : textConfig.width,
          height: isEmptyTextNode ? sourceNode.height : textConfig.height,
          metadata: {
            prompt: effectivePrompt,
            status: NODE_STATUS_LOADING,
            fontSize: 14,
            model: generationConfig.model,
            reasoningEffort: generationConfig.reasoningEffort,
            textCount,
            texts: textIds.map((id) => ({
              id,
              status: NODE_STATUS_LOADING,
              content: "",
            })),
            primaryTextId: textIds[0],
          },
        };
        pendingChildIds = [rootId];
        setNodes((prev) =>
          isEmptyTextNode
            ? prev.map((node) =>
                node.id === nodeId ? { ...node, ...rootNode } : node,
              )
            : [
                ...prev.map((node) =>
                  node.id === nodeId && isConfigNode
                    ? {
                        ...node,
                        metadata: {
                          ...node.metadata,
                          status: NODE_STATUS_LOADING,
                          errorDetails: undefined,
                        },
                      }
                    : node,
                ),
                rootNode,
              ],
        );
        if (!isEmptyTextNode)
          setConnections((prev) => [
            ...prev,
            { id: nanoid(), fromNodeId: nodeId, toNodeId: rootId },
          ]);
        setSelectedNodeIds(new Set([nodeId]));
        setSelectedConnectionId(null);
        setDialogNodeId(nodeId);

        const controller =
          rootId === nodeId
            ? runController
            : startGenerationRequest(rootId, nodeId, nodeId, runController);
        const results = await Promise.all(
          textIds.map(async (textId): Promise<CanvasNodeText | null> => {
            let streamed = "";
            try {
              const answer = await requestImageQuestion(
                generationConfig,
                buildNodeResponseMessages({
                  ...generationContext,
                  prompt: effectivePrompt,
                }),
                (text) => {
                  streamed = text;
                  setNodes((prev) =>
                    prev.map((node) =>
                      node.id === rootId
                        ? {
                            ...node,
                            metadata: {
                              ...node.metadata,
                              ...(node.metadata?.primaryTextId === textId
                                ? { content: text }
                                : {}),
                              texts: node.metadata?.texts?.map((item) =>
                                item.id === textId
                                  ? { ...item, content: text }
                                  : item,
                              ),
                            },
                          }
                        : node,
                    ),
                  );
                },
                { signal: controller.signal },
              );
              const content = answer || streamed;
              setNodes((prev) =>
                prev.map((node) =>
                  node.id === rootId
                    ? {
                        ...node,
                        metadata: {
                          ...node.metadata,
                          ...(node.metadata?.primaryTextId === textId
                            ? { content }
                            : {}),
                          texts: node.metadata?.texts?.map((item) =>
                            item.id === textId
                              ? {
                                  ...item,
                                  content,
                                  status: NODE_STATUS_SUCCESS,
                                }
                              : item,
                          ),
                        },
                      }
                    : node,
                ),
              );
              return {
                id: textId,
                status: NODE_STATUS_SUCCESS,
                content,
              } satisfies CanvasNodeText;
            } catch (error) {
              if (isGenerationCanceled(error)) return null;
              const errorDetails =
                error instanceof Error ? error.message : "生成失败";
              setNodes((prev) =>
                prev.map((node) =>
                  node.id === rootId
                    ? {
                        ...node,
                        metadata: {
                          ...node.metadata,
                          texts: node.metadata?.texts?.map((item) =>
                            item.id === textId
                              ? {
                                  ...item,
                                  status: NODE_STATUS_ERROR,
                                  errorDetails,
                                }
                              : item,
                          ),
                        },
                      }
                    : node,
                ),
              );
              return {
                id: textId,
                status: NODE_STATUS_ERROR,
                content: "",
                errorDetails,
              } satisfies CanvasNodeText;
            }
          }),
        );
        if (rootId !== nodeId) finishGenerationRequest(rootId, controller);
        if (controller.signal.aborted) return;
        const completedTexts = results.flatMap((item) =>
          item?.status === NODE_STATUS_SUCCESS ? [item] : [],
        );
        const failedTexts = results.filter(
          (item) => item?.status === NODE_STATUS_ERROR,
        );
        const firstText = completedTexts[0];
        if (completedTexts.length <= 1)
          setExpandedBatchNodeIds(
            (current) => new Set([...current].filter((id) => id !== rootId)),
          );
        if (failedTexts.length)
          message.error(
            firstText
              ? "部分文本生成失败"
              : failedTexts[0]?.errorDetails || "生成失败",
          );
        setNodes((prev) =>
          prev.map((node) => {
            if (node.id === rootId) {
              const primaryText =
                completedTexts.find(
                  (text) => text.id === node.metadata?.primaryTextId,
                ) || firstText;
              return {
                ...node,
                metadata: {
                  ...node.metadata,
                  content: primaryText?.content || "",
                  texts: completedTexts,
                  primaryTextId: primaryText?.id,
                  status: primaryText ? NODE_STATUS_SUCCESS : NODE_STATUS_ERROR,
                  errorDetails: primaryText ? undefined : "生成失败",
                },
              };
            }
            return node.id === nodeId && isConfigNode
              ? {
                  ...node,
                  metadata: {
                    ...node.metadata,
                    status: firstText ? NODE_STATUS_SUCCESS : NODE_STATUS_ERROR,
                    errorDetails: firstText ? undefined : "生成失败",
                  },
                }
              : node;
          }),
        );
      } catch (error) {
        if (isGenerationCanceled(error)) return;
        const errorDetails =
          error instanceof Error ? error.message : "生成失败";
        message.error(errorDetails);
        setNodes((prev) =>
          prev.map((node) =>
            node.id === nodeId || pendingChildIds.includes(node.id)
              ? node.id === nodeId && !markSourceStatus
                ? node
                : {
                    ...node,
                    metadata: {
                      ...node.metadata,
                      status: NODE_STATUS_ERROR,
                      errorDetails,
                      ...(isVideoTaskFailed(error) &&
                      node.type === CanvasNodeType.Video
                        ? { videoTaskId: undefined }
                        : {}),
                    },
                  }
              : node,
          ),
        );
      } finally {
        finishGenerationRequest(nodeId, runController);
        setRunningNodeId(null);
      }
    },
    [
      effectiveConfig,
      isAiConfigReady,
      startGenerationRequest,
      setNodes,
      generateDocumentNode,
      message,
      finishGenerationRequest,
      setConnections,
      completeVideoNodeTask,
    ],
  );
  useEffect(() => {
    generateNodeRef.current = handleGenerateNode;
  }, [handleGenerateNode]);

  const handleRetryNode = useCallback(
    async (node: CanvasNodeData, imageId?: string) => {
      if (integrationRef.current) {
        const generation = integrationRef.current.generation;
        const run = generation?.runs.find((value) => value.nodeId === node.id);
        if (run && (run.pending || run.state === "ARCHIVE_FAILED")) {
          await generation?.refresh(run, run.state === "UNCONFIRMED");
        } else {
          await generateDocumentNode(node.id);
        }
        return;
      }
      if (hasResumableVideoTask(node)) {
        await pollVideoNodeTask(node);
        return;
      }
      const sourceNode =
        findRetrySourceNode(
          node.id,
          nodesRef.current,
          connectionsRef.current,
        ) || node;
      const savedImageMetadata =
        node.type === CanvasNodeType.Image ? node.metadata : undefined;
      const hasSavedImageMetadata = Boolean(savedImageMetadata?.generationType);
      const generationConfig =
        hasSavedImageMetadata && savedImageMetadata
          ? {
              ...effectiveConfig,
              model:
                savedImageMetadata.model ||
                effectiveConfig.imageModel ||
                effectiveConfig.model,
              quality: savedImageMetadata.quality || effectiveConfig.quality,
              size: savedImageMetadata.size || effectiveConfig.size,
              background:
                savedImageMetadata.background ?? effectiveConfig.background,
              count: "1",
            }
          : {
              ...buildGenerationConfig(
                effectiveConfig,
                sourceNode,
                node.type === CanvasNodeType.Text
                  ? "text"
                  : node.type === CanvasNodeType.Video
                    ? "video"
                    : node.type === CanvasNodeType.Audio
                      ? "audio"
                      : "image",
              ),
              count: "1",
            };
      if (!isAiConfigReady(generationConfig, generationConfig.model)) {
        message.warning("生成服务尚未接入");
        return;
      }

      const context = hasSavedImageMetadata
        ? null
        : await hydrateNodeGenerationContext(
            buildNodeGenerationContext(
              sourceNode.id,
              nodesRef.current,
              connectionsRef.current,
              sourceNode.metadata?.prompt || node.metadata?.prompt || "",
            ),
          );
      const prompt = (
        savedImageMetadata?.prompt ||
        context?.prompt ||
        ""
      ).trim();
      if (!prompt) {
        message.warning("找不到提示词，无法重试");
        return;
      }
      const generationType = savedImageMetadata?.generationType;
      const useReferenceImages = generationType
        ? generationType === "edit"
        : Boolean(context?.referenceImages.length);
      const retryReferenceImages =
        hasSavedImageMetadata && savedImageMetadata
          ? await resolveMetadataReferences(savedImageMetadata)
          : useReferenceImages
            ? context?.referenceImages.length
              ? context.referenceImages
              : sourceNodeReferenceImages(sourceNode)
            : [];
      if (useReferenceImages && !retryReferenceImages) {
        message.error("参考图片已丢失，无法继续重试");
        setNodes((prev) =>
          prev.map((item) =>
            item.id === node.id
              ? {
                  ...item,
                  metadata: {
                    ...item.metadata,
                    status: item.metadata?.content
                      ? NODE_STATUS_SUCCESS
                      : NODE_STATUS_ERROR,
                    errorDetails: item.metadata?.content
                      ? undefined
                      : "参考图片已丢失，无法继续重试",
                    images: item.metadata?.images?.map((image) =>
                      image.id === imageId
                        ? {
                            ...image,
                            status: NODE_STATUS_ERROR,
                            errorDetails: "参考图片已丢失，无法继续重试",
                          }
                        : image,
                    ),
                  },
                }
              : item,
          ),
        );
        return;
      }
      const retryImages = retryReferenceImages || [];

      setRunningNodeId(node.id);
      setNodes((prev) =>
        prev.map((item) =>
          item.id === node.id
            ? {
                ...item,
                metadata: {
                  ...item.metadata,
                  status: NODE_STATUS_LOADING,
                  errorDetails: undefined,
                  images: item.metadata?.images?.map((image) =>
                    image.id === imageId
                      ? {
                          ...image,
                          status: NODE_STATUS_LOADING,
                          errorDetails: undefined,
                        }
                      : image,
                  ),
                },
              }
            : item,
        ),
      );
      const controller = startGenerationRequest(
        node.id,
        sourceNode.id,
        node.id,
      );

      try {
        if (node.type === CanvasNodeType.Text) {
          if (!context) return;
          let streamed = "";
          const answer = await requestImageQuestion(
            generationConfig,
            buildNodeResponseMessages({ ...context, prompt }),
            (text) => {
              streamed = text;
              setNodes((prev) =>
                prev.map((item) =>
                  item.id === node.id
                    ? {
                        ...item,
                        type: CanvasNodeType.Text,
                        metadata: {
                          ...item.metadata,
                          content: text,
                          status: NODE_STATUS_LOADING,
                        },
                      }
                    : item,
                ),
              );
            },
            { signal: controller.signal },
          );
          setNodes((prev) =>
            prev.map((item) =>
              item.id === node.id
                ? {
                    ...item,
                    type: CanvasNodeType.Text,
                    metadata: {
                      ...item.metadata,
                      content: answer || streamed,
                      prompt,
                      status: NODE_STATUS_SUCCESS,
                    },
                  }
                : item,
            ),
          );
          return;
        }
        if (node.type === CanvasNodeType.Video) {
          await completeVideoNodeTask(
            node.id,
            generationConfig,
            prompt,
            retryImages,
            controller.signal,
            {
              size: generationConfig.size,
              seconds: generationConfig.videoSeconds,
              vquality: generationConfig.vquality,
              generateAudio: generationConfig.videoGenerateAudio,
              watermark: generationConfig.videoWatermark,
              videoMode: generationConfig.videoMode,
            },
            context?.referenceVideos || [],
            context?.referenceAudios || [],
          );
          return;
        }
        if (node.type === CanvasNodeType.Audio) {
          const audio = await storeGeneratedAudio(
            await requestAudioGeneration(generationConfig, prompt, {
              signal: controller.signal,
            }),
            generationConfig.audioFormat,
          );
          setNodes((prev) =>
            prev.map((item) =>
              item.id === node.id
                ? {
                    ...item,
                    metadata: {
                      ...item.metadata,
                      ...audioMetadata(audio),
                      prompt,
                      ...buildAudioGenerationMetadata(generationConfig),
                    },
                  }
                : item,
            ),
          );
          return;
        }

        const image = useReferenceImages
          ? await requestEdit(generationConfig, prompt, retryImages, {
              signal: controller.signal,
            }).then((items) => items[0])
          : await requestGeneration(generationConfig, prompt, {
              signal: controller.signal,
            }).then((items) => items[0]);
        const uploadedImage = await uploadImage(image.dataUrl, {
          signal: controller.signal,
        });
        const imageConfig = NODE_DEFAULT_SIZE[CanvasNodeType.Image];
        const retryImage: CanvasNodeImage = {
          id: imageId || node.metadata?.primaryImageId || nanoid(),
          status: NODE_STATUS_SUCCESS,
          content: uploadedImage.url,
          storageKey: uploadedImage.storageKey,
          naturalWidth: uploadedImage.width,
          naturalHeight: uploadedImage.height,
          bytes: uploadedImage.bytes,
          mimeType: uploadedImage.mimeType,
        };
        const generationMetadata = savedImageMetadata?.generationType
          ? {
              generationType: savedImageMetadata.generationType,
              model: generationConfig.model,
              size: generationConfig.size,
              quality: generationConfig.quality,
              ...(generationConfig.background
                ? { background: generationConfig.background }
                : {}),
              count: savedImageMetadata.count || 1,
              references: savedImageMetadata.references,
            }
          : buildImageGenerationMetadata(
              useReferenceImages ? "edit" : "generation",
              generationConfig,
              1,
              retryImages,
            );
        setNodes((prev) =>
          prev.map((item) => {
            if (item.id !== node.id) return item;
            const makePrimary = !imageId || !item.metadata?.content;
            const edge = imageId ? Math.max(item.width, item.height) : 0;
            const imageSize =
              imageId && item.metadata?.freeResize
                ? { width: item.width, height: item.height }
                : imageId
                  ? fitNodeSize(
                      uploadedImage.width,
                      uploadedImage.height,
                      edge,
                      edge,
                    )
                  : fitNodeSize(
                      uploadedImage.width,
                      uploadedImage.height,
                      imageConfig.width,
                      imageConfig.height,
                    );
            return {
              ...item,
              type: CanvasNodeType.Image,
              ...(makePrimary
                ? {
                    width: imageSize.width,
                    height: imageSize.height,
                    ...(imageId
                      ? {
                          position: {
                            x:
                              item.position.x +
                              item.width / 2 -
                              imageSize.width / 2,
                            y:
                              item.position.y +
                              item.height / 2 -
                              imageSize.height / 2,
                          },
                        }
                      : {}),
                  }
                : {}),
              metadata: {
                ...item.metadata,
                ...(makePrimary
                  ? imageMetadata(uploadedImage)
                  : { status: NODE_STATUS_SUCCESS }),
                images: item.metadata?.images?.map((current) =>
                  current.id === retryImage.id ? retryImage : current,
                ),
                primaryImageId: makePrimary
                  ? retryImage.id
                  : item.metadata?.primaryImageId,
                prompt,
                ...generationMetadata,
              },
            };
          }),
        );
      } catch (error) {
        if (isGenerationCanceled(error)) return;
        const errorDetails =
          error instanceof Error ? error.message : "生成失败";
        message.error(errorDetails);
        setNodes((prev) =>
          prev.map((item) =>
            item.id === node.id
              ? {
                  ...item,
                  metadata: {
                    ...item.metadata,
                    status: item.metadata?.content
                      ? NODE_STATUS_SUCCESS
                      : NODE_STATUS_ERROR,
                    errorDetails: item.metadata?.content
                      ? undefined
                      : errorDetails,
                    images: item.metadata?.images?.map((image) =>
                      image.id === imageId
                        ? { ...image, status: NODE_STATUS_ERROR, errorDetails }
                        : image,
                    ),
                    ...(isVideoTaskFailed(error) &&
                    item.type === CanvasNodeType.Video
                      ? { videoTaskId: undefined }
                      : {}),
                  },
                }
              : item,
          ),
        );
      } finally {
        finishGenerationRequest(node.id, controller);
        setRunningNodeId(null);
      }
    },
    [
      effectiveConfig,
      isAiConfigReady,
      setNodes,
      startGenerationRequest,
      generateDocumentNode,
      pollVideoNodeTask,
      message,
      completeVideoNodeTask,
      finishGenerationRequest,
    ],
  );

  const deleteBatchImage = useCallback(
    (nodeId: string, imageId: string) => {
      const node = nodesRef.current.find((item) => item.id === nodeId);
      if ((node?.metadata?.images?.length || 0) <= 2)
        setExpandedBatchNodeIds(
          (current) => new Set([...current].filter((id) => id !== nodeId)),
        );
      setNodes((prev) =>
        prev.map((item) => {
          if (item.id !== nodeId) return item;
          const images =
            item.metadata?.images?.filter((image) => image.id !== imageId) ||
            [];
          return {
            ...item,
            metadata: {
              ...item.metadata,
              images,
              count: images.length,
              primaryImageId:
                item.metadata?.primaryImageId === imageId
                  ? images[0]?.id
                  : item.metadata?.primaryImageId,
            },
          };
        }),
      );
    },
    [setNodes],
  );

  const retryBatchImage = useCallback(
    (node: CanvasNodeData, imageId: string) =>
      void handleRetryNode(node, imageId),
    [handleRetryNode],
  );

  const generateImageFromTextNode = useCallback(
    (node: CanvasNodeData) => {
      if (integrationRef.current) {
        const image = createCanvasNode(CanvasNodeType.Image, {
          x:
            node.position.x +
            node.width +
            96 +
            getNodeSpec(CanvasNodeType.Image).width / 2,
          y: node.position.y + node.height / 2,
        });
        try {
          const next = connectCanvasDocument(
            {
              nodes: [...nodesRef.current, image],
              connections: connectionsRef.current,
              backgroundMode,
              showImageInfo,
              viewport: savedViewport,
            },
            node.id,
            image.id,
          );
          nodesRef.current = next.nodes;
          connectionsRef.current = next.connections;
          setNodes(next.nodes);
          setConnections(next.connections);
          setSelectedNodeIds(new Set([image.id]));
          setSelectedConnectionId(null);
          setDialogNodeId(image.id);
        } catch (error) {
          message.error(
            error instanceof Error ? error.message : "创建图片节点失败",
          );
        }
        return;
      }
      const prompt = (
        node.metadata?.content ||
        node.metadata?.prompt ||
        ""
      ).trim();
      if (!prompt) {
        message.warning("文本节点为空，无法生图");
        return;
      }
      const sourceNode = nodesRef.current.find((item) => item.id === node.id);
      if (!sourceNode) return;
      const nodeSize = getNodeSpec(CanvasNodeType.Config);
      const configNode = createCanvasNode(
        CanvasNodeType.Config,
        {
          x: sourceNode.position.x + sourceNode.width + 96 + nodeSize.width / 2,
          y: sourceNode.position.y + sourceNode.height / 2,
        },
        {
          prompt: "",
          model: effectiveConfig.imageModel || effectiveConfig.model,
          size: effectiveConfig.size,
          count: getGenerationCount(
            effectiveConfig.canvasImageCount || effectiveConfig.count,
          ),
        },
      );
      const connection = {
        id: nanoid(),
        fromNodeId: sourceNode.id,
        toNodeId: configNode.id,
      };
      const nextNodes = nodesRef.current
        .map((item) =>
          item.id === sourceNode.id
            ? {
                ...item,
                metadata: {
                  ...item.metadata,
                  content: prompt,
                  prompt,
                  status: NODE_STATUS_SUCCESS,
                },
              }
            : item,
        )
        .concat(configNode);
      const nextConnections = [...connectionsRef.current, connection];
      nodesRef.current = nextNodes;
      connectionsRef.current = nextConnections;
      setNodes(nextNodes);
      setConnections(nextConnections);
      setSelectedNodeIds(new Set([configNode.id]));
      setSelectedConnectionId(null);
      setDialogNodeId(configNode.id);
    },
    [
      effectiveConfig.imageModel,
      effectiveConfig.model,
      effectiveConfig.size,
      effectiveConfig.canvasImageCount,
      effectiveConfig.count,
      setNodes,
      setConnections,
      backgroundMode,
      showImageInfo,
      savedViewport,
      message,
    ],
  );

  const updateMediaNode = useCallback(
    (updated: CanvasNodeData) => {
      const next = updateCanvasDocumentNode(
        {
          nodes: nodesRef.current,
          connections: connectionsRef.current,
          backgroundMode,
          showImageInfo,
          viewport: savedViewport,
        },
        updated,
      );
      nodesRef.current = next.nodes;
      connectionsRef.current = next.connections;
      setNodes(next.nodes);
      setConnections(next.connections);
    },
    [backgroundMode, showImageInfo, savedViewport, setNodes, setConnections],
  );

  const handleRemoveAsset = useCallback(
    (node: CanvasNodeData) => {
      if (integrationRef.current?.frozen) return;
      updateMediaNode(removeCanvasNodeAsset(node));
      setDialogNodeId(node.id);
    },
    [updateMediaNode],
  );

  const handleAssetInsert = useCallback(
    (asset: InsertAssetPayload, keepPickerOpen = false) => {
      if (integrationRef.current?.frozen) return;
      if (assetTarget) {
        const target = nodesRef.current.find(
          (node) => node.id === assetTarget.id,
        );
        if (!target) {
          message.warning("目标节点已不存在，请关闭素材选择后重试");
          return;
        }
        try {
          const replacement = replaceCanvasNodeAsset(target, asset);
          updateMediaNode(replacement);
          setSelectedNodeIds(new Set([target.id]));
          setAssetPickerOpen(false);
          setAssetTarget(undefined);
        } catch (error) {
          notifyError(error, "素材未替换");
        }
        return;
      }
      if (asset.kind !== "text" && (!asset.fileId || !asset.fileVersion)) {
        message.warning("请选择已上传的固定版本素材");
        return;
      }
      const view = viewportRef.current;
      const node = createCanvasNode(asset.kind as CanvasNodeType, {
        x: (size.width / 2 - view.x) / view.k,
        y: (size.height / 2 - view.y) / view.k,
      });
      const documentData = toCanvasGraph({
        nodes: [node],
        connections: [],
        backgroundMode: "dots",
        showImageInfo: false,
        viewport: view,
      }).nodes[0];
      const inserted =
        asset.kind === "text"
          ? {
              ...node,
              title: asset.title,
              documentData,
              metadata: { ...node.metadata, content: asset.content },
            }
          : replaceCanvasNodeAsset(
              { ...node, title: asset.title, documentData },
              asset,
            );
      setNodes((previous) => [...previous, inserted]);
      setSelectedNodeIds(new Set([node.id]));
      if (!keepPickerOpen) setAssetPickerOpen(false);
    },
    [assetTarget, message, setNodes, size.height, size.width, updateMediaNode],
  );

  // Memoize every callback and render function passed to CanvasNode.
  // CanvasNode uses React.memo, but new prop references would invalidate it on every render and rerender every node
  // during click, hover, or viewport changes, which is especially expensive for Markdown. These useCallback values
  // and their memoized map/handler dependencies remain stable during interaction, so unchanged nodes do not rerender.
  const handleNodeHoverStart = useCallback((nodeId: string) => {
    if (nodeDraggingRef.current) return;
    setHoveredNodeId(nodeId);
  }, []);
  const handleNodeHoverEnd = useCallback((nodeId: string) => {
    setHoveredNodeId((current) => (current === nodeId ? null : current));
  }, []);
  const handleNodeViewImage = useCallback(
    (node: CanvasNodeData, imageId?: string) => {
      setPreviewNodeId(node.id);
      setPreviewImageId(imageId || null);
    },
    [],
  );
  const handleNodeRetry = useCallback(
    (node: CanvasNodeData) => {
      if (
        node.type === CanvasNodeType.Text &&
        (node.metadata?.textCount || 1) > 1
      ) {
        void generateNodeRef.current?.(
          node.id,
          "text",
          node.metadata?.prompt || "",
        );
        return;
      }
      void handleRetryNode(node);
    },
    [handleRetryNode],
  );
  const handleNodeContextMenu = useCallback(
    (event: ReactMouseEvent, nodeId: string) => {
      event.preventDefault();
      event.stopPropagation();
      setSelectedNodeIds((current) =>
        current.has(nodeId) ? current : new Set([nodeId]),
      );
      setSelectedConnectionId(null);
      setContextMenu({
        type: "node",
        x: event.clientX,
        y: event.clientY,
        nodeId,
      });
    },
    [],
  );

  const renderNodePanel = useCallback(
    (panelNode: CanvasNodeData) =>
      integrationRef.current ? (
        <CanvasDocumentNodePanel
          collaboration={integrationRef.current.textBinding?.(panelNode.id)}
          projectId={documentIntegration?.projectId}
          key={panelNode.id}
          node={panelNode}
          pendingForm={documentIntegration?.pendingForms?.[panelNode.id]}
          onPendingForm={(form) =>
            integrationRef.current?.onPendingForm?.(panelNode.id, form)
          }
          generation={integrationRef.current.generation}
          workflow={integrationRef.current.workflow}
          onGenerate={() => {
            integrationRef.current?.onChange(
              {
                nodes,
                connections,
                backgroundMode,
                showImageInfo,
                viewport: savedViewport,
              },
              true,
            );
            if (
              ["PROCESS", "AGENT"].includes(
                panelNode.documentData?.parameters?.mode ?? "",
              )
            ) {
              void integrationRef.current?.workflow?.choose([panelNode.id]);
            } else {
              void generateDocumentNode(panelNode.id);
            }
          }}
          nodes={nodes}
          connectedNodes={connectedNodesByNodeId.get(panelNode.id) || []}
          mentionReferences={
            mentionReferencesByNodeId.get(panelNode.id) || EMPTY_REFERENCES
          }
          onDisconnectReference={disconnectNodeReference}
          onStartReferenceSelection={startNodeReferenceSelection}
          onChange={(updated) => {
            const next = updateCanvasDocumentNode(
              {
                nodes: nodesRef.current,
                connections: connectionsRef.current,
                backgroundMode,
                showImageInfo,
                viewport: savedViewport,
              },
              updated,
            );
            nodesRef.current = next.nodes;
            connectionsRef.current = next.connections;
            setNodes(next.nodes);
            setConnections(next.connections);
          }}
          onClose={() => setDialogNodeId(null)}
        />
      ) : getNodeDefinition(panelNode.type)?.Panel ? (
        renderPluginPanel(panelNode)
      ) : panelNode.type === CanvasNodeType.Config ? (
        <CanvasConfigComposer
          nodeId={panelNode.id}
          nodes={nodes}
          value={
            panelNode.metadata?.composerContent ??
            panelNode.metadata?.prompt ??
            ""
          }
          inputs={configInputsById.get(panelNode.id) || []}
          connectedNodes={connectedNodesByNodeId.get(panelNode.id) || []}
          onChange={(composerContent) =>
            handleConfigNodeChange(panelNode.id, { composerContent })
          }
          onClose={() => setDialogNodeId(null)}
          onDisconnectReference={disconnectNodeReference}
          onStartReferenceSelection={startNodeReferenceSelection}
        />
      ) : (
        <CanvasNodePromptPanel
          node={panelNode}
          nodes={nodes}
          isRunning={runningNodeId === panelNode.id}
          mentionReferences={
            mentionReferencesByNodeId.get(panelNode.id) || EMPTY_REFERENCES
          }
          connectedNodes={connectedNodesByNodeId.get(panelNode.id) || []}
          onPromptChange={handleNodePromptChange}
          onConfigChange={handleConfigNodeChange}
          onGenerate={handleGenerateNode}
          onStop={confirmStopGeneration}
          onClose={() => setDialogNodeId(null)}
          onDisconnectReference={disconnectNodeReference}
          onStartReferenceSelection={startNodeReferenceSelection}
          modeOverride={
            getNodeDefinition(panelNode.type)?.useBuiltinPanel?.mode
          }
          onImageSettingsOpenChange={(open) => {
            setNodeImageSettingsOpen(open);
            if (open) setToolbarNodeId(null);
          }}
        />
      ),
    [
      documentIntegration?.projectId,
      documentIntegration?.pendingForms,
      nodes,
      connectedNodesByNodeId,
      mentionReferencesByNodeId,
      disconnectNodeReference,
      startNodeReferenceSelection,
      renderPluginPanel,
      configInputsById,
      runningNodeId,
      handleNodePromptChange,
      handleConfigNodeChange,
      handleGenerateNode,
      confirmStopGeneration,
      connections,
      backgroundMode,
      showImageInfo,
      savedViewport,
      generateDocumentNode,
      setNodes,
      setConnections,
    ],
  );

  const renderNodeContentPanel = useCallback(
    (contentNode: CanvasNodeData) => {
      if (contentNode.type !== CanvasNodeType.Config) {
        const generation = documentIntegration?.generation;
        if (!generation) return null;
        const run = generation.runs.find(
          (item) => item.nodeId === contentNode.id,
        );
        const state = canvasMediaState(
          contentNode,
          run,
          generation.submitting.includes(contentNode.id),
        );
        if (!state) return null;
        return (
          <CanvasMediaStatus
            state={state}
            color={theme.node.muted}
            run={run}
            frozen={documentIntegration?.frozen}
            onEditInput={() => {
              if (!integrationRef.current?.frozen)
                setDialogNodeId(contentNode.id);
            }}
            onRetry={() => {
              if (!integrationRef.current?.frozen)
                void generation.generate(contentNode.id);
            }}
            onRefresh={() => {
              if (run) void generation.refresh(run);
            }}
          />
        );
      }
      if (integrationRef.current) {
        const values = contentNode.documentData?.parameters?.values ?? {};
        return (
          <div className="grid gap-3 p-4">
            <span className="text-sm font-medium">共享生成参数</span>
            {Object.keys(values).length ? (
              <dl className="max-h-48 overflow-y-auto text-sm">
                {Object.entries(values).map(([key, value]) => (
                  <div key={key} className="grid grid-cols-2 gap-2 py-1">
                    <dt className="break-all">{key}</dt>
                    <dd className="break-all text-muted-foreground">
                      {JSON.stringify(value)}
                    </dd>
                  </div>
                ))}
              </dl>
            ) : (
              <p className="text-sm text-muted-foreground">尚未设置共享参数</p>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDialogNodeId(contentNode.id)}
            >
              编辑共享参数
            </Button>
          </div>
        );
      }
      return (
        <CanvasConfigNodePanel
          node={contentNode}
          isRunning={runningNodeId === contentNode.id}
          inputSummary={getInputSummary(
            configInputsById.get(contentNode.id) || [],
          )}
          onConfigChange={handleConfigNodeChange}
          onComposerToggle={() =>
            setDialogNodeId((current) =>
              current === contentNode.id ? null : contentNode.id,
            )
          }
          onStop={confirmStopGeneration}
          onGenerate={(nodeId) => {
            const target = nodesRef.current.find((item) => item.id === nodeId);
            void handleGenerateNode(
              nodeId,
              target?.metadata?.generationMode || "image",
              target?.metadata?.composerContent ??
                target?.metadata?.prompt ??
                "",
            );
          }}
        />
      );
    },
    [
      configInputsById,
      documentIntegration,
      theme.node.muted,
      confirmStopGeneration,
      handleConfigNodeChange,
      handleGenerateNode,
      runningNodeId,
    ],
  );

  if (!projectLoaded) return <CanvasRefreshShell />;

  return (
    <section
      className="flex h-full min-h-0 flex-1 overflow-hidden"
      style={{ background: theme.canvas.background, color: theme.node.text }}
      onCompositionStartCapture={(event) => {
        composingElement.current = event.target as HTMLElement;
        setComposing(true);
      }}
      onCompositionEndCapture={() => {
        composingElement.current = null;
        setComposing(false);
      }}
      onPointerDownCapture={(event) => {
        const target = event.target as Element;
        if (!target.closest("[data-canvas-no-zoom]")) setPointerActive(true);
      }}
    >
      <CanvasSidePanel
        projectId={documentIntegration?.projectId}
        nodes={nodes}
        selectedNodeIds={selectedNodeIds}
        onFocusNode={focusNode}
        onPreviewNode={setPreviewNodeId}
        onInsertAsset={handleAssetInsert}
        assetsRevision={assetsRevision}
        savedAssetScope={savedAssetScope}
      />
      <section
        className="@container/canvas relative isolate min-w-0 flex-1 overflow-hidden"
        onPointerMoveCapture={(event) => {
          if (containerRef.current?.contains(event.target as Node))
            documentIntegration?.onCursor?.(
              screenToCanvas(event.clientX, event.clientY),
            );
        }}
        onPointerLeave={() => documentIntegration?.onCursor?.(null)}
      >
        {documentIntegration ? (
          documentIntegration.toolbar({
            canUndo: documentIntegration.undo?.canUndo ?? historyState.canUndo,
            canRedo: documentIntegration.undo?.canRedo ?? historyState.canRedo,
            onHome: () => navigate("/"),
            onProjects: () => navigate(workspaceRoutes.projects.href),
            onCreateProject: createAndOpenProject,
            onExportProject: exportCurrentProject,
            onImportImage: () => handleUploadRequest(),
            onUndo: undoCanvas,
            onRedo: redoCanvas,
          })
        ) : (
          <CanvasTopBar
            title={currentProject?.title || "未命名画布"}
            onRenameProject={() => {
              message.info("请从项目空间打开已保存的画布");
            }}
            canUndo={historyState.canUndo}
            canRedo={historyState.canRedo}
            onHome={() => navigate("/")}
            onProjects={() => navigate(workspaceRoutes.projects.href)}
            onCreateProject={createAndOpenProject}
            onDeleteProject={deleteCurrentProject}
            onExportProject={exportCurrentProject}
            onImportImage={() => handleUploadRequest()}
            onUndo={undoCanvas}
            onRedo={redoCanvas}
          />
        )}

        <InfiniteCanvas
          containerRef={containerRef}
          viewport={viewport}
          onViewportPreview={previewViewport}
          tool={canvasTool}
          backgroundMode={backgroundMode}
          onViewportChange={(next) => {
            setViewport(next);
            setContextMenu(null);
          }}
          onCanvasMouseDown={(event) => {
            if (!referencePickerNodeId) handleCanvasMouseDown(event);
          }}
          onCanvasDeselect={referencePickerNodeId ? undefined : deselectCanvas}
          onCanvasDoubleClick={(event) => {
            if (referencePickerNodeId || integrationRef.current?.frozen) return;
            setContextMenu(null);
            setNodeCreatePosition(screenToCanvas(event.clientX, event.clientY));
          }}
          onContextMenu={preventCanvasContextMenu}
          onDrop={handleDrop}
        >
          {documentIntegration?.presenceOverlay}
          <svg
            data-canvas-drag-guides
            aria-hidden="true"
            className="pointer-events-none absolute top-0 left-0 z-[60] overflow-visible"
            style={{ color: theme.node.activeStroke, width: 1, height: 1 }}
          />
          <svg
            className="w-reference-panel h-reference-panel absolute top-0 left-0 overflow-visible"
            style={{
              pointerEvents: "none",
              transform: "translateZ(0)",
              zIndex: 0,
            }}
          >
            {showConnections &&
              visibleConnections.map(({ connection, from, to }) => (
                <ConnectionPath
                  key={connection.id}
                  connection={connection}
                  from={from}
                  to={to}
                  selected={selectedConnectionId === connection.id}
                  active={relatedHighlight.connectionIds.has(connection.id)}
                  generating={
                    documentIntegration
                      ? documentIntegration.generation?.runs.some(
                          (run) =>
                            run.nodeId === to.id &&
                            run.pending &&
                            matchesFrozenNode(to, run),
                        )
                      : to.metadata?.status === NODE_STATUS_LOADING
                  }
                  onSelect={() => {
                    setSelectedConnectionId(connection.id);
                    setSelectedNodeIds(new Set());
                    setContextMenu(null);
                  }}
                  onContextMenu={(event) => {
                    setSelectedConnectionId(connection.id);
                    setSelectedNodeIds(new Set());
                    setContextMenu({
                      type: "connection",
                      x: event.clientX,
                      y: event.clientY,
                      connectionId: connection.id,
                    });
                  }}
                />
              ))}
            {connectingParams ? (
              <ActiveConnectionPath
                node={nodeById.get(connectingParams.nodeId)}
                handle={connectingParams}
                mouseWorld={mouseWorld}
                target={
                  connectionTargetNodeId
                    ? nodeById.get(connectionTargetNodeId)
                    : undefined
                }
              />
            ) : null}
          </svg>

          {visibleNodes.map((node) => (
            <CanvasNode
              readOnly={documentIntegration?.frozen}
              collaboration={documentIntegration?.textBinding?.(node.id)}
              key={node.id}
              data={node}
              scale={viewport.k}
              isSelected={selectedNodeIds.has(node.id)}
              isRelated={relatedHighlight.nodeIds.has(node.id)}
              isFocusRelated={activeNodeId === node.id}
              isConnectionTarget={connectionTargetNodeId === node.id}
              isConnecting={Boolean(connectingParams)}
              referenceSelectionState={
                !referencePickerNodeId
                  ? undefined
                  : node.id === referencePickerNodeId
                    ? "target"
                    : referenceConnectedNodeIds.has(node.id) ||
                        !isCanvasReferenceNode(node, nodes) ||
                        (!!documentIntegration &&
                          !supportsCanvasConnection(
                            node.type,
                            nodes.find(
                              (item) => item.id === referencePickerNodeId,
                            )?.type ?? "",
                          ))
                      ? "disabled"
                      : "available"
              }
              showPanel={
                !documentIntegration?.frozen &&
                !isNodeResizing &&
                dialogNodeId === node.id &&
                !selectionBox &&
                !getNodeDefinition(node.type)?.hidePanel &&
                !isFixedCanvasMedia(node)
              }
              isMobile={isMobile}
              connectionTypes={
                documentIntegration
                  ? {
                      ...canvasConnectionHandles(node.type),
                      input:
                        !isFixedCanvasMedia(node) &&
                        canvasConnectionHandles(node.type).input,
                    }
                  : undefined
              }
              groupChildCount={groupChildCountById.get(node.id) || 0}
              isGroupDropTarget={dropTargetGroupId === node.id}
              batchExpanded={expandedBatchNodeIds.has(node.id)}
              showImageInfo={showImageInfo}
              mentionReferences={
                mentionReferencesByNodeId.get(node.id) || EMPTY_REFERENCES
              }
              pluginHost={pluginHost}
              registryVersion={nodeRegistryVersion}
              renderPanel={renderNodePanel}
              onPanelClose={(nodeId) =>
                setDialogNodeId((current) =>
                  current === nodeId ? null : current,
                )
              }
              renderNodeContent={renderNodeContentPanel}
              onPreviewLoaded={(nodeId, source, dimensions) => {
                measureMedia(nodeId, source, dimensions);
                integrationRef.current?.generation?.previewLoaded?.(
                  nodeId,
                  source,
                );
              }}
              onPreviewError={(nodeId, source) =>
                integrationRef.current?.generation?.recoverPreview?.(
                  nodeId,
                  source,
                )
              }
              onMouseDown={handleNodeMouseDown}
              onSelectCapture={handleNodeSelectCapture}
              onHoverStart={handleNodeHoverStart}
              onHoverEnd={handleNodeHoverEnd}
              onConnectStart={handleConnectStart}
              onResizeStart={handleNodeResizeStart}
              onResize={handleNodeResize}
              onResizeEnd={handleNodeResizeEnd}
              onContentChange={handleNodeContentChange}
              acceptContent={(content, richText) =>
                admitGraph(
                  nodesStateRef.current.map((current) =>
                    current.id === node.id
                      ? {
                          ...current,
                          metadata: { ...current.metadata, content, richText },
                        }
                      : current,
                  ),
                  connectionsStateRef.current,
                )
              }
              onTitleChange={handleNodeTitleChange}
              onToggleBatch={toggleBatchExpanded}
              onSetBatchPrimary={setBatchPrimary}
              onDuplicateBatchImage={duplicateBatchImage}
              onDownloadBatchImage={downloadBatchImage}
              onRetryBatchImage={retryBatchImage}
              onDeleteBatchImage={deleteBatchImage}
              onRetry={handleNodeRetry}
              onViewImage={handleNodeViewImage}
              onSelectReference={selectNodeReference}
              onContextMenu={handleNodeContextMenu}
            />
          ))}

          {referencePickerNodeId ? (
            <Button
              variant="outline"
              type="button"
              className="absolute top-4 left-1/2 z-[90] -translate-x-1/2 border px-4 py-2 backdrop-blur"
              onClick={exitNodeReferenceSelection}
            >
              {"从画布选择参考 · ESC 返回输入框"}
            </Button>
          ) : null}

          {selectionBox ? (
            <svg
              className="pointer-events-none absolute z-[100] overflow-visible"
              style={{
                left: Math.min(
                  selectionBox.startWorldX,
                  selectionBox.currentWorldX,
                ),
                top: Math.min(
                  selectionBox.startWorldY,
                  selectionBox.currentWorldY,
                ),
                width: Math.abs(
                  selectionBox.currentWorldX - selectionBox.startWorldX,
                ),
                height: Math.abs(
                  selectionBox.currentWorldY - selectionBox.startWorldY,
                ),
              }}
            >
              <rect
                width="100%"
                height="100%"
                fill={theme.canvas.selectionFill}
                stroke={theme.canvas.selectionStroke}
                strokeOpacity={0.55}
                strokeWidth={1 / viewport.k}
                strokeDasharray={`${6 / viewport.k} ${4 / viewport.k}`}
              />
            </svg>
          ) : null}
          {pendingConnectionCreate ? (
            <ConnectionCreateMenu
              pending={pendingConnectionCreate}
              existingType={
                nodes.find(
                  (node) =>
                    node.id === pendingConnectionCreate.connection.nodeId,
                )?.type ?? ""
              }
              onCreate={(type) =>
                createConnectedNode(type, pendingConnectionCreate)
              }
              onClose={cancelPendingConnectionCreate}
            />
          ) : null}
          {nodeCreatePosition ? (
            <NodeCreateMenu
              position={nodeCreatePosition}
              onCreate={(type) => {
                createNode(type, nodeCreatePosition);
                setNodeCreatePosition(null);
              }}
              onClose={() => setNodeCreatePosition(null)}
            />
          ) : null}
        </InfiniteCanvas>

        {!nodes.length ? <CanvasEmptyState /> : null}

        <CanvasNodeHoverToolbar
          canvasSize={size}
          node={
            isNodeDragging ||
            isNodeResizing ||
            nodeImageSettingsOpen ||
            expandedBatchNodeIds.has(toolbarNode?.id || "")
              ? null
              : toolbarNode
          }
          viewport={viewport}
          extraTools={
            toolbarNode ? buildNodeToolbarItems(toolbarNode) : undefined
          }
          onKeep={keepNodeToolbar}
          onLeave={hideNodeToolbar}
          onInfo={(node) => setInfoNodeId(node.id)}
          onDecreaseFont={(node) =>
            handleFontSizeChange(
              node.id,
              Math.max(10, (node.metadata?.fontSize || 14) - 2),
            )
          }
          onIncreaseFont={(node) =>
            handleFontSizeChange(
              node.id,
              Math.min(32, (node.metadata?.fontSize || 14) + 2),
            )
          }
          onToggleDialog={(node) =>
            setDialogNodeId((current) => (current === node.id ? null : node.id))
          }
          onGenerateImage={generateImageFromTextNode}
          onUpload={handleUploadRequest}
          onRemoveAsset={handleRemoveAsset}
          onDownload={downloadNodeImage}
          onSaveAsset={(node) => void saveNodeAsset(node)}
          onMaskEdit={(node) => setMaskEditNodeId(node.id)}
          onCrop={(node) => setCropNodeId(node.id)}
          onSplit={(node) => setSplitNodeId(node.id)}
          onUpscale={(node) => setUpscaleNodeId(node.id)}
          onSuperResolve={(node) => setSuperResolveNodeId(node.id)}
          onAngle={(node) => setAngleNodeId(node.id)}
          onViewImage={handleNodeViewImage}
          onReversePrompt={createImageReversePromptNodes}
          onRetry={(node) => void handleRetryNode(node)}
          onToggleFreeResize={(node) => toggleNodeFreeResize(node.id)}
          onDelete={(node) => deleteNodes(new Set([node.id]))}
          onUngroup={(node) => ungroupSelection(new Set([node.id]))}
        />

        {(hasMultipleSelectedNodes ||
          (documentIntegration?.workflow?.enabled &&
            selectedNodes.some(
              (node) => node.type === CanvasNodeType.Group,
            ))) &&
        !selectionBox ? (
          <CanvasSelectionToolbar
            nodes={selectedNodes}
            viewport={viewport}
            showToolbar={!isNodeDragging && !isNodeResizing}
            canGroup={canGroupSelection}
            canUngroup={canUngroupSelection}
            onGroup={groupSelection}
            onUngroup={ungroupSelection}
            runDisabled={
              documentIntegration?.workflow?.busy ||
              !documentIntegration?.workflow?.accepting
            }
            onRun={
              documentIntegration?.workflow?.enabled
                ? () => {
                    integrationRef.current?.onChange(
                      {
                        nodes: nodesRef.current,
                        connections: connectionsRef.current,
                        backgroundMode,
                        showImageInfo,
                        viewport: savedViewport,
                      },
                      true,
                    );
                    void integrationRef.current?.workflow?.choose(
                      selectedNodes.map((node) => node.id),
                    );
                  }
                : undefined
            }
          />
        ) : null}

        <CanvasToolbar
          canEdit={!documentIntegration?.frozen}
          selectedCount={selectedNodeIds.size}
          canvasTool={canvasTool}
          canUndo={documentIntegration?.undo?.canUndo ?? historyState.canUndo}
          canRedo={documentIntegration?.undo?.canRedo ?? historyState.canRedo}
          backgroundMode={backgroundMode}
          showImageInfo={showImageInfo}
          onCreateNode={createNode}
          onUndo={undoCanvas}
          onRedo={redoCanvas}
          onBrowseAssets={() => handleUploadRequest()}
          onUpload={() => {
            handleUploadRequest();
            setAssetUploadOpen(true);
          }}
          onDelete={() => deleteNodes(new Set(selectedNodeIds))}
          onClear={() => setClearConfirmOpen(true)}
          onCanvasToolChange={setCanvasTool}
          onBackgroundModeChange={setBackgroundMode}
          onShowImageInfoChange={setShowImageInfo}
        />

        {isMiniMapOpen ? (
          <Minimap
            nodes={nodes}
            viewport={viewport}
            viewportSize={size}
            onViewportChange={setViewport}
          />
        ) : null}

        <CanvasZoomControls
          canvasTool={canvasTool}
          onCanvasToolChange={setCanvasTool}
          scale={viewport.k}
          onScaleChange={setZoomScale}
          onFit={resetViewport}
          onFitSelection={
            selectedNodeIds.size
              ? () => {
                  if (focusAnimRef.current)
                    cancelAnimationFrame(focusAnimRef.current);
                  setViewport(
                    fitCanvasNodes(
                      nodesRef.current.filter((node) =>
                        selectedNodeIds.has(node.id),
                      ),
                      size,
                    ),
                  );
                  setContextMenu(null);
                }
              : undefined
          }
          isMiniMapOpen={isMiniMapOpen}
          onToggleMiniMap={() => setIsMiniMapOpen(!isMiniMapOpen)}
          showConnections={showConnections}
          onToggleConnections={() => setShowConnections(!showConnections)}
          snapToGrid={snapToGrid}
          onToggleSnap={() => setSnapToGrid(!snapToGrid)}
        />

        {contextMenu ? (
          <CanvasNodeContextMenu
            menu={contextMenu}
            canCaptureVideoFrame={
              contextMenuNode?.type === CanvasNodeType.Video &&
              Boolean(contextMenuNode.metadata?.content)
            }
            canGroup={contextMenu.type === "node" && canGroupSelection}
            canUngroup={contextMenu.type === "node" && canUngroupSelection}
            onClose={() => setContextMenu(null)}
            onEditConnection={
              documentIntegration
                ? () => {
                    if (
                      contextMenu.type !== "connection" ||
                      integrationRef.current?.frozen
                    )
                      return;
                    const edge = connectionsRef.current.find(
                      (item) => item.id === contextMenu.connectionId,
                    )?.documentData;
                    if (edge) setDocumentConnection(edge);
                    setContextMenu(null);
                  }
                : undefined
            }
            onCaptureVideoFrame={(position) => {
              if (contextMenu.type !== "node") return;
              void captureVideoNodeFrame(contextMenu.nodeId, position);
            }}
            onDuplicate={() => {
              if (contextMenu.type !== "node") return;
              duplicateNode(contextMenu.nodeId);
              setContextMenu(null);
            }}
            onGroup={groupSelection}
            onUngroup={ungroupSelection}
            onDelete={() => {
              if (contextMenu.type === "node") {
                deleteNodes(new Set([contextMenu.nodeId]));
              } else {
                deleteConnection(contextMenu.connectionId);
              }
              setContextMenu(null);
            }}
          />
        ) : null}

        <CanvasNodeInfoModal
          node={infoNode}
          open={Boolean(infoNode)}
          onClose={() => setInfoNodeId(null)}
        />

        {cropNode?.metadata?.content ? (
          <CanvasNodeCropDialog
            dataUrl={cropNode.metadata.content}
            open={Boolean(cropNode)}
            onClose={() => setCropNodeId(null)}
            onConfirm={(crop) => void cropImageNode(cropNode!, crop)}
          />
        ) : null}

        {maskEditNode?.metadata?.content ? (
          <CanvasNodeMaskEditDialog
            dataUrl={maskEditNode.metadata.content}
            open={Boolean(maskEditNode)}
            onClose={() => setMaskEditNodeId(null)}
            onConfirm={(payload) =>
              void maskEditImageNode(maskEditNode!, payload)
            }
          />
        ) : null}

        {splitNode?.metadata?.content ? (
          <CanvasNodeSplitDialog
            dataUrl={splitNode.metadata.content}
            open={Boolean(splitNode)}
            onClose={() => setSplitNodeId(null)}
            onConfirm={(params) => void splitImageNode(splitNode!, params)}
          />
        ) : null}

        {upscaleNode?.metadata?.content ? (
          <CanvasNodeUpscaleDialog
            dataUrl={upscaleNode.metadata.content}
            open={Boolean(upscaleNode)}
            onClose={() => setUpscaleNodeId(null)}
            onConfirm={(params) => void upscaleImageNode(upscaleNode!, params)}
          />
        ) : null}

        <Dialog
          open={Boolean(superResolveNode?.metadata?.content)}
          onOpenChange={(open) => {
            if (!open) (() => setSuperResolveNodeId(null))();
          }}
        >
          <DialogContent className={"max-h-dialog overflow-y-auto"}>
            <DialogHeader>
              <DialogTitle>{"AI 超分"}</DialogTitle>
              <DialogDescription className="sr-only">
                超分服务尚未接入。
              </DialogDescription>
            </DialogHeader>
            <div>
              <div className="py-8 text-center text-base font-medium">
                {"暂未实现"}
              </div>
            </div>
          </DialogContent>
        </Dialog>

        {angleNode?.metadata?.content ? (
          <CanvasNodeAngleDialog
            dataUrl={angleNode.metadata.content}
            open={Boolean(angleNode)}
            onClose={() => setAngleNodeId(null)}
            onConfirm={(params) => void generateAngleNode(angleNode!, params)}
          />
        ) : null}

        <Dialog
          open={Boolean(previewContent)}
          onOpenChange={(open) => {
            if (!open) (() => setPreviewNodeId(null))();
          }}
        >
          <DialogContent className="max-h-dialog dialog-canvas overflow-y-auto">
            <DialogHeader>
              <DialogTitle>
                {previewNode?.type === "video" ? "视频详情" : "图片详情"}
              </DialogTitle>
              <DialogDescription className="sr-only">
                查看当前媒体节点的预览内容。
              </DialogDescription>
            </DialogHeader>
            <div className="max-h-media-sheet flex items-center justify-center">
              {previewContent && previewNode?.type === "video" ? (
                <video
                  src={previewContent}
                  controls
                  playsInline
                  preload="metadata"
                  aria-label={previewNode.title || "视频预览"}
                  className="max-h-media-sheet max-w-full rounded-xl object-contain"
                />
              ) : previewContent ? (
                <PreviewImage
                  src={previewContent}
                  alt={previewNode?.title || "图片"}
                  className="max-h-media-sheet max-w-full object-contain"
                />
              ) : null}
            </div>
          </DialogContent>
        </Dialog>

        <Dialog
          open={clearConfirmOpen}
          onOpenChange={(open) => {
            if (!open) (() => setClearConfirmOpen(false))();
          }}
        >
          <DialogContent className={"max-h-dialog overflow-y-auto"}>
            <DialogHeader>
              <DialogTitle>{"清空画布？"}</DialogTitle>
              <DialogDescription className="sr-only">
                清空当前页面的所有节点和连线，此操作仅影响编辑草稿。
              </DialogDescription>
            </DialogHeader>
            <div>
              <p className="text-sm opacity-60">
                {"这会删除当前画布上的所有节点和连线。"}
              </p>
            </div>
            <DialogFooter>
              {
                <>
                  <Button
                    onClick={() => setClearConfirmOpen(false)}
                    type={"button"}
                    variant={"secondary"}
                    size="default"
                  >
                    {"取消"}
                  </Button>
                  <Button
                    onClick={clearCanvas}
                    type={"button"}
                    variant={"destructive"}
                    size="default"
                  >
                    {"清空"}
                  </Button>
                </>
              }
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {documentConnection &&
          (() => {
            const source = nodes.find(
              (node) => node.id === documentConnection.fromNodeId,
            );
            const target = nodes.find(
              (node) => node.id === documentConnection.toNodeId,
            );
            return source && target ? (
              <CanvasConnectionDialog
                key={documentConnection.id}
                edge={documentConnection}
                agentProfiles={
                  documentIntegration?.workflow?.agentProfiles ?? []
                }
                source={source}
                target={target}
                connections={connections.filter(
                  (item) => item.id !== documentConnection.id,
                )}
                onClose={() => setDocumentConnection(null)}
                onConnect={(edge) => {
                  if (integrationRef.current?.frozen) return;
                  try {
                    const current = connectionsRef.current;
                    if (!current.some((item) => item.id === edge.id))
                      throw new Error("连接已不存在");
                    validateCanvasConnection(
                      edge,
                      current.flatMap((item) =>
                        item.id !== edge.id && item.documentData
                          ? [item.documentData]
                          : [],
                      ),
                    );
                    const next = current.map((item) =>
                      item.id === edge.id
                        ? { ...item, documentData: edge }
                        : item,
                    );
                    connectionsRef.current = next;
                    setConnections(next);
                    setDocumentConnection(null);
                  } catch (error) {
                    notifyError(error, "无法调整引用");
                  }
                }}
              />
            ) : null;
          })()}
        {assetSave && account && assetSave.accountId === String(account.id) && (
          <CanvasSaveAssetDialog
            projectId={documentIntegration?.projectId}
            key={`${assetSave.accountId}:${assetSave.node.id}`}
            node={assetSave.node}
            accountId={assetSave.accountId}
            onClose={() => setAssetSave(null)}
            onSaved={(scope) => {
              setSavedAssetScope(scope);
              setAssetsRevision((value) => value + 1);
            }}
          />
        )}
        <AssetPickerModal
          projectId={documentIntegration?.projectId}
          open={assetPickerOpen}
          initialUpload={assetUploadOpen}
          kinds={assetTarget ? [assetTarget.type] : undefined}
          onInsert={(asset) => handleAssetInsert(asset, sourceFiles.length > 1)}
          onSourceFilesComplete={() => {
            setSourceFiles([]);
            setAssetPickerOpen(false);
          }}
          onClose={() => {
            setAssetPickerOpen(false);
            setAssetTarget(undefined);
            setSourceFile(undefined);
            setSourceFiles([]);
          }}
          sourceFile={sourceFile}
          sourceFiles={sourceFiles}
        />
      </section>
    </section>
  );
}
