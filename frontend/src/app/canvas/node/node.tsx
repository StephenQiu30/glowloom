"use client";

import type { Editor } from "@tiptap/core";
import type { Transaction } from "@tiptap/pm/state";
import type { ComposerCollaboration } from "@/components/generation/composer-session";
// BeefTV a2196d3: canvas-node.tsx, NodeExternalHeader and node media shells (MIT).
import { useRef, type PointerEvent, type CSSProperties } from "react";
import {
  AudioLines,
  Box,
  Copy,
  ImageIcon,
  MoreHorizontal,
  Pencil,
  Trash2,
  VideoIcon,
  Type,
  Group,
  Grip,
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
import dynamic from "next/dynamic";
const DocumentEditor = dynamic(() =>
  import("@/components/generation/document-editor").then(
    (m) => m.DocumentEditor,
  ),
);
import { ProjectMediaPreview } from "@/components/media/project-media";
import { isDraft, isEditableNode, type Node } from "../document";
import { ConnectionPort, type ConnectionDrop } from "./connection-port";

export function CanvasNode({
  node,
  projectID,
  selected,
  zoom,
  onResize,
  onResizePreview,
  onSelect,
  onEditTitle,
  onOpen,
  onMove,
  onDragStart,
  onDragMove,
  onDragEnd,
  onDuplicate,
  onRemove,
  canRemove = false,
  onAddAdjacent,
  onConnect,
  addingAdjacent = false,
  readOnly = false,
  editingText = false,
  onEditText,
  onTextChange,
  onTextEditor,
  collaboration,
  onFinishText,
}: {
  editingText?: boolean;
  onEditText?: () => void;
  onTextChange?: (
    text: string,
    transaction?: Transaction,
    editor?: Editor,
  ) => void;
  onTextEditor?: (editor: Editor) => (() => void) | void;
  collaboration?: Pick<ComposerCollaboration, "history" | "composition">;
  onFinishText?: () => void;
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
  onOpen?: () => void;
  onMove: (x: number, y: number) => void;
  onDragStart: (event: PointerEvent<HTMLElement>) => void;
  onDragMove: (event: PointerEvent<HTMLElement>) => void;
  onDragEnd: (cancel?: boolean, event?: PointerEvent<HTMLElement>) => void;
  onDuplicate?: () => void;
  onRemove?: () => void;
  canRemove?: boolean;
  addingAdjacent?: boolean;
  readOnly?: boolean;
  onConnect?: (targetID: string) => void;
  onAddAdjacent?: (
    kind: "text" | "image_generation" | "video_generation",
    side: "left" | "right",
    point?: ConnectionDrop,
  ) => void;
}) {
  const textEditorCleanup = useRef<(() => void) | undefined>(undefined);
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
  const editable = !readOnly && isEditableNode(node);
  const Icon = node.node_type.includes("video")
    ? VideoIcon
    : node.node_type === "audio"
      ? AudioLines
      : ["model", "director"].includes(node.node_type)
        ? Box
        : node.node_type === "text"
          ? Type
          : node.node_type === "group"
            ? Group
            : ImageIcon;
  return (
    <article
      onDragStart={(event) => event.preventDefault()}
      data-node-id={node.id}
      className="canvas-media-node"
      data-selected={selected}
      data-editing={editingText || undefined}
      data-node-kind={node.node_type}
      style={
        {
          zIndex:
            node.node_type === "group"
              ? 0
              : Math.max(selected ? 5 : 1, node.z_index + 1),
          left: node.x,
          top: node.y,
          "--canvas-node-x": `${node.x}px`,
          "--canvas-node-y": `${node.y}px`,
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
        onPointerUp={(event) => onDragEnd(false, event)}
        onPointerCancel={(event) => onDragEnd(true, event)}
        onLostPointerCapture={(event) => onDragEnd(true, event)}
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
        <Icon className="size-3.5 shrink-0" />
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
                  重命名节点
                </DropdownMenuItem>
                {onOpen && (
                  <DropdownMenuItem onSelect={onOpen}>
                    <Box data-icon="inline-start" />
                    打开导演台
                  </DropdownMenuItem>
                )}
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
      {onAddAdjacent &&
        (["left", "right"] as const).map((side) => (
          <ConnectionPort
            key={side}
            side={side}
            disabled={addingAdjacent}
            onAdd={(kind, point) => onAddAdjacent(kind, side, point)}
            onConnect={onConnect}
          />
        ))}
      <div
        className="canvas-node-shell"
        role={editingText ? undefined : "button"}
        tabIndex={editingText ? undefined : 0}
        onDoubleClick={
          node.node_type === "text" && editable ? onEditText : onOpen
        }
        onClick={(event) =>
          onSelect(event.shiftKey || event.metaKey || event.ctrlKey)
        }
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget) return;
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onSelect();
            if (node.node_type === "text" && editable) onEditText?.();
          }
        }}
        onPointerDown={(event) => {
          if (!editingText) onDragStart(event);
        }}
        onPointerMove={onDragMove}
        onPointerUp={(event) => onDragEnd(false, event)}
        onPointerCancel={(event) => onDragEnd(true, event)}
        onLostPointerCapture={(event) => onDragEnd(true, event)}
        aria-label={editingText ? undefined : `选择 ${node.title}`}
        style={{ height: "var(--canvas-node-height)" }}
      >
        {node.node_type === "text" ? (
          <DocumentEditor
            id={node.id}
            text={node.config.text ?? ""}
            editable={editingText && editable}
            onChange={(text, transaction, editor) =>
              onTextChange?.(text, transaction, editor)
            }
            collaboration={collaboration}
            onReady={(editor) => {
              textEditorCleanup.current?.();
              textEditorCleanup.current = onTextEditor?.(editor) || undefined;
            }}
            onDispose={() => {
              textEditorCleanup.current?.();
              textEditorCleanup.current = undefined;
            }}
            onFinish={onFinishText}
          />
        ) : projectID &&
          ["image", "video", "audio"].includes(node.node_type) &&
          node.ref_type === "media_asset" &&
          node.ref_id ? (
          <ProjectMediaPreview
            projectID={projectID}
            id={node.ref_id}
            kind={node.node_type}
            fit="contain"
            interactive={selected}
          />
        ) : (
          <>
            <Icon data-icon="inline-start" strokeWidth={1.5} />
            <span>
              {isDraft(node)
                ? ""
                : node.node_type === "group"
                  ? node.config.collapsed
                    ? "已收起分组"
                    : "分组"
                  : node.node_type === "director"
                    ? "双击进入导演台"
                    : node.title}
            </span>
          </>
        )}
      </div>
      {editable &&
        selected &&
        (["se"] as const).map((corner) => (
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
          >
            <Grip aria-hidden="true" />
          </Button>
        ))}
    </article>
  );
}
