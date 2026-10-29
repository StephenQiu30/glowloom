import { useRef, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from "@/components/ui/popover";
import { ImageIcon, FileText, Video, X } from "lucide-react";
import {
  listNodeDefinitions,
  useNodeRegistryVersion,
} from "@/components/canvas/lib/node-registry";
import {
  CanvasNodeType,
  type ConnectionHandle,
  type Position,
} from "@/components/canvas/types/canvas";

import { supportsCanvasConnection } from "../lib/canvas-document-connections";

export type PendingConnectionCreate = {
  connection: ConnectionHandle;
  position: Position;
  placement?: "adjacent";
};
type ReferenceNodeType =
  | CanvasNodeType.Image
  | CanvasNodeType.Text
  | CanvasNodeType.Video
  | CanvasNodeType.Audio;

export function ConnectionCreateMenu({
  pending,
  existingType,
  onCreate,
  onClose,
}: {
  pending: PendingConnectionCreate;
  existingType: string;
  onCreate: (type: ReferenceNodeType) => void;
  onClose: () => void;
}) {
  const options = [
    { type: CanvasNodeType.Text, title: "文本", icon: <FileText /> },
    { type: CanvasNodeType.Image, title: "图片", icon: <ImageIcon /> },
    { type: CanvasNodeType.Video, title: "视频", icon: <Video /> },
  ] as const;
  return (
    <CanvasCreateMenu
      title="引用该节点生成"
      position={pending.position}
      onClose={onClose}
      options={options
        .filter((item) =>
          pending.connection.handleType === "source"
            ? supportsCanvasConnection(existingType, item.type)
            : supportsCanvasConnection(item.type, existingType),
        )
        .map((item) => ({
          ...item,
          onSelect: () => onCreate(item.type),
        }))}
      reference
    />
  );
}

export function NodeCreateMenu({
  position,
  onCreate,
  onClose,
}: {
  position: Position;
  onCreate: (type: string) => void;
  onClose: () => void;
}) {
  useNodeRegistryVersion();
  const definitions = listNodeDefinitions().filter(
    (def) => def.showInCreateMenu !== false,
  );
  return (
    <CanvasCreateMenu
      title="添加节点"
      position={position}
      onClose={onClose}
      options={definitions.map((def) => ({
        type: def.type,
        title: def.title,
        icon: def.icon,
        onSelect: () => onCreate(def.type),
      }))}
    />
  );
}

function CanvasCreateMenu({
  title,
  position,
  onClose,
  options,
  reference,
}: {
  title: string;
  position: Position;
  onClose: () => void;
  reference?: boolean;
  options: {
    type: string;
    title: string;
    icon: ReactNode;
    onSelect: () => void;
  }[];
}) {
  const command = useRef<HTMLDivElement>(null);
  return (
    <Popover
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <PopoverAnchor asChild>
        <span
          className="absolute"
          style={{ left: position.x, top: position.y }}
        />
      </PopoverAnchor>
      <PopoverContent
        align="start"
        sideOffset={8}
        className="w-60 p-1"
        data-connection-create-menu={reference ? "" : undefined}
        data-canvas-no-zoom
        onEscapeKeyDown={(event) => event.stopPropagation()}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          document
            .querySelector<HTMLElement>("[data-canvas-viewport]")
            ?.focus({ preventScroll: true });
        }}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          command.current?.focus();
        }}
        onMouseDown={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between gap-2 px-2 py-1">
          <span className="text-xs text-muted-foreground">{title}</span>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onClose}
            aria-label="关闭"
          >
            <X />
          </Button>
        </div>
        <Command
          ref={command}
          tabIndex={0}
          label={title}
          className="h-auto"
          loop
        >
          <CommandList>
            <CommandGroup>
              {options.map((option) => (
                <CommandItem
                  key={option.type}
                  value={option.title}
                  className="h-9 gap-2"
                  onSelect={option.onSelect}
                >
                  {option.icon}
                  <span>{option.title}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
