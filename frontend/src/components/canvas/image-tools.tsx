"use client";
import { useEffect, useRef, useState } from "react";
import { CanvasNodeCropDialog } from "./media-tools/crop-dialog";
import { CanvasNodeSplitDialog } from "./media-tools/split-dialog";
import { CanvasNodeUpscaleDialog } from "./media-tools/upscale-dialog";
import { DerivedMediaSave } from "./media-tools/derived-save";
import { readImageMeta } from "./media-tools/image-meta";
import { cropDataUrl, splitDataUrl, upscaleDataUrl } from "./image-data";
import { createNode } from "./document";
import { fitNodeSize } from "./node-size";
import type { CanvasNodeData } from "./types";

/** Media controls using the current project media and graph contracts. */
export function ImageTools({
  node,
  mode,
  projectID,
  onClose,
  onResult,
}: {
  node: CanvasNodeData;
  mode: "crop" | "split" | "resize";
  projectID?: string;
  onClose: () => void;
  onResult: (nodes: CanvasNodeData[]) => boolean | Promise<boolean>;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [derived, setDerived] = useState<CanvasNodeData[] | null>(null);
  const mounted = useRef(true),
    running = useRef(false);
  const source = node.metadata?.content ?? "";
  useEffect(() => {
    mounted.current = true;
    void readImageMeta(source).catch((error) => {
      if (mounted.current) setError(error.message);
    });
    return () => {
      mounted.current = false;
    };
  }, [source]);
  const process = async (
    produce: () => Promise<{ content: string; row: number; column: number }[]>,
  ) => {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setError("");
    try {
      const pieces = await produce();
      const results = await Promise.all(
        pieces.map(async ({ content, row, column }, i) => {
          const image = await readImageMeta(content);
          return {
            ...createNode("image", { x: 0, y: 0 }),
            title: `${node.title} · ${{ crop: "裁剪", split: "切片", resize: "像素调整" }[mode]} ${i + 1}`,
            position: {
              x: node.position.x + node.width + 40 + column * 360,
              y: node.position.y + row * 360,
            },
            ...fitNodeSize(image.width, image.height, 320, 320),
            metadata: {
              content,
              naturalWidth: image.width,
              naturalHeight: image.height,
              mimeType: "image/png",
            },
          };
        }),
      );
      if (!mounted.current) return;
      if (projectID) setDerived(results);
      else if (await onResult(results)) onClose();
      else throw new Error("处理结果尚未保存，请重试");
    } catch (error) {
      if (mounted.current)
        setError(error instanceof Error ? error.message : "图片处理失败");
    } finally {
      running.current = false;
      if (mounted.current) setBusy(false);
    }
  };
  if (derived && projectID)
    return (
      <DerivedMediaSave
        nodes={derived}
        projectID={projectID}
        onSave={onResult}
        onClose={onClose}
      />
    );
  const props = {
    dataUrl: source,
    open: true,
    busy,
    error,
    onClose: () => {
      if (!running.current) onClose();
    },
  };
  if (mode === "crop")
    return (
      <CanvasNodeCropDialog
        {...props}
        onConfirm={(crop) =>
          void process(async () => [
            { content: await cropDataUrl(source, crop), row: 0, column: 0 },
          ])
        }
      />
    );
  if (mode === "split")
    return (
      <CanvasNodeSplitDialog
        {...props}
        onConfirm={(params) =>
          void process(async () =>
            (await splitDataUrl(source, params)).map((piece) => ({
              ...piece,
              content: piece.dataUrl,
            })),
          )
        }
      />
    );
  return (
    <CanvasNodeUpscaleDialog
      {...props}
      onConfirm={(params) =>
        void process(async () => [
          { content: await upscaleDataUrl(source, params), row: 0, column: 0 },
        ])
      }
    />
  );
}
