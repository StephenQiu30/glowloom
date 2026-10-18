"use client";
// BeefTV a2196d3: canvas-node.tsx, NodeExternalHeader and node media shells (MIT).
import { useRef, type PointerEvent, type CSSProperties } from "react";
import {
  AudioLines,
  Box,
  Copy,
  ImageIcon,
  MoreHorizontal,
  Pencil,
  Plus,
  Trash2,
  VideoIcon,
  Type,
  Group,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { isDraft, isEditableNode, type Node } from "./editor-state";

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
  onDuplicate,
  onRemove,
  canRemove = false,
  onAddAdjacent,
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
  onSelect: (additive?: boolean) => void;
  onEditTitle: () => void;
  onMove: (x: number, y: number) => void;
  onDragStart: (event: PointerEvent<HTMLButtonElement>) => void;
  onDragMove: (event: PointerEvent<HTMLButtonElement>) => void;
  onDragEnd: (cancel?: boolean) => void;
  onDuplicate?: () => void;
  onRemove?: () => void;
  canRemove?: boolean;
  onAddAdjacent?: (
    kind: "text" | "image_generation" | "video_generation",
    side: "left" | "right",
  ) => void;
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
  const maximumDimension = node.node_type === "group" ? 100000 : 2000;
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
    const clamp = (value: number) =>
      Math.max(40, Math.min(maximumDimension, value));
    const dx = (event.clientX - d.startX) / d.zoom,
      dy = (event.clientY - d.startY) / d.zoom;
    let width = clamp(Math.round(d.original.width + (d.left ? -dx : dx))),
      height = clamp(Math.round(d.original.height + (d.top ? -dy : dy)));
    if (event.shiftKey) {
      const ratio = d.original.width / d.original.height;
      width = Math.max(
        40,
        Math.min(
          maximumDimension,
          maximumDimension * ratio,
          Math.max(40, 40 * ratio, width),
        ),
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
  const editable = isEditableNode(node);
  const Icon = node.node_type.includes("video")
    ? VideoIcon
    : node.node_type === "audio"
      ? AudioLines
      : node.node_type === "model"
        ? Box
        : node.node_type === "text"
          ? Type
          : node.node_type === "group"
            ? Group
            : ImageIcon;
  return (
    <article
      data-node-id={node.id}
      className="canvas-media-node"
      data-selected={selected}
      data-node-kind={node.node_type}
      style={
        {
          zIndex:
            node.node_type === "group"
              ? 0
              : Math.max(selected ? 5 : 1, node.z_index + 1),
          left: node.x,
          top: node.y,
          width: node.width ?? 288,
          "--canvas-node-height": `${node.height ?? 200}px`,
        } as CSSProperties
      }
    >
      <Button
        variant="ghost"
        type="button"
        className="canvas-node-external-header h-auto p-0"
        aria-label={editable ? `移动 ${node.title}` : `查看 ${node.title}`}
        onClick={(event) =>
          onSelect(event.shiftKey || event.metaKey || event.ctrlKey)
        }
        onPointerDown={onDragStart}
        onPointerMove={onDragMove}
        onPointerUp={() => onDragEnd()}
        onPointerCancel={() => onDragEnd(true)}
        onLostPointerCapture={() => onDragEnd(true)}
        onKeyDown={(event) => {
          if (!editable) return;
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
      </Button>
      {editable &&
        (onDuplicate ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                className="canvas-node-rename"
                variant="ghost"
                size="icon-xs"
                aria-label={`节点操作：${node.title}`}
                data-canvas-no-zoom
                onPointerDown={(e) => e.stopPropagation()}
              >
                <MoreHorizontal data-icon="inline-start" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuGroup>
                <DropdownMenuItem onSelect={onEditTitle}>
                  <Pencil data-icon="inline-start" />
                  编辑节点参数
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={onDuplicate}>
                  <Copy data-icon="inline-start" />
                  复制节点
                </DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem
                  variant="destructive"
                  disabled={!canRemove}
                  onSelect={onRemove}
                >
                  <Trash2 data-icon="inline-start" />
                  移除节点
                </DropdownMenuItem>
                {!canRemove && (
                  <DropdownMenuLabel>
                    此节点受服务端生成版本保护
                  </DropdownMenuLabel>
                )}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <Button
            className="canvas-node-rename"
            variant="ghost"
            size="icon-xs"
            aria-label={`编辑节点名称：${node.title}`}
            onClick={onEditTitle}
            data-canvas-no-zoom
          >
            <Pencil data-icon="inline-start" />
          </Button>
        ))}
      {selected &&
        onAddAdjacent &&
        (["left", "right"] as const).map((side) => (
          <DropdownMenu key={side}>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                size="icon-xs"
                className={`canvas-node-port canvas-node-port-${side}`}
                aria-label={`在${side === "left" ? "左" : "右"}侧添加节点`}
                data-canvas-no-zoom
                onPointerDown={(event) => event.stopPropagation()}
              >
                <Plus data-icon="inline-start" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuGroup>
                <DropdownMenuItem onSelect={() => onAddAdjacent("text", side)}>
                  <Type data-icon="inline-start" />
                  文本
                </DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() => onAddAdjacent("image_generation", side)}
                >
                  <ImageIcon data-icon="inline-start" />
                  图片
                </DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() => onAddAdjacent("video_generation", side)}
                >
                  <VideoIcon data-icon="inline-start" />
                  视频
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        ))}
      <Button
        variant="secondary"
        type="button"
        className="canvas-node-shell h-auto p-0"
        onClick={(event) =>
          onSelect(event.shiftKey || event.metaKey || event.ctrlKey)
        }
        onPointerDown={onDragStart}
        onPointerMove={onDragMove}
        onPointerUp={() => onDragEnd()}
        onPointerCancel={() => onDragEnd(true)}
        onLostPointerCapture={() => onDragEnd(true)}
        aria-label={`选择 ${node.title}`}
        style={{ height: "var(--canvas-node-height)" }}
      >
        {projectID &&
        (node.node_type === "image" ||
          (["video", "audio"].includes(node.node_type) && selected)) &&
        node.ref_type === "media_asset" &&
        node.ref_id ? (
          <ProjectMediaPreview
            projectID={projectID}
            id={node.ref_id}
            kind={node.node_type}
          />
        ) : (
          <Icon className="size-5 opacity-50" strokeWidth={1.5} />
        )}
        <span>
          {isDraft(node)
            ? ""
            : node.node_type === "text"
              ? node.config.text || "编辑文本内容"
              : node.node_type === "group"
                ? node.config.collapsed
                  ? "已收起分组"
                  : "分组"
                : node.ref_type === "media_asset"
                  ? ""
                  : node.title}
        </span>
      </Button>
      {editable &&
        selected &&
        (["nw", "ne", "sw", "se"] as const).map((corner) => (
          <Button
            variant="ghost"
            size="icon-xs"
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
                    maximumDimension,
                    width + (corner.includes("w") ? -delta[0] : delta[0]),
                  ),
                ),
                h = Math.max(
                  40,
                  Math.min(
                    maximumDimension,
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
    </article>
  );
}
