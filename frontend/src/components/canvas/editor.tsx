"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type SetStateAction,
} from "react";
import { useStore } from "zustand";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  Plus,
  Minus,
  Maximize2,
  Undo2,
  Redo2,
  Group,
  Ungroup,
  Copy,
  Clipboard,
  Trash2,
  Search,
  ImageIcon,
  AlignStartHorizontal,
  LayoutGrid,
  Expand,
  Minimize2,
  Keyboard,
  Upload,
} from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel } from "@/components/ui/field";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ApiError } from "@/lib/request";
import { createCanvasStore } from "./store";
import {
  CanvasNodeType,
  type CanvasCommand,
  type CanvasDocument,
  type CanvasNodeData,
  type ConnectionHandle,
  type Position,
  type ViewportTransform,
} from "./model";
import { applyCommands, createNode, diffNodes, prepareEdit } from "./document";
import { listMediaAssets, uploadCanvasMedia, type MediaAsset } from "./queries";
import { InfiniteCanvas } from "./engine/infinite-canvas";
import { useCanvasSelectionController } from "./engine/use-selection-controller";
import {
  applyCanvasLiveViewport,
  subscribeCanvasSelectionPreview,
} from "./engine/viewport-dom";
import {
  viewportAtScale,
  getCanvasNodesBounds,
  viewportForBounds,
} from "./engine/viewport";
import { useCanvasViewportTransition } from "./engine/use-viewport-transition";
import { shouldRefreshCanvasVirtualization } from "./engine/viewport-render-sync";
import {
  alignCanvasNodes,
  layoutCanvasAuto,
  layoutCanvasNodes,
  layoutCanvasFlow,
  spreadCanvasNodes,
  type CanvasLayoutMode,
  type CanvasAlignmentMode,
} from "./engine/layout";
import {
  isNodeHiddenByCollapsedFrame,
  getFrameChildIds,
  selectionRootNodes,
  moveNodesWithChildren,
} from "./engine/frame";
import {
  createRenderIndex,
  visibleCanvasNodes,
  canvasDetailLevel,
  createConnectionRenderIndex,
  visibleCanvasConnections,
} from "./engine/virtualization";
import {
  copySelection,
  pasteSelection,
  readCanvasClipboard,
  writeCanvasClipboard,
  decodeCanvasClipboard,
} from "./engine/clipboard";
import { useCanvasKeyboard } from "./engine/keyboard";
import { CanvasShortcutsDialog } from "./engine/shortcuts-dialog";
import { MediaPreviewDialog } from "./media-preview-dialog";
import {
  MediaUploadDialog,
  type CanvasMediaUpload,
} from "./media-upload-dialog";
import { mediaAssetsToCanvasNodes } from "./media-import";
import {
  activeConnectionPath,
  ConnectionPath,
  bindCanvasConnectionPreview,
} from "./engine/connections";
import { Minimap } from "./engine/minimap";
import {
  CanvasDock,
  CanvasSelectionToolbar,
  CanvasToolMode,
} from "./engine/toolbar";
import { NodeShell } from "./nodes/node-shell";
import { NodeContent } from "./nodes/node-content";
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuGroup,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubTrigger,
  ContextMenuSubContent,
} from "@/components/ui/context-menu";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import type { CanvasBackgroundMode } from "./engine/theme";
import "./canvas.css";

type Attempt = {
  revision: number;
  commands: CanvasCommand[];
  key: string;
  kind: "edit" | "undo" | "redo";
};
export type CanvasEditorProps = {
  document: CanvasDocument;
  readOnly?: boolean;
  save: (
    revision: number,
    commands: CanvasCommand[],
    key: string,
  ) => Promise<CanvasDocument>;
  reload: () => Promise<CanvasDocument>;
  onDirtyChange: (dirty: boolean) => void;
  onBusyChange?: (busy: boolean) => void;
};
export function CanvasEditor({
  document: initial,
  readOnly = false,
  save,
  reload,
  onDirtyChange,
  onBusyChange,
}: CanvasEditorProps) {
  const [store] = useState(() => createCanvasStore(initial));
  const document = useStore(store, (state) => state.document),
    selectedIds = useStore(store, (state) => state.selectedIds),
    selectedEdge = useStore(store, (state) => state.selectedEdge),
    draft = useStore(store, (state) => state.draft),
    past = useStore(store, (state) => state.past),
    future = useStore(store, (state) => state.future);
  const [viewport, setViewport] = useState(initial.viewport),
    [renderViewport, setRenderViewport] = useState(initial.viewport),
    [size, setSize] = useState({ width: 1200, height: 700 });
  const [tool, setTool] = useState<"select" | "move">("select"),
    [saving, setSaving] = useState(false),
    [failed, setFailed] = useState<{ attempt: Attempt; error: Error }>(),
    [notice, setNotice] = useState(`已读取修订 ${initial.revision}`);
  const [mediaOpen, setMediaOpen] = useState(false),
    [searchOpen, setSearchOpen] = useState(false),
    [search, setSearch] = useState(""),
    [titleDrafts, setTitleDrafts] = useState<string[]>([]);
  const [background, setBackground] = useState<CanvasBackgroundMode>("dots"),
    [minimapVisible, setMinimapVisible] = useState(true),
    [focusMode, setFocusMode] = useState(false),
    [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [previewNode, setPreviewNode] = useState<CanvasNodeData>();
  const [uploadOpen, setUploadOpen] = useState(false),
    [uploadFiles, setUploadFiles] = useState<File[]>([]),
    [importing, setImporting] = useState(false),
    [fileDragOver, setFileDragOver] = useState(false);
  const uploadTarget = useRef<
    { point: Position; connection?: ConnectionHandle } | undefined
  >(undefined);
  const importAttempt = useRef<
    { assetIds: string; attempt: Attempt } | undefined
  >(undefined);
  const interactionLocked = useRef(false),
    mounted = useRef(true);
  const [creationMenu, setCreationMenu] = useState<{
    x: number;
    y: number;
    world: Position;
    connection?: ConnectionHandle;
  }>();
  const contextPoint = useRef<Position | undefined>(undefined);
  const mediaTarget = useRef<
    { point: Position; connection?: ConnectionHandle } | undefined
  >(undefined);
  const containerRef = useRef<HTMLDivElement>(null),
    nodesRef = useRef(document.nodes),
    viewportRef = useRef(viewport),
    selectedRef = useRef(new Set(selectedIds)),
    paused = useRef(false),
    busy = useRef(false),
    lastRender = useRef(0);
  const [connection, setConnection] = useState<{
    handle: ConnectionHandle;
    mouse: Position;
  }>();
  const connecting = useRef<ConnectionHandle | undefined>(undefined);
  const connectionFrame = useRef<number | null>(null);
  const searchOpener = useRef<HTMLElement | null>(null);
  const mediaOpener = useRef<HTMLElement | null>(null);
  const openSearch = () => {
    const target = window.document.activeElement;
    searchOpener.current = target instanceof HTMLElement ? target : null;
    setSearchOpen(true);
  };
  const openMedia = () => {
    const target = window.document.activeElement;
    mediaOpener.current = target instanceof HTMLElement ? target : null;
    setMediaOpen(true);
  };
  const selected = useMemo(() => new Set(selectedIds), [selectedIds]);
  useLayoutEffect(() => {
    nodesRef.current = document.nodes;
    selectedRef.current = selected;
  }, [document.nodes, selected]);
  useLayoutEffect(() => {
    viewportRef.current = viewport;
  }, [viewport]);
  const textDirty = Boolean(
    draft &&
    draft.text !==
      document.nodes.find((node) => node.id === draft.id)?.metadata?.content,
  );
  const dirty =
    saving ||
    importing ||
    Boolean(failed) ||
    textDirty ||
    titleDrafts.length > 0;
  const titleDraftChanged = useCallback(
    (id: string, isDirty: boolean) =>
      setTitleDrafts((current) =>
        isDirty
          ? [...current.filter((item) => item !== id), id]
          : current.filter((item) => item !== id),
      ),
    [],
  );
  const locked =
    readOnly || saving || importing || Boolean(failed) || textDirty;
  useLayoutEffect(() => {
    interactionLocked.current = locked;
  }, [locked]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);
  useEffect(() => {
    onBusyChange?.(saving || importing);
  }, [saving, importing, onBusyChange]);
  useEffect(
    () => () => {
      onDirtyChange(false);
    },
    [onDirtyChange],
  );
  useEffect(() => {
    const target = containerRef.current;
    if (!target) return;
    const observer = new ResizeObserver((entries) => {
      const rect = entries[0]?.contentRect;
      if (rect) setSize({ width: rect.width, height: rect.height });
    });
    observer.observe(target);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    const target = containerRef.current;
    if (!target) return;
    return subscribeCanvasSelectionPreview(target, (selection) => {
      const element = target.querySelector<HTMLElement>(
        "[data-canvas-selection-box]",
      );
      if (element) {
        const left = Math.min(selection.startWorldX, selection.currentWorldX),
          top = Math.min(selection.startWorldY, selection.currentWorldY);
        element.style.transform = `translate(${left}px,${top}px)`;
        element.style.width = `${Math.abs(selection.currentWorldX - selection.startWorldX)}px`;
        element.style.height = `${Math.abs(selection.currentWorldY - selection.startWorldY)}px`;
      }
    });
  }, []);
  async function persist(attempt: Attempt) {
    if (busy.current || readOnly) return false;
    busy.current = true;
    setSaving(true);
    setNotice("正在等待服务端确认…");
    try {
      if (attempt.kind === "edit") {
        const prepared = prepareEdit(
          store.getState().document,
          attempt.commands,
        );
        attempt.commands = prepared.forward;
      } else applyCommands(store.getState().document, attempt.commands);
      const saved = await save(attempt.revision, attempt.commands, attempt.key);
      store.getState().confirm(saved, attempt.commands, attempt.kind);
      viewportRef.current = saved.viewport;
      setViewport(saved.viewport);
      setRenderViewport(saved.viewport);
      setFailed(undefined);
      setNotice(`已保存 · 修订 ${saved.revision}`);
      return true;
    } catch (error) {
      const failure =
        error instanceof Error ? error : new Error("保存未完成。");
      setFailed({ attempt, error: failure });
      setNotice("修改尚未确认保存。");
      return false;
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }
  function submit(
    commands: CanvasCommand[],
    kind: Attempt["kind"] = "edit",
    allowDraft = false,
  ) {
    if (
      readOnly ||
      busy.current ||
      failed ||
      (textDirty && !allowDraft) ||
      !commands.length
    )
      return;
    void persist({
      commands,
      kind,
      revision: store.getState().document.revision,
      key: crypto.randomUUID(),
    });
  }
  async function readLatest() {
    if (busy.current) return;
    busy.current = true;
    setSaving(true);
    try {
      const latest = await reload();
      store.getState().reset(latest);
      importAttempt.current = undefined;
      setViewport(latest.viewport);
      viewportRef.current = latest.viewport;
      setRenderViewport(latest.viewport);
      setFailed(undefined);
      setNotice(`已读取修订 ${latest.revision}，撤销历史已清空。`);
    } catch (error) {
      const failure = error instanceof Error ? error : new Error("读取失败。");
      setNotice(failure.message);
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }
  const previewViewport = useCallback(
    (next: ViewportTransform) => {
      viewportRef.current = next;
      applyCanvasLiveViewport(containerRef.current, next);
      const now = performance.now();
      if (
        shouldRefreshCanvasVirtualization(
          renderViewport,
          next,
          lastRender.current,
          now,
        )
      ) {
        lastRender.current = now;
        setRenderViewport(next);
      }
    },
    [renderViewport],
  );
  function commitViewport(next: ViewportTransform) {
    viewportRef.current = next;
    setViewport(next);
    setRenderViewport(next);
    if (
      JSON.stringify(next) !==
      JSON.stringify(store.getState().document.viewport)
    )
      submit([{ type: "SetViewport", viewport: next }]);
  }
  const transition = useCanvasViewportTransition(
    viewportRef,
    previewViewport,
    commitViewport,
  );
  function screenToCanvas(x: number, y: number) {
    const rect = containerRef.current?.getBoundingClientRect();
    return {
      x:
        (x - (rect?.left ?? 0) - viewportRef.current.x) / viewportRef.current.k,
      y: (y - (rect?.top ?? 0) - viewportRef.current.y) / viewportRef.current.k,
    };
  }
  function canvasCenter() {
    return {
      x: (size.width / 2 - viewportRef.current.x) / viewportRef.current.k,
      y: (size.height / 2 - viewportRef.current.y) / viewportRef.current.k,
    };
  }
  function nodeCreationCommands(
    node: CanvasNodeData,
    handle?: ConnectionHandle,
  ): CanvasCommand[] {
    return [
      { type: "AddNodes", nodes: [node] },
      ...(handle
        ? [
            {
              type: "Connect" as const,
              edges: [
                {
                  id: crypto.randomUUID(),
                  fromNodeId:
                    handle.handleType === "source" ? handle.nodeId : node.id,
                  toNodeId:
                    handle.handleType === "source" ? node.id : handle.nodeId,
                },
              ],
            },
          ]
        : []),
    ];
  }
  function createAt(
    type: CanvasNodeType.Text | CanvasNodeType.Frame,
    point = canvasCenter(),
    handle?: ConnectionHandle,
  ) {
    if (locked || document.nodes.length >= 2000) return;
    const node = createNode(type, point);
    submit(nodeCreationCommands(node, handle));
  }
  function openCreation(x: number, y: number, handle?: ConnectionHandle) {
    if (locked || document.nodes.length >= 2000) return;
    setCreationMenu({ x, y, world: screenToCanvas(x, y), connection: handle });
  }
  function openMediaAt(point = canvasCenter(), handle?: ConnectionHandle) {
    mediaTarget.current = { point, connection: handle };
    openMedia();
  }
  function openUpload(
    files: File[] = [],
    point = canvasCenter(),
    handle?: ConnectionHandle,
  ) {
    if (interactionLocked.current || document.nodes.length >= 2000) return;
    uploadTarget.current = { point, connection: handle };
    importAttempt.current = undefined;
    setUploadFiles(files);
    setUploadOpen(true);
  }
  const uploadMedia: CanvasMediaUpload = (file, key, options) =>
    uploadCanvasMedia(document.projectId, file, key, options);
  async function addUploadedMedia(assets: MediaAsset[]) {
    if (
      !mounted.current ||
      readOnly ||
      textDirty ||
      assets.some(
        (asset) => asset.project_id !== store.getState().document.projectId,
      )
    )
      throw new Error("当前画布不能接收此批素材。");
    const assetIds = assets.map((asset) => asset.id).join(",");
    if (!importAttempt.current || importAttempt.current.assetIds !== assetIds) {
      const nodes = mediaAssetsToCanvasNodes(
        assets,
        uploadTarget.current?.point ?? canvasCenter(),
      );
      const handle = uploadTarget.current?.connection;
      const commands: CanvasCommand[] = [{ type: "AddNodes", nodes }];
      if (handle && nodes[0])
        commands.push(nodeCreationCommands(nodes[0], handle)[1]);
      importAttempt.current = {
        assetIds,
        attempt: {
          commands,
          key: crypto.randomUUID(),
          revision: store.getState().document.revision,
          kind: "edit",
        },
      };
    }
    if (!(await persist(importAttempt.current.attempt)))
      throw new Error("素材已接管，画布未确认保存。");
    importAttempt.current = undefined;
    void media.refetch();
  }
  const setSelected = useCallback(
    (value: SetStateAction<Set<string>>) => {
      const current = selectedRef.current;
      const next = typeof value === "function" ? value(current) : value;
      selectedRef.current = next;
      store.getState().select(next);
    },
    [store],
  );
  function commitNodes(value: SetStateAction<CanvasNodeData[]>) {
    const before = store.getState().document.nodes;
    const next = typeof value === "function" ? value(before) : value;
    submit(diffNodes(before, next));
  }
  const {
    alignmentGuides,
    cancelSelectionBox,
    deselectCanvas,
    handleCanvasMouseDown,
    handleNodeMouseDown,
    isNodeDragging,
    selectionBoundsElementRef,
    selectionBox,
  } = useCanvasSelectionController({
    containerRef,
    nodesRef,
    viewportRef,
    selectedNodeIdsRef: selectedRef,
    historyPausedRef: paused,
    screenToCanvas,
    setNodes: commitNodes,
    setSelectedNodeIds: setSelected,
    setSelectedConnectionId: (value) => {
      const next =
        typeof value === "function"
          ? value(store.getState().selectedEdge ?? null)
          : value;
      if (next) store.getState().selectEdge(next);
    },
    cancelPendingConnectionCreate: () => {
      connecting.current = undefined;
      setConnection(undefined);
    },
    onCanvasSelectionStart: () => {},
    onNodeInteractionStart: () => {},
    onNodeClick: () => {},
    onDeselect: () => {},
  });
  const index = useMemo(
    () => createRenderIndex(document.nodes),
    [document.nodes],
  );
  const [renderedNodes, setRenderedNodes] = useState(() => ({
    index,
    viewport: renderViewport,
    size,
    selected,
    dragging: isNodeDragging,
    nodes: visibleCanvasNodes(
      index,
      renderViewport,
      size,
      selected,
      document.nodes,
    ),
  }));
  let visible = renderedNodes.nodes;
  if (
    renderedNodes.index !== index ||
    renderedNodes.viewport !== renderViewport ||
    renderedNodes.size !== size ||
    renderedNodes.selected !== selected ||
    renderedNodes.dragging !== isNodeDragging
  ) {
    // 在输入改变时调整组件状态，保留上一已渲染集合；不在 render 读写 ref 或 effect 派生视图。
    visible = visibleCanvasNodes(
      index,
      renderViewport,
      size,
      selected,
      document.nodes,
      {
        previouslyRenderedIds: new Set(
          renderedNodes.nodes.map((node) => node.id),
        ),
        interactingNodeIds: isNodeDragging ? selected : undefined,
      },
    );
    setRenderedNodes({
      index,
      viewport: renderViewport,
      size,
      selected,
      dragging: isNodeDragging,
      nodes: visible,
    });
  }
  const connectionIndex = useMemo(
    () => createConnectionRenderIndex(document.nodes, document.connections),
    [document.nodes, document.connections],
  );
  const visibleConnections = useMemo(
    () =>
      visibleCanvasConnections(connectionIndex, renderViewport, size, {
        selectedEdgeId: selectedEdge,
        interactingNodeIds: isNodeDragging ? selected : undefined,
      }),
    [
      connectionIndex,
      renderViewport,
      size,
      selectedEdge,
      isNodeDragging,
      selected,
    ],
  );
  useLayoutEffect(() => {
    const container = containerRef.current;
    if (container) return bindCanvasConnectionPreview(container, document);
  }, [document, visibleConnections]);
  const detail = canvasDetailLevel(renderViewport.k);
  const nodeById = useMemo(
    () => new Map(document.nodes.map((node) => [node.id, node])),
    [document.nodes],
  );
  const bounds = getCanvasNodesBounds(document.nodes) ?? {
    left: -1000,
    top: -1000,
    right: 1000,
    bottom: 1000,
  };
  function fit(selectionOnly = false) {
    if (locked) return;
    const candidates = document.nodes.filter(
      (node) =>
        !isNodeHiddenByCollapsedFrame(node, document.nodes) &&
        (!selectionOnly || selected.has(node.id)),
    );
    const bounds = getCanvasNodesBounds(candidates);
    if (bounds) transition.transitionTo(viewportForBounds(bounds, size));
  }
  async function copy() {
    const copied = copySelection(
      document.nodes,
      document.connections,
      selected,
    );
    if (!copied.nodes.length) return;
    try {
      await writeCanvasClipboard(copied, document.projectId);
      setNotice(`已复制 ${copied.nodes.length} 个节点到系统剪贴板。`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "复制未完成。");
    }
  }
  async function paste() {
    if (locked) return;
    try {
      if (navigator.clipboard?.read) {
        let items: ClipboardItems;
        try {
          items = await navigator.clipboard.read();
        } catch {
          throw new Error("读取系统剪贴板失败，请允许剪贴板权限后重试。");
        }
        if (!mounted.current || interactionLocked.current) return;
        const files: File[] = [];
        for (const item of items) {
          const mime = item.types.find((type) =>
            ["image/png", "image/jpeg", "image/webp"].includes(type),
          );
          if (mime)
            files.push(
              new File(
                [
                  await item.getType(mime).catch(() => {
                    throw new Error(
                      "读取剪贴板图片失败，请重新复制图片后重试。",
                    );
                  }),
                ],
                `粘贴图片-${Date.now()}.${mime === "image/jpeg" ? "jpg" : mime.split("/")[1]}`,
                { type: mime },
              ),
            );
        }
        if (files.length) {
          openUpload(files);
          return;
        }
      }
      const graph = await readCanvasClipboard(
        document.projectId,
        contextPoint.current ?? canvasCenter(),
      );
      if (!mounted.current || interactionLocked.current) return;
      insertClipboard(graph);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "粘贴未完成。");
    }
  }
  function insertClipboard(graph: ReturnType<typeof decodeCanvasClipboard>) {
    const copied = pasteSelection(graph);
    if (
      document.nodes.length + copied.nodes.length > 2000 ||
      document.connections.length + copied.connections.length > 4000
    ) {
      setNotice("粘贴会超过画布的 2,000 节点或 4,000 连接上限。");
      return;
    }
    submit([
      { type: "AddNodes", nodes: copied.nodes },
      ...(copied.connections.length
        ? [{ type: "Connect" as const, edges: copied.connections }]
        : []),
    ]);
  }
  function group() {
    const items = selectionRootNodes(document.nodes, selected);
    if (items.length < 2 || locked) return;
    const bounds = getCanvasNodesBounds(items)!;
    const node = createNode(CanvasNodeType.Frame, {
      x: bounds.left - 24,
      y: bounds.top - 60,
    });
    node.width = bounds.right - bounds.left + 48;
    node.height = bounds.bottom - bounds.top + 84;
    node.metadata = {
      frame: {
        collapsed: false,
        expandedWidth: node.width,
        expandedHeight: node.height,
      },
    };
    submit([
      { type: "AddNodes", nodes: [node] },
      {
        type: "SetNodeParents",
        parents: items.map((item) => ({ id: item.id, parent_id: node.id })),
      },
    ]);
  }
  function ungroup() {
    const groups = new Set(
      document.nodes
        .filter(
          (node) => selected.has(node.id) && node.type === CanvasNodeType.Frame,
        )
        .map((node) => node.id),
    );
    const children = document.nodes.filter(
      (node) =>
        node.parentId && (groups.has(node.parentId) || selected.has(node.id)),
    );
    if (!children.length || locked) return;
    submit([
      {
        type: "SetNodeParents",
        parents: children.map((node) => ({ id: node.id, parent_id: null })),
      },
    ]);
  }
  function remove() {
    if (locked) return;
    if (selectedEdge) submit([{ type: "Disconnect", ids: [selectedEdge] }]);
    else if (selected.size)
      submit([{ type: "DeleteNodes", ids: [...selected] }]);
  }
  function arrange(
    mode: CanvasLayoutMode | "auto" | "flow" | "spread" = "auto",
  ) {
    if (locked) return;
    const candidates = selected.size
      ? selectionRootNodes(document.nodes, selected)
      : document.nodes.filter((node) => !node.parentId);
    const positions =
      mode === "auto"
        ? layoutCanvasAuto(candidates, document.connections)
        : mode === "flow"
          ? layoutCanvasFlow(candidates, document.connections)
          : mode === "spread"
            ? spreadCanvasNodes(candidates)
            : layoutCanvasNodes(candidates, mode);
    submit(
      diffNodes(
        document.nodes,
        moveNodesWithChildren(document.nodes, positions),
      ),
    );
  }
  function align(mode: CanvasAlignmentMode) {
    const positions = alignCanvasNodes(
      selectionRootNodes(document.nodes, selected),
      mode,
    );
    if (positions.size)
      submit(
        diffNodes(
          document.nodes,
          moveNodesWithChildren(document.nodes, positions),
        ),
      );
  }
  function moveSelected(x: number, y: number) {
    if (locked) return;
    const ids = new Set(selected);
    selected.forEach((id) =>
      getFrameChildIds(id, document.nodes).forEach((child) => ids.add(child)),
    );
    submit([
      {
        type: "MoveNodes",
        moves: document.nodes
          .filter((node) => ids.has(node.id))
          .map((node) => ({
            id: node.id,
            x: node.position.x + x,
            y: node.position.y + y,
          })),
      },
    ]);
  }
  function changeLayer(front: boolean) {
    const indices = document.nodes.map((node) => node.zIndex);
    const start = front
      ? Math.max(0, ...indices) + 1
      : Math.min(0, ...indices) - selected.size;
    if (Math.abs(start) > 1e6 || Math.abs(start + selected.size) > 1e6) {
      const ordered = [...document.nodes].sort((a, b) => a.zIndex - b.zIndex),
        picked = ordered.filter((node) => selected.has(node.id)),
        rest = ordered.filter((node) => !selected.has(node.id));
      const rebased = front ? [...rest, ...picked] : [...picked, ...rest];
      submit([
        {
          type: "SetNodeZIndex",
          z_indices: rebased.map((node, index) => ({
            id: node.id,
            z_index: index,
          })),
        },
      ]);
      return;
    }
    submit([
      {
        type: "SetNodeZIndex",
        z_indices: [...selected].map((id, i) => ({ id, z_index: start + i })),
      },
    ]);
  }
  useCanvasKeyboard(
    {
      tool: setTool,
      save: () => {
        if (draft)
          submit(
            [
              {
                type: "UpdateNodeConfig",
                id: draft.id,
                config: { text: draft.text },
              },
            ],
            "edit",
            true,
          );
        else commitViewport(viewportRef.current);
      },
      undo: () => {
        const entry = store.getState().past.at(-1);
        if (entry) submit(entry.inverse, "undo");
      },
      redo: () => {
        const entry = store.getState().future.at(-1);
        if (entry) submit(entry.forward, "redo");
      },
      remove,
      copy,
      paste,
      selectAll: () =>
        setSelected(
          new Set(
            document.nodes
              .filter(
                (node) => !isNodeHiddenByCollapsedFrame(node, document.nodes),
              )
              .map((node) => node.id),
          ),
        ),
      fit: () => fit(),
      fitSelection: () => fit(true),
      zoom: (direction) => {
        if (!locked)
          transition.transitionTo(
            viewportAtScale(
              viewportRef.current,
              size,
              viewportRef.current.k * (direction > 0 ? 1.25 : 0.8),
            ),
          );
      },
      group,
      ungroup,
      arrange: () => arrange(),
      cancel: () => {
        connecting.current = undefined;
        setConnection(undefined);
        cancelSelectionBox();
      },
      search: openSearch,
      move: moveSelected,
      zoomReset: () => {
        if (!locked)
          transition.transitionTo(
            viewportAtScale(viewportRef.current, size, 1),
          );
      },
      toggleFocus: () => setFocusMode((value) => !value),
      showShortcuts: () => setShortcutsOpen(true),
    },
    !readOnly && !saving && !failed,
  );
  const connectHandlers = useRef({ screenToCanvas, submit, openCreation });
  useLayoutEffect(() => {
    connectHandlers.current = { screenToCanvas, submit, openCreation };
  });
  useEffect(() => {
    if (locked) {
      connecting.current = undefined;
    }
  }, [locked]);
  useEffect(() => {
    function pointerMove(event: PointerEvent) {
      const handle = connecting.current;
      if (!handle || connectionFrame.current !== null) return;
      const mouse = connectHandlers.current.screenToCanvas(
        event.clientX,
        event.clientY,
      );
      connectionFrame.current = requestAnimationFrame(() => {
        connectionFrame.current = null;
        if (connecting.current) setConnection({ handle, mouse });
      });
    }
    function finish(event: PointerEvent) {
      const handle = connecting.current;
      if (!handle) return;
      connecting.current = undefined;
      setConnection(undefined);
      const target = documentGlobal
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest<HTMLElement>("[data-node-handle]");
      const nodeId =
        target?.closest<HTMLElement>("[data-node-id]")?.dataset.nodeId;
      const kind = target?.dataset.nodeHandle;
      if (!nodeId) {
        const surface = documentGlobal.elementFromPoint(
          event.clientX,
          event.clientY,
        );
        if (
          surface?.closest("[data-canvas-viewport]") &&
          !surface.closest(
            "[data-node-id],[data-connection-id],[data-canvas-no-zoom]",
          )
        )
          connectHandlers.current.openCreation(
            event.clientX,
            event.clientY,
            handle,
          );
        return;
      }
      if (nodeId === handle.nodeId || kind === handle.handleType) return;
      connectHandlers.current.submit([
        {
          type: "Connect",
          edges: [
            {
              id: crypto.randomUUID(),
              fromNodeId:
                handle.handleType === "source" ? handle.nodeId : nodeId,
              toNodeId: handle.handleType === "source" ? nodeId : handle.nodeId,
            },
          ],
        },
      ]);
    }
    const documentGlobal = window.document;
    const cancel = () => {
      connecting.current = undefined;
      setConnection(undefined);
    };
    window.addEventListener("pointermove", pointerMove);
    window.addEventListener("pointerup", finish);
    window.addEventListener("pointercancel", cancel);
    window.addEventListener("blur", cancel);
    return () => {
      if (connectionFrame.current !== null)
        cancelAnimationFrame(connectionFrame.current);
      window.removeEventListener("pointermove", pointerMove);
      window.removeEventListener("pointerup", finish);
      window.removeEventListener("pointercancel", cancel);
      window.removeEventListener("blur", cancel);
    };
  }, []);
  const media = useInfiniteQuery({
    queryKey: ["canvas", "media-library", document.projectId],
    enabled: mediaOpen,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listMediaAssets(document.projectId, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const selectedNode =
    selectedIds.length === 1 ? nodeById.get(selectedIds[0]) : undefined;
  const selectedBounds = getCanvasNodesBounds(
    document.nodes.filter((node) => selected.has(node.id)),
  );
  const searchMatches = document.nodes
    .filter((node) =>
      `${node.title} ${node.metadata?.content ?? ""}`
        .toLowerCase()
        .includes(search.toLowerCase()),
    )
    .slice(0, 100);
  return (
    <section
      className="canvas-editor relative flex flex-col gap-4"
      aria-label="无限画布编辑器"
      onPaste={(event) => {
        const target = event.target instanceof Element ? event.target : null;
        if (
          locked ||
          target?.closest(
            "input,textarea,select,[contenteditable=true],[role=dialog],[role=menu]",
          )
        )
          return;
        const files = Array.from(event.clipboardData.files);
        if (files.length) {
          event.preventDefault();
          openUpload(files);
          return;
        }
        const text = event.clipboardData.getData("text/plain");
        if (!text) return;
        event.preventDefault();
        try {
          insertClipboard(
            decodeCanvasClipboard(text, document.projectId, canvasCenter()),
          );
        } catch (error) {
          setNotice(error instanceof Error ? error.message : "粘贴未完成。");
        }
      }}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p
          role="status"
          aria-live="polite"
          className="text-xs text-muted-foreground"
        >
          {notice} · {document.nodes.length} 节点 /{" "}
          {document.connections.length} 连接
        </p>
        <div className="flex gap-2">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="快捷键帮助"
            onClick={() => setShortcutsOpen(true)}
          >
            <Keyboard />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={focusMode ? "退出焦点模式" : "进入焦点模式"}
            onClick={() => setFocusMode(!focusMode)}
          >
            {focusMode ? <Minimize2 /> : <Expand />}
          </Button>
          <Button
            variant="ghost"
            disabled={saving}
            onClick={() => void readLatest()}
          >
            {dirty ? "放弃本页修改并读取最新" : "读取最新"}
          </Button>
        </div>
      </div>
      {readOnly && (
        <Alert role="status">
          <AlertTitle>只读画布</AlertTitle>
          <AlertDescription>此项目已归档，画布只可查看。</AlertDescription>
        </Alert>
      )}
      {failed && (
        <Alert variant="destructive">
          <AlertTitle>操作尚未确认保存</AlertTitle>
          <AlertDescription>
            <p>{failed.error.message}</p>
            {failed.error instanceof ApiError && failed.error.requestId && (
              <p className="text-xs">请求编号：{failed.error.requestId}</p>
            )}
            <p>重试复用同一幂等键；读取最新会放弃失败修改并清空撤销历史。</p>
            <div className="flex gap-2">
              {!(
                failed.error instanceof ApiError &&
                [401, 403, 409].includes(failed.error.status)
              ) && (
                <Button
                  disabled={saving}
                  onClick={() => void persist(failed.attempt)}
                >
                  重试保存
                </Button>
              )}
              <Button
                variant="secondary"
                disabled={saving}
                onClick={() => void readLatest()}
              >
                放弃并读取最新
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      )}
      <div
        className={cn(
          "grid gap-4",
          !focusMode && "xl:grid-cols-[minmax(0,1fr)_260px]",
        )}
      >
        <ContextMenu>
          <ContextMenuTrigger asChild>
            <div
              className={cn(
                "relative min-h-[480px] overflow-hidden rounded-2xl",
                focusMode ? "h-[85vh]" : "h-[min(70vh,780px)]",
              )}
              role="region"
              aria-label="画布编辑区"
            >
              <InfiniteCanvas
                containerRef={containerRef}
                viewport={viewport}
                interactive={!locked}
                backgroundMode={background}
                boxSelectEnabled={tool === "select"}
                onViewportPreviewChange={previewViewport}
                onViewportChange={commitViewport}
                onCanvasMouseDown={handleCanvasMouseDown}
                onCanvasDeselect={deselectCanvas}
                onCanvasDoubleClick={(event) =>
                  openCreation(event.clientX, event.clientY)
                }
                onDrop={(event) => {
                  event.preventDefault();
                  setFileDragOver(false);
                  const files = Array.from(event.dataTransfer.files);
                  if (files.length)
                    openUpload(
                      files,
                      screenToCanvas(event.clientX, event.clientY),
                    );
                }}
                onFileDragEnter={(event) => {
                  if (!locked && event.dataTransfer.types.includes("Files"))
                    setFileDragOver(true);
                }}
                onFileDragOver={(event) => {
                  event.dataTransfer.dropEffect = locked ? "none" : "copy";
                }}
                onFileDragLeave={(event) => {
                  if (
                    !event.currentTarget.contains(
                      event.relatedTarget as Node | null,
                    )
                  )
                    setFileDragOver(false);
                }}
                onContextMenu={(event) => {
                  const target =
                    event.target instanceof Element ? event.target : null;
                  if (
                    target?.closest(
                      "[data-canvas-no-zoom],input,textarea,[contenteditable=true]",
                    )
                  ) {
                    event.stopPropagation();
                    return;
                  }
                  contextPoint.current = screenToCanvas(
                    event.clientX,
                    event.clientY,
                  );
                  const id =
                    target?.closest<HTMLElement>("[data-node-id]")?.dataset
                      .nodeId;
                  if (!textDirty && id && !selected.has(id))
                    setSelected(new Set([id]));
                }}
              >
                <svg
                  className="absolute overflow-visible"
                  style={{
                    left: bounds.left - 200,
                    top: bounds.top - 200,
                    pointerEvents: "none",
                    zIndex: 1,
                  }}
                  width={bounds.right - bounds.left + 400}
                  height={bounds.bottom - bounds.top + 400}
                  viewBox={`${bounds.left - 200} ${bounds.top - 200} ${bounds.right - bounds.left + 400} ${bounds.bottom - bounds.top + 400}`}
                >
                  {visibleConnections.map(({ edge, from, to }) => (
                    <ConnectionPath
                      key={edge.id}
                      edge={edge}
                      from={from}
                      to={to}
                      selected={edge.id === selectedEdge}
                      onSelect={() => {
                        if (!textDirty) store.getState().selectEdge(edge.id);
                      }}
                    />
                  ))}
                  {!locked &&
                    connection &&
                    nodeById.has(connection.handle.nodeId) && (
                      <path
                        d={activeConnectionPath(
                          nodeById.get(connection.handle.nodeId)!,
                          connection.handle,
                          connection.mouse,
                        )}
                        fill="none"
                        stroke="var(--foreground)"
                        strokeWidth={2}
                        vectorEffect="non-scaling-stroke"
                      />
                    )}
                </svg>
                {visible.map((node) => (
                  <NodeShell
                    key={node.id}
                    node={node}
                    selected={selected.has(node.id)}
                    scale={viewport.k}
                    locked={locked}
                    overview={detail === "overview"}
                    onSelect={(id) => {
                      if (!textDirty && !selected.has(id))
                        setSelected(new Set([id]));
                    }}
                    onDrag={handleNodeMouseDown}
                    onResize={(id, width, height, position) =>
                      submit([
                        { type: "ResizeNodes", sizes: [{ id, width, height }] },
                        { type: "MoveNodes", moves: [{ id, ...position }] },
                      ])
                    }
                    onRename={(id, title) =>
                      submit([{ type: "RenameNodes", names: [{ id, title }] }])
                    }
                    onTitleDraftChange={titleDraftChanged}
                    onToggleGroup={(id) => {
                      const node = nodeById.get(id)!;
                      submit([
                        {
                          type: "UpdateNodeConfig",
                          id,
                          config: {
                            collapsed: !node.metadata?.frame?.collapsed,
                          },
                        },
                      ]);
                    }}
                    onConnectStart={(event, handle) => {
                      event.stopPropagation();
                      event.preventDefault();
                      connecting.current = handle;
                      setConnection({
                        handle,
                        mouse: screenToCanvas(event.clientX, event.clientY),
                      });
                    }}
                    onConnectTarget={() => {}}
                  >
                    <NodeContent
                      node={node}
                      projectId={document.projectId}
                      active={detail === "full" || selected.has(node.id)}
                      playable={
                        !previewNode &&
                        selectedIds.length === 1 &&
                        selected.has(node.id)
                      }
                      inMotion={isNodeDragging}
                      onPreview={() => setPreviewNode(node)}
                    />
                  </NodeShell>
                ))}
                {selectionBox && (
                  <div
                    data-canvas-selection-box
                    className="pointer-events-none absolute bg-foreground/10 outline-1 outline-foreground"
                    style={{ zIndex: 100000 }}
                  />
                )}
                {selectedBounds && selected.size > 0 && (
                  <div
                    ref={selectionBoundsElementRef}
                    data-canvas-selection-bounds
                    className={`pointer-events-none absolute ${selected.size > 1 ? "outline-1 outline-foreground/50 outline-dashed" : ""}`}
                    style={{
                      transform: `translate(${selectedBounds.left - 8}px,${selectedBounds.top - 8}px)`,
                      width: selectedBounds.right - selectedBounds.left + 16,
                      height: selectedBounds.bottom - selectedBounds.top + 16,
                      zIndex: 100000,
                    }}
                  />
                )}
                {alignmentGuides.vertical !== undefined && (
                  <div
                    className="pointer-events-none absolute w-px bg-foreground/40"
                    style={{
                      left: alignmentGuides.vertical,
                      top: bounds.top - 100,
                      height: bounds.bottom - bounds.top + 200,
                      zIndex: 100000,
                    }}
                  />
                )}
                {alignmentGuides.horizontal !== undefined && (
                  <div
                    className="pointer-events-none absolute h-px bg-foreground/40"
                    style={{
                      top: alignmentGuides.horizontal,
                      left: bounds.left - 100,
                      width: bounds.right - bounds.left + 200,
                      zIndex: 100000,
                    }}
                  />
                )}
              </InfiniteCanvas>
              {fileDragOver && (
                <div className="pointer-events-none absolute inset-3 flex items-center justify-center rounded-2xl bg-background/90 outline-2 outline-primary">
                  <p>松开文件，上传图片、视频或音频到当前画布</p>
                </div>
              )}
              {!locked && creationMenu && (
                <DropdownMenu
                  open
                  onOpenChange={(open) => {
                    if (!open) setCreationMenu(undefined);
                  }}
                >
                  <DropdownMenuTrigger asChild>
                    <button
                      aria-label="创建节点位置"
                      data-canvas-no-zoom
                      style={{
                        position: "fixed",
                        left: creationMenu.x,
                        top: creationMenu.y,
                        width: 1,
                        height: 1,
                      }}
                    />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent
                    onCloseAutoFocus={(event) => {
                      event.preventDefault();
                      containerRef.current?.focus();
                    }}
                  >
                    <DropdownMenuGroup>
                      <DropdownMenuItem
                        onSelect={() =>
                          createAt(
                            CanvasNodeType.Text,
                            creationMenu.world,
                            creationMenu.connection,
                          )
                        }
                      >
                        <Plus />
                        文字节点
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() =>
                          openMediaAt(
                            creationMenu.world,
                            creationMenu.connection,
                          )
                        }
                      >
                        <ImageIcon />
                        项目媒体
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() =>
                          openUpload(
                            [],
                            creationMenu.world,
                            creationMenu.connection,
                          )
                        }
                      >
                        <Upload />
                        上传本地媒体
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() =>
                          createAt(
                            CanvasNodeType.Frame,
                            creationMenu.world,
                            creationMenu.connection,
                          )
                        }
                      >
                        <Group />
                        空分组
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
              <div
                className="pointer-events-none absolute inset-x-4 bottom-6 z-30 flex justify-center"
                data-canvas-no-zoom
              >
                <CanvasDock
                  controls={
                    <CanvasToolMode
                      value={tool}
                      onValueChange={setTool}
                      disabled={locked}
                    />
                  }
                  commands={[
                    {
                      id: "text",
                      label: "文字",
                      icon: <Plus />,
                      disabled: locked || document.nodes.length >= 2000,
                      onClick: () => createAt(CanvasNodeType.Text),
                    },
                    {
                      id: "media",
                      label: "媒体库",
                      icon: <ImageIcon />,
                      disabled: locked,
                      onClick: () => openMediaAt(),
                    },
                    {
                      id: "undo",
                      label: "撤销",
                      icon: <Undo2 />,
                      disabled: locked || !past.length,
                      onClick: () => submit(past.at(-1)!.inverse, "undo"),
                    },
                    {
                      id: "upload",
                      label: "上传媒体",
                      icon: <Upload />,
                      disabled: locked || document.nodes.length >= 2000,
                      onClick: () => openUpload(),
                    },
                    {
                      id: "redo",
                      label: "重做",
                      icon: <Redo2 />,
                      disabled: locked || !future.length,
                      onClick: () => submit(future.at(-1)!.forward, "redo"),
                    },
                    {
                      id: "paste",
                      label: "粘贴",
                      icon: <Clipboard />,
                      disabled: locked,
                      onClick: paste,
                    },
                    {
                      id: "search",
                      label: "搜索",
                      icon: <Search />,
                      onClick: openSearch,
                    },
                  ]}
                />
              </div>
              {selectedBounds && selected.size > 0 && (
                <CanvasSelectionToolbar
                  anchorRef={selectionBoundsElementRef}
                  containerRef={containerRef}
                  count={selected.size}
                >
                  <CanvasDock
                    label="节点布局工具"
                    commands={[
                      {
                        id: "copy",
                        label: "复制",
                        icon: <Copy />,
                        onClick: copy,
                      },
                      {
                        id: "group",
                        label: "分组",
                        icon: <Group />,
                        disabled:
                          locked ||
                          selectionRootNodes(document.nodes, selected).length <
                            2,
                        onClick: group,
                      },
                      {
                        id: "ungroup",
                        label: "解组",
                        icon: <Ungroup />,
                        disabled: locked,
                        onClick: ungroup,
                      },
                      {
                        id: "align",
                        label: "左对齐",
                        icon: <AlignStartHorizontal />,
                        disabled: locked || selected.size < 2,
                        onClick: () => align("left"),
                      },
                      {
                        id: "arrange",
                        label: "自动布局所选节点",
                        icon: <LayoutGrid />,
                        disabled: locked || selected.size < 2,
                        onClick: () => arrange(),
                      },
                      {
                        id: "delete",
                        label: "删除",
                        icon: <Trash2 />,
                        disabled: locked,
                        danger: true,
                        onClick: remove,
                      },
                    ]}
                  />
                </CanvasSelectionToolbar>
              )}
              <div
                className="absolute bottom-4 left-4 flex items-center gap-1 rounded-xl bg-background/95 p-1 shadow-sm max-md:top-4 max-md:bottom-auto"
                data-canvas-no-zoom
              >
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="缩小画布"
                  disabled={locked}
                  onClick={() =>
                    transition.transitionTo(
                      viewportAtScale(
                        viewportRef.current,
                        size,
                        viewportRef.current.k * 0.8,
                      ),
                    )
                  }
                >
                  <Minus />
                </Button>
                <span className="w-12 text-center text-xs tabular-nums">
                  {Math.round(renderViewport.k * 100)}%
                </span>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="放大画布"
                  disabled={locked}
                  onClick={() =>
                    transition.transitionTo(
                      viewportAtScale(
                        viewportRef.current,
                        size,
                        viewportRef.current.k * 1.25,
                      ),
                    )
                  }
                >
                  <Plus />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="显示全部节点"
                  disabled={locked}
                  onClick={() => fit()}
                >
                  <Maximize2 />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={locked}
                  aria-label="缩放到百分之百"
                  onClick={() =>
                    transition.transitionTo(
                      viewportAtScale(viewportRef.current, size, 1),
                    )
                  }
                >
                  100%
                </Button>
              </div>
              <div
                className="absolute top-4 right-4 flex gap-2"
                data-canvas-no-zoom
              >
                <Select
                  value={background}
                  onValueChange={(mode) =>
                    setBackground(mode as CanvasBackgroundMode)
                  }
                >
                  <SelectTrigger aria-label="画布背景">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="dots">点阵</SelectItem>
                      <SelectItem value="lines">网格</SelectItem>
                      <SelectItem value="blank">纯色</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  variant="secondary"
                  size="sm"
                  aria-pressed={minimapVisible}
                  onClick={() => setMinimapVisible(!minimapVisible)}
                >
                  小地图
                </Button>
                {focusMode && (
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setFocusMode(false)}
                  >
                    退出焦点模式
                  </Button>
                )}
              </div>
              {minimapVisible && (
                <div className="absolute right-4 bottom-4 hidden md:block">
                  <Minimap
                    nodes={document.nodes}
                    viewport={viewport}
                    size={size}
                    container={containerRef}
                    onPreview={previewViewport}
                    onCommit={commitViewport}
                    disabled={locked}
                  />
                </div>
              )}
              {!document.nodes.length && (
                <p className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-muted-foreground">
                  添加文字，或从媒体库选择图片、视频和音频。
                </p>
              )}
            </div>
          </ContextMenuTrigger>
          <ContextMenuContent
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              containerRef.current?.focus();
            }}
          >
            <ContextMenuGroup>
              <ContextMenuItem
                disabled={locked || document.nodes.length >= 2000}
                onSelect={() =>
                  createAt(CanvasNodeType.Text, contextPoint.current)
                }
              >
                <Plus />
                添加文字
              </ContextMenuItem>
              <ContextMenuItem
                disabled={locked}
                onSelect={() => openMediaAt(contextPoint.current)}
              >
                <ImageIcon />
                从媒体库添加
              </ContextMenuItem>
              <ContextMenuItem
                disabled={locked || document.nodes.length >= 2000}
                onSelect={() => openUpload([], contextPoint.current)}
              >
                <Upload />
                上传本地媒体
              </ContextMenuItem>
              <ContextMenuItem
                disabled={locked || document.nodes.length >= 2000}
                onSelect={() =>
                  createAt(CanvasNodeType.Frame, contextPoint.current)
                }
              >
                <Group />
                添加空分组
              </ContextMenuItem>
              <ContextMenuItem disabled={locked} onSelect={paste}>
                <Clipboard />
                粘贴
              </ContextMenuItem>
            </ContextMenuGroup>
            <ContextMenuSeparator />
            <ContextMenuGroup>
              <ContextMenuItem disabled={!selected.size} onSelect={copy}>
                <Copy />
                复制节点
              </ContextMenuItem>
              <ContextMenuItem
                disabled={locked || selected.size < 2}
                onSelect={group}
              >
                <Group />
                组成分组
              </ContextMenuItem>
              <ContextMenuItem
                disabled={locked || !selected.size}
                onSelect={ungroup}
              >
                <Ungroup />
                移出分组
              </ContextMenuItem>
              <ContextMenuSub>
                <ContextMenuSubTrigger
                  disabled={locked || document.nodes.length < 2}
                >
                  排列节点
                </ContextMenuSubTrigger>
                <ContextMenuSubContent>
                  <ContextMenuGroup>
                    {(
                      [
                        ["auto", "自动整理"],
                        ["row", "水平排列"],
                        ["column", "垂直排列"],
                        ["grid", "网格排列"],
                        ["flow", "关系布局"],
                        ["spread", "拉开间距"],
                      ] as const
                    ).map(([mode, label]) => (
                      <ContextMenuItem
                        key={mode}
                        onSelect={() => arrange(mode)}
                      >
                        {label}
                      </ContextMenuItem>
                    ))}
                  </ContextMenuGroup>
                </ContextMenuSubContent>
              </ContextMenuSub>
              <ContextMenuItem
                disabled={locked || !selected.size}
                onSelect={() => changeLayer(true)}
              >
                移到最前
              </ContextMenuItem>
              <ContextMenuItem
                disabled={locked || !selected.size}
                onSelect={() => changeLayer(false)}
              >
                移到最后
              </ContextMenuItem>
              <ContextMenuItem
                variant="destructive"
                disabled={locked || (!selected.size && !selectedEdge)}
                onSelect={remove}
              >
                <Trash2 />
                删除所选
              </ContextMenuItem>
            </ContextMenuGroup>
          </ContextMenuContent>
        </ContextMenu>
        {!focusMode && (
          <aside
            className="space-y-5 rounded-2xl bg-muted/30 p-5"
            aria-label="节点属性"
          >
            {selectedNode ? (
              <>
                <p className="font-medium">{selectedNode.title}</p>
                <p className="text-xs text-muted-foreground">
                  {Math.round(selectedNode.width)} ×{" "}
                  {Math.round(selectedNode.height)} · (
                  {Math.round(selectedNode.position.x)},{" "}
                  {Math.round(selectedNode.position.y)})
                </p>
                {selectedNode.assetId && (
                  <Button
                    variant="secondary"
                    onClick={() => setPreviewNode(selectedNode)}
                  >
                    <Expand data-icon="inline-start" />
                    预览素材
                  </Button>
                )}
                {selectedNode.type === CanvasNodeType.Text && (
                  <>
                    <Field>
                      <FieldLabel htmlFor="canvas-text">文字内容</FieldLabel>
                      <Textarea
                        id="canvas-text"
                        value={
                          draft?.id === selectedNode.id
                            ? draft.text
                            : (selectedNode.metadata?.content ?? "")
                        }
                        disabled={readOnly || saving || Boolean(failed)}
                        onChange={(event) =>
                          store
                            .getState()
                            .editText(selectedNode.id, event.target.value)
                        }
                        className="min-h-48"
                      />
                    </Field>
                    <p className="text-xs text-muted-foreground">
                      {
                        Array.from(
                          draft?.id === selectedNode.id
                            ? draft.text
                            : (selectedNode.metadata?.content ?? ""),
                        ).length
                      }{" "}
                      / 10,000 字符
                    </p>
                    <Button
                      disabled={
                        !textDirty ||
                        saving ||
                        readOnly ||
                        Boolean(failed) ||
                        Array.from(draft?.text ?? "").length > 10000
                      }
                      onClick={() => {
                        if (draft)
                          submit(
                            [
                              {
                                type: "UpdateNodeConfig",
                                id: draft.id,
                                config: { text: draft.text },
                              },
                            ],
                            "edit",
                            true,
                          );
                      }}
                    >
                      保存文字
                    </Button>
                    {textDirty && (
                      <Button
                        variant="ghost"
                        disabled={saving}
                        onClick={() => store.getState().discardText()}
                      >
                        放弃文字草稿
                      </Button>
                    )}
                  </>
                )}
              </>
            ) : (
              <p className="text-sm leading-7 text-muted-foreground">
                {selectedEdge
                  ? "已选中注释连接，可删除或使用撤销恢复。"
                  : `已选 ${selected.size} 个节点。鼠标框选，Space + 拖动平移；连接点从左至右表达关系。`}
              </p>
            )}
            <div className="space-y-2">
              <Select
                disabled={locked || selected.size < 2}
                onValueChange={(value) => align(value as CanvasAlignmentMode)}
              >
                <SelectTrigger aria-label="对齐选中节点">
                  <SelectValue placeholder="对齐与均分" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {[
                      ["left", "左对齐"],
                      ["centerX", "水平居中"],
                      ["right", "右对齐"],
                      ["top", "顶对齐"],
                      ["centerY", "垂直居中"],
                      ["bottom", "底对齐"],
                      ["distributeX", "水平均分"],
                      ["distributeY", "垂直均分"],
                    ].map(([value, label]) => (
                      <SelectItem key={value} value={value}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <Select
                disabled={locked || document.nodes.length < 2}
                onValueChange={(mode) =>
                  arrange(mode as CanvasLayoutMode | "auto" | "flow" | "spread")
                }
              >
                <SelectTrigger aria-label="排列节点">
                  <SelectValue placeholder="排列与布局" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {(
                      [
                        ["auto", "自动整理"],
                        ["row", "水平排列"],
                        ["column", "垂直排列"],
                        ["grid", "网格排列"],
                        ["flow", "关系布局"],
                        ["spread", "拉开间距"],
                      ] as const
                    ).map(([value, label]) => (
                      <SelectItem key={value} value={value}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <div className="flex gap-2">
                <Button
                  variant="ghost"
                  disabled={locked || !selected.size}
                  onClick={() => changeLayer(true)}
                >
                  移到最前
                </Button>
                <Button
                  variant="ghost"
                  disabled={locked || !selected.size}
                  onClick={() => changeLayer(false)}
                >
                  移到最后
                </Button>
              </div>
            </div>
            <p className="text-xs leading-6 text-muted-foreground">
              ⌘/Ctrl Z 撤销 · C/V 复制粘贴 · G 分组 · Shift G 解组 · F 搜索 · 0
              全览。媒体只引用当前项目授权的真实资产。
            </p>
          </aside>
        )}
      </div>
      <CanvasShortcutsDialog
        open={shortcutsOpen}
        onOpenChange={setShortcutsOpen}
      />
      <MediaUploadDialog
        open={uploadOpen}
        onOpenChange={(open) => {
          setUploadOpen(open);
          if (!open) setUploadFiles([]);
        }}
        initialFiles={uploadFiles}
        remainingSlots={2000 - document.nodes.length}
        upload={uploadMedia}
        onImported={addUploadedMedia}
        onBusyChange={setImporting}
        disabled={textDirty}
        readOnly={readOnly}
      />
      {previewNode && (
        <MediaPreviewDialog
          node={previewNode}
          projectId={document.projectId}
          onClose={() => setPreviewNode(undefined)}
        />
      )}
      <Dialog open={searchOpen} onOpenChange={setSearchOpen}>
        <DialogContent
          onCloseAutoFocus={(event) => {
            if (searchOpener.current?.isConnected) {
              event.preventDefault();
              searchOpener.current.focus();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>搜索节点</DialogTitle>
            <DialogDescription>按节点名称或文字内容定位。</DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor="canvas-node-search">节点名称或文字</FieldLabel>
            <Input
              id="canvas-node-search"
              aria-label="搜索节点名称或文字"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </Field>
          {!searchMatches.length && (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>没有匹配的节点</EmptyTitle>
                <EmptyDescription>尝试其他名称或文字内容。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
          <div className="flex max-h-80 flex-col gap-1 overflow-auto">
            {searchMatches.map((node) => (
              <Button
                className="w-full justify-start"
                variant="ghost"
                key={node.id}
                onClick={() => {
                  if (textDirty) return;
                  setSelected(new Set([node.id]));
                  setSearchOpen(false);
                  transition.transitionTo(
                    viewportForBounds(
                      {
                        left: node.position.x,
                        top: node.position.y,
                        right: node.position.x + node.width,
                        bottom: node.position.y + node.height,
                      },
                      size,
                      { maxScale: 1 },
                    ),
                  );
                }}
              >
                {node.title}
              </Button>
            ))}
          </div>
        </DialogContent>
      </Dialog>
      <Dialog open={mediaOpen} onOpenChange={setMediaOpen}>
        <DialogContent
          onCloseAutoFocus={(event) => {
            if (uploadOpen) {
              event.preventDefault();
              return;
            }
            if (mediaOpener.current?.isConnected) {
              event.preventDefault();
              mediaOpener.current.focus();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>项目媒体库</DialogTitle>
            <DialogDescription>
              选择当前项目已就绪的资产，作为画布引用。
            </DialogDescription>
          </DialogHeader>
          <Button
            variant="secondary"
            disabled={locked || document.nodes.length >= 2000}
            onClick={() => {
              setMediaOpen(false);
              openUpload(
                [],
                mediaTarget.current?.point,
                mediaTarget.current?.connection,
              );
            }}
          >
            <Upload data-icon="inline-start" />
            上传本地媒体
          </Button>
          {media.isPending && (
            <div
              role="status"
              aria-label="正在读取媒体库"
              className="flex flex-col gap-2"
            >
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <span className="sr-only">正在读取媒体库…</span>
            </div>
          )}
          {media.error && (
            <Alert variant="destructive">
              <AlertTitle>媒体库读取失败</AlertTitle>
              <AlertDescription>
                <p>{media.error.message}</p>
                <Button
                  variant="secondary"
                  onClick={() => void media.refetch()}
                >
                  重新读取
                </Button>
              </AlertDescription>
            </Alert>
          )}
          <div className="flex max-h-96 flex-col gap-2 overflow-auto">
            {media.data?.pages
              .flatMap((page) => page.items)
              .map((asset) => (
                <Button
                  variant="ghost"
                  key={asset.id}
                  className="w-full justify-start"
                  disabled={locked}
                  onClick={() => {
                    const target = mediaTarget.current;
                    const node = createNode(
                      asset.kind as CanvasNodeType,
                      target?.point ?? canvasCenter(),
                      undefined,
                      { assetId: asset.id },
                    );
                    node.title = Array.from(asset.file_name || "媒体素材")
                      .slice(0, 128)
                      .join("");
                    if (asset.width && asset.height) {
                      node.width = 320;
                      node.height = Math.max(
                        40,
                        Math.min(2000, (320 * asset.height) / asset.width),
                      );
                    }
                    submit(nodeCreationCommands(node, target?.connection));
                    mediaTarget.current = undefined;
                    setMediaOpen(false);
                  }}
                >
                  {asset.file_name} · {asset.kind}
                </Button>
              ))}
          </div>
          {!media.error &&
            media.data &&
            !media.data.pages.some((page) => page.items.length) && (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>此项目暂无可用媒体</EmptyTitle>
                  <EmptyDescription>
                    点击上传媒体，将本机图片、视频和音频加入此项目。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
          {media.hasNextPage && (
            <Button
              disabled={media.isFetchingNextPage}
              onClick={() => void media.fetchNextPage()}
            >
              加载更多媒体
            </Button>
          )}
        </DialogContent>
      </Dialog>
    </section>
  );
}
