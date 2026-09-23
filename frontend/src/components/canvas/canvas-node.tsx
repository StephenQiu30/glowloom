import type { MouseEventHandler, PointerEventHandler } from "react";
import {
  AudioLines,
  Clapperboard,
  FileText,
  Group,
  ImageIcon,
  Layers3,
  StickyNote,
  Video,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import type { CanvasNode } from "./canvas-model";

export function CanvasNodeCard({
  node,
  selected,
  canEdit,
  onClick,
  onPointerDown,
  onPointerMove,
  onPointerUp,
}: {
  node: CanvasNode;
  selected: boolean;
  canEdit: boolean;
  onClick: MouseEventHandler<HTMLButtonElement>;
  onPointerDown?: PointerEventHandler<HTMLButtonElement>;
  onPointerMove?: PointerEventHandler<HTMLButtonElement>;
  onPointerUp?: PointerEventHandler<HTMLButtonElement>;
}) {
  return (
    <Button
      aria-label={`选择${node.title}`}
      aria-pressed={selected}
      className={`absolute flex flex-col items-start overflow-hidden rounded-2xl p-6 text-left shadow-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring ${selected ? "bg-background shadow-lg" : node.kind === "group" ? "bg-secondary/90 hover:bg-secondary" : "bg-background/90 hover:bg-background"}`}
      data-canvas-node=""
      onClick={onClick}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      style={{ left: node.x, top: node.y, width: node.width, height: node.height }}
      variant="ghost"
    >
      <span className="flex size-9 items-center justify-center rounded-xl bg-foreground text-background">
        <NodeIcon kind={node.kind} />
      </span>
      <span className="mt-4 block max-w-full truncate text-base font-semibold">{node.title}</span>
      <span className="mt-1 block max-w-full truncate text-xs text-muted-foreground">
        {node.subtitle}
      </span>
      {node.saved && canEdit ? (
        <span
          aria-hidden="true"
          className="absolute bottom-2 right-2 size-4 cursor-nwse-resize rounded-br-md bg-foreground/15"
          data-canvas-resize=""
        />
      ) : null}
    </Button>
  );
}

function NodeIcon({ kind }: { kind: CanvasNode["kind"] }) {
  const className = "size-4";
  switch (kind) {
    case "project":
      return <Layers3 className={className} aria-hidden="true" />;
    case "episode":
      return <Clapperboard className={className} aria-hidden="true" />;
    case "image":
      return <ImageIcon className={className} aria-hidden="true" />;
    case "video":
      return <Video className={className} aria-hidden="true" />;
    case "audio":
      return <AudioLines className={className} aria-hidden="true" />;
    case "group":
      return <Group className={className} aria-hidden="true" />;
    case "note":
      return <StickyNote className={className} aria-hidden="true" />;
    default:
      return <FileText className={className} aria-hidden="true" />;
  }
}

export function CanvasNodeList({
  nodes,
  selectedId,
  onSelect,
}: {
  nodes: CanvasNode[];
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="absolute inset-0 overflow-y-auto p-4 md:hidden">
      <div className="space-y-2 pb-44">
        {nodes.map((node) => (
          <Button
            aria-pressed={selectedId === node.id}
            className={`flex w-full items-center gap-3 rounded-xl p-4 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring ${selectedId === node.id ? "bg-background shadow-sm" : "bg-background/70"}`}
            key={node.id}
            onClick={() => onSelect(node.id)}
            variant="ghost"
          >
            <NodeIcon kind={node.kind} />
            <span className="min-w-0">
              <span className="block truncate font-medium">{node.title}</span>
              <span className="block truncate text-xs text-muted-foreground">{node.subtitle}</span>
            </span>
          </Button>
        ))}
      </div>
    </div>
  );
}
