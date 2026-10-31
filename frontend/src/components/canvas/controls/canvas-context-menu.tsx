import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
  DropdownMenuShortcut,
} from "@/components/ui/dropdown-menu";
import {
  BetweenHorizontalStart,
  GalleryHorizontal,
  GalleryHorizontalEnd,
  Group,
  Plus,
  Copy,
  ClipboardPaste,
  Undo2,
  Redo2,
  Settings2,
  Trash2,
  Ungroup,
} from "lucide-react";

import type { VideoFramePosition } from "@/components/canvas/lib/canvas-video-frame";
import type { ContextMenuState } from "@/components/canvas/types/canvas";

export function CanvasNodeContextMenu({
  menu,
  canCaptureVideoFrame,
  frozen = false,
  canUndo = false,
  canRedo = false,
  onCopy,
  onTransfer,
  onPaste,
  onUndo,
  onRedo,
  onCreate,
  canGroup,
  canUngroup,
  onClose,
  onCaptureVideoFrame,
  onDuplicate,
  onGroup,
  onUngroup,
  onDelete,
  onEditConnection,
}: {
  menu: ContextMenuState;
  canCaptureVideoFrame: boolean;
  frozen?: boolean;
  canUndo?: boolean;
  canRedo?: boolean;
  onCopy?: () => void;
  onTransfer?: () => void;
  onPaste?: () => void;
  onUndo?: () => void;
  onRedo?: () => void;
  onCreate?: (type: string) => void;
  canGroup?: boolean;
  canUngroup?: boolean;
  onClose: () => void;
  onCaptureVideoFrame: (position: VideoFramePosition) => void;
  onDuplicate: () => void;
  onGroup?: () => void;
  onUngroup?: () => void;
  onDelete: () => void;
  onEditConnection?: () => void;
}) {
  return (
    <DropdownMenu
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DropdownMenuTrigger asChild>
        <span className="fixed size-px" style={{ left: menu.x, top: menu.y }} />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        onPointerDown={(event) => event.stopPropagation()}
        align="start"
        sideOffset={0}
        data-canvas-no-zoom
        onMouseDown={(event) => event.stopPropagation()}
        onEscapeKeyDown={(event) => event.stopPropagation()}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          document
            .querySelector<HTMLElement>("[data-canvas-viewport]")
            ?.focus({ preventScroll: true });
        }}
      >
        <DropdownMenuGroup>
          {menu.type === "canvas" && onCreate ? (
            <DropdownMenuSub>
              <DropdownMenuSubTrigger disabled={frozen}>
                <Plus />
                新建节点
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent
                data-canvas-no-zoom
                onPointerDown={(event) => event.stopPropagation()}
                onMouseDown={(event) => event.stopPropagation()}
              >
                {[
                  ["text", "文本"],
                  ["image", "图片"],
                  ["video", "视频"],
                  ["audio", "音频"],
                  ["timeline", "时间线"],
                  ["subject", "主体"],
                  ["director", "导演台"],
                ].map(([type, label]) => (
                  <DropdownMenuItem
                    key={type}
                    disabled={frozen}
                    onSelect={() => onCreate(type)}
                  >
                    {label}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          ) : null}
          {menu.type === "node" && onCopy ? (
            <DropdownMenuItem onSelect={onCopy}>
              <Copy />
              复制<DropdownMenuShortcut>⌘C</DropdownMenuShortcut>
            </DropdownMenuItem>
          ) : null}
          {menu.type === "node" && onTransfer ? (
            <DropdownMenuItem onSelect={onTransfer}>
              <Copy />
              保存到其他画布
            </DropdownMenuItem>
          ) : null}
          {menu.type === "node" ? (
            <DropdownMenuItem disabled={frozen} onSelect={onDuplicate}>
              <Plus />
              复制副本<DropdownMenuShortcut>⌘D</DropdownMenuShortcut>
            </DropdownMenuItem>
          ) : null}
          {menu.type !== "connection" && onPaste ? (
            <DropdownMenuItem disabled={frozen} onSelect={onPaste}>
              <ClipboardPaste />
              粘贴<DropdownMenuShortcut>⌘V</DropdownMenuShortcut>
            </DropdownMenuItem>
          ) : null}
          {menu.type === "connection" && onEditConnection ? (
            <DropdownMenuItem disabled={frozen} onSelect={onEditConnection}>
              <Settings2 />
              调整引用
            </DropdownMenuItem>
          ) : null}
          {canCaptureVideoFrame ? (
            <>
              <DropdownMenuItem
                disabled={frozen}
                onSelect={() => onCaptureVideoFrame("first")}
              >
                <BetweenHorizontalStart />
                截取首帧
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={frozen}
                onSelect={() => onCaptureVideoFrame("last")}
              >
                <GalleryHorizontalEnd />
                截取尾帧
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={frozen}
                onSelect={() => onCaptureVideoFrame("current")}
              >
                <GalleryHorizontal />
                截取当前帧
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          ) : null}
          {menu.type === "node" && canGroup ? (
            <DropdownMenuItem disabled={frozen} onSelect={onGroup}>
              <Group />
              打组
            </DropdownMenuItem>
          ) : null}
          {menu.type === "node" && canUngroup ? (
            <DropdownMenuItem disabled={frozen} onSelect={() => onUngroup?.()}>
              <Ungroup />
              解散组
            </DropdownMenuItem>
          ) : null}
          {menu.type !== "connection" ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem disabled={frozen || !canRedo} onSelect={onRedo}>
                <Redo2 />
                重做<DropdownMenuShortcut>⇧⌘Z</DropdownMenuShortcut>
              </DropdownMenuItem>
              <DropdownMenuItem disabled={frozen || !canUndo} onSelect={onUndo}>
                <Undo2 />
                撤销<DropdownMenuShortcut>⌘Z</DropdownMenuShortcut>
              </DropdownMenuItem>
            </>
          ) : null}
          {menu.type !== "canvas" ? (
            <DropdownMenuItem
              disabled={frozen}
              variant="destructive"
              onSelect={onDelete}
            >
              <Trash2 />
              删除<DropdownMenuShortcut>⌫</DropdownMenuShortcut>
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
