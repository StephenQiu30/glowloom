"use client";

import type { ReactNode } from "react";
import {
  Type,
  ImageIcon,
  VideoIcon,
  Folder,
  Upload,
  Copy,
  Trash2,
  Undo2,
  Redo2,
} from "lucide-react";
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuGroup,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuShortcut,
} from "@/components/ui/context-menu";

export function CanvasContextMenu({
  readOnly = false,
  children,
  onAdd,
  onMaterials,
  onUpload,
  onCopy,
  onPaste,
  onDuplicate,
  onRemove,
  onUndo,
  onRedo,
  canUndo,
  canRedo,
  canDuplicate,
  canRemove,
}: {
  readOnly?: boolean;
  children: ReactNode;
  onAdd: (kind: "text" | "image_generation" | "video_generation") => void;
  onMaterials: () => void;
  onUpload: () => void;
  onCopy: () => void;
  onPaste: () => void;
  onDuplicate: () => void;
  onRemove: () => void;
  onUndo: () => void;
  onRedo: () => void;
  canUndo: boolean;
  canRedo: boolean;
  canDuplicate: boolean;
  canRemove: boolean;
}) {
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild disabled={readOnly}>
        {children}
      </ContextMenuTrigger>
      <ContextMenuContent
        className="canvas-view-menu"
        aria-label="画布右键菜单"
      >
        <ContextMenuGroup>
          <ContextMenuItem onSelect={() => onAdd("text")}>
            <Type />
            文本
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => onAdd("image_generation")}>
            <ImageIcon />
            图片
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => onAdd("video_generation")}>
            <VideoIcon />
            视频
          </ContextMenuItem>
          <ContextMenuItem onSelect={onMaterials}>
            <Folder />
            从素材库添加
          </ContextMenuItem>
          <ContextMenuItem onSelect={onUpload}>
            <Upload />
            本地上传
          </ContextMenuItem>
        </ContextMenuGroup>
        <ContextMenuSeparator />
        <ContextMenuGroup>
          <ContextMenuItem onSelect={onCopy} disabled={!canDuplicate}>
            <Copy />
            复制<ContextMenuShortcut>⌘ C</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onSelect={onPaste}>
            粘贴<ContextMenuShortcut>⌘ V</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onSelect={onDuplicate} disabled={!canDuplicate}>
            <Copy />
            复制副本<ContextMenuShortcut>⌘ D</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onSelect={onRedo} disabled={!canRedo}>
            <Redo2 />
            重做<ContextMenuShortcut>⌘ ⇧ Z</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onSelect={onUndo} disabled={!canUndo}>
            <Undo2 />
            撤销<ContextMenuShortcut>⌘ Z</ContextMenuShortcut>
          </ContextMenuItem>
          <ContextMenuItem onSelect={onRemove} disabled={!canRemove}>
            <Trash2 />
            删除<ContextMenuShortcut>⌫</ContextMenuShortcut>
          </ContextMenuItem>
        </ContextMenuGroup>
      </ContextMenuContent>
    </ContextMenu>
  );
}
