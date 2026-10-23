"use client";
import { canvasConnectionGeometry } from "./connection-geometry";

import { TooltipProvider } from "@/components/ui/tooltip";

import { useEffect, useRef, useState } from "react";
import { Type, ImageIcon, Video, AudioLines, Box } from "lucide-react";
import {
  CanvasHeader,
  CanvasToolPalette,
  CanvasViewControls,
  CanvasAssistantEntry,
} from "./chrome";
import type { CanvasBackgroundMode } from "./theme";
import { ProjectMediaPreview } from "@/components/media/project-media";
import {
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { InfiniteCanvas } from "./infinite-canvas";
import { CanvasTextPreview } from "./text-preview";
import type { ViewportTransform } from "./types";

const labels: Record<string, string> = {
  image_generation: "图片生成",
  video_generation: "视频生成",
  generation: "生成",
  director: "导演",
  timeline: "时间线",
  batch_table: "批量任务",
  model: "3D 模型",
  image: "图片",
  video: "视频",
  audio: "音频",
  text: "文本",
  group: "分组",
};

function NodeContent({
  node,
  projectID,
}: {
  node: API.NodeDTO;
  projectID: string;
}) {
  const content = node.config?.text ?? node.config?.draft?.prompt;
  const raw = node.config?.composer ?? node.config?.draft?.composer;
  let composer: PromptDraft | undefined;
  if (raw) {
    try {
      validatePromptDraft(raw);
      composer = raw;
    } catch {
      /* Preserve opaque content in the original snapshot. */
    }
  }
  if (
    node.media_ref?.asset_id &&
    ["image", "video", "audio"].includes(node.node_type ?? "")
  )
    return (
      <div className="size-full overflow-hidden rounded-xl" data-canvas-no-zoom>
        <ProjectMediaPreview
          projectID={projectID}
          id={node.media_ref.asset_id}
          kind={node.node_type}
          fit="contain"
          large
        />
      </div>
    );
  return (
    <div
      className="size-full overflow-auto p-4 text-sm whitespace-pre-wrap"
      data-canvas-no-zoom
    >
      <p className="mb-3 text-xs text-muted-foreground">
        {labels[node.node_type ?? ""] ?? "历史节点"}
      </p>
      {composer || content ? (
        <CanvasTextPreview
          node={{
            id: node.id!,
            type: "text",
            title: node.title ?? "",
            position: { x: node.x!, y: node.y! },
            width: node.width!,
            height: node.height!,
            metadata: { content, composer },
          }}
        />
      ) : (
        node.node_type !== "group" && (
          <p className="text-muted-foreground">
            此节点的内容已保留，暂不支持编辑
          </p>
        )
      )}
    </div>
  );
}

/** Display the entire authorized snapshot without exposing any mutation path. */
export function CanvasSnapshotViewer({
  document,
  projectID,
  onRefresh,
}: {
  document: API.DocumentDTO;
  projectID: string;
  onRefresh: () => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const [viewport, setViewport] = useState<ViewportTransform>({
    x: 0,
    y: 0,
    k: 1,
  });
  const nodes = document.nodes ?? [];
  const [background, setBackground] = useState<CanvasBackgroundMode>("dots");
  const [connections, setConnections] = useState(true);
  function locate(id: string) {
    const node = nodes.find((n) => n.id === id),
      rect = container.current?.getBoundingClientRect();
    if (!node || !rect) return;
    setViewport((v) => ({
      ...v,
      x: rect.width / 2 - (node.x! + node.width! / 2) * v.k,
      y: rect.height / 2 - (node.y! + node.height! / 2) * v.k,
    }));
  }
  function zoom(value: number) {
    const rect = container.current?.getBoundingClientRect();
    if (!rect) return;
    const k = Math.max(0.05, Math.min(5, value));
    setViewport((v) => ({
      k,
      x: rect.width / 2 - ((rect.width / 2 - v.x) * k) / v.k,
      y: rect.height / 2 - ((rect.height / 2 - v.y) * k) / v.k,
    }));
  }
  function fit() {
    const rect = container.current?.getBoundingClientRect();
    if (!rect || !nodes.length) return;
    const left = Math.min(...nodes.map((n) => n.x!));
    const top = Math.min(...nodes.map((n) => n.y!));
    const right = Math.max(...nodes.map((n) => n.x! + n.width!));
    const bottom = Math.max(...nodes.map((n) => n.y! + n.height!));
    const k = Math.max(
      0.05,
      Math.min(
        1,
        (rect.width - 160) / (right - left),
        (rect.height - 220) / (bottom - top),
      ),
    );
    setViewport({
      x: rect.width / 2 - ((left + right) / 2) * k,
      y: rect.height / 2 - ((top + bottom) / 2) * k,
      k,
    });
  }
  useEffect(() => {
    const element = container.current;
    if (!element) return;
    const observer = new ResizeObserver(fit);
    observer.observe(element);
    fit();
    return () => observer.disconnect();
    // This view owns its local viewport; only a new server snapshot resets it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [document]);
  const byID = new Map(nodes.map((node) => [node.id, node]));
  return (
    <TooltipProvider>
      <div className="relative h-dvh overflow-hidden bg-background text-foreground">
        <InfiniteCanvas
          containerRef={container}
          viewport={viewport}
          tool="pan"
          backgroundMode={background}
          onViewportChange={setViewport}
        >
          <svg
            className="pointer-events-none absolute top-0 left-0 size-px overflow-visible"
            aria-label="画布关系"
          >
            {connections &&
              document.edges?.map((edge) => {
                const from = byID.get(edge.source_node_id),
                  to = byID.get(edge.target_node_id);
                if (!from || !to) return null;
                return (
                  <path
                    key={edge.id}
                    data-connection-id={edge.id}
                    d={
                      canvasConnectionGeometry(
                        {
                          x: from.x! + from.width!,
                          y: from.y! + from.height! / 2,
                        },
                        { x: to.x!, y: to.y! + to.height! / 2 },
                      ).path
                    }
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={1.25}
                    className="text-muted-foreground"
                  />
                );
              })}
          </svg>
          {nodes.map((node) => (
            <section
              key={node.id}
              data-node-id={node.id}
              aria-label={`节点：${node.title || labels[node.node_type ?? ""] || "历史节点"}`}
              className={`absolute rounded-xl border ${node.node_type === "group" ? "bg-muted/30" : "bg-card"}`}
              style={{
                transform: `translate(${node.x}px, ${node.y}px)`,
                width: node.width,
                height: node.height,
                zIndex: node.node_type === "group" ? 0 : 1,
              }}
            >
              <h2 className="absolute -top-7 flex max-w-full items-center gap-1.5 truncate text-sm text-muted-foreground">
                {node.node_type === "text" ? (
                  <Type className="size-3.5 shrink-0" />
                ) : node.node_type?.includes("image") ? (
                  <ImageIcon className="size-3.5 shrink-0" />
                ) : node.node_type?.includes("video") ? (
                  <Video className="size-3.5 shrink-0" />
                ) : node.node_type === "audio" ? (
                  <AudioLines className="size-3.5 shrink-0" />
                ) : (
                  <Box className="size-3.5 shrink-0" />
                )}
                {node.title || labels[node.node_type ?? ""] || "历史节点"}
              </h2>
              <NodeContent node={node} projectID={projectID} />
            </section>
          ))}
        </InfiniteCanvas>
        <CanvasHeader
          name={document.name || "未命名画布"}
          status="只读"
          nodes={nodes.map((n) => ({
            id: n.id!,
            title: n.title || labels[n.node_type ?? ""] || "历史节点",
          }))}
          onLocate={locate}
          onRefresh={onRefresh}
        />
        <CanvasToolPalette readOnly />
        <CanvasViewControls
          zoom={viewport.k}
          onZoom={zoom}
          onFit={fit}
          tool="pan"
          connections={connections}
          onConnections={() => setConnections((v) => !v)}
          background={background}
          onBackground={setBackground}
        />
        <CanvasAssistantEntry />
        <p
          role="status"
          className="absolute right-4 bottom-16 left-20 mx-auto max-w-xl rounded-lg bg-background/95 p-2 text-center text-xs leading-5 text-muted-foreground sm:right-40 sm:bottom-3 sm:left-64"
        >
          此画布包含暂不支持编辑的历史内容，当前可查看、平移和缩放。原节点与关系均已保留。
        </p>
      </div>
    </TooltipProvider>
  );
}
