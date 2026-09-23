"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent, MouseEvent, PointerEvent, WheelEvent } from "react";
import Link from "next/link";
import { ArrowLeft, AudioLines, Compass, FileText, Group, ImageIcon, Maximize2, Minus, Plus, Redo2, StickyNote, Undo2, Ungroup, Video } from "lucide-react";

import { Button } from "@/components/ui/button";
import { appApiErrorMessage } from "@/lib/server-state";
import { fitCanvas, MAX_ZOOM, MIN_ZOOM, zoomAtPoint, type CanvasViewport } from "./canvas-geometry";
import { CanvasInspector } from "./canvas-inspector";
import { CanvasMiniMap } from "./canvas-mini-map";
import { createNodes, type CanvasNode } from "./canvas-model";
import { CanvasNodeCard, CanvasNodeList } from "./canvas-node";

type NodeKind = GeneratedAPI.CanvasNode["kind"];
type Geometry = Pick<GeneratedAPI.CanvasNode, "x" | "y" | "width" | "height">;
type LayoutChange = { nodeId: string; before: Geometry; after: Geometry };
type NodeDrag = {
  pointerId: number;
  node: CanvasNode;
  startX: number;
  startY: number;
  mode: "move" | "resize";
  preview: (Geometry & { id: string }) | null;
};

const nodeSizes: Record<NodeKind, { width: number; height: number; title: string }> = {
  text: { width: 290, height: 190, title: "文本创作" },
  image: { width: 310, height: 230, title: "图片创作" },
  video: { width: 330, height: 240, title: "视频创作" },
  audio: { width: 300, height: 190, title: "音频创作" },
  group: { width: 400, height: 280, title: "分组" },
  note: { width: 260, height: 170, title: "便签" },
};

function geometry(node: GeneratedAPI.CanvasNode): Geometry {
  return { x: node.x, y: node.y, width: node.width, height: node.height };
}

function curve(from: CanvasNode, to: CanvasNode): string {
  const sx = from.x + from.width;
  const sy = from.y + from.height / 2;
  const ex = to.x;
  const ey = to.y + to.height / 2;
  const bend = Math.max(50, Math.abs(ex - sx) / 2);
  return `M ${sx} ${sy} C ${sx + bend} ${sy}, ${ex - bend} ${ey}, ${ex} ${ey}`;
}

export function CanvasBoard({ project, episodes, document, canEdit, onApply }: {
  project: API.ProjectResponse;
  episodes: API.EpisodeResponse[];
  document: GeneratedAPI.CanvasDocument;
  canEdit: boolean;
  onApply: (operations: GeneratedAPI.CanvasOperation[]) => Promise<GeneratedAPI.CanvasApplyResponse>;
}) {
  const nodes = useMemo(() => createNodes(project, episodes, document), [project, episodes, document]);
  const savedByID = useMemo(() => new Map(document.nodes.map((node) => [node.id, node])), [document.nodes]);
  const [viewport, setViewport] = useState<CanvasViewport>({ x: 0, y: 0, k: 1 });
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [selectedId, setSelectedId] = useState(`project:${project.id}`);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [connectingFrom, setConnectingFrom] = useState<string | null>(null);
  const [dragPreview, setDragPreview] = useState<(Geometry & { id: string }) | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [undoStack, setUndoStack] = useState<LayoutChange[]>([]);
  const [redoStack, setRedoStack] = useState<LayoutChange[]>([]);
  const stageRef = useRef<HTMLDivElement>(null);
  const panRef = useRef<{ pointerId: number; x: number; y: number } | null>(null);
  const dragRef = useRef<NodeDrag | null>(null);
  const ignoreClickRef = useRef(false);
  const fittedRef = useRef(false);

  const visibleNodes = useMemo(() => {
    if (!dragPreview) return nodes;
    const source = nodes.find((node) => node.id === dragPreview.id);
    if (!source) return nodes;
    const dx = dragPreview.x - source.x;
    const dy = dragPreview.y - source.y;
    return nodes.map((node) => {
      if (node.id === source.id) return { ...node, ...dragPreview };
      if (source.kind !== "group" || !node.saved) return node;
      let parentID = node.saved.group_id;
      while (parentID) {
        if (parentID === source.id) return { ...node, x: node.x + dx, y: node.y + dy };
        parentID = savedByID.get(parentID)?.group_id;
      }
      return node;
    });
  }, [dragPreview, nodes, savedByID]);
  const selectedNode = visibleNodes.find((node) => node.id === selectedId) ?? visibleNodes[0];
  const nodeByID = useMemo(() => new Map(visibleNodes.map((node) => [node.id, node])), [visibleNodes]);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const observer = new ResizeObserver(([entry]) => {
      const nextSize = { width: entry.contentRect.width, height: entry.contentRect.height };
      setSize(nextSize);
      if (!fittedRef.current && nextSize.width && nextSize.height) {
        setViewport(fitCanvas(nodes, nextSize));
        fittedRef.current = true;
      }
    });
    observer.observe(stage);
    return () => observer.disconnect();
  }, [nodes]);

  const fit = useCallback(() => setViewport(fitCanvas(nodes, size)), [nodes, size]);
  const zoom = useCallback((factor: number) => setViewport((current) =>
    zoomAtPoint(current, current.k * factor, { x: size.width / 2, y: size.height / 2 }),
  ), [size]);

  async function apply(operations: GeneratedAPI.CanvasOperation[]) {
    if (busy || !canEdit) return null;
    setBusy(true);
    setError(null);
    try {
      return await onApply(operations);
    } catch (cause) {
      setError(appApiErrorMessage(cause));
      return null;
    } finally {
      setBusy(false);
    }
  }

  function createNode(kind: NodeKind) {
    const definition = nodeSizes[kind];
    const x = size.width ? (size.width / 2 - viewport.x) / viewport.k - definition.width / 2 : 760;
    const y = size.height ? (size.height / 2 - viewport.y) / viewport.k - definition.height / 2 : 120;
    const node: GeneratedAPI.CanvasNode = {
      id: crypto.randomUUID(), kind, title: definition.title, content: "", prompt: "",
      x: Math.round(x), y: Math.round(y), width: definition.width, height: definition.height, revision: 0,
    };
    void apply([{ kind: "create_node", node, expected_revision: 0 }]).then((result) => {
      if (result) { setSelectedId(node.id); setSelectedIds([node.id]); }
    });
  }

  function updateNode(node: GeneratedAPI.CanvasNode, changes: Partial<GeneratedAPI.CanvasNode>, trackLayout = false) {
    const next = { ...node, ...changes };
    void apply([{ kind: "update_node", node: next, expected_revision: node.revision }]).then((result) => {
      if (result && trackLayout) {
        setUndoStack((stack) => [...stack.slice(-49), { nodeId: node.id, before: geometry(node), after: geometry(next) }]);
        setRedoStack([]);
      }
    });
  }

  function removeNode(node: GeneratedAPI.CanvasNode) {
    if (document.nodes.some((child) => child.group_id === node.id)) {
      setError("请先拆分组，再删除分组卡。");
      return;
    }
    void apply([{ kind: "delete_node", node_id: node.id, expected_revision: node.revision }]).then((result) => {
      if (result) {
        setSelectedId(`project:${project.id}`); setSelectedIds([]);
        setUndoStack((stack) => stack.filter((entry) => entry.nodeId !== node.id));
        setRedoStack((stack) => stack.filter((entry) => entry.nodeId !== node.id));
      }
    });
  }

  function duplicateNode(node: GeneratedAPI.CanvasNode) {
    const copy: GeneratedAPI.CanvasNode = { ...node, id: crypto.randomUUID(), revision: 0, group_id: undefined, x: node.x + 40, y: node.y + 40 };
    void apply([{ kind: "create_node", node: copy, expected_revision: 0 }]).then((result) => {
      if (result) { setSelectedId(copy.id); setSelectedIds([copy.id]); }
    });
  }

  function connectNodes(fromID: string, toID: string) {
    setConnectingFrom(null);
    if (fromID === toID || document.connections.some((edge) => edge.from_node_id === fromID && edge.to_node_id === toID)) return;
    const edge: GeneratedAPI.CanvasConnection = { id: crypto.randomUUID(), from_node_id: fromID, to_node_id: toID, purpose: "visual", revision: 0 };
    void apply([{ kind: "create_edge", connection: edge, expected_revision: 0 }]);
  }

  function groupSelected() {
    const chosen = selectedIds.map((id) => nodeByID.get(id)).filter((node): node is CanvasNode => Boolean(node?.saved && !node.saved.group_id));
    if (chosen.length < 2) return;
    const left = Math.min(...chosen.map((node) => node.x)) - 24;
    const top = Math.min(...chosen.map((node) => node.y)) - 52;
    const right = Math.max(...chosen.map((node) => node.x + node.width)) + 24;
    const bottom = Math.max(...chosen.map((node) => node.y + node.height)) + 24;
    const group: GeneratedAPI.CanvasNode = {
      id: crypto.randomUUID(), kind: "group", title: "分组", content: "", prompt: "",
      x: left, y: top, width: Math.max(120, right - left), height: Math.max(80, bottom - top), revision: 0,
    };
    const operations: GeneratedAPI.CanvasOperation[] = [
      { kind: "create_node", node: group, expected_revision: 0 },
      ...chosen.map((node) => ({ kind: "update_node" as const, node: { ...node.saved!, group_id: group.id, x: node.x - group.x, y: node.y - group.y }, expected_revision: node.saved!.revision })),
    ];
    void apply(operations).then((result) => {
      if (result) { setSelectedId(group.id); setSelectedIds([group.id]); }
    });
  }

  function ungroup(node: GeneratedAPI.CanvasNode) {
    const children = document.nodes.filter((child) => child.group_id === node.id);
    const parent = node.group_id ? nodeByID.get(node.group_id) : undefined;
    const operations: GeneratedAPI.CanvasOperation[] = children.map((child) => {
      const world = nodeByID.get(child.id)!;
      return { kind: "update_node", expected_revision: child.revision, node: { ...child, group_id: node.group_id, x: world.x - (parent?.x ?? 0), y: world.y - (parent?.y ?? 0) } };
    });
    operations.push({ kind: "delete_node", node_id: node.id, expected_revision: node.revision });
    void apply(operations).then((result) => {
      if (result) { setSelectedId(`project:${project.id}`); setSelectedIds([]); }
    });
  }

  function changeLayout(change: LayoutChange, target: "before" | "after") {
    const current = document.nodes.find((node) => node.id === change.nodeId);
    if (!current) return;
    void apply([{ kind: "update_node", node: { ...current, ...change[target] }, expected_revision: current.revision }]).then((result) => {
      if (!result) return;
      if (target === "before") { setUndoStack((stack) => stack.slice(0, -1)); setRedoStack((stack) => [...stack, change]); }
      else { setRedoStack((stack) => stack.slice(0, -1)); setUndoStack((stack) => [...stack, change]); }
    });
  }

  function selectNode(event: MouseEvent<HTMLButtonElement>, node: CanvasNode) {
    if (ignoreClickRef.current) { ignoreClickRef.current = false; return; }
    if (connectingFrom && node.saved) { connectNodes(connectingFrom, node.id); return; }
    setSelectedId(node.id);
    if (event.shiftKey && node.saved) setSelectedIds((ids) => ids.includes(node.id) ? ids.filter((id) => id !== node.id) : [...ids, node.id]);
    else setSelectedIds(node.saved ? [node.id] : []);
  }

  function nodePointerDown(event: PointerEvent<HTMLButtonElement>, node: CanvasNode) {
    if (!node.saved || !canEdit || busy || event.button !== 0) return;
    event.stopPropagation();
    dragRef.current = { pointerId: event.pointerId, node, startX: event.clientX, startY: event.clientY, mode: (event.target as HTMLElement).closest("[data-canvas-resize]") ? "resize" : "move", preview: null };
    event.currentTarget.setPointerCapture(event.pointerId);
  }

  function nodePointerMove(event: PointerEvent<HTMLButtonElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    const dx = (event.clientX - drag.startX) / viewport.k;
    const dy = (event.clientY - drag.startY) / viewport.k;
    if (Math.abs(dx) + Math.abs(dy) <= 3) return;
    const node = drag.node;
    drag.preview = drag.mode === "move"
      ? { id: node.id, x: node.x + dx, y: node.y + dy, width: node.width, height: node.height }
      : { id: node.id, x: node.x, y: node.y, width: Math.max(120, Math.min(3000, node.width + dx)), height: Math.max(80, Math.min(3000, node.height + dy)) };
    setDragPreview(drag.preview);
  }

  function nodePointerUp(event: PointerEvent<HTMLButtonElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    dragRef.current = null;
    if (!drag.preview || !drag.node.saved) return;
    ignoreClickRef.current = true;
    setTimeout(() => { ignoreClickRef.current = false; }, 0);
    const saved = drag.node.saved;
    const changes = drag.mode === "move"
      ? { x: saved.x + (drag.preview.x - drag.node.x), y: saved.y + (drag.preview.y - drag.node.y) }
      : { width: drag.preview.width, height: drag.preview.height };
    setDragPreview(null);
    updateNode(saved, changes, true);
  }

  function panStart(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0 || (event.target as HTMLElement).closest("[data-canvas-node]")) return;
    panRef.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY };
    event.currentTarget.setPointerCapture(event.pointerId);
  }
  function panMove(event: PointerEvent<HTMLDivElement>) {
    const pan = panRef.current;
    if (!pan || pan.pointerId !== event.pointerId) return;
    const dx = event.clientX - pan.x;
    const dy = event.clientY - pan.y;
    panRef.current = { pointerId: pan.pointerId, x: event.clientX, y: event.clientY };
    setViewport((current) => ({ ...current, x: current.x + dx, y: current.y + dy }));
  }
  function wheel(event: WheelEvent<HTMLDivElement>) {
    event.preventDefault();
    const bounds = event.currentTarget.getBoundingClientRect();
    setViewport((current) => zoomAtPoint(current, current.k * Math.exp(-event.deltaY * 0.001), { x: event.clientX - bounds.left, y: event.clientY - bounds.top }));
  }
  function keyDown(event: KeyboardEvent<HTMLDivElement>) {
    if ((event.target as HTMLElement).closest("input,textarea,[contenteditable='true']")) return;
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "z") {
      event.preventDefault();
      if (event.shiftKey && redoStack.at(-1)) changeLayout(redoStack.at(-1)!, "after");
      else if (undoStack.at(-1)) changeLayout(undoStack.at(-1)!, "before");
    } else if (event.key === "+" || event.key === "=") zoom(1.2);
    else if (event.key === "-") zoom(1 / 1.2);
    else if (event.key === "0") fit();
    else if ((event.key === "Delete" || event.key === "Backspace") && selectedNode.saved && canEdit) { event.preventDefault(); removeNode(selectedNode.saved); }
    else if (event.key.startsWith("Arrow")) {
      event.preventDefault();
      const delta = 60;
      setViewport((current) => ({ ...current, x: current.x + (event.key === "ArrowRight" ? -delta : event.key === "ArrowLeft" ? delta : 0), y: current.y + (event.key === "ArrowDown" ? -delta : event.key === "ArrowUp" ? delta : 0) }));
    }
  }

  const gridSize = 24 * viewport.k;
  const contextualEpisodes = visibleNodes.slice(1, episodes.length + 1);
  const canGroup = selectedIds.filter((id) => { const node = nodeByID.get(id); return node?.saved && !node.saved.group_id; }).length >= 2;

  return (
    <section aria-label="项目画布" className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-4 px-1">
        <div>
          <Link className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground" href={`/projects/${project.id}`}><ArrowLeft className="size-4" aria-hidden="true" /> 返回项目</Link>
          <p className="mt-5 text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">Canvas / 项目空间</p>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">{project.name}</h1>
          <p className="mt-2 text-sm text-muted-foreground">创建和组织媒体节点，所有修改由项目服务保存。</p>
        </div>
        <span className="rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">{document.nodes.length} 个创作节点 · {episodes.length} 集</span>
      </div>
      {error ? <p aria-live="polite" className="rounded-xl bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</p> : null}

      <div className="relative min-h-[590px] overflow-hidden rounded-2xl bg-muted/45 md:h-[min(72vh,800px)]" onKeyDown={keyDown}>
        <div aria-label="无限画布；拖动平移，滚轮缩放，方向键移动，0 重置视图" className="absolute inset-0 touch-none overflow-hidden outline-none focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring max-md:hidden" onPointerCancel={() => { panRef.current = null; dragRef.current = null; setDragPreview(null); }} onPointerDown={panStart} onPointerMove={panMove} onPointerUp={() => { panRef.current = null; }} onWheel={wheel} ref={stageRef} role="application" style={{ backgroundImage: "radial-gradient(circle, color-mix(in oklch, var(--foreground) 20%, transparent) 0.8px, transparent 1px)", backgroundPosition: `${viewport.x}px ${viewport.y}px`, backgroundSize: `${gridSize}px ${gridSize}px`, cursor: "grab" }} tabIndex={0}>
          <div className="absolute left-0 top-0 origin-top-left" style={{ transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.k})` }}>
            <svg aria-hidden="true" className="pointer-events-none absolute left-0 top-0 overflow-visible" height="1" width="1">
              {contextualEpisodes.map((node) => <path className="stroke-foreground/25" d={curve(visibleNodes[0], node)} fill="none" key={node.id} strokeWidth="2" />)}
              {document.connections.map((edge) => { const from = nodeByID.get(edge.from_node_id); const to = nodeByID.get(edge.to_node_id); return from && to ? <path className="stroke-foreground/35" d={curve(from, to)} fill="none" key={edge.id} strokeWidth="2" /> : null; })}
            </svg>
            {visibleNodes.map((node) => <CanvasNodeCard canEdit={canEdit} key={node.id} node={node} onClick={(event) => selectNode(event, node)} onPointerDown={(event) => nodePointerDown(event, node)} onPointerMove={nodePointerMove} onPointerUp={nodePointerUp} selected={selectedId === node.id || selectedIds.includes(node.id)} />)}
          </div>
        </div>

        <div className="absolute left-5 top-5 hidden items-center gap-2 rounded-xl bg-background/95 p-1.5 shadow-sm backdrop-blur-sm md:flex">
          <Button aria-label="缩小画布" disabled={viewport.k <= MIN_ZOOM} onClick={() => zoom(1 / 1.2)} size="icon" variant="ghost"><Minus aria-hidden="true" /></Button>
          <span className="w-11 text-center text-xs tabular-nums text-muted-foreground">{Math.round(viewport.k * 100)}%</span>
          <Button aria-label="放大画布" disabled={viewport.k >= MAX_ZOOM} onClick={() => zoom(1.2)} size="icon" variant="ghost"><Plus aria-hidden="true" /></Button>
          <Button aria-label="适合画布内容" onClick={fit} size="icon" variant="ghost"><Maximize2 aria-hidden="true" /></Button>
        </div>
        {canEdit ? <div aria-label="创作节点工具" className="absolute right-5 top-5 z-10 flex max-w-[calc(100%-10rem)] flex-wrap items-center justify-end gap-1 rounded-xl bg-background/95 p-1.5 shadow-sm backdrop-blur-sm max-md:left-4 max-md:right-4 max-md:max-w-none">
          <Button aria-label="新增文本节点" disabled={busy} onClick={() => createNode("text")} size="icon-sm" variant="ghost"><FileText aria-hidden="true" /></Button>
          <Button aria-label="新增图片节点" disabled={busy} onClick={() => createNode("image")} size="icon-sm" variant="ghost"><ImageIcon aria-hidden="true" /></Button>
          <Button aria-label="新增视频节点" disabled={busy} onClick={() => createNode("video")} size="icon-sm" variant="ghost"><Video aria-hidden="true" /></Button>
          <Button aria-label="新增音频节点" disabled={busy} onClick={() => createNode("audio")} size="icon-sm" variant="ghost"><AudioLines aria-hidden="true" /></Button>
          <Button aria-label="新增便签节点" disabled={busy} onClick={() => createNode("note")} size="icon-sm" variant="ghost"><StickyNote aria-hidden="true" /></Button>
          <Button aria-label="将选中节点编组" disabled={!canGroup || busy} onClick={groupSelected} size="icon-sm" variant="ghost"><Group aria-hidden="true" /></Button>
          <Button aria-label="拆分选中分组" disabled={selectedNode.kind !== "group" || !selectedNode.saved || busy} onClick={() => selectedNode.saved && ungroup(selectedNode.saved)} size="icon-sm" variant="ghost"><Ungroup aria-hidden="true" /></Button>
          <Button aria-label="撤销布局移动" disabled={!undoStack.length || busy} onClick={() => changeLayout(undoStack.at(-1)!, "before")} size="icon-sm" variant="ghost"><Undo2 aria-hidden="true" /></Button>
          <Button aria-label="重做布局移动" disabled={!redoStack.length || busy} onClick={() => changeLayout(redoStack.at(-1)!, "after")} size="icon-sm" variant="ghost"><Redo2 aria-hidden="true" /></Button>
        </div> : null}
        <div className="absolute left-5 top-20 hidden max-w-[220px] rounded-xl bg-background/90 px-3 py-2 text-xs text-muted-foreground shadow-sm backdrop-blur-sm md:block"><Compass className="mr-1 inline size-3.5" aria-hidden="true" /> 拖动平移 · 滚轮缩放</div>

        <CanvasInspector canEdit={canEdit} connecting={connectingFrom === selectedNode.id} connections={document.connections} key={`${selectedNode.id}:${selectedNode.saved?.revision ?? 0}`} node={selectedNode} nodes={visibleNodes} onConnect={() => setConnectingFrom((current) => current === selectedNode.id ? null : selectedNode.id)} onDelete={() => selectedNode.saved && removeNode(selectedNode.saved)} onDisconnect={(edge) => void apply([{ kind: "delete_edge", connection_id: edge.id, expected_revision: edge.revision }])} onDuplicate={() => selectedNode.saved && duplicateNode(selectedNode.saved)} onSave={(changes) => selectedNode.saved && updateNode(selectedNode.saved, changes)} pending={busy} />
        <CanvasMiniMap nodes={visibleNodes} onNavigate={(worldX, worldY) => setViewport((current) => ({ ...current, x: size.width / 2 - worldX * current.k, y: size.height / 2 - worldY * current.k }))} size={size} viewport={viewport} />
        <CanvasNodeList nodes={visibleNodes} onSelect={(id) => { setSelectedId(id); setSelectedIds(savedByID.has(id) ? [id] : []); }} selectedId={selectedNode.id} />
      </div>
    </section>
  );
}
