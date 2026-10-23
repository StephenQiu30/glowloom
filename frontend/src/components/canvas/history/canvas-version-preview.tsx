"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { RichTextContent } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-content";
import { documentText } from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
import { MediaPreview } from "@/components/canvas/runtime/components/generation/workspace/media-preview";
import { fromCanvasGraph } from "../lib/canvas-document-adapter";
import { useCanvasFilePreviews } from "../hooks/use-canvas-file-previews";

/** Isolated read model: no editing store, save callback or generation action. */
export function CanvasVersionPreview({
  graph,
  enabled,
  mediaContext,
}: {
  graph: CanvasContract.CanvasDocumentGraphDTO;
  enabled: boolean;
  mediaContext?: import("../lib/canvas-file-preview").CanvasFilePreviewContext;
}) {
  const [editor] = useState(() => fromCanvasGraph(graph));
  const [nodes, setNodes] = useState(editor.nodes);
  const [selectedId, setSelectedId] = useState(nodes[0]?.id);
  useCanvasFilePreviews(nodes, enabled, setNodes, mediaContext);
  const selected = nodes.find((node) => node.id === selectedId);
  const left = Math.min(0, ...nodes.map((n) => n.position.x)) - 24;
  const top = Math.min(0, ...nodes.map((n) => n.position.y)) - 24;
  const width =
    Math.max(320, ...nodes.map((n) => n.position.x + n.width)) - left + 24;
  const height =
    Math.max(180, ...nodes.map((n) => n.position.y + n.height)) - top + 24;
  const byId = new Map(nodes.map((n) => [n.id, n]));
  return (
    <section aria-label="历史画布只读预览" className="min-w-0 space-y-4">
      <svg
        viewBox={`${left} ${top} ${width} ${height}`}
        role="img"
        aria-label="节点和连线概览"
        className="h-64 w-full rounded-lg border bg-muted/30"
      >
        {editor.connections.map((edge) => {
          const from = byId.get(edge.fromNodeId),
            to = byId.get(edge.toNodeId);
          return from && to ? (
            <line
              key={edge.id}
              x1={from.position.x + from.width}
              y1={from.position.y + from.height / 2}
              x2={to.position.x}
              y2={to.position.y + to.height / 2}
              className="stroke-muted-foreground"
              strokeWidth={2}
            />
          ) : null;
        })}
        {nodes.map((node) => (
          <g key={node.id}>
            <rect
              x={node.position.x}
              y={node.position.y}
              width={node.width}
              height={node.height}
              rx={8}
              className={
                node.id === selectedId
                  ? "fill-background stroke-primary"
                  : "fill-background stroke-border"
              }
              strokeWidth={3}
            />
            <text
              x={node.position.x + 12}
              y={node.position.y + 28}
              className="fill-foreground"
              fontSize={18}
            >
              {node.title.slice(0, 24)}
            </text>
          </g>
        ))}
      </svg>
      <p className="text-sm text-muted-foreground">
        {nodes.length} 个节点 · {editor.connections.length} 条连线 · 只读预览
      </p>
      <div
        className="flex max-h-32 flex-wrap gap-2 overflow-auto"
        aria-label="预览节点"
      >
        {nodes.map((node) => (
          <Button
            key={node.id}
            size="sm"
            variant={selectedId === node.id ? "secondary" : "outline"}
            aria-pressed={selectedId === node.id}
            onClick={() => setSelectedId(node.id)}
          >
            {node.title}
          </Button>
        ))}
      </div>
      {!nodes.length && <p className="text-muted-foreground">此版本是空画布</p>}
      {selected && (
        <div className="space-y-3 rounded-lg border p-4">
          <h3 className="font-medium">{selected.title}</h3>
          {selected.metadata?.richText && (
            <RichTextContent
              value={documentText(selected.metadata.richText.content)}
              document={selected.metadata.richText}
            />
          )}
          {selected.metadata?.content && selected.type === "image" && (
            <MediaPreview
              src={selected.metadata.content}
              alt={selected.title}
              className="max-h-64 w-full object-contain"
            />
          )}
          {selected.metadata?.content && selected.type === "video" && (
            <video
              src={selected.metadata.content}
              controls
              preload="metadata"
              className="max-h-64 w-full"
            />
          )}
          {selected.metadata?.content && selected.type === "audio" && (
            <audio
              src={selected.metadata.content}
              controls
              preload="metadata"
              className="w-full"
            />
          )}
          {!!selected.documentData?.fileReferences?.length &&
            !selected.metadata?.content && (
              <p className="text-sm text-muted-foreground">
                素材预览不可用时，仍保留原始文件引用。
              </p>
            )}
          <dl className="space-y-2 text-sm text-muted-foreground">
            <div>
              <dt>内容方式</dt>
              <dd>
                {selected.documentData?.parameters?.mode === "GENERATE"
                  ? "生成"
                  : "手动内容"}
              </dd>
            </div>
            <div>
              <dt>参考来源</dt>
              <dd>
                {graph.connections
                  .filter((edge) => edge.toNodeId === selected.id)
                  .map((edge) => byId.get(edge.fromNodeId)?.title)
                  .join("、") || "无"}
              </dd>
            </div>
            <div>
              <dt>固定素材</dt>
              <dd>{selected.documentData?.fileReferences?.length ?? 0} 项</dd>
            </div>
          </dl>
        </div>
      )}
    </section>
  );
}
