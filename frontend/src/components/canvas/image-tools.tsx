/* eslint-disable @next/next/no-img-element -- Local user media stays in the browser; no image server upload. */
"use client";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Label } from "@/components/ui/label";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { cropDataUrl, splitDataUrl, upscaleDataUrl } from "./image-data";
import { createNode } from "./document";
import { fitNodeSize } from "./node-size";
import type { CanvasNodeData } from "./types";

export function ImageTools({
  node,
  mode,
  onClose,
  onResult,
}: {
  node: CanvasNodeData;
  mode: "crop" | "split" | "resize";
  onClose: () => void;
  onResult: (nodes: CanvasNodeData[]) => void;
}) {
  const [values, setValues] = useState({
    x: 0,
    y: 0,
    width: 100,
    height: 100,
    rows: 2,
    columns: 2,
    edge: 2048,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [algorithm, setAlgorithm] = useState<"nearest" | "bilinear" | "high">(
    "high",
  );
  const title = { crop: "裁剪图片", split: "拆分图片", resize: "调整图片像素" }[
    mode
  ];
  const fields =
    mode === "crop"
      ? (["x", "y", "width", "height"] as const)
      : mode === "split"
        ? (["rows", "columns"] as const)
        : (["edge"] as const);
  const labels = {
    x: "左侧 (%)",
    y: "顶部 (%)",
    width: "宽度 (%)",
    height: "高度 (%)",
    rows: "行数",
    columns: "列数",
    edge: "长边像素",
  };
  const run = async () => {
    setError("");
    setBusy(true);
    try {
      if (
        mode === "crop" &&
        (values.x < 0 ||
          values.y < 0 ||
          values.width <= 0 ||
          values.height <= 0 ||
          values.x + values.width > 100 ||
          values.y + values.height > 100)
      )
        throw new Error("裁剪范围须位于原图内");
      if (
        mode === "split" &&
        ![values.rows, values.columns].every(
          (v) => Number.isInteger(v) && v >= 1 && v <= 8,
        )
      )
        throw new Error("行列数须为 1–8 的整数");
      if (
        mode === "resize" &&
        (!Number.isInteger(values.edge) ||
          values.edge < 1 ||
          values.edge > 4096)
      )
        throw new Error("长边须为 1–4096 像素");
      const source = node.metadata?.content || "";
      const urls =
        mode === "crop"
          ? [
              await cropDataUrl(source, {
                x: values.x / 100,
                y: values.y / 100,
                width: values.width / 100,
                height: values.height / 100,
              }),
            ]
          : mode === "resize"
            ? [
                await upscaleDataUrl(source, {
                  targetLongEdge: values.edge,
                  algorithm,
                }),
              ]
            : (await splitDataUrl(source, values)).map((p) => p.dataUrl);
      const results = await Promise.all(
        urls.map(async (content, i) => {
          const image = await new Promise<HTMLImageElement>(
            (resolve, reject) => {
              const image = new Image();
              image.onload = () => resolve(image);
              image.onerror = () => reject(new Error("处理结果无法读取"));
              image.src = content;
            },
          );
          const item = createNode("image", { x: 0, y: 0 });
          return {
            ...item,
            title: `${node.title} · ${title}${urls.length > 1 ? ` ${i + 1}` : ""}`,
            position: {
              x: node.position.x + node.width + 40 + (i % values.columns) * 360,
              y: node.position.y + Math.floor(i / values.columns) * 360,
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
      onResult(results);
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "图片处理失败");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className="sm:max-w-xl"
        onEscapeKeyDown={(e) => {
          if (busy) e.preventDefault();
        }}
      >
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>本地处理并创建新图片，保留原件。</DialogDescription>
        <div className="relative mx-auto max-h-64 max-w-full overflow-hidden">
          <img
            src={node.metadata?.content}
            alt="处理预览"
            className="max-h-64 max-w-full object-contain"
          />
          {mode === "crop" && (
            <div
              className="pointer-events-none absolute border-2 border-primary bg-primary/10"
              style={{
                left: `${values.x}%`,
                top: `${values.y}%`,
                width: `${values.width}%`,
                height: `${values.height}%`,
              }}
            />
          )}
        </div>
        <div className="grid grid-cols-2 gap-3">
          {fields.map((key) => (
            <Label key={key} className="space-y-1 text-sm">
              {labels[key]}
              <Input
                type="number"
                value={values[key]}
                disabled={busy}
                onChange={(e) =>
                  setValues((v) => ({ ...v, [key]: Number(e.target.value) }))
                }
              />
            </Label>
          ))}
        </div>
        {mode === "resize" && (
          <Label className="text-sm">
            插值算法
            <NativeSelect
              className="ml-2 rounded-md border bg-background p-2"
              value={algorithm}
              onChange={(e) => setAlgorithm(e.target.value as typeof algorithm)}
            >
              <NativeSelectOption value="nearest">最近邻</NativeSelectOption>
              <NativeSelectOption value="bilinear">双线性</NativeSelectOption>
              <NativeSelectOption value="high">高质量插值</NativeSelectOption>
            </NativeSelect>
          </Label>
        )}
        {error && (
          <p role="alert" className="text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button disabled={busy} onClick={() => void run()}>
            {busy ? "处理中…" : "应用"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
