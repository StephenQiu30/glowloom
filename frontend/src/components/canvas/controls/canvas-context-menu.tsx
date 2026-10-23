import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  BetweenHorizontalStart,
  GalleryHorizontal,
  GalleryHorizontalEnd,
  Group,
  Plus,
  Settings2,
  Trash2,
  Ungroup,
} from "lucide-react";

import type { VideoFramePosition } from "@/components/canvas/lib/canvas-video-frame";
import type { ContextMenuState } from "@/components/canvas/types/canvas";

export function CanvasNodeContextMenu({
  menu,
  canCaptureVideoFrame,
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
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <DropdownMenuGroup>
          {menu.type === "connection" && onEditConnection ? (
            <DropdownMenuItem onSelect={onEditConnection}>
              <Settings2 />
              调整引用
            </DropdownMenuItem>
          ) : null}
          {canCaptureVideoFrame ? (
            <>
              <DropdownMenuItem onSelect={() => onCaptureVideoFrame("first")}>
                <BetweenHorizontalStart />
                截取首帧
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onCaptureVideoFrame("last")}>
                <GalleryHorizontalEnd />
                截取尾帧
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onCaptureVideoFrame("current")}>
                <GalleryHorizontal />
                截取当前帧
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          ) : null}
          {menu.type === "node" && canGroup ? (
            <DropdownMenuItem onSelect={onGroup}>
              <Group />
              打组
            </DropdownMenuItem>
          ) : null}
          {menu.type === "node" && canUngroup ? (
            <DropdownMenuItem onSelect={() => onUngroup?.()}>
              <Ungroup />
              解散组
            </DropdownMenuItem>
          ) : null}
          {menu.type === "node" ? (
            <DropdownMenuItem onSelect={onDuplicate}>
              <Plus />
              复制
            </DropdownMenuItem>
          ) : null}
          <DropdownMenuItem variant="destructive" onSelect={onDelete}>
            <Trash2 />
            删除
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
