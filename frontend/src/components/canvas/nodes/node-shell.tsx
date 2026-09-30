// Core node positioning, title, resize and connection rails adapted from BeefTV canvas-node.tsx.
import { memo, useRef, useState, useEffect, type ReactNode } from "react";
import { Pencil } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  canvasNodeRenderHeight,
  type CanvasNodeData,
  type ConnectionHandle,
  type Position,
} from "../model";
type Props = {
  node: CanvasNodeData;
  selected: boolean;
  scale: number;
  locked: boolean;
  overview: boolean;
  children: ReactNode;
  onSelect: (id: string) => void;
  onDrag: (event: React.PointerEvent, id: string) => void;
  onResize: (
    id: string,
    width: number,
    height: number,
    position: Position,
  ) => void;
  onRename: (id: string, title: string) => void;
  onTitleDraftChange: (id: string, dirty: boolean) => void;
  onConnectStart: (event: React.PointerEvent, handle: ConnectionHandle) => void;
  onConnectTarget: (handle: ConnectionHandle) => void;
  onToggleGroup: (id: string) => void;
};
type Resize = {
  corner: string;
  x: number;
  y: number;
  width: number;
  height: number;
  position: Position;
};
export const NodeShell = memo(function NodeShell(props: Props) {
  const { node, selected, scale, locked, overview, onTitleDraftChange } = props;
  const root = useRef<HTMLDivElement>(null),
    resizing = useRef<Resize | null>(null),
    pending = useRef<{
      width: number;
      height: number;
      position: Position;
    } | null>(null),
    frame = useRef<number | null>(null);
  const [title, setTitle] = useState(node.title),
    [editing, setEditing] = useState(false);
  useEffect(() => {
    setTitle(node.title);
  }, [node.title]);
  useEffect(
    () => () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
    },
    [],
  );
  useEffect(
    () => () => onTitleDraftChange(node.id, false),
    [onTitleDraftChange, node.id],
  );
  function resizeMove(event: React.PointerEvent) {
    const start = resizing.current;
    if (!start) return;
    const dx = (event.clientX - start.x) / scale,
      dy = (event.clientY - start.y) / scale,
      left = start.corner.includes("left"),
      top = start.corner.includes("top"),
      max = node.type === "group" ? 100000 : 2000;
    const width = Math.max(40, Math.min(max, start.width + (left ? -dx : dx))),
      height = Math.max(40, Math.min(max, start.height + (top ? -dy : dy)));
    pending.current = {
      width,
      height,
      position: {
        x: start.position.x + (left ? start.width - width : 0),
        y: start.position.y + (top ? start.height - height : 0),
      },
    };
    if (frame.current !== null) return;
    frame.current = requestAnimationFrame(() => {
      frame.current = null;
      const next = pending.current;
      if (root.current && next) {
        root.current.style.width = `${next.width}px`;
        root.current.style.height = `${next.height}px`;
        root.current.style.transform = `translate(${next.position.x}px, ${next.position.y}px)`;
      }
    });
  }
  function finishResize(event: React.PointerEvent, cancelled = false) {
    if (!resizing.current) return;
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    const next = pending.current;
    resizing.current = null;
    pending.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId);
    if (root.current) {
      root.current.style.width = `${node.width}px`;
      root.current.style.height = `${node.height}px`;
      root.current.style.transform = `translate(${node.position.x}px, ${node.position.y}px)`;
    }
    if (!cancelled && next)
      props.onResize(node.id, next.width, next.height, next.position);
  }
  function rename() {
    const next = title.trim();
    setEditing(false);
    props.onTitleDraftChange(node.id, false);
    if (next && next !== node.title) props.onRename(node.id, next);
    else setTitle(node.title);
  }
  return (
    <div
      ref={root}
      data-node-id={node.id}
      data-node-selected={selected ? "true" : "false"}
      data-canvas-node-type={node.type}
      data-canvas-parent-id={node.parentId}
      className={`node-element absolute flex flex-col rounded-2xl ${node.type === "group" ? "bg-foreground/5" : "bg-background shadow-sm"} ${selected ? "outline-2 outline-foreground" : ""}`}
      style={{
        transform: `translate(${node.position.x}px, ${node.position.y}px)`,
        width: node.width,
        height: canvasNodeRenderHeight(node),
        zIndex: node.type === "group" ? 0 : node.zIndex + 2,
        contain: "layout style",
      }}
      tabIndex={0}
      role="group"
      aria-label={`${node.title} 节点`}
      onFocus={() => props.onSelect(node.id)}
      onPointerDown={(event) => {
        const target = event.target instanceof Element ? event.target : null;
        if (
          target?.closest(
            "[data-canvas-no-zoom],button,input,textarea,video,audio",
          )
        )
          return;
        if (locked) {
          event.stopPropagation();
          props.onSelect(node.id);
        } else props.onDrag(event, node.id);
      }}
    >
      <header
        className="flex h-10 shrink-0 items-center gap-2 px-4 text-xs"
        style={
          overview
            ? { fontSize: Math.min(50, 12 / Math.max(scale, 0.05)) }
            : undefined
        }
      >
        {editing ? (
          <Input
            value={title}
            maxLength={128}
            autoFocus
            aria-label="节点名称"
            data-canvas-no-zoom
            onChange={(event) => {
              setTitle(event.target.value);
              props.onTitleDraftChange(
                node.id,
                event.target.value !== node.title,
              );
            }}
            onBlur={rename}
            onKeyDown={(event) => {
              if (event.key === "Enter") rename();
              if (event.key === "Escape") {
                setEditing(false);
                setTitle(node.title);
                props.onTitleDraftChange(node.id, false);
              }
            }}
            className="h-7"
          />
        ) : (
          <>
            <span className="truncate font-medium">{node.title}</span>
            <button
              type="button"
              data-canvas-no-zoom
              aria-label={`重命名 ${node.title}`}
              disabled={locked}
              className="ml-auto rounded p-1 hover:bg-muted focus-visible:ring-2"
              onClick={() => setEditing(true)}
            >
              <Pencil size={12} />
            </button>
          </>
        )}
        {node.type === "group" && (
          <button
            type="button"
            data-canvas-no-zoom
            className="rounded px-1 focus-visible:ring-2"
            disabled={locked}
            aria-label={
              node.metadata?.frame?.collapsed ? "展开分组" : "折叠分组"
            }
            onClick={() => props.onToggleGroup(node.id)}
          >
            {node.metadata?.frame?.collapsed ? "+" : "−"}
          </button>
        )}
      </header>
      {!overview && (
        <div className="min-h-0 flex-1 overflow-hidden rounded-b-2xl">
          {props.children}
        </div>
      )}
      {!locked &&
        !overview &&
        (["target", "source"] as const).map((handleType) => (
          <button
            key={handleType}
            type="button"
            data-canvas-no-zoom
            data-node-handle={handleType}
            aria-label={`${node.title} ${handleType === "source" ? "输出连接点" : "输入连接点"}`}
            className="absolute top-1/2 h-5 w-5 -translate-y-1/2 rounded-full bg-muted ring-1 ring-foreground/20 hover:bg-foreground hover:text-background focus-visible:ring-2"
            style={{ [handleType === "source" ? "right" : "left"]: -10 }}
            onPointerDown={(event) =>
              props.onConnectStart(event, { nodeId: node.id, handleType })
            }
            onPointerEnter={() =>
              props.onConnectTarget({ nodeId: node.id, handleType })
            }
            onPointerUp={() => {
              props.onConnectTarget({ nodeId: node.id, handleType });
            }}
          >
            <span className="mx-auto block h-1.5 w-1.5 rounded-full bg-current" />
          </button>
        ))}
      {selected &&
        !locked &&
        !overview &&
        !node.metadata?.frame?.collapsed &&
        ["top-left", "top-right", "bottom-left", "bottom-right"].map(
          (corner) => (
            <button
              key={corner}
              type="button"
              data-canvas-no-zoom
              aria-label={`调整 ${node.title} ${corner} 尺寸`}
              className={`absolute h-3 w-3 rounded-sm bg-foreground ${corner.includes("top") ? "-top-1" : "-bottom-1"} ${corner.includes("left") ? "-left-1" : "-right-1"}`}
              style={{
                cursor:
                  corner === "top-left" || corner === "bottom-right"
                    ? "nwse-resize"
                    : "nesw-resize",
              }}
              onPointerDown={(event) => {
                event.stopPropagation();
                event.preventDefault();
                event.currentTarget.setPointerCapture(event.pointerId);
                resizing.current = {
                  corner,
                  x: event.clientX,
                  y: event.clientY,
                  width: node.width,
                  height: node.height,
                  position: node.position,
                };
              }}
              onPointerMove={resizeMove}
              onPointerUp={(event) => finishResize(event)}
              onPointerCancel={(event) => finishResize(event, true)}
            />
          ),
        )}
    </div>
  );
});
