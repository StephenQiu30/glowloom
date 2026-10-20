"use client";

import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useProjectPresentation } from "@/components/projects/project-presentation";
import { publishProjectInvalidation } from "@/components/projects/project-presentation-store";
import { useSpace } from "@/components/spaces/space-context";

import {
  useCallback,
  useEffectEvent,
  useEffect,
  useRef,
  useState,
  type PointerEvent,
} from "react";
import Link from "next/link";
import dynamic from "next/dynamic";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyContent,
  EmptyTitle,
} from "@/components/ui/empty";
import { UploadDialog } from "@/components/media/upload-dialog";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { subjectText, type Subject } from "@/components/subjects/contracts";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  GenerationDialog,
  type GenerationSeed,
} from "@/components/generation/generation-dialog";
import type { PromptDraft } from "@/components/generation/prompt-document";
import { getSession } from "@/api/identity";
import { canonical } from "@/components/generation/draft-save";
import {
  RequestError,
  getSessionGeneration,
  getScopeGeneration,
} from "@/lib/request";
import { getMediaPreview } from "@/api/media";
import { applyCanvasCommands, getCanvas } from "@/api/canvases";
import type { Project } from "@/components/projects/project";
import {
  commandBatch,
  richCanvasDraft,
  createDraftNode,
  createMediaNode,
  duplicateNode,
  isDraft,
  isEditableNode,
  isMediaNode,
  requireCanvas,
  requireSaveResult,
  sameEditable,
  createResourceNode,
  removeCanvasNode,
  defaultViewport,
  isAnnotation,
  type Canvas,
  type Node,
} from "./document";

import { RichGeneration } from "@/components/generation/rich-generation";
import { CanvasNodeLayer } from "./node/layer";
import { placeNode } from "./node/placement";
import { createHistory } from "./history/history";
import { descendants, moveSelection } from "./viewport/render-model";
import { groupWrapRect } from "./group/geometry";
import { reconcileGroupDrop } from "./group/membership";
import { isCanvasShortcut } from "./chrome/shortcut";
import { CanvasSelectionToolbar } from "./chrome/selection-toolbar";
import { CanvasMediaActions } from "./image/actions";
import type { ImageOperation } from "./image/operations";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ViewportSurface } from "./viewport/surface";
import { CanvasNode } from "./node/node";
import type { ConnectionDrop } from "./node/connection-port";
import { CanvasHeader } from "./chrome/header";
import { CanvasContextMenu } from "./chrome/context-menu";
import {
  encodeCanvasClipboard,
  pasteCanvasClipboard,
  remapCopiedNodeReferences,
} from "./history/clipboard";
import { createDirectorConfig } from "./director/model";
import { fitNodes, zoomAt } from "./viewport/frame";
import { canvasWorkspaceRect } from "./viewport/workspace-rect";
import { CanvasToolbar } from "./chrome/toolbar";
import { CanvasMiniMap } from "./viewport/minimap";
import {
  CANVAS_NODE_DRAG_EVENT,
  type SelectionArea,
  type ViewportTransform,
} from "./viewport/transform";
import { CanvasNodePanel } from "./node/panel";
import {
  adjacentGraph,
  nodePromptDraft,
  referenceEdges,
  referenceSources,
  connectedReferenceSources,
  materializeReferences,
} from "./composer/references";
import { CanvasLive, type LiveStatus } from "./live";
import "./editor.css";

type Pending = {
  key: string;
  base: Canvas;
  nodes: Node[];
  body: API.CommandsInput;
  edges?: API.Edge[];
};
function nodePreviewText(node: Node): string {
  if (node.node_type === "text") return node.config.text ?? "";
  if (!isDraft(node)) return "";
  const draft = node.config.draft;
  return draft.composer
    ? draft.composer.document.content
        .map((paragraph) =>
          (paragraph.content ?? [])
            .map((part) =>
              part.type === "text"
                ? part.text
                : part.type === "hardBreak"
                  ? "\n"
                  : "[引用]",
            )
            .join(""),
        )
        .join("\n")
    : [draft.prompt, draft.negative_prompt].filter(Boolean).join("\n");
}
const limit = (value: number) => Math.max(-1e6, Math.min(1e6, value));
const CanvasMaterialDialog = dynamic(
  () =>
    import("@/components/media/project-asset-dialog").then(
      (module) => module.ProjectAssetDialog,
    ),
  { loading: () => null },
);
const CanvasDirector = dynamic(
  () => import("./director/director").then((m) => m.CanvasDirector),
  { ssr: false },
);
const CanvasAIPanel = dynamic(
  () => import("./composer/ai-panel").then((m) => m.CanvasAIPanel),
  { loading: () => null },
);
const CanvasImageDialog = dynamic(
  () => import("./image/dialog").then((module) => module.CanvasImageDialog),
  { loading: () => null },
);

export function CanvasEditor({
  project: initialProject,
  initial,
  developmentPreview = false,
}: {
  project: Project;
  initial: Canvas;
  developmentPreview?: boolean;
}) {
  const presentation = useProjectPresentation(initialProject);
  const project = presentation.project,
    creationSpace = useSpace();
  const [graphReadOnlyReason, setGraphReadOnlyReason] = useState(
    initial.read_only_reason ?? "",
  );
  const [identityPending, setIdentityPending] = useState(false);
  const [liveStatus, setLiveStatus] = useState<LiveStatus>(
    developmentPreview ? "live" : "connecting",
  );
  const [socketWritable, setSocketWritable] = useState(developmentPreview);
  const [syncBlocked, setSyncBlocked] = useState(false);
  const live = useRef<CanvasLive | null>(null);
  const deferredSnapshot = useRef<{
    document: unknown;
    writable: boolean;
  } | null>(null);
  const verifyIdentity = useRef<() => void>(() => {});
  const readOnly =
    liveStatus !== "live" ||
    !socketWritable ||
    syncBlocked ||
    identityPending ||
    !!graphReadOnlyReason ||
    project.status !== "active" ||
    !!presentation.error ||
    creationSpace.selected?.writable === false;
  const readOnlyRef = useRef(readOnly);
  useEffect(() => {
    readOnlyRef.current = readOnly;
  }, [readOnly]);
  const [aiOpen, setAIOpen] = useState(false);
  const [aiWidth, setAIWidth] = useState(400);
  const aiResize = useRef<{ x: number; width: number } | null>(null);
  const [openToolID, setOpenToolID] = useState<string>();
  const [editingTextID, setEditingTextID] = useState<string>();
  const [textDrafts, setTextDrafts] = useState(new Map<string, string>());
  const [generation, setGeneration] = useState<GenerationSeed | null>(null);
  const [materials, setMaterials] = useState<{
    tab: "assets" | "subjects";
    kind?: string;
  } | null>(null);
  const [upload, setUpload] = useState(false);
  const [imageTool, setImageTool] = useState<{
    node: Node;
    kind: ImageOperation["kind"];
  } | null>(null);
  const [preview, setPreview] = useState<Node | null>(null);
  const [base, setBaseState] = useState(initial);
  const savedBase = useRef(initial);
  const setBase = useCallback((value: Canvas) => {
    savedBase.current = value;
    setGraphReadOnlyReason(value.read_only_reason ?? "");
    setBaseState(value);
  }, []);
  const privateInvalid = useRef(false);
  const [identityInvalid, setIdentityInvalid] = useState(false);
  const [nodes, setNodes] = useState(initial.nodes);
  const local = useRef(initial.nodes);
  const [edges, setEdges] = useState(initial.edges);
  const localEdges = useRef(initial.edges);
  const [history] = useState(createHistory);
  const [selected, setSelected] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [autoSavePaused, setAutoSavePaused] = useState(false);
  const [unconfirmed, setUnconfirmed] = useState(false);
  const [message, setMessage] = useState("");
  const [conflict, setConflict] = useState<{ server: Canvas | null } | null>(
    null,
  );
  const [leave, setLeave] = useState(false);
  const leaving = useRef(false);
  const pending = useRef<Pending | null>(null);
  const busy = useRef(false);
  const alive = useRef(false);
  const request = useRef<AbortController | null>(null);
  const board = useRef<HTMLDivElement>(null);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [showEdges, setShowEdges] = useState(true);
  const contextPoint = useRef<{ x: number; y: number } | null>(null);
  const [tool, setTool] = useState<"select" | "pan">("select");
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
  const [renaming, setRenaming] = useState<{
    id: string;
    title: string;
  } | null>(null);
  const [miniMap, setMiniMap] = useState(false);
  const [size, setSize] = useState({ width: 1024, height: 768 });
  const [camera, setCamera] = useState(initial.viewport ?? defaultViewport);
  const view = useRef(camera);
  const drag = useRef<{
    id: string;
    clientX: number;
    clientY: number;
    x: number;
    y: number;
    nextX: number;
    nextY: number;
    card: HTMLElement;
    button: HTMLElement;
    pointerId: number;
    members: {
      id: string;
      x: number;
      y: number;
      element: HTMLElement | null;
    }[];
  } | null>(null);
  const baseline = new Map(base.nodes.map((n) => [n.id, n]));
  const dirty =
    canonical(edges) !== canonical(base.edges) ||
    nodes.length !== base.nodes.length ||
    nodes.some((node) => {
      const previous = baseline.get(node.id);
      if (!previous) return true;
      // A protected snapshot must not go through current-version config normalization.
      return graphReadOnlyReason
        ? canonical(node) !== canonical(previous)
        : !sameEditable(node, previous);
    });
  const current = nodes.find((n) => n.id === selected);
  const effectiveAIWidth = Math.min(aiWidth, Math.max(240, size.width - 84));
  const workspace = canvasWorkspaceRect(
    size.width,
    size.height,
    aiOpen ? effectiveAIWidth : 0,
  );
  const receiveSnapshot = useCallback(
    (document: unknown, writable: boolean) => {
      if (privateInvalid.current) return;
      const server = requireCanvas(document, initial.project_id, initial.id);
      setSocketWritable(writable);
      if (busy.current || pending.current) {
        deferredSnapshot.current = { document, writable };
        return;
      }
      if (server.revision <= savedBase.current.revision) return;
      const prior = new Map(
        savedBase.current.nodes.map((node) => [node.id, node]),
      );
      const changed =
        canonical(localEdges.current) !== canonical(savedBase.current.edges) ||
        local.current.length !== prior.size ||
        local.current.some((node) => {
          const old = prior.get(node.id);
          return (
            !old ||
            (savedBase.current.read_only_reason
              ? canonical(node) !== canonical(old)
              : !sameEditable(node, old))
          );
        });
      if (changed) {
        readOnlyRef.current = true;
        setSyncBlocked(true);
        setConflict({ server });
        setMessage("画布已有新版本，本地编辑已保留。请对照后明确选择。");
      } else {
        history.reset();
        local.current = server.nodes;
        localEdges.current = server.edges;
        setNodes(server.nodes);
        setEdges(server.edges);
        setBase(server);
      }
    },
    [history, initial.id, initial.project_id, setBase],
  );
  useEffect(() => {
    if (developmentPreview) return;
    const socket = new CanvasLive(initial.id, {
      snapshot: (document, writable) => receiveSnapshot(document, writable),
      status: (status, writable) => {
        setLiveStatus(status);
        if (writable !== undefined) setSocketWritable(writable);
      },
      revoked: () =>
        window.dispatchEvent(new Event("glowloom:private-cache-cleared")),
    });
    live.current = socket;
    socket.start();
    return () => {
      live.current = null;
      socket.stop();
    };
  }, [initial.id, developmentPreview, receiveSnapshot]);
  useEffect(() => {
    const suspend = () => {
      live.current?.stop();
      privateInvalid.current = true;
      request.current?.abort();
      setIdentityInvalid(true);
      pending.current = null;
      local.current = [];
      localEdges.current = [];
      savedBase.current = { ...savedBase.current, nodes: [], edges: [] };
      setNodes([]);
      setEdges([]);
      setBaseState(savedBase.current);
      history.reset();
      setGeneration(null);
      setPreview(null);
      setImageTool(null);
      setMaterials(null);
      setRenaming(null);
      setConflict(null);
      setOpenToolID(undefined);
    };
    const channel =
      typeof BroadcastChannel !== "undefined"
        ? new BroadcastChannel("glowloom-private-cache")
        : null;
    if (channel) channel.onmessage = suspend;
    window.addEventListener("glowloom:private-cache-cleared", suspend);
    let owner: string | undefined;
    let active = true;
    const verify = () => {
      if (document.visibilityState === "visible") {
        void getSession()
          .then((identity) => {
            if (!active) return;
            const actor = identity.actor?.id;
            if (!actor || (owner && owner !== actor)) suspend();
            else {
              owner = actor;
              setIdentityPending(false);
            }
          })
          .catch((error: unknown) => {
            if (!active) return;
            if (
              error instanceof RequestError &&
              [401, 403].includes(error.status)
            ) {
              suspend();
            } else {
              setIdentityPending(true);
            }
          });
      }
    };
    verifyIdentity.current = verify;
    verify();
    document.addEventListener("visibilitychange", verify);
    return () => {
      active = false;
      channel?.close();
      window.removeEventListener("glowloom:private-cache-cleared", suspend);
      document.removeEventListener("visibilitychange", verify);
    };
  }, [history]);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      request.current?.abort();
    };
  }, []);
  useEffect(() => {
    if (!dirty && !saving && !unconfirmed) return;
    const warn = (e: BeforeUnloadEvent) => {
      if (leaving.current) return;
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty, saving, unconfirmed]);

  useEffect(() => {
    const el = board.current;
    if (!el) return;
    const measure = () =>
      setSize({
        width: el.clientWidth || 1024,
        height: el.clientHeight || 768,
      });
    measure();
    const observer =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(measure);
    observer?.observe(el);
    window.addEventListener("resize", measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);
  const viewportChange = useCallback((next: ViewportTransform) => {
    setAutoSavePaused(false);
    view.current = { x: next.x, y: next.y, zoom: next.k };
    setCamera(view.current);
  }, []);
  const viewportPreview = useCallback((next: ViewportTransform) => {
    view.current = { x: next.x, y: next.y, zoom: next.k };
  }, []);
  function changeCamera(next: typeof camera) {
    view.current = next;
    setCamera(next);
  }
  function selectNode(id?: string, additive = false) {
    setEditingTextID(undefined);
    const target = local.current.find((n) => n.id === id);
    if (target && !additive) {
      const v = view.current;
      if (
        (target.x + (target.width ?? 288)) * v.zoom + v.x < 0 ||
        target.x * v.zoom + v.x > size.width ||
        (target.y + (target.height ?? 200)) * v.zoom + v.y < 64 ||
        target.y * v.zoom + v.y > size.height
      ) {
        changeCamera({
          ...v,
          x:
            workspace.centerX - (target.x + (target.width ?? 288) / 2) * v.zoom,
          y:
            workspace.centerY -
            (target.y + (target.height ?? 200) / 2) * v.zoom,
        });
      }
    }
    setSelected(id);
    setSelectedIDs((ids) =>
      additive && id
        ? ids.includes(id)
          ? ids.filter((value) => value !== id)
          : [...ids, id]
        : id
          ? [id]
          : [],
    );
  }
  function selectArea(area: SelectionArea) {
    const ids = local.current
      .filter(
        (node) =>
          node.x <= area.x + area.width &&
          node.x + (node.width ?? 288) >= area.x &&
          node.y <= area.y + area.height &&
          node.y + (node.height ?? 200) >= area.y,
      )
      .map((node) => node.id);
    setSelectedIDs(ids);
    setSelected(ids.at(-1));
  }
  function edit(
    next: Node[],
    key?: string,
    nextEdges = localEdges.current,
    record = true,
  ) {
    if (privateInvalid.current || (record && readOnlyRef.current)) return;
    if (record) setAutoSavePaused(false);
    if (record)
      history.record(
        { nodes: local.current, edges: localEdges.current },
        { nodes: next, edges: nextEdges },
        key,
      );
    local.current = next;
    localEdges.current = nextEdges;
    setNodes(next);
    setEdges(nextEdges);
  }
  function update(id: string, patch: Partial<Node>) {
    edit(
      local.current.map((n) => (n.id === id ? { ...n, ...patch } : n)),
      `${id}/${Object.keys(patch).join()}`,
    );
  }
  function stepHistory(direction: "undo" | "redo") {
    if (
      readOnlyRef.current ||
      busy.current ||
      pending.current ||
      conflict ||
      privateInvalid.current
    )
      return;
    const graph = history[direction](savedBase.current);
    if (graph) {
      edit(graph.nodes, undefined, graph.edges, false);
      if (!graph.nodes.some((n) => n.id === selected)) selectNode(undefined);
    }
  }
  const historyShortcut = useEffectEvent((direction: "undo" | "redo") =>
    stepHistory(direction),
  );
  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if (
        !(event.metaKey || event.ctrlKey) ||
        event.altKey ||
        event.defaultPrevented
      )
        return;
      const target = event.target instanceof Element ? event.target : null;
      if (
        target?.closest(
          "input,textarea,select,[contenteditable=true],[role=dialog],[role=alertdialog],[role=menu],[role=listbox],[data-canvas-ai-panel]",
        )
      )
        return;
      if (!["z", "y"].includes(event.key.toLowerCase())) return;
      event.preventDefault();
      historyShortcut(
        event.shiftKey || event.key.toLowerCase() === "y" ? "redo" : "undo",
      );
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, []);
  function addText(point?: { x: number; y: number }) {
    if (local.current.length >= 2000) {
      setMessage("画布已达到 2000 个节点上限。");
      return;
    }
    const n = createResourceNode("text", {
      x:
        point?.x ??
        (workspace.centerX - view.current.x) / view.current.zoom - 160,
      y:
        point?.y ??
        (workspace.centerY - 260 - view.current.y) / view.current.zoom,
    });
    edit([...local.current, n]);
    selectNode(n.id);
  }
  function groupSelection() {
    const selected = local.current.filter((n) => selectedIDs.includes(n.id));
    if (
      selected.length < 2 ||
      selected.some((n) => !isEditableNode(n)) ||
      local.current.length >= 2000
    )
      return;
    const frame = groupWrapRect(selected);
    if (frame.width > 100000 || frame.height > 100000) {
      setMessage("所选节点距离过远，请缩小分组范围。");
      return;
    }
    const group = {
      ...createResourceNode("group", { x: limit(frame.x), y: limit(frame.y) }),
      width: frame.width,
      height: frame.height,
    };
    const chosen = new Set(selectedIDs);
    // When a selected ancestor already owns a selected child, retain that internal relation.
    const next = local.current.map((n) =>
      chosen.has(n.id) && (!n.parent_id || !chosen.has(n.parent_id))
        ? { ...n, parent_id: group.id }
        : n,
    );
    edit([...next, group]);
    selectNode(group.id);
  }
  function ungroup() {
    const selected = new Set(selectedIDs);
    const groups = new Set(
      local.current
        .filter((n) => selected.has(n.id) && n.node_type === "group")
        .map((n) => n.id),
    );
    if (
      local.current.some(
        (n) =>
          (selected.has(n.id) || (n.parent_id && groups.has(n.parent_id))) &&
          !isEditableNode(n),
      )
    ) {
      setMessage("分组包含只读成员，不能解除关系。");
      return;
    }
    edit(
      local.current.map((n) =>
        n.parent_id && (groups.has(n.parent_id) || selected.has(n.id))
          ? { ...n, parent_id: undefined }
          : n,
      ),
    );
  }
  function connectNodes(sourceID: string, targetID: string) {
    if (
      readOnlyRef.current ||
      busy.current ||
      pending.current ||
      unconfirmed ||
      conflict
    )
      return;
    const source = local.current.find((n) => n.id === sourceID);
    const target = local.current.find((n) => n.id === targetID);
    if (
      !source ||
      !target ||
      source === target ||
      !isEditableNode(source) ||
      !isEditableNode(target) ||
      source.node_type === "group" ||
      target.node_type === "group"
    ) {
      setMessage("此节点不能建立连接。");
      return;
    }
    if (
      localEdges.current.some(
        (e) =>
          (e.source_node_id === sourceID && e.target_node_id === targetID) ||
          (e.source_node_id === targetID && e.target_node_id === sourceID),
      )
    ) {
      setMessage("这两个节点已连接。");
      return;
    }
    if (localEdges.current.length >= 4000) {
      setMessage("画布已达到 4000 条连线上限。");
      return;
    }
    edit(local.current, undefined, [
      ...localEdges.current,
      {
        id: crypto.randomUUID(),
        edge_type: "annotation",
        source_node_id: sourceID,
        target_node_id: targetID,
      },
    ]);
    selectNode(targetID);
  }
  function connectSelection() {
    const selected = local.current.filter((n) => selectedIDs.includes(n.id));
    if (selected.length !== 2 || selected.some((n) => !isEditableNode(n)))
      return;
    if (localEdges.current.length >= 4000) {
      setMessage("画布已达到 4000 条连线上限。");
      return;
    }
    const [source, target] = selected;
    if (
      localEdges.current.some(
        (e) =>
          isAnnotation(e) &&
          e.source_node_id === source.id &&
          e.target_node_id === target.id,
      )
    )
      return;
    edit(local.current, undefined, [
      ...localEdges.current,
      {
        id: crypto.randomUUID(),
        edge_type: "annotation",
        source_node_id: source.id,
        target_node_id: target.id,
      },
    ]);
  }
  function disconnectSelection() {
    const chosen = new Set(selectedIDs);
    edit(
      local.current,
      undefined,
      localEdges.current.filter(
        (e) =>
          !isAnnotation(e) ||
          !chosen.has(e.source_node_id ?? "") ||
          !chosen.has(e.target_node_id ?? ""),
      ),
    );
  }
  function removeSelection() {
    if (readOnlyRef.current || busy.current || unconfirmed || conflict) return;
    try {
      let next = local.current;
      for (const id of selectedIDs) next = removeCanvasNode(next, id);
      const kept = new Set(next.map((node) => node.id));
      // Validate the complete transaction before changing the local graph.
      const nextEdges = localEdges.current.filter(
        (edge) =>
          kept.has(edge.source_node_id ?? "") &&
          kept.has(edge.target_node_id ?? ""),
      );
      commandBatch(savedBase.current, next, { edges: nextEdges });
      edit(next, undefined, nextEdges);
      selectNode(undefined);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "无法移除所选节点。");
    }
  }
  const adjacentRequest = useRef<AbortController | null>(null);
  const [addingAdjacent, setAddingAdjacent] = useState(false);
  useEffect(() => () => adjacentRequest.current?.abort(), []);
  async function addAdjacent(
    source: Node,
    kind: "text" | "image_generation" | "video_generation",
    side: "left" | "right",
    point?: ConnectionDrop,
  ) {
    if (
      adjacentRequest.current ||
      privateInvalid.current ||
      pending.current ||
      conflict ||
      busy.current
    )
      return;
    if (local.current.length >= 2000 || localEdges.current.length >= 4000) {
      setMessage("画布节点或连线已达到上限。");
      return;
    }
    const original = local.current;
    const sessionGeneration = getSessionGeneration(),
      scopeGeneration = getScopeGeneration();
    const controller = new AbortController();
    adjacentRequest.current = controller;
    setAddingAdjacent(true);
    try {
      let asset: API.AssetSummary | undefined;
      if (
        side === "right" &&
        kind !== "text" &&
        source.ref_type === "media_asset" &&
        source.ref_id &&
        ["image", "video", "audio"].includes(source.node_type)
      ) {
        const preview = await getMediaPreview(
          { pid: project.id, asset_id: source.ref_id },
          { signal: controller.signal },
        );
        if (preview.asset?.project_id !== project.id)
          throw new RequestError("invalid_receipt");
        asset = preview.asset;
      }
      if (
        controller.signal.aborted ||
        !alive.current ||
        privateInvalid.current ||
        getSessionGeneration() !== sessionGeneration ||
        getScopeGeneration() !== scopeGeneration
      )
        return;
      if (
        local.current !== original ||
        busy.current ||
        pending.current ||
        conflict
      )
        throw new Error("画布已变化，请重新选择连接来源。");
      const result = adjacentGraph(
        source,
        kind,
        side,
        project.aspect_ratio,
        asset,
        original,
      );
      if (point && board.current) {
        const bounds = board.current.getBoundingClientRect();
        result.node.x = limit(
          (point.x - bounds.left - view.current.x) / view.current.zoom -
            (side === "left" ? (result.node.width ?? 320) : 0),
        );
        result.node.y = limit(
          (point.y - bounds.top - view.current.y) / view.current.zoom -
            (result.node.height ?? 200) / 2,
        );
      }
      edit([...local.current, result.node], undefined, [
        ...localEdges.current,
        result.edge,
      ]);
      selectNode(result.node.id);
    } catch (error) {
      if (!controller.signal.aborted && alive.current)
        setMessage(
          error instanceof Error
            ? error.message
            : "参考来源暂时无法核验，请重试。",
        );
    } finally {
      adjacentRequest.current = null;
      if (alive.current) setAddingAdjacent(false);
    }
  }
  function add(
    kind: "image_generation" | "video_generation",
    point?: { x: number; y: number },
  ) {
    if (local.current.length >= 2000) {
      setMessage("画布已达到 2000 个节点上限。");
      return;
    }
    const n = createDraftNode(kind, project.aspect_ratio, local.current.length);
    const bounds = board.current?.getBoundingClientRect();
    if (bounds?.width && bounds.height) {
      n.x = limit(
        Math.round(
          (workspace.centerX - view.current.x) / view.current.zoom - 160,
        ),
      );
      n.y = limit(
        Math.round(
          (Math.max(80, workspace.centerY - 260) - view.current.y) /
            view.current.zoom,
        ),
      );
    }
    if (point) {
      n.x = limit(point.x);
      n.y = limit(point.y);
    }
    edit([...local.current, n]);
    selectNode(n.id);
  }
  function addTool(kind: "director") {
    if (local.current.length >= 2000) {
      setMessage("画布已达到 2000 个节点上限。");
      return;
    }
    const n: Node = {
      id: crypto.randomUUID(),
      title: "导演台",
      node_type: kind,
      node_action: "tool",
      config: { director: createDirectorConfig() },
      x: limit((workspace.centerX - view.current.x) / view.current.zoom - 160),
      y: limit((workspace.centerY - 260 - view.current.y) / view.current.zoom),
      width: 320,
      height: 320,
      z_index: 0,
    };
    edit([...local.current, n]);
    selectNode(n.id);
    setOpenToolID(n.id);
  }
  const pendingAIText = useRef(new Map<string, string>());
  async function insertAIText(text: string) {
    if (privateInvalid.current || busy.current || conflict)
      throw new Error("请先解决当前画布保存状态。");
    if (Array.from(text).length > 10000)
      throw new Error("文本超过单节点10000字上限，请先缩短结果。");
    if (
      pendingAIText.current.has(text) &&
      local.current.some((n) => n.id === pendingAIText.current.get(text))
    ) {
      if (!(await save())) throw new Error("采用结果尚未确认保存，请重试。");
      pendingAIText.current.delete(text);
      return;
    }
    if (pending.current || unconfirmed)
      throw new Error("请先确认当前画布保存结果。");
    if (local.current.length >= 2000) {
      throw new Error("画布已达到 2000 个节点上限。");
    }
    const n = createResourceNode("text", {
      x: limit((workspace.centerX - view.current.x) / view.current.zoom - 160),
      y: limit((workspace.centerY - view.current.y) / view.current.zoom - 160),
    });
    n.config.text = text;
    pendingAIText.current.set(text, n.id);
    edit([...local.current, n]);
    selectNode(n.id);
    if (!(await save())) throw new Error("采用结果尚未确认保存，请重试。");
    pendingAIText.current.delete(text);
  }
  function addMaterials(assets: API.AssetSummary[], source?: Node) {
    if (readOnlyRef.current || privateInvalid.current) return false;
    try {
      const requested = [
        ...new Map(assets.map((a) => [`${a.kind}/${a.id}`, a])).values(),
      ];
      const added = requested
        .map((a, i) => createMediaNode(a, project.id, local.current.length + i))
        .filter(
          (node) =>
            !local.current.some(
              (existing) =>
                existing.ref_type === "media_asset" &&
                existing.ref_id === node.ref_id &&
                existing.node_type === node.node_type,
            ),
        );
      if (local.current.length + added.length > 2000) {
        setMessage("画布已达到 2000 个节点上限，请减少所选素材。");
        return false;
      }
      const bounds = board.current?.getBoundingClientRect();
      if (bounds?.width && bounds.height) {
        const x =
          (workspace.centerX - view.current.x) / view.current.zoom - 144;
        const y =
          (Math.max(96, (bounds.height - 450) / 2) - view.current.y) /
          view.current.zoom;
        added.forEach((n, i) => {
          n.x = limit(Math.round(x + (i % 3) * 340));
          n.y = limit(Math.round(y + Math.floor(i / 3) * 260));
        });
      }
      if (source) {
        const top =
          Math.max(...local.current.map((n) => n.y + (n.height ?? 200))) + 64;
        const row = Math.max(...added.map((n) => n.height ?? 200)) + 64;
        added.forEach((node, index) => {
          node.x = limit(Math.round(source.x + (index % 3) * 340));
          node.y = limit(Math.round(top + Math.floor(index / 3) * row));
        });
      }
      const placed = [...local.current];
      for (const node of added) placed.push(placeNode(node, placed));
      if (added.length) edit(placed);
      const last = requested.at(-1);
      selectNode(
        local.current.find(
          (node) =>
            node.ref_type === "media_asset" &&
            node.ref_id === last?.id &&
            node.node_type === last?.kind,
        )?.id,
      );
      setMaterials(null);
      setMessage(
        added.length
          ? `已添加 ${added.length} 个素材，请保存画布。`
          : "已定位已有素材节点。",
      );
      return true;
    } catch {
      setMessage("素材信息不完整或不属于当前项目，请重新读取后添加。");
      return false;
    }
  }
  function copyNode(node: Node) {
    if (local.current.length >= 2000) {
      setMessage("画布已达到 2000 个节点上限。");
      return;
    }
    const ids = descendants(local.current, new Set([node.id]));
    const originals = local.current.filter((n) => ids.has(n.id));
    if (
      originals.some((n) => !isEditableNode(n)) ||
      originals.length + local.current.length > 2000
    ) {
      setMessage("所选分组包含只读节点或超出节点上限，不能复制。");
      return;
    }
    let copies: Node[];
    try {
      copies = remapCopiedNodeReferences(
        originals,
        originals.map(duplicateNode),
        local.current,
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "无法复制引用关系。");
      return;
    }
    const identity = new Map(originals.map((n, i) => [n.id, copies[i].id]));
    copies.forEach((copy) => {
      if (copy.parent_id && identity.has(copy.parent_id))
        copy.parent_id = identity.get(copy.parent_id);
    });
    const copiedEdges = localEdges.current
      .filter(
        (e) =>
          isAnnotation(e) &&
          ids.has(e.source_node_id ?? "") &&
          ids.has(e.target_node_id ?? ""),
      )
      .map((e) => ({
        ...e,
        id: crypto.randomUUID(),
        source_node_id: identity.get(e.source_node_id!),
        target_node_id: identity.get(e.target_node_id!),
      }));
    if (localEdges.current.length + copiedEdges.length > 4000) {
      setMessage("复制将超出 4000 条连线上限。");
      return;
    }
    edit([...local.current, ...copies], undefined, [
      ...localEdges.current,
      ...copiedEdges,
    ]);
    selectNode(identity.get(node.id));
  }
  function selectedClipboard() {
    return encodeCanvasClipboard(
      project.id,
      local.current,
      localEdges.current,
      new Set(selectedIDs),
    );
  }
  function insertClipboard(text: string, atContext = false) {
    if (busy.current || unconfirmed || conflict) {
      setMessage("请先确认画布保存状态，再粘贴节点。");
      return;
    }
    const pasted = pasteCanvasClipboard(
      text,
      project.id,
      atContext ? (contextPoint.current ?? undefined) : undefined,
    );
    if (
      local.current.length + pasted.nodes.length > 2000 ||
      localEdges.current.length + pasted.edges.length > 4000
    )
      throw new Error("粘贴将超过画布容量，请分批操作。");
    edit([...local.current, ...pasted.nodes], undefined, [
      ...localEdges.current,
      ...pasted.edges,
    ]);
    setSelected(pasted.nodes.at(-1)?.id);
    setSelectedIDs(pasted.nodes.map((node) => node.id));
  }
  async function copyClipboard() {
    try {
      await navigator.clipboard.writeText(selectedClipboard());
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "无法复制，请使用键盘复制。",
      );
    }
  }
  async function pasteClipboard() {
    try {
      insertClipboard(await navigator.clipboard.readText(), true);
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "无法读取剪贴板，请使用键盘粘贴。",
      );
    }
  }
  const clipboardCopy = useEffectEvent((event: ClipboardEvent) => {
    try {
      const text = selectedClipboard();
      if (!event.clipboardData) return;
      event.clipboardData.setData("text/plain", text);
      event.preventDefault();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "无法复制所选节点。");
    }
  });
  const clipboardPaste = useEffectEvent((event: ClipboardEvent) => {
    const text = event.clipboardData?.getData("text/plain");
    if (!text || !text.includes('"glowloom.canvas"')) return;
    event.preventDefault();
    try {
      insertClipboard(text);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "无法粘贴节点。");
    }
  });
  useEffect(() => {
    const ignored = (event: ClipboardEvent) =>
      event.defaultPrevented ||
      (event.target instanceof Element &&
        !!event.target.closest(
          "input,textarea,select,[contenteditable=true],[role=dialog],[role=alertdialog],[role=menu],[data-canvas-ai-panel]",
        ));
    const copy = (event: ClipboardEvent) => {
      if (!ignored(event)) clipboardCopy(event);
    };
    const paste = (event: ClipboardEvent) => {
      if (!ignored(event)) clipboardPaste(event);
    };
    window.addEventListener("copy", copy);
    window.addEventListener("paste", paste);
    return () => {
      window.removeEventListener("copy", copy);
      window.removeEventListener("paste", paste);
    };
  }, []);
  function removeLocal(node: Node) {
    if (
      busy.current ||
      unconfirmed ||
      conflict ||
      (baseline.has(node.id) && isDraft(node)) ||
      !isEditableNode(node)
    )
      return;
    let remaining: Node[];
    try {
      remaining = removeCanvasNode(local.current, node.id);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "无法删除节点。");
      return;
    }
    edit(
      remaining,
      undefined,
      localEdges.current.filter(
        (e) => e.source_node_id !== node.id && e.target_node_id !== node.id,
      ),
    );
    selectNode(undefined);
  }
  function useSubject(subject: Subject) {
    setMaterials(null);
    setGeneration({
      project,
      prompt: `${subjectText(subject).name}。${subjectText(subject).description ?? ""}`,
      capability: "image",
      subject,
    });
  }
  function fit(selectionOnly = false) {
    const visible = selectionOnly
      ? nodes.filter((n) => selectedIDs.includes(n.id))
      : nodes;
    if (!board.current || !visible.length) {
      changeCamera({ x: 0, y: 0, zoom: 1 });
      return;
    }
    const frame =
      selectionOnly && visible.length === 1 && isDraft(visible[0])
        ? { ...workspace, height: Math.max(260, workspace.height - 320) }
        : workspace;
    changeCamera(fitNodes(visible, frame, selectionOnly));
  }
  function beginDrag(e: PointerEvent<HTMLElement>, node: Node) {
    if (
      !isEditableNode(node) ||
      e.button !== 0 ||
      tool === "pan" ||
      e.shiftKey ||
      e.metaKey ||
      e.ctrlKey
    )
      return;
    const members = descendants(
      local.current,
      new Set(selectedIDs.includes(node.id) ? selectedIDs : [node.id]),
    );
    if (local.current.some((n) => members.has(n.id) && !isEditableNode(n))) {
      setMessage("所选分组包含只读节点，不能整体移动。");
      return;
    }
    e.stopPropagation();
    if (!selectedIDs.includes(node.id)) selectNode(node.id);
    e.currentTarget.setPointerCapture(e.pointerId);
    const card = e.currentTarget.parentElement!;
    drag.current = {
      id: node.id,
      clientX: e.clientX,
      clientY: e.clientY,
      x: node.x,
      y: node.y,
      nextX: node.x,
      nextY: node.y,
      card,
      button: e.currentTarget,
      pointerId: e.pointerId,
      members: local.current
        .filter((n) => members.has(n.id))
        .map((n) => ({
          id: n.id,
          x: n.x,
          y: n.y,
          element:
            board.current?.querySelector<HTMLElement>(
              `[data-node-id="${n.id}"]`,
            ) ?? null,
        })),
    };
  }
  function moveDrag(e: PointerEvent<HTMLElement>) {
    const d = drag.current;
    if (!d) return;
    d.nextX = limit(
      Math.round(d.x + (e.clientX - d.clientX) / view.current.zoom),
    );
    d.nextY = limit(
      Math.round(d.y + (e.clientY - d.clientY) / view.current.zoom),
    );
    for (const member of d.members) {
      if (member.element) {
        member.element.style.left = `${limit(member.x + d.nextX - d.x)}px`;
        member.element.style.top = `${limit(member.y + d.nextY - d.y)}px`;
      }
    }
    board.current?.dispatchEvent(new Event(CANVAS_NODE_DRAG_EVENT));
  }
  function endDrag(cancel = false) {
    const d = drag.current;
    if (!d) return;
    drag.current = null;
    if (d.button.hasPointerCapture(d.pointerId))
      d.button.releasePointerCapture(d.pointerId);
    if (cancel) {
      for (const member of d.members)
        if (member.element) {
          member.element.style.left = `${member.x}px`;
          member.element.style.top = `${member.y}px`;
        }
      board.current?.dispatchEvent(new Event(CANVAS_NODE_DRAG_EVENT));
      return;
    }
    const moved = new Map(
      d.members.map((member) => [
        member.id,
        {
          x: limit(member.x + d.nextX - d.x),
          y: limit(member.y + d.nextY - d.y),
        },
      ]),
    );
    if (d.nextX === d.x && d.nextY === d.y) return;
    edit(
      reconcileGroupDrop(
        local.current.map((node) =>
          moved.has(node.id) ? { ...node, ...moved.get(node.id)! } : node,
        ),
        new Set(moved.keys()),
      ),
    );
  }
  async function readConflict() {
    const controller = new AbortController();
    request.current = controller;
    try {
      const server = requireCanvas(
        await getCanvas({ id: base.id }, { signal: controller.signal }),
        base.project_id,
        base.id,
      );
      if (alive.current) {
        if (server.read_only_reason)
          setGraphReadOnlyReason(server.read_only_reason);
        setConflict({ server });
      }
    } catch {
      if (alive.current && !controller.signal.aborted)
        setMessage("未能读取服务端版本，本地编辑仍保留，请重试读取。");
    }
  }
  const saveFlight = useRef<Promise<Canvas | undefined> | null>(null);
  const saveLatest = useRef<() => Promise<Canvas | undefined>>(
    async () => undefined,
  );
  async function save() {
    if (saveFlight.current) return saveFlight.current;
    const flight = (async () => {
      try {
        let result = await saveOnce();
        while (
          result &&
          alive.current &&
          !privateInvalid.current &&
          commandBatch(result, local.current, {
            edges: localEdges.current,
          }).commands?.length
        )
          result = await saveOnce();
        return result;
      } catch {
        setAutoSavePaused(true);
        setMessage("请检查节点名称、提示词和参数；本地编辑仍保留。");
        return undefined;
      }
    })();
    saveFlight.current = flight;
    try {
      return await flight;
    } finally {
      saveFlight.current = null;
    }
  }
  async function saveOnce() {
    if (
      readOnlyRef.current ||
      busy.current ||
      conflict ||
      privateInvalid.current
    )
      return;
    const base = savedBase.current;
    let operation = pending.current;
    if (!operation) {
      try {
        const frozen = structuredClone(local.current);
        const frozenEdges = structuredClone(localEdges.current);
        const body = commandBatch(base, frozen, {
          edges: frozenEdges,
        });
        if (!body.commands?.length) {
          return base;
        }
        operation = {
          key: crypto.randomUUID(),
          base,
          nodes: frozen,
          body,
          edges: frozenEdges,
        };
        pending.current = operation;
      } catch {
        setMessage(
          "请检查节点名称、提示词和参数；本次最多提交 100 条编辑命令。",
        );
        return;
      }
    }
    busy.current = true;
    setSaving(true);
    setMessage("");
    const controller = new AbortController();
    request.current = controller;
    try {
      const value = developmentPreview
        ? await applyCanvasCommands({ id: base.id }, operation.body, {
            headers: { "Idempotency-Key": operation.key },
            signal: controller.signal,
          })
        : await live.current!.submit(operation.key, operation.body);
      const committed = requireSaveResult(
        value,
        operation.base,
        operation.nodes,
        operation.body,
      );
      if (!alive.current || controller.signal.aborted) return;
      const frozen = new Map(operation.nodes.map((n) => [n.id, n]));
      const saved = new Map(committed.nodes.map((n) => [n.id, n]));
      edit(
        local.current.map((n) => {
          const original = frozen.get(n.id),
            receipt = saved.get(n.id);
          if (!receipt || !original) return n;
          return sameEditable(n, original)
            ? receipt
            : { ...n, input_revision: receipt.input_revision };
        }),
        undefined,
        canonical(localEdges.current) ===
          canonical(operation.edges ?? operation.base.edges)
          ? committed.edges
          : localEdges.current,
        false,
      );
      setBase(committed);
      setAutoSavePaused(false);
      pending.current = null;
      setUnconfirmed(false);
      publishProjectInvalidation(project.id);
      return committed;
    } catch (error) {
      if (!alive.current || controller.signal.aborted) return;
      if (
        error instanceof RequestError &&
        error.code === "canvas_graph_read_only"
      ) {
        pending.current = null;
        setUnconfirmed(false);
        setAutoSavePaused(true);
        setGraphReadOnlyReason(error.code);
        setMessage(
          "画布包含尚未兼容的数据，已停止编辑。当前未保存内容仅留在此页面。",
        );
      } else if (
        error instanceof RequestError &&
        error.code === "revision_conflict"
      ) {
        pending.current = null;
        setUnconfirmed(false);
        readOnlyRef.current = true;
        setSyncBlocked(true);
        setConflict({ server: null });
        setMessage(
          "画布已被其他窗口修改，本地编辑已保留。请对照服务端内容后明确选择。",
        );
        await readConflict();
      } else if (
        error instanceof RequestError &&
        [401, 403, 404, 409, 413, 422].includes(error.status)
      ) {
        pending.current = null;
        setUnconfirmed(false);
        setAutoSavePaused(true);
        setMessage(
          error.status === 401
            ? "登录已失效，本地编辑仍保留，请重新登录后重开画布。"
            : error.status === 413
              ? "本次编辑内容过大，已停止自动保存。本地内容仍保留，请减少本次编辑内容后保存。"
              : "服务端拒绝保存，本地编辑仍保留。请检查权限、节点及参数。",
        );
      } else {
        setUnconfirmed(true);
        setMessage(
          "保存结果未确认，本地编辑仍保留。请重试原保存，避免重复提交。",
        );
      }
    } finally {
      busy.current = false;
      if (alive.current) {
        setSaving(false);
        const latest = deferredSnapshot.current;
        deferredSnapshot.current = null;
        if (latest) receiveSnapshot(latest.document, latest.writable);
      }
    }
  }
  useEffect(() => {
    saveLatest.current = save;
  });
  const resumeSave = useEffectEvent(() => {
    if (
      liveStatus === "live" &&
      unconfirmed &&
      !readOnly &&
      !conflict &&
      !saving
    )
      void saveLatest.current();
  });
  useEffect(() => {
    resumeSave();
  }, [liveStatus]);
  useEffect(() => {
    if (
      !dirty ||
      readOnly ||
      saving ||
      autoSavePaused ||
      unconfirmed ||
      conflict ||
      identityInvalid
    )
      return;
    const timer = setTimeout(() => {
      void saveLatest.current();
    }, 900);
    return () => clearTimeout(timer);
  }, [
    nodes,
    edges,
    camera,
    dirty,
    saving,
    autoSavePaused,
    unconfirmed,
    conflict,
    identityInvalid,
    readOnly,
  ]);
  function changeNodeDraft(ownerNode: string, value: PromptDraft) {
    const n = local.current.find((n) => n.id === ownerNode);
    if (n && isDraft(n)) {
      try {
        const graph = materializeReferences(
          local.current,
          localEdges.current,
          ownerNode,
          value,
          project.id,
        );
        const textOnly =
          graph.nodes.length === local.current.length &&
          graph.edges.length === localEdges.current.length;
        edit(
          graph.nodes,
          textOnly ? `${ownerNode}/config` : undefined,
          graph.edges,
        );
      } catch (error) {
        setMessage(
          error instanceof Error ? error.message : "引用无法同步到画布。",
        );
      }
    }
  }
  async function flushNodeDraft(ownerNode: string, composer: PromptDraft) {
    if (privateInvalid.current || readOnlyRef.current)
      throw new Error("画布保存尚未确认，请重新打开画布。");
    const existing = local.current.find((n) => n.id === ownerNode);
    if (!existing || !isDraft(existing))
      throw new Error("画布节点已不可编辑。");
    if (!existing.config.draft.composer)
      update(ownerNode, { config: { draft: richCanvasDraft(composer) } });
    const committed = await saveLatest.current();
    if (!committed)
      throw new Error("画布保存尚未确认，请先处理冲突或重试原保存。");
    const owner = committed.nodes.find((n) => n.id === ownerNode);
    if (!owner || !isDraft(owner) || !owner.config.draft.composer)
      throw new Error("请先完成节点草稿保存。");
    const latest = local.current.find((n) => n.id === ownerNode);
    if (!latest || !sameEditable(latest, owner))
      throw new Error("保存期间节点已变化，请重新保存当前输入。");
    return {
      kind: "canvas_input" as const,
      canvas_id: committed.id,
      node_id: ownerNode,
      canvas_revision: committed.revision,
      input_revision: owner.input_revision,
    };
  }
  let currentSeed: GenerationSeed | undefined;
  if (!readOnly && current && isDraft(current)) {
    const composer = nodePromptDraft(current);
    const sources = referenceSources(nodes);
    const ownerNode = current.id;
    currentSeed = {
      project,
      prompt: "",
      composer,
      capability: current.node_type === "video_generation" ? "video" : "image",
      source: {
        canvas_id: initial.id,
        node_id: ownerNode,
        revision: base.revision,
      },
      canvasOwner: {
        contextKey: `canvas/${initial.id}/${ownerNode}`,
        draft: composer,
        referenceSources: sources.filter(
          (source) => source.nodeID !== ownerNode,
        ),
        incomingSources: connectedReferenceSources(nodes, edges, ownerNode),
        onChange: (value) => changeNodeDraft(ownerNode, value),
        flush: () => flushNodeDraft(ownerNode, composer),
      },
    };
  }

  async function adoptResult(id: string, kind: string) {
    if (readOnlyRef.current)
      throw new Error("项目当前只读，无法采用生成结果。");
    if (!["image", "video", "audio"].includes(kind) || busy.current || conflict)
      throw new Error("请先解决画布保存状态，再采用结果。");
    const existing = local.current.find(
      (n) => n.ref_type === "media_asset" && n.ref_id === id,
    );
    if (!existing) {
      const node: Node = {
        id: crypto.randomUUID(),
        title: kind === "video" ? "生成视频" : "生成图片",
        node_type: kind,
        node_action: "resource",
        ref_type: "media_asset",
        ref_id: id,
        config: {},
        x: 80 + (local.current.length % 3) * 340,
        y: 80 + Math.floor(local.current.length / 3) * 260,
        z_index: 0,
        width: 288,
        height: 200,
      };
      edit([...local.current, node]);
      selectNode(node.id);
    } else selectNode(existing.id);
    const saved = await save();
    if (!saved)
      throw new Error(
        "结果已保留在本地画布，保存尚未确认。请重试采用以继续同一次保存。",
      );
    setGeneration(null);
    setMessage("生成结果已添加并保存到画布。");
  }
  function zoomAtCenter(zoom: number) {
    changeCamera(
      zoomAt(
        view.current,
        { x: workspace.centerX, y: workspace.centerY },
        zoom,
      ),
    );
  }
  const viewShortcut = useEffectEvent((event: KeyboardEvent) => {
    const modified = event.metaKey || event.ctrlKey;
    const key = event.key.toLowerCase();
    if (!modified && !event.shiftKey && key === "v") {
      event.preventDefault();
      setTool("select");
    } else if (modified && (key === "+" || key === "=" || key === "-")) {
      event.preventDefault();
      zoomAtCenter(
        key === "-" ? view.current.zoom / 1.2 : view.current.zoom * 1.2,
      );
    } else if (key === "1" && modified) {
      event.preventDefault();
      zoomAtCenter(1);
    } else if (
      (event.code === "Digit1" || key === "1" || key === "!") &&
      event.shiftKey &&
      !modified
    ) {
      event.preventDefault();
      fit();
    } else if (
      (event.code === "Digit2" || key === "2" || key === "@") &&
      event.shiftKey &&
      !modified
    ) {
      event.preventDefault();
      fit(true);
    } else if (modified && key === "d" && current) {
      event.preventDefault();
      copyNode(current);
    } else if (
      !modified &&
      (key === "delete" || key === "backspace") &&
      current
    ) {
      event.preventDefault();
      if (baseline.has(current.id) && isDraft(current))
        setMessage("生成草稿已关联服务端版本，不能直接删除。");
      else removeLocal(current);
    }
  });

  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (!isCanvasShortcut(event)) return;
      viewShortcut(event);
    };
    window.addEventListener("keydown", keydown);
    return () => window.removeEventListener("keydown", keydown);
  }, []);
  const saveShortcut = useEffectEvent(() => {
    void save();
  });
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        saveShortcut();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  function resolveConflict(discard: boolean) {
    const server = conflict?.server;
    if (!server || (!discard && server.read_only_reason)) return;
    if (discard) {
      history.reset();
      edit(server.nodes, undefined, server.edges, false);
      selectNode(undefined);
    } else {
      const own = new Map(local.current.map((n) => [n.id, n]));
      const previous = new Map(base.nodes.map((n) => [n.id, n]));
      let merged = server.nodes.map((n) => {
        const ours = own.get(n.id),
          old = previous.get(n.id);
        return ours && old && !sameEditable(ours, old)
          ? { ...ours, input_revision: n.input_revision }
          : n;
      });
      const remoteIDs = new Set(server.nodes.map((n) => n.id));
      for (const n of local.current) if (!remoteIDs.has(n.id)) merged.push(n);
      const removed = base.nodes.filter((n) => !own.has(n.id));
      try {
        for (const n of removed)
          if (merged.some((remote) => remote.id === n.id))
            merged = removeCanvasNode(merged, n.id);
      } catch (error) {
        setMessage(
          error instanceof Error
            ? error.message
            : "无法保留本地删除，请核对引用。",
        );
        return;
      }
      const oldEdges = new Map(base.edges.map((edge) => [edge.id, edge]));
      const localIDs = new Set(localEdges.current.map((edge) => edge.id));
      const combined = server.edges.filter(
        (edge) =>
          !isAnnotation(edge) ||
          !oldEdges.has(edge.id) ||
          localIDs.has(edge.id),
      );
      const serverIDs = new Set(server.edges.map((edge) => edge.id));
      combined.push(
        ...localEdges.current.filter(
          (edge) => !oldEdges.has(edge.id) && !serverIDs.has(edge.id),
        ),
      );
      const ids = new Set(merged.map((n) => n.id));
      const finalEdges = combined.filter(
        (e) =>
          !isAnnotation(e) ||
          (ids.has(e.source_node_id ?? "") && ids.has(e.target_node_id ?? "")),
      );
      try {
        commandBatch(server, merged, { edges: finalEdges });
      } catch {
        setMessage("本地改动与服务端受保护节点或引用冲突，请核对后再选择。");
        return;
      }
      history.reset();
      edit(merged, undefined, finalEdges, false);
    }
    setBase(server);
    setConflict(null);
    setSyncBlocked(false);
    setMessage("");
  }
  const status =
    liveStatus !== "live"
      ? liveStatus === "connecting"
        ? "正在连接画布…"
        : "连接中断，正在重连；编辑暂存于当前页面"
      : saving
        ? "正在保存…"
        : unconfirmed
          ? "保存未确认"
          : conflict
            ? "版本冲突，本地编辑保留"
            : dirty
              ? "有未保存的编辑"
              : `已保存 · 版本 ${base.revision}`;
  if (identityInvalid)
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>请重新打开画布</EmptyTitle>
          <EmptyDescription>
            登录状态已变化，当前页面数据已清除。请重新登录后读取数据库中的画布。
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button asChild>
            <Link href="/login">重新登录</Link>
          </Button>
        </EmptyContent>
      </Empty>
    );
  return (
    <main className="creation-canvas relative h-dvh overflow-hidden">
      <header
        className="canvas-header"
        onClickCapture={(e) => {
          if (
            (dirty || saving || unconfirmed) &&
            e.target instanceof Element &&
            e.target.closest('a[href="/projects"],a[href="/"]')
          ) {
            e.preventDefault();
            setLeave(true);
          }
        }}
      >
        <CanvasHeader
          project={project}
          nodes={nodes}
          status={
            liveStatus !== "live"
              ? status
              : graphReadOnlyReason
                ? "画布数据待兼容 · 只读"
                : syncBlocked
                  ? "版本冲突，本地编辑保留"
                  : !socketWritable
                    ? "画布只读"
                    : readOnly
                      ? graphReadOnlyReason
                        ? "画布数据待兼容 · 只读"
                        : identityPending
                          ? "登录状态待核验 · 只读"
                          : project.status === "archived"
                            ? "已归档 · 只读"
                            : "项目资料未确认 · 只读"
                      : status
          }
          saveLabel={
            saving ? "正在保存…" : unconfirmed ? "重试原保存" : "保存画布"
          }
          saving={saving}
          canSave={!readOnly && !conflict && (dirty || unconfirmed)}
          canRename={!readOnly}
          onSave={() => void save()}
          onSelect={(id) => selectNode(id)}
          onMessage={setMessage}
          onShortcuts={() => setShortcutsOpen(true)}
        />
      </header>
      {message && <FeedbackToast message={message} tone="info" />}
      {identityPending && (
        <section
          className="canvas-feedback flex flex-wrap items-center gap-3 text-sm"
          aria-label="登录状态待核验"
        >
          <p>暂时无法核验登录状态，当前未保存内容仍在页面内存。</p>
          <Button variant="outline" onClick={() => verifyIdentity.current()}>
            重新核验登录
          </Button>
        </section>
      )}
      {presentation.error && (
        <FeedbackToast
          message={presentation.error}
          action={{
            label: "重新读取项目",
            onClick: () => void presentation.refresh().catch(() => {}),
          }}
        />
      )}
      <Dialog
        open={!!renaming}
        onOpenChange={(open) => {
          if (!open) setRenaming(null);
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogTitle>重命名节点</DialogTitle>
          <DialogDescription>名称用于画布与引用列表。</DialogDescription>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!renaming?.title.trim()) return;
              update(renaming.id, { title: renaming.title.trim() });
              setRenaming(null);
            }}
          >
            <Field>
              <FieldLabel htmlFor="canvas-node-name">节点名称</FieldLabel>
              <Input
                id="canvas-node-name"
                autoFocus
                maxLength={128}
                value={renaming?.title ?? ""}
                onChange={(event) =>
                  setRenaming(
                    (value) => value && { ...value, title: event.target.value },
                  )
                }
              />
            </Field>
            <DialogFooter className="mt-4">
              <Button
                type="button"
                variant="ghost"
                onClick={() => setRenaming(null)}
              >
                取消
              </Button>
              <Button type="submit" disabled={!renaming?.title.trim()}>
                保存名称
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={leave} onOpenChange={setLeave}>
        <DialogContent role="alertdialog" showCloseButton={false}>
          <DialogTitle>确认离开画布</DialogTitle>
          <DialogDescription>
            未保存的编辑在离开后会丢失；未确认保存可能已写入，请重开后核对实际结果。
          </DialogDescription>
          <DialogFooter>
            <Button variant="outline" onClick={() => setLeave(false)}>
              留在画布
            </Button>
            <Button asChild variant="destructive">
              <Link
                href="/projects"
                onClick={() => {
                  leaving.current = true;
                }}
              >
                放弃未保存编辑并返回项目库
              </Link>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {conflict && (
        <section
          aria-label="版本冲突处理"
          className="canvas-feedback flex flex-col gap-3 text-sm"
        >
          <p>
            本地基于版本 {base.revision}；
            {conflict.server
              ? `服务端版本 ${conflict.server.revision}`
              : "正在读取服务端版本…"}
            。编辑已暂停，本地内容仍保留在当前页面。
          </p>
          <div
            aria-label="本地未保存内容"
            className="max-h-40 overflow-auto rounded border p-3"
          >
            <p className="mb-2 font-medium">
              本地内容（仅当前页面） · {nodes.length} 个节点 · {edges.length}{" "}
              条连线
            </p>
            {nodes.map((node) => (
              <p key={node.id} className="break-words whitespace-pre-wrap">
                {node.title}
                <br />
                {base.read_only_reason
                  ? "数据待兼容，仅展示名称"
                  : nodePreviewText(node)}
              </p>
            ))}
          </div>
          {conflict.server && (
            <div
              aria-label="服务端内容"
              className="max-h-56 overflow-auto rounded border p-3"
            >
              <p className="mb-2 font-medium">
                服务端内容 · {conflict.server.nodes.length} 个节点 ·{" "}
                {conflict.server.edges.length} 条连线
              </p>
              <p>
                较本地基线删除{" "}
                {
                  base.nodes.filter(
                    (node) =>
                      !conflict.server!.nodes.some(
                        (remote) => remote.id === node.id,
                      ),
                  ).length
                }{" "}
                个节点；新增{" "}
                {
                  conflict.server.nodes.filter(
                    (node) => !base.nodes.some((old) => old.id === node.id),
                  ).length
                }{" "}
                个节点
              </p>
              {conflict.server.read_only_reason && (
                <p>
                  服务端数据待兼容，当前仅可查看名称。载入前请明确放弃本地未保存编辑。
                </p>
              )}
              {conflict.server.nodes.map((node) => (
                <p key={node.id} className="break-words whitespace-pre-wrap">
                  {node.title}
                  <br />
                  {conflict.server!.read_only_reason
                    ? ""
                    : nodePreviewText(node)}
                </p>
              ))}
              <p>
                连线变化：新增{" "}
                {
                  conflict.server.edges.filter(
                    (edge) => !base.edges.some((old) => old.id === edge.id),
                  ).length
                }{" "}
                条，删除{" "}
                {
                  base.edges.filter(
                    (edge) =>
                      !conflict.server!.edges.some(
                        (remote) => remote.id === edge.id,
                      ),
                  ).length
                }{" "}
                条
              </p>
              {conflict.server.edges.map((edge) => (
                <p key={edge.id}>
                  {conflict.server!.nodes.find(
                    (node) => node.id === edge.source_node_id,
                  )?.title ?? edge.source_node_id}{" "}
                  →{" "}
                  {conflict.server!.nodes.find(
                    (node) => node.id === edge.target_node_id,
                  )?.title ?? edge.target_node_id}
                  （{edge.edge_type}）
                </p>
              ))}
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            {conflict.server ? (
              <>
                <Button
                  variant="outline"
                  disabled={!!conflict.server.read_only_reason}
                  onClick={() => resolveConflict(false)}
                >
                  保留本地编辑，以版本 {conflict.server.revision} 继续
                </Button>
                <Button
                  variant="destructive"
                  onClick={() => resolveConflict(true)}
                >
                  放弃本地编辑，载入服务端版本
                </Button>
              </>
            ) : (
              <Button variant="outline" onClick={() => void readConflict()}>
                重试读取服务端版本
              </Button>
            )}
          </div>
        </section>
      )}
      <CanvasContextMenu
        readOnly={readOnly}
        onCopy={() => void copyClipboard()}
        onPaste={() => void pasteClipboard()}
        onAdd={(kind) => {
          const point = contextPoint.current ?? undefined;
          if (kind === "text") addText(point);
          else add(kind, point);
        }}
        onMaterials={() => setMaterials({ tab: "assets" })}
        onUpload={() => setUpload(true)}
        onDuplicate={() => {
          if (current) copyNode(current);
        }}
        onRemove={() => {
          if (current) removeLocal(current);
        }}
        onUndo={() => stepHistory("undo")}
        onRedo={() => stepHistory("redo")}
        canUndo={!saving && !unconfirmed && !conflict && history.canUndo(base)}
        canRedo={!saving && !unconfirmed && !conflict && history.canRedo(base)}
        canDuplicate={!!current && isEditableNode(current)}
        canRemove={
          !!current &&
          (!baseline.has(current.id) || !isDraft(current)) &&
          !saving &&
          !unconfirmed &&
          !conflict
        }
      >
        <ViewportSurface
          onContextMenu={(event) => {
            const bounds = board.current?.getBoundingClientRect();
            contextPoint.current = {
              x: limit(
                (event.clientX - (bounds?.left ?? 0) - view.current.x) /
                  view.current.zoom,
              ),
              y: limit(
                (event.clientY - (bounds?.top ?? 0) - view.current.y) /
                  view.current.zoom,
              ),
            };
            const id =
              event.target instanceof Element
                ? event.target.closest<HTMLElement>("[data-node-id]")?.dataset
                    .nodeId
                : undefined;
            if (id) selectNode(id);
            else selectNode(undefined);
          }}
          onCanvasDoubleClick={(event) =>
            event.currentTarget.dispatchEvent(
              new MouseEvent("contextmenu", {
                bubbles: true,
                cancelable: true,
                button: 2,
                clientX: event.clientX,
                clientY: event.clientY,
              }),
            )
          }
          containerRef={board}
          viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
          onViewportChange={viewportChange}
          onViewportPreviewChange={viewportPreview}
          backgroundMode="dots"
          boxSelectEnabled={tool === "select"}
          onSelectArea={selectArea}
          onCanvasDeselect={() => selectNode(undefined)}
        >
          <CanvasNodeLayer
            containerRef={board}
            nodes={nodes}
            edges={
              showEdges
                ? graphReadOnlyReason
                  ? edges
                  : referenceEdges(nodes, edges)
                : []
            }
            viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
            size={size}
            selectedIDs={selectedIDs}
            renderNode={(node, liveZoom) =>
              graphReadOnlyReason ? (
                <article
                  key={node.id}
                  data-node-id={node.id}
                  className="absolute rounded-lg border bg-card p-4 text-card-foreground"
                  style={{
                    left: node.x,
                    top: node.y,
                    width: node.width ?? 288,
                    minHeight: 100,
                  }}
                >
                  <h2 className="truncate">{node.title}</h2>
                  <p className="text-sm text-muted-foreground">
                    节点数据待兼容，当前仅可查看。
                  </p>
                </article>
              ) : (
                <CanvasNode
                  key={node.id}
                  node={node}
                  projectID={project.id}
                  readOnly={readOnly}
                  selected={selectedIDs.includes(node.id)}
                  editingText={
                    !readOnly &&
                    editingTextID === node.id &&
                    selectedIDs.length === 1 &&
                    selected === node.id
                  }
                  onEditText={() => {
                    selectNode(node.id);
                    setEditingTextID(node.id);
                  }}
                  onFinishText={() => setEditingTextID(undefined)}
                  onTextChange={(text) => {
                    if (!readOnlyRef.current && isEditableNode(node))
                      update(node.id, { config: { text } });
                  }}
                  zoom={liveZoom}
                  onResize={(size) =>
                    update(node.id, {
                      ...size,
                      x: limit(size.x),
                      y: limit(size.y),
                    })
                  }
                  onResizePreview={() =>
                    board.current?.dispatchEvent(
                      new Event(CANVAS_NODE_DRAG_EVENT),
                    )
                  }
                  onSelect={(additive) => {
                    if (!additive && selectedIDs.includes(node.id))
                      setSelected(node.id);
                    else selectNode(node.id, additive);
                  }}
                  addingAdjacent={addingAdjacent}
                  onAddAdjacent={
                    !readOnly &&
                    isEditableNode(node) &&
                    node.node_type !== "group"
                      ? (kind, side, point) =>
                          void addAdjacent(node, kind, side, point)
                      : undefined
                  }
                  onConnect={(targetID) => connectNodes(node.id, targetID)}
                  onEditTitle={() => {
                    if (readOnly) return;
                    selectNode(node.id);
                    setRenaming({ id: node.id, title: node.title });
                  }}
                  onOpen={
                    node.node_type === "director"
                      ? () => setOpenToolID(node.id)
                      : undefined
                  }
                  onMove={(x, y) => {
                    try {
                      edit(
                        moveSelection(
                          local.current,
                          new Set([node.id]),
                          x - node.x,
                          y - node.y,
                        ),
                      );
                    } catch (error) {
                      setMessage((error as Error).message);
                    }
                  }}
                  onDragStart={(event) => beginDrag(event, node)}
                  onDragMove={moveDrag}
                  onDragEnd={endDrag}
                  onDuplicate={() => copyNode(node)}
                  canRemove={
                    (!baseline.has(node.id) || !isDraft(node)) &&
                    !saving &&
                    !unconfirmed &&
                    !conflict
                  }
                  onRemove={() => removeLocal(node)}
                />
              )
            }
          />
        </ViewportSurface>
      </CanvasContextMenu>
      {!nodes.length && (
        <div className="pointer-events-none absolute inset-0 grid place-items-center">
          <Empty>
            <EmptyHeader>
              <EmptyTitle className="canvas-empty-hint">
                双击画布，创建节点
              </EmptyTitle>
            </EmptyHeader>
          </Empty>
        </div>
      )}
      <div
        className="canvas-edit-actions"
        role="toolbar"
        aria-label="画布编辑操作"
        data-canvas-no-zoom
      >
        <Button
          size="sm"
          variant="outline"
          aria-label="撤销编辑"
          disabled={
            readOnly ||
            saving ||
            unconfirmed ||
            !!conflict ||
            !history.canUndo(base)
          }
          onClick={() => stepHistory("undo")}
        >
          撤销
        </Button>
        <Button
          size="sm"
          variant="outline"
          aria-label="重做编辑"
          disabled={
            readOnly ||
            saving ||
            unconfirmed ||
            !!conflict ||
            !history.canRedo(base)
          }
          onClick={() => stepHistory("redo")}
        >
          重做
        </Button>
      </div>
      {!readOnly &&
        (selectedIDs.length > 1 ||
          current?.node_type === "group" ||
          (current && isMediaNode(current))) && (
          <CanvasSelectionToolbar
            onReveal={(dy) =>
              changeCamera({ ...view.current, y: view.current.y + dy })
            }
            rightInset={aiOpen ? effectiveAIWidth + 24 : 0}
            nodes={nodes.filter((node) => selectedIDs.includes(node.id))}
            containerRef={board}
            canEdit={nodes
              .filter((node) => selectedIDs.includes(node.id))
              .every(isEditableNode)}
            canRemove={
              !saving &&
              !unconfirmed &&
              !conflict &&
              nodes
                .filter((node) => selectedIDs.includes(node.id))
                .every(
                  (node) =>
                    isEditableNode(node) &&
                    (!baseline.has(node.id) || !isDraft(node)),
                )
            }
            onToggleGroup={
              current?.node_type === "group" && selectedIDs.length === 1
                ? () =>
                    update(current.id, {
                      config: { collapsed: !current.config.collapsed },
                    })
                : undefined
            }
            onGroup={groupSelection}
            onUngroup={ungroup}
            onCopy={() => {
              try {
                insertClipboard(selectedClipboard());
              } catch (error) {
                setMessage((error as Error).message);
              }
            }}
            onRemove={removeSelection}
            onConnect={connectSelection}
            onDisconnect={disconnectSelection}
          >
            {selectedIDs.length === 1 && current && isMediaNode(current) ? (
              <CanvasMediaActions
                node={current}
                projectID={project.id}
                disabled={
                  saving ||
                  unconfirmed ||
                  !!conflict ||
                  !isEditableNode(current)
                }
                onPreview={() => setPreview(current)}
                onImageTool={(kind) => setImageTool({ node: current, kind })}
                onCreate={() =>
                  void addAdjacent(current, "image_generation", "right")
                }
              />
            ) : undefined}
          </CanvasSelectionToolbar>
        )}
      <CanvasToolbar
        materialsOpen={!!materials}
        uploadOpen={upload}
        readOnly={readOnly}
        tool={tool}
        onToolChange={setTool}
        onAddTool={addTool}
        onAI={() => setAIOpen((value) => !value)}
        onAdd={(kind) => add(kind)}
        onAddText={() => addText()}
        onMaterials={(tab = "assets", kind) => setMaterials({ tab, kind })}
        onUpload={() => setUpload(true)}
        zoom={camera.zoom}
        onZoom={zoomAtCenter}
        onFit={() => fit()}
        onFitSelection={() => fit(true)}
        shortcutsOpen={shortcutsOpen}
        onShortcutsOpenChange={setShortcutsOpen}
        hasSelection={selectedIDs.length > 0}
        showEdges={showEdges}
        onShowEdges={() => setShowEdges((value) => !value)}
        miniMap={miniMap}
        onMiniMap={() => setMiniMap(!miniMap)}
      />
      {miniMap && (
        <CanvasMiniMap
          nodes={nodes}
          viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
          width={size.width}
          height={size.height}
          onNavigate={(x, y) =>
            changeCamera({
              x: workspace.centerX - x * view.current.zoom,
              y: workspace.centerY - y * view.current.zoom,
              zoom: view.current.zoom,
            })
          }
        />
      )}
      {!readOnly &&
        current &&
        selectedIDs.length === 1 &&
        isEditableNode(current) &&
        (isDraft(current) || current.node_type === "text") &&
        current.id !== editingTextID &&
        current.id !== openToolID && (
          <CanvasNodePanel
            onReveal={(dy) =>
              changeCamera({ ...view.current, y: view.current.y + dy })
            }
            rightInset={aiOpen ? effectiveAIWidth + 24 : 0}
            node={current}
            viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
            containerRef={board}
          >
            {current.node_type === "text" ? (
              <RichGeneration
                key={`text-prompt/${current.id}`}
                projectID={project.id}
                seed={{ project, capability: "text", prompt: "" }}
                draftID={textDrafts.get(current.id)}
                onDraftReady={(id) =>
                  setTextDrafts((previous) =>
                    previous.get(current.id) === id
                      ? previous
                      : new Map(previous).set(current.id, id),
                  )
                }
                presentation="canvas"
              />
            ) : isDraft(current) ? (
              <aside
                aria-label="节点编辑"
                className="canvas-node-composer"
                data-generation="true"
                data-canvas-no-zoom
                data-canvas-wheel-scroll
              >
                <RichGeneration
                  key={current.id}
                  projectID={project.id}
                  seed={currentSeed!}
                  canvas={currentSeed!.canvasOwner}
                  presentation="canvas"
                  onAdopt={adoptResult}
                />
              </aside>
            ) : null}
          </CanvasNodePanel>
        )}
      {!readOnly && aiOpen && (
        <div
          className="canvas-ai-shell"
          style={{ width: effectiveAIWidth }}
          data-canvas-no-zoom
        >
          <div
            role="separator"
            aria-label="调整 AI 对话宽度"
            aria-orientation="vertical"
            aria-valuemin={240}
            aria-valuemax={Math.max(240, size.width - 84)}
            aria-valuenow={effectiveAIWidth}
            tabIndex={0}
            className="canvas-ai-resize"
            onKeyDown={(event) => {
              if (["ArrowLeft", "ArrowRight"].includes(event.key)) {
                event.preventDefault();
                setAIWidth((value) =>
                  Math.max(
                    240,
                    Math.min(
                      size.width - 84,
                      value + (event.key === "ArrowLeft" ? 32 : -32),
                    ),
                  ),
                );
              }
            }}
            onPointerDown={(event) => {
              if (event.button !== 0) return;
              aiResize.current = { x: event.clientX, width: effectiveAIWidth };
              event.currentTarget.setPointerCapture(event.pointerId);
            }}
            onPointerMove={(event) => {
              const d = aiResize.current;
              if (d)
                setAIWidth(
                  Math.max(
                    240,
                    Math.min(size.width - 84, d.width + d.x - event.clientX),
                  ),
                );
            }}
            onPointerUp={() => {
              aiResize.current = null;
            }}
            onPointerCancel={() => {
              if (aiResize.current) setAIWidth(aiResize.current.width);
              aiResize.current = null;
            }}
            onLostPointerCapture={() => {
              aiResize.current = null;
            }}
          />
          <CanvasAIPanel
            project={project}
            nodes={nodes}
            selectedNode={current}
            onGenerate={setGeneration}
            onInsertText={insertAIText}
            onClose={() => setAIOpen(false)}
          />
        </div>
      )}
      {!readOnly &&
        nodes.find((n) => n.id === openToolID)?.node_type === "director" && (
          <CanvasDirector
            onSave={async () => !!(await save())}
            saveMessage={
              message ||
              (unconfirmed
                ? "保存结果待确认"
                : conflict
                  ? "画布版本冲突，请关闭导演台处理"
                  : dirty
                    ? "有未保存的编辑"
                    : "已保存")
            }
            saving={saving}
            key={openToolID}
            node={nodes.find((n) => n.id === openToolID)!}
            nodes={nodes}
            projectID={project.id}
            onUpdate={(config) =>
              update(openToolID!, {
                config: {
                  ...local.current.find((n) => n.id === openToolID)?.config,
                  director: config,
                },
              })
            }
            onClose={() => setOpenToolID(undefined)}
          />
        )}
      {!readOnly && generation && (
        <GenerationDialog
          seed={generation}
          onClose={() => setGeneration(null)}
          onAdopt={adoptResult}
        />
      )}
      {!readOnly && materials && (
        <CanvasMaterialDialog
          projectID={project.id}
          initialTab={materials.tab}
          initialKind={materials.kind}
          onClose={() => setMaterials(null)}
          onAdd={(assets) => {
            if (!addMaterials(assets))
              throw new Error("无法添加素材，请检查画布状态。");
          }}
          onSubject={useSubject}
        />
      )}
      {!readOnly && upload && (
        <UploadDialog
          projectID={project.id}
          onClose={() => setUpload(false)}
          onUploaded={(asset) => {
            setUpload(false);
            if (
              asset &&
              ["image", "video", "audio", "model"].includes(asset.kind ?? "")
            )
              addMaterials([asset]);
            else
              setMessage(
                "文件已上传。当前画布支持图片、视频、音频及 3D 素材，请在项目素材中查看文件。",
              );
          }}
        />
      )}
      {!readOnly && imageTool?.node.ref_id && (
        <CanvasImageDialog
          key={`${imageTool.node.id}/${imageTool.kind}`}
          projectID={project.id}
          assetID={imageTool.node.ref_id}
          kind={imageTool.kind}
          availableSlots={2000 - nodes.length}
          onClose={() => setImageTool(null)}
          onCreated={(assets) => addMaterials(assets, imageTool.node)}
        />
      )}
      {preview?.ref_id && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setPreview(null);
          }}
        >
          <DialogContent className="sm:max-w-4xl">
            <DialogTitle>{preview.title}</DialogTitle>
            <DialogDescription>当前项目的素材预览</DialogDescription>
            <div className="relative flex h-[min(60dvh,560px)] items-center justify-center overflow-hidden rounded-lg">
              <ProjectMediaPreview
                projectID={project.id}
                id={preview.ref_id}
                kind={preview.node_type}
                large
              />
            </div>
          </DialogContent>
        </Dialog>
      )}
    </main>
  );
}
