"use client";
/* eslint-disable @next/next/no-img-element -- These are authorized short-lived canvas media previews. */
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Download, Layers, LayoutGrid, Palette, Ungroup } from "lucide-react";
import type { CanvasNodeData, ViewportTransform } from "../types/canvas";
import {
  containerKind,
  type ContainerBackground,
} from "../lib/canvas-containers";
import { canvasTextBackgrounds } from "../lib/canvas-text-background";
import { RichTextContent } from "../runtime/components/editor/rich-text/rich-text-content";
import { canvasNodeTypeLabel } from "../search/canvas-node-search-state";

export function CanvasStackCard({
  members,
  onOpen,
}: {
  members: CanvasNodeData[];
  onOpen: () => void;
}) {
  return (
    <div
      className="relative flex h-full flex-col items-center justify-center gap-3 p-6"
      data-canvas-no-zoom
      onMouseDown={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <div className="relative h-24 w-4/5" aria-hidden>
        {members
          .slice(0, 3)
          .reverse()
          .map((node, index) => (
            <div
              key={node.id}
              className="absolute inset-0 overflow-hidden rounded-xl border bg-card p-3 shadow-sm"
              style={{
                transform: `translate(${index * 5}px,${index * -5}px) rotate(${(index - 1) * 3}deg)`,
              }}
            >
              {node.type === "image" && node.metadata?.content ? (
                <img
                  src={node.metadata.content}
                  alt=""
                  className="h-full w-full object-cover"
                />
              ) : (
                <p className="line-clamp-3 text-xs whitespace-pre-wrap">
                  {node.metadata?.content && node.type === "text"
                    ? node.metadata.content
                    : node.title}
                </p>
              )}
            </div>
          ))}
      </div>
      <span className="text-sm">堆叠 · {members.length} 个节点</span>
      <Button
        variant="secondary"
        size="sm"
        onClick={onOpen}
        aria-label="打开堆叠画廊"
      >
        <Layers className="size-4" />
        打开画廊
      </Button>
    </div>
  );
}
function MemberContent({ node }: { node: CanvasNodeData }) {
  const source = node.metadata?.content;
  if (node.type === "text")
    return node.metadata?.richText ? (
      <RichTextContent value={source ?? ""} document={node.metadata.richText} />
    ) : (
      <p className="whitespace-pre-wrap">{source ?? ""}</p>
    );
  if (source && node.type === "image")
    return (
      <img
        src={source}
        alt={node.title}
        className="max-h-[60vh] w-full object-contain"
      />
    );
  if (source && node.type === "video")
    return (
      <video
        src={source}
        controls
        playsInline
        className="max-h-[60vh] w-full"
      />
    );
  if (source && node.type === "audio")
    return <audio src={source} controls className="w-full" />;
  return (
    <p className="text-sm text-muted-foreground">
      {canvasNodeTypeLabel(node.type, node)} ·{" "}
      {source ? "请在对应编辑器查看完整内容" : "暂无可预览内容"}
    </p>
  );
}
export function CanvasStackGallery({
  container,
  members,
  onClose,
  scale = 0.6,
}: {
  container: CanvasNodeData | null;
  members: CanvasNodeData[];
  onClose: () => void;
  scale?: number;
}) {
  return (
    <Dialog
      open={!!container}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="top-0 left-0 h-dvh max-h-none w-screen max-w-none translate-x-0 translate-y-0 overflow-y-auto rounded-none sm:max-w-none"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onMouseDown={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>
            {container?.title} · {members.length} 个节点
          </DialogTitle>
          <DialogDescription>查看堆叠中的原始节点内容</DialogDescription>
        </DialogHeader>
        <div className="flex min-h-full flex-wrap content-center items-center justify-center gap-8 p-10">
          {members.map((node) => (
            <section
              key={node.id}
              aria-label={`${canvasNodeTypeLabel(node.type, node)}：${node.title}`}
              className="min-w-0 shrink-0"
              style={{
                width: Math.max(160, Math.min(node.width * scale, 480)),
              }}
            >
              <h3 className="mb-4 text-sm font-medium">{node.title}</h3>
              <div
                className="overflow-auto rounded-xl border bg-card p-4"
                style={{
                  minHeight: Math.max(160, Math.min(node.height * scale, 480)),
                }}
              >
                <MemberContent node={node} />
              </div>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
export function CanvasContainerToolbar({
  node,
  members,
  viewport,
  readOnly,
  onRelease,
  onLayout,
  onBackground,
  onDownload,
}: {
  node: CanvasNodeData;
  members: CanvasNodeData[];
  viewport: ViewportTransform;
  readOnly: boolean;
  onRelease: () => void;
  onLayout: (mode: "grid" | "smart") => void;
  onBackground: (value: ContainerBackground) => void;
  onDownload: () => void;
}) {
  const stack = containerKind(node) === "stack";
  const background = node.serviceData?.config?.container?.background ?? "none";
  const storyboardReason =
    members.some((member) => member.type !== "image") || members.length > 49
      ? "分镜组仅支持图片，最多支持49个图片节点"
      : !members.some(
            (member) =>
              member.documentData?.fileReferences?.some(
                (ref) => ref.purpose === "CONTENT",
              ) && member.metadata?.content,
          )
        ? "所选节点均无可用图片"
        : "分镜组成功态尚未接入";
  return (
    <div
      role="toolbar"
      aria-label={stack ? "堆叠操作" : "组操作"}
      className="absolute left-1/2 z-[70] flex -translate-x-1/2 flex-wrap items-center gap-1 rounded-xl border bg-popover p-2 shadow-lg"
      style={{
        top: Math.max(116, viewport.y + node.position.y * viewport.k - 64),
      }}
      data-canvas-no-zoom
      onMouseDown={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <span className="px-2 text-sm">{members.length} 个节点</span>
      {!stack && (
        <Button variant="ghost" size="sm" disabled title={storyboardReason}>
          转为分镜组<span className="sr-only">{storyboardReason}</span>
        </Button>
      )}
      <Button variant="ghost" size="sm" disabled={readOnly} onClick={onRelease}>
        <Ungroup className="size-4" />
        {stack ? "解除堆叠" : "解除编组"}
      </Button>
      {!stack && (
        <>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" disabled={readOnly}>
                <LayoutGrid className="size-4" />
                布局
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              data-canvas-no-zoom
              onMouseDown={(e) => e.stopPropagation()}
            >
              <DropdownMenuItem onSelect={() => onLayout("grid")}>
                宫格布局
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onLayout("smart")}>
                智能布局
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" disabled={readOnly}>
                <Palette className="size-4" />
                背景色
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              data-canvas-no-zoom
              onMouseDown={(e) => e.stopPropagation()}
            >
              {canvasTextBackgrounds.map((color) => {
                const value = color.value === "default" ? "none" : color.value;
                return (
                  <DropdownMenuItem
                    key={value}
                    onSelect={() => onBackground(value)}
                  >
                    <span
                      className="size-4 rounded-full border"
                      style={{
                        background:
                          color.value === "default"
                            ? "transparent"
                            : color.color,
                      }}
                    />
                    {color.label}
                    {background === value ? " ✓" : ""}
                  </DropdownMenuItem>
                );
              })}
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
      <Button variant="ghost" size="sm" onClick={onDownload}>
        <Download className="size-4" />
        下载
      </Button>
    </div>
  );
}
