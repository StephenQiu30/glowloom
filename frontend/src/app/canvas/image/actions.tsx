"use client";
import { useEffect, useRef } from "react";
import {
  Crop,
  Download,
  Grid2X2,
  Maximize2,
  Scan,
  Sparkles,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { DownloadButton } from "@/components/media/download-button";
import { canvasMediaSource } from "./source";
import type { ImageOperation } from "./operations";
import type { Node } from "../document";

export function CanvasMediaActions({
  node,
  projectID,
  onPreview,
  onImageTool,
  onCreate,
  disabled,
}: {
  node: Node;
  projectID: string;
  onPreview: () => void;
  onImageTool: (kind: ImageOperation["kind"]) => void;
  onCreate: () => void;
  disabled: boolean;
}) {
  const download = useRef<AbortController | null>(null);
  useEffect(() => () => download.current?.abort(), [projectID, node.id]);
  const actions = [
    { label: "预览素材", Icon: Scan, run: onPreview, disabled: false },
    ...(node.node_type === "image"
      ? [
          {
            label: "裁剪图片",
            Icon: Crop,
            run: () => onImageTool("crop"),
            disabled,
          },
          {
            label: "拆分图片",
            Icon: Grid2X2,
            run: () => onImageTool("split"),
            disabled,
          },
          {
            label: "缩放图片",
            Icon: Maximize2,
            run: () => onImageTool("resize"),
            disabled,
          },
          { label: "以此图继续创作", Icon: Sparkles, run: onCreate, disabled },
        ]
      : []),
  ];
  return (
    <>
      {actions.map(({ label, Icon, run, disabled }) => (
        <Tooltip key={label}>
          <TooltipTrigger asChild>
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={label}
              onClick={run}
              disabled={disabled}
            >
              <Icon data-icon="inline-start" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{label}</TooltipContent>
        </Tooltip>
      ))}
      <DownloadButton
        filename={node.title}
        load={async () => {
          const controller = new AbortController();
          download.current = controller;
          const result = await canvasMediaSource(
            projectID,
            node.ref_id!,
            controller.signal,
          );
          controller.signal.throwIfAborted();
          return result.blob;
        }}
      >
        <Download data-icon="inline-start" />
        下载原件
      </DownloadButton>
    </>
  );
}
