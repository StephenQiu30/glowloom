"use client";
// BeefTV a2196d3: canvas-node.tsx, NodeExternalHeader and node media shells (MIT).
import { useRef, type PointerEvent, type CSSProperties } from "react";
import { ImageIcon, VideoIcon, Pencil } from "lucide-react";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { isDraft, type Node } from "./editor-state";

export function CanvasNode({
  node,
  projectID,
  selected,
  zoom,
  onResize,
  onResizePreview,
  onSelect,
  onEditTitle,
  onMove,
  onDragStart,
  onDragMove,
  onDragEnd,
}: {
  node: Node;
  projectID?: string;
  selected: boolean;
  zoom: number;
  onResize: (size: {
    x: number;
    y: number;
    width: number;
    height: number;
  }) => void;
  onResizePreview: () => void;
  onSelect: () => void;
  onEditTitle: () => void;
  onMove: (x: number, y: number) => void;
  onDragStart: (event: PointerEvent<HTMLButtonElement>) => void;
  onDragMove: (event: PointerEvent<HTMLButtonElement>) => void;
  onDragEnd: (cancel?: boolean) => void;
}) {
  const resize = useRef<{
    pointerID: number;
    button: HTMLButtonElement;
    card: HTMLElement;
    startX: number;
    startY: number;
    left: boolean;
    top: boolean;
    zoom: number;
    original: { x: number; y: number; width: number; height: number };
    next: { x: number; y: number; width: number; height: number };
  } | null>(null);
  function beginResize(event: PointerEvent<HTMLButtonElement>) {
    if (event.button !== 0) return;
    event.stopPropagation();
    event.preventDefault();
    onSelect();
    const corner = event.currentTarget.dataset.corner ?? "se";
    const original = {
      x: node.x,
      y: node.y,
      width: node.width ?? 288,
      height: node.height ?? 200,
    };
    resize.current = {
      pointerID: event.pointerId,
      button: event.currentTarget,
      card: event.currentTarget.parentElement!,
      startX: event.clientX,
      startY: event.clientY,
      left: corner.includes("w"),
      top: corner.includes("n"),
      zoom,
      original,
      next: original,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  }
  function previewResize(event: PointerEvent<HTMLButtonElement>) {
    const d = resize.current;
    if (!d || d.pointerID !== event.pointerId) return;
    event.stopPropagation();
    const clamp = (value: number) => Math.max(40, Math.min(2000, value));
    const dx = (event.clientX - d.startX) / d.zoom,
      dy = (event.clientY - d.startY) / d.zoom;
    let width = clamp(Math.round(d.original.width + (d.left ? -dx : dx))),
      height = clamp(Math.round(d.original.height + (d.top ? -dy : dy)));
    if (event.shiftKey) {
      const ratio = d.original.width / d.original.height;
      width = Math.max(
        40,
        Math.min(2000, 2000 * ratio, Math.max(40, 40 * ratio, width)),
      );
      height = width / ratio;
    }
    d.next = {
      width,
      height,
      x: d.original.x + (d.left ? d.original.width - width : 0),
      y: d.original.y + (d.top ? d.original.height - height : 0),
    };
    d.card.style.width = `${width}px`;
    d.card.style.setProperty("--canvas-node-height", `${height}px`);
    d.card.style.left = `${d.next.x}px`;
    d.card.style.top = `${d.next.y}px`;
    onResizePreview();
  }
  function endResize(cancel = false) {
    const d = resize.current;
    if (!d) return;
    resize.current = null;
    if (d.button.hasPointerCapture(d.pointerID))
      d.button.releasePointerCapture(d.pointerID);
    if (cancel) {
      d.card.style.width = `${d.original.width}px`;
      d.card.style.setProperty(
        "--canvas-node-height",
        `${d.original.height}px`,
      );
      d.card.style.left = `${d.original.x}px`;
      d.card.style.top = `${d.original.y}px`;
      onResizePreview();
    } else onResize(d.next);
  }
  const Icon = node.node_type === "video_generation" ? VideoIcon : ImageIcon;
  return (
    <article
      data-node-id={node.id}
      className="canvas-media-node"
      data-selected={selected}
      style={
        {
          left: node.x,
          top: node.y,
          width: node.width ?? 288,
          "--canvas-node-height": `${node.height ?? 200}px`,
        } as CSSProperties
      }
    >
      <button
        type="button"
        className="canvas-node-external-header"
        aria-label={isDraft(node) ? `移动 ${node.title}` : `查看 ${node.title}`}
        onClick={onSelect}
        onPointerDown={onDragStart}
        onPointerMove={onDragMove}
        onPointerUp={() => onDragEnd()}
        onPointerCancel={() => onDragEnd(true)}
        onLostPointerCapture={() => onDragEnd(true)}
        onKeyDown={(event) => {
          if (!isDraft(node)) return;
          const step = event.shiftKey ? 10 : 1;
          const move: Record<string, [number, number]> = {
            ArrowLeft: [-step, 0],
            ArrowRight: [step, 0],
            ArrowUp: [0, -step],
            ArrowDown: [0, step],
          };
          const delta = move[event.key];
          if (delta) {
            event.preventDefault();
            onSelect();
            onMove(node.x + delta[0], node.y + delta[1]);
          }
        }}
      >
        <Icon className="size-3 shrink-0" />
        <span className="truncate">{node.title}</span>
      </button>
      {isDraft(node) && (
        <button
          className="canvas-node-rename"
          type="button"
          aria-label={`编辑节点名称：${node.title}`}
          onClick={onEditTitle}
          data-canvas-no-zoom
        >
          <Pencil className="size-3" />
        </button>
      )}
      <button
        type="button"
        className="canvas-node-shell"
        onClick={onSelect}
        onPointerDown={onDragStart}
        onPointerMove={onDragMove}
        onPointerUp={() => onDragEnd()}
        onPointerCancel={() => onDragEnd(true)}
        onLostPointerCapture={() => onDragEnd(true)}
        aria-label={`选择 ${node.title}`}
        style={{ height: "var(--canvas-node-height)" }}
      >
        {projectID && node.ref_type === "media_asset" && node.ref_id ? (
          <ProjectMediaPreview
            projectID={projectID}
            id={node.ref_id}
            kind={node.node_type}
          />
        ) : (
          <Icon className="size-10 opacity-30" strokeWidth={1} />
        )}
        <span>
          {isDraft(node)
            ? `${node.node_type === "image_generation" ? "图片" : "视频"}草稿 · 尚未生成`
            : node.ref_type === "media_asset"
              ? ""
              : node.title}
        </span>
      </button>
      {isDraft(node) &&
        selected &&
        (["nw", "ne", "sw", "se"] as const).map((corner) => (
          <button
            key={corner}
            className={`canvas-resize-handle canvas-resize-${corner}`}
            type="button"
            aria-label={`调整 ${node.title} ${{ nw: "左上", ne: "右上", sw: "左下", se: "右下" }[corner]}尺寸`}
            data-canvas-no-zoom
            data-corner={corner}
            onPointerDown={beginResize}
            onPointerMove={previewResize}
            onPointerUp={() => endResize()}
            onPointerCancel={() => endResize(true)}
            onLostPointerCapture={() => endResize(true)}
            onKeyDown={(e) => {
              const step = e.shiftKey ? 10 : 1;
              const width = node.width ?? 288,
                height = node.height ?? 200;
              const changes: Record<string, [number, number]> = {
                ArrowLeft: [-step, 0],
                ArrowRight: [step, 0],
                ArrowUp: [0, -step],
                ArrowDown: [0, step],
              };
              const delta = changes[e.key];
              if (!delta) return;
              e.preventDefault();
              e.stopPropagation();
              const w = Math.max(
                  40,
                  Math.min(
                    2000,
                    width + (corner.includes("w") ? -delta[0] : delta[0]),
                  ),
                ),
                h = Math.max(
                  40,
                  Math.min(
                    2000,
                    height + (corner.includes("n") ? -delta[1] : delta[1]),
                  ),
                );
              onResize({
                width: w,
                height: h,
                x: node.x + (corner.includes("w") ? width - w : 0),
                y: node.y + (corner.includes("n") ? height - h : 0),
              });
            }}
          />
        ))}
      {isDraft(node) && (
        <div className="canvas-node-caption">
          <span>{node.config.draft.aspect_ratio}</span>
          <span>输入版本 {node.input_revision ?? "待保存"}</span>
        </div>
      )}
    </article>
  );
}
