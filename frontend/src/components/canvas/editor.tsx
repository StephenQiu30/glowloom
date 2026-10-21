/* eslint-disable @next/next/no-img-element -- Local user media stays in the browser; no image server upload. */
"use client";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Input } from "@/components/ui/input";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  MousePointer2,
  Hand,
  Type,
  Upload,
  Undo2,
  Redo2,
  Copy,
  Trash2,
  Group,
  Ungroup,
  Maximize,
  Minus,
  Plus,
  Download,
  FolderOpen,
  Crop,
  Grid2X2,
  Scaling,
  ImageIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { InfiniteCanvas } from "./infinite-canvas";
import { CanvasNode } from "./node";
import { ConnectionPath, ActiveConnectionPath } from "./connections";
import { Minimap } from "./minimap";
import { ImageTools } from "./image-tools";
import {
  applyGroupSelection,
  applyUngroupSelection,
  collectGroupMemberNodes,
  getGroupWrapRect,
  nodeBounds,
  normalizeConnection,
  findContainingGroupId,
} from "./geometry";
import {
  createNode,
  downloadFile,
  duplicateSelection,
  emptyDocument,
  parseDocument,
  readMedia,
  removeSelection,
  selectionMembers,
} from "./document";
import type {
  CanvasDocument,
  CanvasGraph,
  CanvasNodeData,
  ConnectionHandle,
  Position,
  ViewportTransform,
} from "./types";
import type { CanvasBackgroundMode } from "./theme";
export function CanvasEditor() {
  const [availability, setAvailability] = useState({
    undo: false,
    redo: false,
  });
  const [doc, setDoc] = useState<CanvasDocument>(emptyDocument);
  const current = useRef(doc);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [tool, setTool] = useState<"select" | "pan">("select");
  const [background, setBackground] = useState<CanvasBackgroundMode>("lines");
  const [map, setMap] = useState(false);
  const [size, setSize] = useState({ width: 1000, height: 700 });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<CanvasNodeData | null>(null);
  const [imageTool, setImageTool] = useState<{
    node: CanvasNodeData;
    mode: "crop" | "split" | "resize";
  } | null>(null);
  const [box, setBox] = useState<{
    start: Position;
    end: Position;
  } | null>(null);
  const [connection, setConnection] = useState<{
    handle: ConnectionHandle;
    point: Position;
  } | null>(null);
  const spacePressed = useRef(false);
  const surface = useRef<HTMLDivElement>(null);
  const root = useRef<HTMLDivElement>(null);
  const files = useRef<HTMLInputElement>(null);
  const projectFile = useRef<HTMLInputElement>(null);
  const replaceID = useRef<string | null>(null);
  const history = useRef<{
    past: CanvasDocument[];
    future: CanvasDocument[];
  }>({
    past: [],
    future: [],
  });
  const clipboard = useRef<CanvasGraph | null>(null);
  const gesture = useRef<{
    abort: AbortController;
    before: CanvasDocument;
  } | null>(null);
  const mounted = useRef(true);
  const apply = useCallback((next: CanvasDocument) => {
    current.current = next;
    setDoc(next);
  }, []);
  const remember = (before: CanvasDocument) => {
    history.current.past = [...history.current.past.slice(-49), before];
    history.current.future = [];
    setAvailability({ undo: true, redo: false });
  };
  const commit = (graph: CanvasGraph) => {
    remember(current.current);
    apply({
      ...current.current,
      nodes: graph.nodes,
      connections: graph.connections,
    });
  };
  const viewportChange = useCallback(
    (viewport: ViewportTransform) => apply({ ...current.current, viewport }),
    [apply],
  );
  const clearSelection = useCallback(() => setSelected(new Set()), []);
  useEffect(() => {
    mounted.current = true;
    const container = surface.current;
    if (!container) return;
    const observer = new ResizeObserver(([entry]) =>
      setSize({
        width: entry.contentRect.width,
        height: entry.contentRect.height,
      }),
    );
    observer.observe(container);
    const leave = (event: BeforeUnloadEvent) => {
      if (current.current.nodes.length) event.preventDefault();
    };
    const releaseSpace = () => {
      spacePressed.current = false;
    };
    window.addEventListener("blur", releaseSpace);
    window.addEventListener("beforeunload", leave);
    return () => {
      mounted.current = false;
      observer.disconnect();
      gesture.current?.abort.abort();
      window.removeEventListener("beforeunload", leave);
      window.removeEventListener("blur", releaseSpace);
    };
  }, []);
  const world = (x: number, y: number): Position => {
    const rect = surface.current?.getBoundingClientRect();
    const v = current.current.viewport;
    return {
      x: (x - (rect?.left || 0) - v.x) / v.k,
      y: (y - (rect?.top || 0) - v.y) / v.k,
    };
  };
  const center = () =>
    world(
      (surface.current?.getBoundingClientRect().left || 0) + size.width / 2,
      (surface.current?.getBoundingClientRect().top || 0) + size.height / 2,
    );
  const add = (nodes: CanvasNodeData[]) => {
    const d = current.current;
    commit({ nodes: [...d.nodes, ...nodes], connections: d.connections });
    setSelected(new Set(nodes.map((n) => n.id)));
  };
  const undo = () => {
    const previous = history.current.past.pop();
    if (!previous) return;
    history.current.future.push(current.current);
    setAvailability({ undo: !!history.current.past.length, redo: true });
    apply(previous);
    clearSelection();
  };
  const redo = () => {
    const next = history.current.future.pop();
    if (!next) return;
    history.current.past.push(current.current);
    setAvailability({ undo: true, redo: !!history.current.future.length });
    apply(next);
    clearSelection();
  };
  const remove = () => {
    if (selected.size) {
      commit(removeSelection(current.current, selected));
      clearSelection();
    }
  };
  const duplicate = () => {
    const copy = duplicateSelection(current.current, selected);
    if (!copy.nodes.length) return;
    const d = current.current;
    commit({
      nodes: [...d.nodes, ...copy.nodes],
      connections: [...d.connections, ...copy.connections],
    });
    setSelected(new Set(copy.nodes.map((n) => n.id)));
  };
  const group = () => {
    const d = current.current;
    const members = collectGroupMemberNodes(selected, d.nodes);
    if (members.length < 2) return;
    const rect = getGroupWrapRect(members);
    const n = {
      ...createNode("group", center()),
      position: { x: rect.x, y: rect.y },
      width: rect.width,
      height: rect.height,
    };
    const next = applyGroupSelection(selected, d.nodes, d.connections, n);
    if (next) {
      commit(next);
      setSelected(new Set(next.selectedIds));
    }
  };
  const ungroup = () => {
    const next = applyUngroupSelection(
      selected,
      current.current.nodes,
      current.current.connections,
    );
    if (next) {
      commit(next);
      setSelected(new Set(next.selectedIds));
    }
  };
  const fit = () => {
    const nodes = current.current.nodes;
    if (!nodes.length) return viewportChange({ x: 0, y: 0, k: 1 });
    const b = nodeBounds(nodes);
    const k = Math.min(
      1,
      Math.max(
        0.05,
        Math.min(
          (size.width - 120) / (b.right - b.left),
          (size.height - 200) / (b.bottom - b.top),
        ),
      ),
    );
    viewportChange({
      x: size.width / 2 - ((b.left + b.right) / 2) * k,
      y: size.height / 2 - ((b.top + b.bottom) / 2) * k,
      k,
    });
  };
  const zoom = (factor: number) => {
    const v = current.current.viewport;
    const k = Math.min(5, Math.max(0.05, v.k * factor));
    viewportChange({
      x: size.width / 2 - ((size.width / 2 - v.x) * k) / v.k,
      y: size.height / 2 - ((size.height / 2 - v.y) * k) / v.k,
      k,
    });
  };
  const startGesture = (
    event: ReactPointerEvent,
    move: (event: PointerEvent) => void,
    finish: (event: PointerEvent) => void,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    root.current?.focus({ preventScroll: true });
    gesture.current?.abort.abort();
    const abort = new AbortController();
    const before = current.current;
    gesture.current = { abort, before };
    const stop = (e: PointerEvent) => {
      if (e.pointerId !== event.pointerId) return;
      abort.abort();
      gesture.current = null;
      if (e.type === "pointercancel") apply(before);
      else finish(e);
      setBox(null);
      setConnection(null);
    };
    window.addEventListener(
      "pointermove",
      (e) => {
        if (e.pointerId === event.pointerId) move(e);
      },
      { signal: abort.signal },
    );
    window.addEventListener("pointerup", stop, { signal: abort.signal });
    window.addEventListener("pointercancel", stop, { signal: abort.signal });
    window.addEventListener(
      "blur",
      () => {
        abort.abort();
        gesture.current = null;
        apply(before);
        setBox(null);
        setConnection(null);
      },
      { signal: abort.signal, once: true },
    );
  };
  const moveNode = (event: ReactPointerEvent, node: CanvasNodeData) => {
    if (tool === "pan" || spacePressed.current) return;
    const selection = event.shiftKey
      ? new Set(selected)
      : selected.has(node.id)
        ? new Set(selected)
        : new Set<string>();
    if (event.shiftKey && selection.has(node.id)) {
      selection.delete(node.id);
      setSelected(selection);
      event.stopPropagation();
      return;
    }
    selection.add(node.id);
    setSelected(selection);
    const d = current.current;
    const ids = selectionMembers(d, selection);
    const start = { x: event.clientX, y: event.clientY };
    let moved = false;
    startGesture(
      event,
      (e) => {
        const dx = (e.clientX - start.x) / d.viewport.k;
        const dy = (e.clientY - start.y) / d.viewport.k;
        moved ||= Math.abs(dx) + Math.abs(dy) > 2;
        if (moved)
          apply({
            ...d,
            nodes: d.nodes.map((n) =>
              ids.has(n.id)
                ? {
                    ...n,
                    position: { x: n.position.x + dx, y: n.position.y + dy },
                  }
                : n,
            ),
          });
      },
      () => {
        if (!moved) return;
        remember(d);
        const next = current.current;
        apply({
          ...next,
          nodes: next.nodes.map((n) =>
            ids.has(n.id) && n.type !== "group"
              ? {
                  ...n,
                  metadata: {
                    ...n.metadata,
                    groupId: findContainingGroupId(n, next.nodes),
                  },
                }
              : n,
          ),
        });
      },
    );
  };
  const resizeNode = (event: ReactPointerEvent, node: CanvasNodeData) => {
    const d = current.current;
    const start = { x: event.clientX, y: event.clientY };
    let changed = false;
    // Same ratio behavior as upstream: media preserves ratio; text/groups resize freely.
    startGesture(
      event,
      (e) => {
        let width = Math.max(
          80,
          node.width + (e.clientX - start.x) / d.viewport.k,
        );
        let height = Math.max(
          60,
          node.height + (e.clientY - start.y) / d.viewport.k,
        );
        if (node.type === "image" || node.type === "video") {
          const ratio = node.width / node.height;
          height = Math.max(60, width / ratio);
          width = height * ratio;
        }
        changed = true;
        apply({
          ...d,
          nodes: d.nodes.map((n) =>
            n.id === node.id ? { ...n, width, height } : n,
          ),
        });
      },
      () => {
        if (changed) remember(d);
      },
    );
  };
  const boxSelect = (event: ReactPointerEvent<HTMLDivElement>) => {
    const start = world(event.clientX, event.clientY);
    const previous = event.shiftKey ? selected : new Set<string>();
    clearSelection();
    startGesture(
      event,
      (e) => {
        const end = world(e.clientX, e.clientY);
        setBox({ start, end });
        const ids = current.current.nodes
          .filter(
            (n) =>
              n.position.x + n.width >= Math.min(start.x, end.x) &&
              n.position.x <= Math.max(start.x, end.x) &&
              n.position.y + n.height >= Math.min(start.y, end.y) &&
              n.position.y <= Math.max(start.y, end.y),
          )
          .map((n) => n.id);
        setSelected(new Set([...previous, ...ids]));
      },
      () => {},
    );
  };
  const connect = (event: ReactPointerEvent, handle: ConnectionHandle) => {
    const d = current.current;
    setConnection({ handle, point: world(event.clientX, event.clientY) });
    startGesture(
      event,
      (e) => setConnection({ handle, point: world(e.clientX, e.clientY) }),
      (e) => {
        const target = document
          .elementFromPoint(e.clientX, e.clientY)
          ?.closest<HTMLElement>("[data-node-id]")?.dataset.nodeId;
        if (!target) return;
        const edge = normalizeConnection(
          handle.nodeId,
          target,
          d.nodes,
          handle.handleType,
        );
        if (
          edge &&
          !d.connections.some(
            (c) =>
              c.fromNodeId === edge.fromNodeId && c.toNodeId === edge.toNodeId,
          )
        )
          commit({
            ...d,
            connections: [
              ...d.connections,
              { id: crypto.randomUUID(), ...edge },
            ],
          });
      },
    );
  };
  const importMedia = async (list: File[], position = center()) => {
    if (!list.length || busy) return;
    setBusy(true);
    setError("");
    const replace = replaceID.current;
    replaceID.current = null;
    try {
      const nodes = await Promise.all(
        list.map((f, i) =>
          readMedia(f, { x: position.x + i * 40, y: position.y + i * 40 }),
        ),
      );
      if (!mounted.current) return;
      if (replace) {
        const d = current.current;
        commit({
          ...d,
          nodes: d.nodes.map((n) =>
            n.id === replace
              ? {
                  ...nodes[0],
                  id: n.id,
                  position: n.position,
                  metadata: {
                    ...nodes[0].metadata,
                    groupId: n.metadata?.groupId,
                  },
                }
              : n,
          ),
        });
      } else add(nodes);
    } catch (e) {
      if (mounted.current)
        setError(e instanceof Error ? e.message : "导入失败");
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  const single =
    selected.size === 1 ? doc.nodes.find((n) => selected.has(n.id)) : undefined;
  return (
    <div
      ref={root}
      tabIndex={-1}
      className="relative h-dvh w-full overflow-hidden bg-background text-foreground outline-none"
      role="region"
      aria-label="独立画布编辑器"
      onKeyUp={(e) => {
        if (e.code === "Space") spacePressed.current = false;
      }}
      onKeyDown={(e) => {
        if (
          e.target instanceof Element &&
          e.target.closest(
            "input,textarea,select,[contenteditable=true],[role=dialog]",
          )
        )
          return;
        if (imageTool || preview) return;
        if (e.code === "Space") spacePressed.current = true;
        const modifier = e.metaKey || e.ctrlKey;
        if (e.key === "Escape") {
          const g = gesture.current;
          if (g) {
            g.abort.abort();
            apply(g.before);
            gesture.current = null;
          }
          setConnection(null);
          setBox(null);
          clearSelection();
        } else if (modifier && e.key.toLowerCase() === "z") {
          e.preventDefault();
          if (e.shiftKey) redo();
          else undo();
        } else if (modifier && e.key.toLowerCase() === "a") {
          e.preventDefault();
          setSelected(new Set(doc.nodes.map((n) => n.id)));
        } else if (modifier && e.key.toLowerCase() === "c") {
          e.preventDefault();
          const ids = selectionMembers(doc, selected);
          clipboard.current = {
            nodes: doc.nodes.filter((n) => ids.has(n.id)),
            connections: doc.connections.filter(
              (c) => ids.has(c.fromNodeId) && ids.has(c.toNodeId),
            ),
          };
        } else if (modifier && e.key.toLowerCase() === "d") {
          e.preventDefault();
          duplicate();
        } else if (modifier && e.key.toLowerCase() === "g") {
          e.preventDefault();
          if (e.shiftKey) ungroup();
          else group();
        } else if (e.key === "Delete" || e.key === "Backspace") {
          e.preventDefault();
          remove();
        } else if (!modifier && e.key.toLowerCase() === "v") setTool("select");
        else if (!modifier && e.key.toLowerCase() === "h") setTool("pan");
      }}
      onPaste={(e) => {
        if (
          e.target instanceof Element &&
          e.target.closest("input,textarea,[role=dialog]")
        )
          return;
        if (e.clipboardData.files.length) {
          e.preventDefault();
          void importMedia(Array.from(e.clipboardData.files));
        } else if (clipboard.current) {
          e.preventDefault();
          const copied = duplicateSelection(
            clipboard.current,
            new Set(clipboard.current.nodes.map((n) => n.id)),
          );
          const d = current.current;
          commit({
            nodes: [...d.nodes, ...copied.nodes],
            connections: [...d.connections, ...copied.connections],
          });
          setSelected(new Set(copied.nodes.map((n) => n.id)));
        }
      }}
    >
      <InfiniteCanvas
        containerRef={surface}
        viewport={doc.viewport}
        tool={tool}
        backgroundMode={background}
        onViewportChange={viewportChange}
        onCanvasMouseDown={boxSelect}
        onCanvasDeselect={clearSelection}
        onCanvasDoubleClick={(e) =>
          add([createNode("text", world(e.clientX, e.clientY))])
        }
        onDrop={(e) => {
          e.preventDefault();
          void importMedia(
            Array.from(e.dataTransfer.files),
            world(e.clientX, e.clientY),
          );
        }}
      >
        <svg
          className="pointer-events-none absolute overflow-visible"
          width="1"
          height="1"
        >
          {doc.connections.map((edge) => {
            const from = doc.nodes.find((n) => n.id === edge.fromNodeId);
            const to = doc.nodes.find((n) => n.id === edge.toNodeId);
            return from && to ? (
              <ConnectionPath
                key={edge.id}
                connection={edge}
                from={from}
                to={to}
                active={selected.has(edge.id)}
                onSelect={() => setSelected(new Set([edge.id]))}
              />
            ) : null;
          })}
          {connection && (
            <ActiveConnectionPath
              node={doc.nodes.find((n) => n.id === connection.handle.nodeId)}
              handle={connection.handle}
              mouseWorld={connection.point}
            />
          )}
        </svg>
        {doc.nodes.map((node) => (
          <CanvasNode
            key={node.id}
            node={node}
            selected={selected.has(node.id)}
            onMove={moveNode}
            onResize={resizeNode}
            onChange={(n) =>
              commit({
                ...current.current,
                nodes: current.current.nodes.map((old) =>
                  old.id === n.id ? n : old,
                ),
              })
            }
            onConnect={connect}
            onPreview={setPreview}
          />
        ))}
        {box && (
          <div
            className="pointer-events-none absolute border border-primary bg-primary/10"
            style={{
              left: Math.min(box.start.x, box.end.x),
              top: Math.min(box.start.y, box.end.y),
              width: Math.abs(box.end.x - box.start.x),
              height: Math.abs(box.end.y - box.start.y),
              zIndex: 2,
            }}
          />
        )}
      </InfiniteCanvas>
      <div className="absolute top-4 right-4 left-4 flex flex-wrap items-center justify-between gap-2 rounded-xl border bg-popover/95 p-2 shadow-sm">
        <div className="px-2">
          <h1 className="text-sm font-medium">无限画布</h1>
          <p className="text-xs text-muted-foreground">
            独立编辑 · 内容仅在本页，离开前请导出
          </p>
        </div>
        <div className="flex items-center gap-1">
          {
            <ToolButton
              label={"导入画布文件"}
              icon={<FolderOpen />}
              onClick={() => projectFile.current?.click()}
              disabled={busy}
            />
          }
          {
            <ToolButton
              label={"导出画布文件"}
              icon={<Download />}
              onClick={() =>
                downloadFile(
                  new Blob([JSON.stringify(current.current)], {
                    type: "application/json",
                  }),
                  "canvas.json",
                )
              }
              disabled={busy}
            />
          }
          {
            <ToolButton
              label={"撤销"}
              icon={<Undo2 />}
              onClick={undo}
              disabled={!availability.undo}
            />
          }
          {
            <ToolButton
              label={"重做"}
              icon={<Redo2 />}
              onClick={redo}
              disabled={!availability.redo}
            />
          }
        </div>
      </div>
      <div
        role="toolbar"
        aria-label="画布工具"
        className="absolute bottom-5 left-1/2 flex max-w-[calc(100%-2rem)] -translate-x-1/2 items-center gap-1 overflow-x-auto rounded-xl border bg-popover p-2 shadow-lg"
      >
        <Button
          aria-label="选择工具"
          title="选择 (V)"
          aria-pressed={tool === "select"}
          variant={tool === "select" ? "secondary" : "ghost"}
          size="icon"
          onClick={() => setTool("select")}
        >
          <MousePointer2 />
        </Button>
        <Button
          aria-label="平移工具"
          title="平移 (H / 空格)"
          aria-pressed={tool === "pan"}
          variant={tool === "pan" ? "secondary" : "ghost"}
          size="icon"
          onClick={() => setTool("pan")}
        >
          <Hand />
        </Button>
        {
          <ToolButton
            label={"添加文本"}
            icon={<Type />}
            onClick={() => add([createNode("text", center())])}
          />
        }
        {
          <ToolButton
            label={"导入素材"}
            icon={<Upload />}
            onClick={() => {
              replaceID.current = null;
              files.current?.click();
            }}
            disabled={busy}
          />
        }
        {
          <ToolButton
            label={"复制选中节点"}
            icon={<Copy />}
            onClick={duplicate}
            disabled={!selected.size}
          />
        }
        {
          <ToolButton
            label={"分组"}
            icon={<Group />}
            onClick={group}
            disabled={collectGroupMemberNodes(selected, doc.nodes).length < 2}
          />
        }
        {
          <ToolButton
            label={"取消分组"}
            icon={<Ungroup />}
            onClick={ungroup}
            disabled={
              !doc.nodes.some(
                (n) =>
                  selected.has(n.id) &&
                  (n.type === "group" || n.metadata?.groupId),
              )
            }
          />
        }
        {
          <ToolButton
            label={"删除选中内容"}
            icon={<Trash2 />}
            onClick={remove}
            disabled={!selected.size}
          />
        }
      </div>
      {single && single.type !== "text" && single.type !== "group" && (
        <div className="absolute top-24 left-1/2 flex -translate-x-1/2 gap-1 rounded-xl border bg-popover p-2 shadow-sm">
          {
            <ToolButton
              label={"替换素材"}
              icon={<Upload />}
              onClick={() => {
                replaceID.current = single.id;
                files.current?.click();
              }}
              disabled={busy}
            />
          }
          {
            <ToolButton
              label={"预览素材"}
              icon={<ImageIcon />}
              onClick={() => setPreview(single)}
              disabled={!single.metadata?.content}
            />
          }
          {
            <ToolButton
              label={"下载素材"}
              icon={<Download />}
              onClick={() => {
                const a = document.createElement("a");
                a.href = single.metadata?.content || "";
                a.download = single.title;
                a.click();
              }}
              disabled={!single.metadata?.content}
            />
          }
          {single.type === "image" && (
            <>
              {
                <ToolButton
                  label={"裁剪图片"}
                  icon={<Crop />}
                  onClick={() => setImageTool({ node: single, mode: "crop" })}
                />
              }
              {
                <ToolButton
                  label={"拆分图片"}
                  icon={<Grid2X2 />}
                  onClick={() => setImageTool({ node: single, mode: "split" })}
                />
              }
              {
                <ToolButton
                  label={"调整图片像素"}
                  icon={<Scaling />}
                  onClick={() => setImageTool({ node: single, mode: "resize" })}
                />
              }
            </>
          )}
        </div>
      )}
      <div className="absolute right-4 bottom-24 flex items-center gap-1 rounded-xl border bg-popover p-1 shadow-sm">
        {
          <ToolButton
            label={"缩小"}
            icon={<Minus />}
            onClick={() => zoom(1 / 1.2)}
          />
        }
        <span className="w-12 text-center text-xs">
          {Math.round(doc.viewport.k * 100)}%
        </span>
        {
          <ToolButton
            label={"放大"}
            icon={<Plus />}
            onClick={() => zoom(1.2)}
          />
        }
        {<ToolButton label={"适合屏幕"} icon={<Maximize />} onClick={fit} />}
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={map}
          onClick={() => setMap((v) => !v)}
        >
          小地图
        </Button>
        <NativeSelect
          aria-label="画布背景"
          className="rounded bg-popover text-xs"
          value={background}
          onChange={(e) =>
            setBackground(e.target.value as CanvasBackgroundMode)
          }
        >
          <NativeSelectOption value="lines">网格</NativeSelectOption>
          <NativeSelectOption value="dots">点阵</NativeSelectOption>
          <NativeSelectOption value="blank">纯色</NativeSelectOption>
        </NativeSelect>
      </div>
      {map && (
        <Minimap
          nodes={doc.nodes}
          viewport={doc.viewport}
          viewportSize={size}
          onViewportChange={viewportChange}
        />
      )}
      {!doc.nodes.length && (
        <div className="pointer-events-none absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-center text-muted-foreground">
          <p>拖入图片、视频或音频</p>
          <p className="mt-2 text-xs">双击添加文本 · 滚轮缩放 · 空格平移</p>
        </div>
      )}
      {(error || busy) && (
        <p
          role={error ? "alert" : "status"}
          className="absolute top-24 left-4 max-w-sm rounded-lg border bg-popover p-3 text-sm"
        >
          {error || "正在读取素材…"}
        </p>
      )}
      <a
        className="absolute bottom-1 left-3 text-[10px] text-muted-foreground"
        href="https://github.com/basketikun/infinite-canvas"
        target="_blank"
        rel="noreferrer"
      >
        Canvas by basketikun · MIT
      </a>
      <Input
        ref={files}
        type="file"
        accept="image/*,video/*,audio/*"
        multiple
        className="hidden"
        aria-label="选择素材文件"
        onChange={(e) => {
          void importMedia(Array.from(e.target.files || []));
          e.target.value = "";
        }}
      />
      <Input
        ref={projectFile}
        type="file"
        accept=".json"
        className="hidden"
        aria-label="选择画布文件"
        onChange={async (e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (!file) return;
          setError("");
          try {
            const imported = parseDocument(await file.text());
            if (!mounted.current) return;
            remember(current.current);
            apply(imported);
            clearSelection();
          } catch (error) {
            setError(error instanceof Error ? error.message : "导入失败");
          }
        }}
      />
      {imageTool && (
        <ImageTools
          node={imageTool.node}
          mode={imageTool.mode}
          onClose={() => setImageTool(null)}
          onResult={add}
        />
      )}
      <Dialog
        open={!!preview}
        onOpenChange={(open) => {
          if (!open) setPreview(null);
        }}
      >
        <DialogContent className="sm:max-w-4xl">
          <DialogTitle>{preview?.title}</DialogTitle>
          <DialogDescription>素材原件预览</DialogDescription>

          {preview?.type === "image" && (
            <img
              src={preview.metadata?.content}
              alt={preview.title}
              className="max-h-[75dvh] w-full object-contain"
            />
          )}
          {preview?.type === "video" && (
            <video
              src={preview.metadata?.content}
              controls
              className="max-h-[75dvh] w-full"
            />
          )}
          {preview?.type === "audio" && (
            <audio
              src={preview.metadata?.content}
              controls
              className="w-full"
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}

function ToolButton({
  label,
  icon,
  onClick,
  disabled = false,
}: {
  label: string;
  icon: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <Button
      aria-label={label}
      title={label}
      variant="ghost"
      size="icon"
      onClick={onClick}
      disabled={disabled}
    >
      {icon}
    </Button>
  );
}
