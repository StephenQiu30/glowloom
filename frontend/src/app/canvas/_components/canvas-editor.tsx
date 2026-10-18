"use client";

import { FeedbackToast } from "@/components/feedback/feedback-toast";

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
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { getSession } from "@/api/identity";
import {
  branchJournal,
  canonical,
  type LocalBranch,
} from "@/components/generation/draft-save";
import { RequestError } from "@/lib/request";
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
  requireCanvas,
  requireSaveResult,
  sameEditable,
  createResourceNode,
  removeCanvasNode,
  defaultViewport,
  isAnnotation,
  type CanvasViewport,
  type Canvas,
  type Draft,
  type DraftNode,
  type Node,
} from "./editor-state";

import { RichGeneration } from "@/components/generation/rich-generation";
import { CanvasNodeLayer } from "./canvas-node-layer";
import { createHistory } from "./canvas-history";
import { descendants, moveSelection } from "./canvas-render-model";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { BeefCanvas } from "./beeftv-canvas";
import { CanvasNode } from "./canvas-node";
import { CanvasHeader } from "./canvas-header";
import { CanvasContextMenu } from "./canvas-context-menu";
import {
  encodeCanvasClipboard,
  pasteCanvasClipboard,
  remapCopiedNodeReferences,
} from "./canvas-clipboard";
import { createDirectorConfig } from "./canvas-director-model";
import { canvasWorkspaceRect } from "./canvas-workspace-rect";
import { CanvasToolbar } from "./canvas-toolbar";
import { CanvasComposer } from "./canvas-composer";
import { CanvasMiniMap } from "./canvas-mini-map";
import {
  CANVAS_NODE_DRAG_EVENT,
  type SelectionArea,
  type ViewportTransform,
} from "./canvas-viewport";
import { CanvasNodePanel } from "./canvas-node-panel";
import "./canvas.css";

type Pending = {
  key: string;
  base: Canvas;
  nodes: Node[];
  body: API.CommandsInput;
  edges?: API.Edge[];
  viewport?: CanvasViewport;
};
const limit = (value: number) => Math.max(-1e6, Math.min(1e6, value));
const CanvasMaterialDialog = dynamic(() =>
  import("./canvas-material-dialog").then(
    (module) => module.CanvasMaterialDialog,
  ),
);

const CanvasDirector = dynamic(
  () => import("./canvas-director").then((m) => m.CanvasDirector),
  { ssr: false },
);
const CanvasAIPanel = dynamic(() =>
  import("./canvas-ai-panel").then((m) => m.CanvasAIPanel),
);

export function CanvasEditor({
  project,
  initial,
}: {
  project: Project;
  initial: Canvas;
}) {
  const [aiOpen, setAIOpen] = useState(false);
  const [aiWidth, setAIWidth] = useState(400);
  const aiResize = useRef<{ x: number; width: number } | null>(null);
  const [openToolID, setOpenToolID] = useState<string>();
  const [generation, setGeneration] = useState<GenerationSeed | null>(null);
  const [materials, setMaterials] = useState<{
    tab: "assets" | "subjects";
    kind?: string;
  } | null>(null);
  const [upload, setUpload] = useState(false);
  const [preview, setPreview] = useState<Node | null>(null);
  const [base, setBaseState] = useState(initial);
  const savedBase = useRef(initial);
  const setBase = (value: Canvas) => {
    savedBase.current = value;
    setBaseState(value);
  };
  const privateInvalid = useRef(false);
  const [identityInvalid, setIdentityInvalid] = useState(false);
  const [nodes, setNodes] = useState(initial.nodes);
  const local = useRef(initial.nodes);
  const [edges, setEdges] = useState(initial.edges);
  const localEdges = useRef(initial.edges);
  const [history] = useState(createHistory);
  const [selected, setSelected] = useState<string>();
  const [saving, setSaving] = useState(false);
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
  const [details, setDetails] = useState(false);
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
    button: HTMLButtonElement;
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
    canonical(camera) !== canonical(base.viewport ?? defaultViewport) ||
    nodes.length !== base.nodes.length ||
    nodes.some(
      (n) => !baseline.has(n.id) || !sameEditable(n, baseline.get(n.id)!),
    );
  const current = nodes.find((n) => n.id === selected);
  const effectiveAIWidth = Math.min(aiWidth, Math.max(240, size.width - 84));
  const workspace = canvasWorkspaceRect(
    size.width,
    size.height,
    aiOpen ? effectiveAIWidth : 0,
  );
  const [replicaId] = useState(() => crypto.randomUUID());
  const [recoveryContext, setRecoveryContext] = useState("");
  const [recoveryBranches, setRecoveryBranches] = useState<LocalBranch[]>([]);
  const recordRevision = useRef(0),
    journal = useRef(Promise.resolve());
  useEffect(() => {
    let active = true;
    getSession()
      .then(async (identity) => {
        if (!identity.actor?.id) throw new Error("session_invalid");
        const context = `${identity.actor.id}/${project.id}/canvas/${initial.id}`;
        const branches = await branchJournal.list(context);
        if (active) {
          setRecoveryContext(context);
          setRecoveryBranches(
            branches.filter(
              (b) =>
                b.replicaId !== replicaId && b.ackedSeq !== b.snapshot.localSeq,
            ),
          );
        }
      })
      .catch(() => {
        if (active)
          setMessage("本地恢复待核验；请保持当前页面并确认登录状态。");
      });
    return () => {
      active = false;
    };
  }, [project.id, initial.id, replicaId]);
  useEffect(() => {
    const suspend = () => {
      privateInvalid.current = true;
      request.current?.abort();
      setIdentityInvalid(true);
      setGeneration(null);
    };
    const channel =
      typeof BroadcastChannel !== "undefined"
        ? new BroadcastChannel("glowloom-private-cache")
        : null;
    if (channel) channel.onmessage = suspend;
    window.addEventListener("glowloom:private-cache-cleared", suspend);
    const verify = () => {
      if (document.visibilityState === "visible" && recoveryContext) {
        void getSession()
          .then((identity) => {
            if (
              !identity.actor?.id ||
              !recoveryContext.startsWith(identity.actor.id + "/")
            )
              suspend();
          })
          .catch(suspend);
      }
    };
    document.addEventListener("visibilitychange", verify);
    return () => {
      channel?.close();
      window.removeEventListener("glowloom:private-cache-cleared", suspend);
      document.removeEventListener("visibilitychange", verify);
    };
  }, [recoveryContext]);
  const persistCanvas = useCallback(async () => {
    if (!recoveryContext || privateInvalid.current) return;
    const revision = ++recordRevision.current;
    const record: LocalBranch = {
      contextKey: recoveryContext,
      replicaId,
      localRecordRevision: revision,
      snapshot: {
        contextKey: recoveryContext,
        replicaId,
        localSeq: revision,
        draft: emptyPromptDraft(),
      },
      serverRevision: savedBase.current.revision,
      ackedSeq: dirty || pending.current ? -1 : revision,
      ownerState: {
        base: savedBase.current,
        nodes: structuredClone(local.current),
        edges: structuredClone(localEdges.current),
        viewport: { ...view.current },
        pending: structuredClone(pending.current),
      },
    };
    journal.current = journal.current
      .catch(() => {})
      .then(() =>
        privateInvalid.current ? undefined : branchJournal.write(record),
      );
    try {
      await journal.current;
    } catch {
      if (alive.current)
        setMessage("本地恢复不可用；请保存到服务端后再关闭页面。");
    }
  }, [recoveryContext, replicaId, dirty]);
  useEffect(() => {
    void persistCanvas();
  }, [
    nodes,
    edges,
    camera,
    base,
    recoveryContext,
    saving,
    unconfirmed,
    persistCanvas,
  ]);

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
    if (id !== selected) setDetails(false);
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
    if (privateInvalid.current) return;
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
    if (busy.current || pending.current || conflict || privateInvalid.current)
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
    const left = Math.min(...selected.map((n) => n.x)) - 24,
      top = Math.min(...selected.map((n) => n.y)) - 48;
    const width =
      Math.max(...selected.map((n) => n.x + (n.width ?? 288))) - left + 24;
    const height =
      Math.max(...selected.map((n) => n.y + (n.height ?? 200))) - top + 24;
    if (width > 100000 || height > 100000) {
      setMessage("所选节点距离过远，请缩小分组范围。");
      return;
    }
    const group = {
      ...createResourceNode("group", { x: limit(left), y: limit(top) }),
      width,
      height,
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
    if (!current || current.node_type !== "group") return;
    if (
      local.current.some(
        (n) => n.parent_id === current.id && !isEditableNode(n),
      )
    ) {
      setMessage("分组包含只读成员，不能解除关系。");
      return;
    }
    edit(
      local.current.map((n) =>
        n.parent_id === current.id ? { ...n, parent_id: undefined } : n,
      ),
    );
  }
  function connectSelection() {
    const selected = local.current.filter((n) => selectedIDs.includes(n.id));
    if (
      selected.length !== 2 ||
      selected.some((n) => !isEditableNode(n) || isDraft(n))
    )
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
  function draft(patch: Partial<Draft>) {
    const n = local.current.find((n) => n.id === selected);
    if (n && isDraft(n))
      update(n.id, { config: { draft: { ...n.config.draft, ...patch } } });
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
  function addMaterials(assets: API.AssetSummary[]) {
    if (local.current.length + assets.length > 2000) {
      setMessage("画布已达到 2000 个节点上限，请减少所选素材。");
      return;
    }
    try {
      const added = assets.map((a, i) =>
        createMediaNode(a, project.id, local.current.length + i),
      );
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
      edit([...local.current, ...added]);
      selectNode(added.at(-1)?.id);
      setMaterials(null);
      setMessage(`已添加 ${added.length} 个素材，请保存画布。`);
    } catch {
      setMessage("素材信息不完整或不属于当前项目，请重新读取后添加。");
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
    const minX = Math.min(...visible.map((n) => n.x)),
      minY = Math.min(...visible.map((n) => n.y));
    const maxX = Math.max(...visible.map((n) => n.x + (n.width ?? 288))),
      maxY = Math.max(...visible.map((n) => n.y + (n.height ?? 200)));
    const { width, height, left, top } = workspace;
    const zoom = Math.max(
      0.08,
      Math.min(
        selectionOnly ? 1 : 0.5,
        (width - 96) / (maxX - minX),
        (height - 192) / (maxY - minY),
      ),
    );
    changeCamera({
      x: left + width / 2 - ((minX + maxX) / 2) * zoom,
      y: top + height / 2 - ((minY + maxY) / 2) * zoom,
      zoom,
    });
  }
  function beginDrag(e: PointerEvent<HTMLButtonElement>, node: Node) {
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
  function moveDrag(e: PointerEvent<HTMLButtonElement>) {
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
    edit(
      local.current.map((node) =>
        moved.has(node.id) ? { ...node, ...moved.get(node.id)! } : node,
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
      if (alive.current) setConflict({ server });
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
      let result = await saveOnce();
      while (
        result &&
        alive.current &&
        !privateInvalid.current &&
        commandBatch(result, local.current, {
          edges: localEdges.current,
          viewport: view.current,
        }).commands?.length
      ) {
        result = await saveOnce();
      }
      return result;
    })();
    saveFlight.current = flight;
    try {
      return await flight;
    } finally {
      saveFlight.current = null;
    }
  }
  async function saveOnce() {
    if (busy.current || conflict || privateInvalid.current) return;
    const base = savedBase.current;
    let operation = pending.current;
    if (!operation) {
      try {
        const frozen = structuredClone(local.current);
        const frozenEdges = structuredClone(localEdges.current);
        const frozenViewport = { ...view.current };
        const body = commandBatch(base, frozen, {
          edges: frozenEdges,
          viewport: frozenViewport,
        });
        if (!body.commands?.length) return base;
        operation = {
          key: crypto.randomUUID(),
          base,
          nodes: frozen,
          body,
          edges: frozenEdges,
          viewport: frozenViewport,
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
    await persistCanvas();
    const controller = new AbortController();
    request.current = controller;
    try {
      const value = await applyCanvasCommands({ id: base.id }, operation.body, {
        headers: { "Idempotency-Key": operation.key },
        signal: controller.signal,
      });
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
      if (
        canonical(view.current) ===
        canonical(
          operation.viewport ?? operation.base.viewport ?? defaultViewport,
        )
      )
        changeCamera(committed.viewport ?? defaultViewport);
      setBase(committed);
      pending.current = null;
      setUnconfirmed(false);
      return committed;
    } catch (error) {
      if (!alive.current || controller.signal.aborted) return;
      if (error instanceof RequestError && error.code === "revision_conflict") {
        pending.current = null;
        setUnconfirmed(false);
        setConflict({ server: null });
        setMessage(
          "画布已被其他窗口修改，本地编辑已保留。请对照服务端内容后明确选择。",
        );
        await readConflict();
      } else if (
        error instanceof RequestError &&
        [401, 403, 404, 409, 422].includes(error.status)
      ) {
        pending.current = null;
        setUnconfirmed(false);
        setMessage(
          error.status === 401
            ? "登录已失效，本地编辑仍保留，请重新登录后重开画布。"
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
      if (alive.current) setSaving(false);
    }
  }
  useEffect(() => {
    saveLatest.current = save;
  });
  useEffect(() => {
    if (!dirty || saving || unconfirmed || conflict || identityInvalid) return;
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
    unconfirmed,
    conflict,
    identityInvalid,
  ]);
  async function recoverCanvas(branch: LocalBranch) {
    if (busy.current || privateInvalid.current) return;
    if (pending.current) {
      setMessage("请先重试原保存，确认结果后再恢复其他分支。");
      return;
    }
    const expectedNodes = local.current;
    const expectedEdges = localEdges.current;
    const expectedCamera = view.current;
    busy.current = true;
    setSaving(true);
    try {
      const value = branch.ownerState as {
        base: Canvas;
        nodes: Node[];
        pending: Pending | null;
        edges?: API.Edge[];
        viewport?: CanvasViewport;
      };
      let baseline = requireCanvas(value.base, project.id, initial.id);
      commandBatch(baseline, value.nodes, {
        edges: value.edges ?? baseline.edges,
        viewport: value.viewport ?? baseline.viewport,
      });
      if (value.pending) {
        if (
          canonical(value.pending.body) !==
          canonical(
            commandBatch(value.pending.base, value.pending.nodes, {
              edges: value.pending.edges,
              viewport: value.pending.viewport,
            }),
          )
        )
          throw new Error("恢复请求不完整");
        const replay = await applyCanvasCommands(
          { id: initial.id },
          value.pending.body,
          { headers: { "Idempotency-Key": value.pending.key } },
        );
        baseline = requireSaveResult(
          replay,
          value.pending.base,
          value.pending.nodes,
          value.pending.body,
        );
      }
      const server = requireCanvas(
        await getCanvas({ id: initial.id }),
        project.id,
        initial.id,
      );
      if (!alive.current || privateInvalid.current) return;
      if (
        local.current !== expectedNodes ||
        localEdges.current !== expectedEdges ||
        view.current !== expectedCamera
      )
        throw new Error("恢复期间有新编辑，未覆盖；原分支仍保留。");
      history.reset();
      edit(value.nodes, undefined, value.edges ?? baseline.edges, false);
      changeCamera(value.viewport ?? baseline.viewport ?? defaultViewport);
      if (server.revision !== baseline.revision) {
        setConflict({ server });
        setMessage("已恢复所选本地分支，请处理服务端冲突。");
      } else setBase(server);
    } catch (e) {
      if (alive.current && !privateInvalid.current)
        setMessage(
          e instanceof Error ? e.message : "本地恢复失败；原分支仍保留。",
        );
    } finally {
      busy.current = false;
      if (alive.current) setSaving(false);
    }
  }
  function seedForNode(node: DraftNode): GenerationSeed {
    let composer = node.config.draft.composer;
    if (!composer) {
      composer = emptyPromptDraft();
      composer.document = {
        type: "doc",
        content: node.config.draft.prompt.split("\n").map((text) => ({
          type: "paragraph",
          ...(text ? { content: [{ type: "text" as const, text }] } : {}),
        })),
      };
      composer.params = {
        aspect_ratio: node.config.draft.aspect_ratio,
        output_count: node.config.draft.candidate_count,
        ...(node.config.draft.negative_prompt
          ? { negative_prompt: node.config.draft.negative_prompt }
          : {}),
      };
    }
    const ownerNode = node.id;
    return {
      project,
      prompt: "",
      composer,
      capability: node.node_type === "video_generation" ? "video" : "image",
      source: {
        canvas_id: initial.id,
        node_id: ownerNode,
        revision: base.revision,
      },
      canvasOwner: {
        contextKey: `canvas/${initial.id}/${ownerNode}`,
        draft: composer,
        onChange: (value: PromptDraft) => {
          const n = local.current.find((n) => n.id === ownerNode);
          if (n && isDraft(n))
            update(ownerNode, { config: { draft: richCanvasDraft(value) } });
        },
        flush: async () => {
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
            kind: "canvas_input",
            canvas_id: committed.id,
            node_id: ownerNode,
            canvas_revision: committed.revision,
            input_revision: owner.input_revision,
          };
        },
      },
    };
  }

  async function adoptResult(id: string, kind: string) {
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
    const v = view.current,
      next = Math.max(0.08, Math.min(8, zoom));
    const x = workspace.centerX,
      y = workspace.centerY;
    changeCamera({
      x: x - ((x - v.x) * next) / v.zoom,
      y: y - ((y - v.y) * next) / v.zoom,
      zoom: next,
    });
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
      if (event.defaultPrevented || event.isComposing || event.altKey) return;
      if (
        event.target instanceof Element &&
        event.target.closest(
          "input,textarea,select,[contenteditable=true],[role=dialog],[role=alertdialog],[role=menu],[role=listbox],[data-canvas-ai-panel]",
        )
      )
        return;
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
    if (!server) return;
    if (discard) {
      history.reset();
      edit(server.nodes, undefined, server.edges, false);
      changeCamera(server.viewport ?? defaultViewport);
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
    setMessage("");
  }
  const status = saving
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
          <EmptyDescription>登录状态已变化，本地编辑已保留。</EmptyDescription>
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
          status={status}
          saveLabel={
            saving ? "正在保存…" : unconfirmed ? "重试原保存" : "保存画布"
          }
          saving={saving}
          canSave={!conflict && (dirty || unconfirmed)}
          onSave={() => void save()}
          onSelect={(id) => selectNode(id)}
          onMessage={setMessage}
          onShortcuts={() => setShortcutsOpen(true)}
        />
      </header>
      {message && <FeedbackToast message={message} tone="info" />}
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
            。本地稿仍在编辑面板中。
          </p>
          {conflict.server && (
            <div className="max-h-40 overflow-auto rounded border p-3">
              <p className="mb-2 font-medium">服务端草稿</p>
              {conflict.server.nodes.filter(isDraft).map((n) => (
                <p key={n.id} className="break-words whitespace-pre-wrap">
                  {n.title} · {n.config.draft.aspect_ratio} ·{" "}
                  {n.config.draft.candidate_count} 个候选
                  <br />
                  {n.config.draft.prompt || "（空提示词）"}
                  <br />
                  {n.config.draft.negative_prompt}
                </p>
              ))}
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            {conflict.server ? (
              <>
                <Button
                  variant="outline"
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
        <BeefCanvas
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
            edges={showEdges ? edges : []}
            viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
            size={size}
            selectedIDs={selectedIDs}
            renderNode={(node, liveZoom) => (
              <CanvasNode
                key={node.id}
                node={node}
                projectID={project.id}
                selected={selectedIDs.includes(node.id)}
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
                onAddAdjacent={(kind, side) => {
                  const point = {
                    x: limit(
                      node.x +
                        (side === "left" ? -400 : (node.width ?? 320) + 80),
                    ),
                    y: node.y,
                  };
                  if (kind === "text") addText(point);
                  else add(kind, point);
                }}
                onEditTitle={() => {
                  selectNode(node.id);
                  if (node.node_type === "director") setOpenToolID(node.id);
                  else setDetails(true);
                }}
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
            )}
          />
        </BeefCanvas>
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
        data-multi={selectedIDs.length > 1}
        role="toolbar"
        aria-label="画布编辑操作"
        data-canvas-no-zoom
      >
        <Button
          size="sm"
          variant="outline"
          aria-label="撤销编辑"
          disabled={
            saving || unconfirmed || !!conflict || !history.canUndo(base)
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
            saving || unconfirmed || !!conflict || !history.canRedo(base)
          }
          onClick={() => stepHistory("redo")}
        >
          重做
        </Button>
        {selectedIDs.length > 1 && (
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={groupSelection}
              disabled={selectedIDs.some(
                (id) => !nodes.some((n) => n.id === id && isEditableNode(n)),
              )}
            >
              组合所选节点
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={connectSelection}
              disabled={
                selectedIDs.length !== 2 ||
                nodes.some(
                  (n) =>
                    selectedIDs.includes(n.id) &&
                    (!isEditableNode(n) || isDraft(n)),
                )
              }
            >
              连接所选节点
            </Button>
            <Button size="sm" variant="outline" onClick={disconnectSelection}>
              移除所选标注
            </Button>
          </>
        )}
      </div>
      <CanvasToolbar
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
      {current &&
        current.node_type !== "timeline" &&
        current.id !== openToolID && (
          <CanvasNodePanel
            rightInset={aiOpen ? effectiveAIWidth + 24 : 0}
            node={current}
            viewport={{ x: camera.x, y: camera.y, k: camera.zoom }}
            containerRef={board}
          >
            {current.node_type === "director" ? (
              <aside className="canvas-node-composer" data-canvas-no-zoom>
                <Button onClick={() => setOpenToolID(current.id)}>
                  打开导演台
                </Button>
              </aside>
            ) : current.node_type === "text" ||
              current.node_type === "group" ? (
              <aside
                className="canvas-node-composer"
                aria-label="节点编辑"
                data-canvas-no-zoom
              >
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="canvas-resource-title">
                      节点名称
                    </FieldLabel>
                    <Input
                      id="canvas-resource-title"
                      aria-label="节点名称"
                      value={current.title}
                      maxLength={128}
                      disabled={!isEditableNode(current)}
                      onChange={(event) =>
                        update(current.id, { title: event.target.value })
                      }
                    />
                  </Field>
                </FieldGroup>
                {current.node_type === "text" ? (
                  <Textarea
                    aria-label="文本内容"
                    value={current.config.text ?? ""}
                    maxLength={10000}
                    disabled={!isEditableNode(current)}
                    onChange={(event) =>
                      update(current.id, {
                        config: { text: event.target.value },
                      })
                    }
                  />
                ) : (
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!isEditableNode(current)}
                      onClick={() =>
                        update(current.id, {
                          config: { collapsed: !current.config.collapsed },
                        })
                      }
                    >
                      {current.config.collapsed ? "展开分组" : "收起分组"}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!isEditableNode(current)}
                      onClick={ungroup}
                    >
                      解除分组关系
                    </Button>
                  </div>
                )}
              </aside>
            ) : (
              <CanvasComposer
                node={current}
                details={details}
                onDetails={setDetails}
                onUpdate={(patch) => update(current.id, patch)}
                onDraft={draft}
                richComposer={
                  isDraft(current)
                    ? (() => {
                        const seed = seedForNode(current);
                        return (
                          <RichGeneration
                            key={current.id}
                            projectID={project.id}
                            seed={seed}
                            canvas={seed.canvasOwner}
                            presentation="canvas"
                            onAdopt={adoptResult}
                          />
                        );
                      })()
                    : undefined
                }
                onPreview={() => setPreview(current)}
                onContinue={() => {
                  if (
                    current.ref_id &&
                    ["image", "video", "audio"].includes(current.node_type)
                  )
                    setGeneration({
                      project,
                      prompt: "",
                      capability:
                        current.node_type === "image" ? "image" : "video",
                      reference: {
                        assetID: current.ref_id,
                        kind: current.node_type as "image" | "video" | "audio",
                      },
                    });
                }}
                generating={saving}
                blocked={!!conflict || unconfirmed}
              />
            )}
          </CanvasNodePanel>
        )}
      {recoveryBranches.length > 0 && (
        <div
          className="fixed bottom-4 left-4 z-40 max-w-lg space-y-2 rounded-lg border bg-background p-4"
          role="status"
        >
          <p>发现 {recoveryBranches.length} 份未同步画布分支</p>
          {recoveryBranches.map((branch) => (
            <div key={branch.replicaId} className="flex gap-2">
              <span>基线 r{branch.serverRevision}</span>
              <Button
                disabled={saving}
                onClick={() => void recoverCanvas(branch)}
              >
                恢复此分支
              </Button>
              <Button
                variant="ghost"
                onClick={() => {
                  void branchJournal.remove(branch).then((removed) => {
                    if (removed)
                      setRecoveryBranches((v) =>
                        v.filter((b) => b.replicaId !== branch.replicaId),
                      );
                    else setMessage("该分支已有新修改，未删除。");
                  });
                }}
              >
                丢弃此分支
              </Button>
            </div>
          ))}
        </div>
      )}
      {aiOpen && (
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
      {nodes.find((n) => n.id === openToolID)?.node_type === "director" && (
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
      {generation && (
        <GenerationDialog
          seed={generation}
          onClose={() => setGeneration(null)}
          onAdopt={adoptResult}
        />
      )}
      {materials && (
        <CanvasMaterialDialog
          projectID={project.id}
          initialTab={materials.tab}
          initialKind={materials.kind}
          onClose={() => setMaterials(null)}
          onAdd={addMaterials}
          onSubject={useSubject}
        />
      )}
      {upload && (
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
