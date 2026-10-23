/* eslint-disable @next/next/no-img-element -- Local user media stays in the browser. */
"use client";
// Editing shell adapted from upstream canvas-node.tsx; app/plugin/generation panels removed.
import { useState, useRef, type PointerEvent } from "react";
import { Input } from "@/components/ui/input";
import dynamic from "next/dynamic";
import { CanvasTextPreview } from "./text-preview";
import type { CanvasReferenceSource } from "@/components/generation/reference-source";
import { Button } from "@/components/ui/button";
import {
  Plus,
  Grip,
  Type,
  ImageIcon,
  Video,
  AudioLines,
  Group,
} from "lucide-react";
import type { CanvasNodeData, ConnectionHandle } from "./types";

const CanvasTextEditor = dynamic(
  () => import("./text-editor").then((m) => m.CanvasTextEditor),
  { ssr: false },
);

type Props = {
  node: CanvasNodeData;
  selected: boolean;
  showConnections?: boolean;
  onMove: (event: PointerEvent, node: CanvasNodeData) => void;
  onResize: (event: PointerEvent, node: CanvasNodeData) => void;
  onChange: (node: CanvasNodeData) => void | boolean | Promise<boolean>;
  projectID?: string;
  sources?: CanvasReferenceSource[];
  onConnect: (event: PointerEvent, handle: ConnectionHandle) => void;
  onPreview: (node: CanvasNodeData) => void;
};
export function CanvasNode({
  node,
  selected,
  showConnections = true,
  onMove,
  onResize,
  onChange,
  onConnect,
  onPreview,
  projectID,
  sources,
}: Props) {
  const element = useRef<HTMLDivElement>(null);
  const [editing, setEditing] = useState<"title" | "content" | null>(null);
  const [draft, setDraft] = useState("");
  const startEdit = (mode: "title" | "content") => {
    setDraft(mode === "title" ? node.title : node.metadata?.content || "");
    setEditing(mode);
  };
  const finish = () => {
    if (editing === "title" && draft.trim())
      onChange({ ...node, title: draft.trim() });
    setEditing(null);
  };
  const content = node.metadata?.content;
  const Icon = {
    text: Type,
    image: ImageIcon,
    video: Video,
    audio: AudioLines,
    group: Group,
  }[node.type];
  return (
    <div
      ref={element}
      role="group"
      aria-label={`节点：${node.title}`}
      data-node-id={node.id}
      tabIndex={0}
      className={`group/node absolute rounded-xl border outline-none focus-visible:ring-1 focus-visible:ring-ring ${selected ? "border-muted-foreground" : "border-border"} ${node.type === "group" ? "bg-muted/30" : "bg-card"}`}
      style={{
        transform: `translate(${node.position.x}px, ${node.position.y}px)`,
        width: node.width,
        height: node.height,
        zIndex: node.type === "group" ? 0 : 1,
        contain: "layout style",
      }}
      onPointerDown={(e) => {
        if (e.button === 0 && !e.ctrlKey) onMove(e, node);
      }}
      onDoubleClick={(e) => {
        e.stopPropagation();
        if (node.type === "text") startEdit("content");
        else if (content) onPreview(node);
      }}
    >
      <div className="absolute -top-8 left-0 max-w-full text-xs text-muted-foreground">
        {editing === "title" ? (
          <Input
            autoFocus
            aria-label="节点名称"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={finish}
            onPointerDown={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229)
                return;
              if (e.key === "Enter") finish();
              if (e.key === "Escape") setEditing(null);
            }}
          />
        ) : (
          <Button
            variant="ghost"
            size="xs"
            className="max-w-full gap-1.5 px-0 font-normal"
            onDoubleClick={(e) => {
              e.stopPropagation();
              startEdit("title");
            }}
          >
            <Icon className="size-3.5 shrink-0" />
            <span className="truncate">{node.title}</span>
          </Button>
        )}
      </div>
      {node.type === "text" && (
        <>
          <div className="size-full overflow-hidden p-6 text-sm leading-[22px] whitespace-pre-wrap">
            <CanvasTextPreview node={node} />
          </div>
          {editing === "content" && (
            <CanvasTextEditor
              node={node}
              anchorRef={element}
              projectID={projectID}
              sources={sources}
              onSave={onChange}
              onClose={() => setEditing(null)}
            />
          )}
        </>
      )}
      {/* User-owned local images are data URLs, not remotely optimized images. */}

      {node.type === "image" && content && (
        <img
          src={content}
          alt={node.title}
          draggable={false}
          className="size-full rounded-xl object-contain"
        />
      )}
      {node.type === "video" && content && (
        <video
          src={content}
          controls={selected}
          preload="metadata"
          className="size-full rounded-xl object-contain"
          onPointerDown={(e) => {
            if (selected) e.stopPropagation();
          }}
          data-canvas-no-zoom={selected || undefined}
        />
      )}
      {node.type === "audio" && content && (
        <div className="flex size-full items-center p-4">
          <audio
            src={content}
            controls
            preload="metadata"
            className="w-full"
            onPointerDown={(e) => e.stopPropagation()}
            data-canvas-no-zoom
          />
        </div>
      )}
      {node.type !== "text" && node.type !== "group" && !content && (
        <div className="flex size-full items-center justify-center text-sm text-muted-foreground">
          导入素材后显示
        </div>
      )}
      {showConnections &&
        node.type !== "group" &&
        ["target", "source"].map((side) => (
          <Button
            key={side}
            aria-label={side === "source" ? "创建连线" : "连接到此节点"}
            variant="ghost"
            size="icon-lg"
            data-port-node={node.id}
            data-port-side={side}
            className={`absolute top-1/2 size-11 -translate-y-1/2 rounded-full hover:bg-transparent ${side === "source" ? "-right-[22px]" : "-left-[22px]"} ${selected ? "" : "opacity-0 group-hover/node:opacity-100 focus-visible:opacity-100"}`}
            onPointerDown={(e) =>
              onConnect(e, {
                nodeId: node.id,
                handleType: side as "source" | "target",
              })
            }
          >
            <span className="flex size-7 items-center justify-center rounded-full border bg-card">
              <Plus className="size-4" />
            </span>
          </Button>
        ))}
      {selected && (
        <Button
          aria-label="调整节点尺寸"
          variant="ghost"
          size="icon-xs"
          className="absolute right-0 bottom-0 size-11 cursor-nwse-resize text-muted-foreground"
          onPointerDown={(e) => onResize(e, node)}
        >
          <Grip />
        </Button>
      )}
    </div>
  );
}
