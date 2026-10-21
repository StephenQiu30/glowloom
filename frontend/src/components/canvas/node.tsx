/* eslint-disable @next/next/no-img-element -- Local user media stays in the browser. */
"use client";
// Editing shell adapted from upstream canvas-node.tsx; app/plugin/generation panels removed.
import { useState, type PointerEvent } from "react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { Plus, Grip } from "lucide-react";
import type { CanvasNodeData, ConnectionHandle } from "./types";

type Props = {
  node: CanvasNodeData;
  selected: boolean;
  onMove: (event: PointerEvent, node: CanvasNodeData) => void;
  onResize: (event: PointerEvent, node: CanvasNodeData) => void;
  onChange: (node: CanvasNodeData) => void;
  onConnect: (event: PointerEvent, handle: ConnectionHandle) => void;
  onPreview: (node: CanvasNodeData) => void;
};
export function CanvasNode({
  node,
  selected,
  onMove,
  onResize,
  onChange,
  onConnect,
  onPreview,
}: Props) {
  const [editing, setEditing] = useState<"title" | "content" | null>(null);
  const [draft, setDraft] = useState("");
  const startEdit = (mode: "title" | "content") => {
    setDraft(mode === "title" ? node.title : node.metadata?.content || "");
    setEditing(mode);
  };
  const finish = () => {
    if (editing === "title" && draft.trim())
      onChange({ ...node, title: draft.trim() });
    if (editing === "content")
      onChange({ ...node, metadata: { ...node.metadata, content: draft } });
    setEditing(null);
  };
  const content = node.metadata?.content;
  return (
    <div
      role="group"
      aria-label={`节点：${node.title}`}
      data-node-id={node.id}
      tabIndex={0}
      className={`absolute rounded-xl border shadow-sm outline-none focus-visible:ring-1 focus-visible:ring-ring ${selected ? "border-primary ring-1 ring-primary" : "border-border"} ${node.type === "group" ? "bg-muted/30" : "bg-card"}`}
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
              if (e.key === "Enter") finish();
              if (e.key === "Escape") setEditing(null);
            }}
          />
        ) : (
          <Button
            variant="ghost"
            size="xs"
            className="max-w-full truncate"
            onDoubleClick={(e) => {
              e.stopPropagation();
              startEdit("title");
            }}
          >
            {node.title}
          </Button>
        )}
      </div>
      {node.type === "text" &&
        (editing === "content" ? (
          <Textarea
            autoFocus
            data-canvas-no-zoom
            aria-label="文本内容"
            className="size-full resize-none border-0 bg-transparent p-4"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={finish}
            onPointerDown={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key === "Escape") finish();
            }}
          />
        ) : (
          <div className="size-full overflow-hidden p-4 text-sm whitespace-pre-wrap">
            {content || "双击输入文本"}
          </div>
        ))}
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
      {node.type !== "group" &&
        ["target", "source"].map((side) => (
          <Button
            key={side}
            aria-label={side === "source" ? "创建连线" : "连接到此节点"}
            variant="outline"
            size="icon-xs"
            data-port-node={node.id}
            data-port-side={side}
            className={`absolute top-1/2 -translate-y-1/2 rounded-full ${side === "source" ? "-right-3" : "-left-3"}`}
            onPointerDown={(e) =>
              onConnect(e, {
                nodeId: node.id,
                handleType: side as "source" | "target",
              })
            }
          >
            <Plus />
          </Button>
        ))}
      {selected && (
        <Button
          aria-label="调整节点尺寸"
          variant="ghost"
          size="icon-xs"
          className="absolute right-0 bottom-0 cursor-nwse-resize"
          onPointerDown={(e) => onResize(e, node)}
        >
          <Grip />
        </Button>
      )}
    </div>
  );
}
