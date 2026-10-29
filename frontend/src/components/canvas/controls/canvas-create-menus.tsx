import { useRef, useState, type ReactNode } from "react";
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
import {
  ImageIcon,
  Type,
  SquarePlay,
  AudioLines,
  Clapperboard,
  Contact,
  Box,
  X,
} from "lucide-react";
import {
  listNodeDefinitions,
  useNodeRegistryVersion,
} from "@/components/canvas/lib/node-registry";
import {
  CanvasNodeType,
  type ConnectionHandle,
  type CanvasNodeData,
  type Position,
} from "@/components/canvas/types/canvas";

import { supportsCanvasConnection } from "../lib/canvas-document-connections";
import {
  assertCanvasDomainSources,
  isCanvasDomainTarget,
} from "../lib/canvas-domain-connection-preview";

export type PendingConnectionCreate = {
  connection: ConnectionHandle;
  position: Position;
  placement?: "adjacent";
  sourceNodeIds?: string[];
};
type ReferenceNodeType =
  | CanvasNodeType.Image
  | CanvasNodeType.Text
  | CanvasNodeType.Video
  | CanvasNodeType.Audio
  | "timeline";

export function ConnectionCreateMenu({
  pending,
  existingType,
  existingNode,
  existingNodes,
  resolveMedia = (node) => node,
  onCreate,
  onClose,
}: {
  pending: PendingConnectionCreate;
  existingType: string;
  existingNode?: CanvasNodeData;
  existingNodes?: CanvasNodeData[];
  resolveMedia?: (node: CanvasNodeData) => CanvasNodeData;
  onCreate: (type: ReferenceNodeType) => string | void;
  onClose: () => void;
}) {
  const options = [
    { type: CanvasNodeType.Text, title: "文本", icon: <Type /> },
    { type: CanvasNodeType.Image, title: "图片", icon: <ImageIcon /> },
    { type: CanvasNodeType.Video, title: "视频", icon: <SquarePlay /> },
    { type: CanvasNodeType.Audio, title: "音频", icon: <AudioLines /> },
    { type: "timeline", title: "时间线", icon: <Clapperboard /> },
    { type: "subject", title: "主体", icon: <Contact /> },
    { type: "director", title: "导演台", icon: <Box /> },
  ] as const;
  return (
    <CanvasCreateMenu
      title="添加节点"
      position={pending.position}
      onClose={onClose}
      options={options.map((item) => {
        let domainReason: string | undefined;
        let supported =
          pending.connection.handleType === "source"
            ? (existingNodes?.length ? existingNodes : [existingNode]).every(
                (node) =>
                  supportsCanvasConnection(
                    node?.type ?? existingType,
                    item.type,
                  ) &&
                  !(
                    (node?.type ?? existingType) === "text" &&
                    item.type === "audio"
                  ),
              )
            : supportsCanvasConnection(item.type, existingType);
        if (
          pending.connection.handleType === "source" &&
          isCanvasDomainTarget(item.type)
        ) {
          try {
            assertCanvasDomainSources(
              (existingNodes?.length
                ? existingNodes
                : existingNode
                  ? [existingNode]
                  : []
              ).map(resolveMedia),
              item.type,
            );
            supported = true;
          } catch (error) {
            supported = false;
            domainReason =
              error instanceof Error ? error.message : "素材尚未就绪";
          }
        }
        const reason =
          domainReason ??
          (!supported
            ? existingType === "cinema_studio" && item.type === "video"
              ? "暂不支持这些设置的模型"
              : "无法连接这些节点"
            : undefined);
        return {
          ...item,
          disabled: !supported,
          reason,
          onSelect: () => onCreate(item.type as ReferenceNodeType),
        };
      })}
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
    onSelect: () => string | void;
    disabled?: boolean;
    reason?: string;
  }[];
}) {
  const command = useRef<HTMLDivElement>(null);
  const [keyboardNavigation, setKeyboardNavigation] = useState(false);
  const createdNodeId = useRef<string | void>(undefined);
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
        sideOffset={reference ? 0 : 8}
        className={
          reference
            ? "canvas-connection-create-menu w-[200px] gap-0 rounded-xl border-0 p-1 ring-0 data-open:animate-none data-closed:animate-none"
            : "w-60 p-1"
        }
        data-keyboard-navigation={keyboardNavigation}
        onKeyDownCapture={(event) => {
          if (["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key))
            setKeyboardNavigation(true);
        }}
        onMouseMove={() => setKeyboardNavigation(false)}
        data-connection-create-menu={reference ? "" : undefined}
        data-canvas-no-zoom
        onEscapeKeyDown={(event) => event.stopPropagation()}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          document
            .querySelector<HTMLElement>(
              createdNodeId.current
                ? `[data-node-id="${CSS.escape(createdNodeId.current)}"]`
                : "[data-canvas-viewport]",
            )
            ?.focus({ preventScroll: true });
        }}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          command.current?.focus();
        }}
        onMouseDown={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        {reference ? (
          <div className="canvas-connection-create-heading">{title}</div>
        ) : (
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
        )}
        <Command
          ref={command}
          tabIndex={0}
          label={title}
          className={reference ? "h-auto p-0" : "h-auto"}
          loop
        >
          <CommandList className={reference ? "max-h-none" : undefined}>
            <CommandGroup
              className={
                reference
                  ? "p-0 [&_[cmdk-group-items]]:flex [&_[cmdk-group-items]]:flex-col [&_[cmdk-group-items]]:gap-1"
                  : undefined
              }
            >
              {options.map((option) => (
                <CommandItem
                  key={option.type}
                  value={option.title}
                  className={
                    reference
                      ? "canvas-connection-create-item h-9 min-h-9 gap-2 rounded-lg px-3 py-[9px] text-[13px] leading-[22px]"
                      : "min-h-9 gap-2"
                  }
                  title={option.reason}
                  disabled={option.disabled}
                  onSelect={() => {
                    createdNodeId.current = option.onSelect();
                  }}
                >
                  {option.icon}
                  <span>{option.title}</span>
                  {option.reason ? (
                    <span
                      className={
                        reference
                          ? "sr-only"
                          : "ml-auto text-xs text-muted-foreground"
                      }
                    >
                      {option.reason}
                    </span>
                  ) : null}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
