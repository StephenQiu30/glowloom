/* eslint-disable @next/next/no-img-element -- Source media previews for local canvas processing. */
// Attribution and source revision are recorded in /licenses/canvas-sources.md.
// See /licenses/canvas-sources.md.
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
} from "@/components/ui/field";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ImagePlus } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  MAX_UPSCALE_LONG_EDGE,
  resolveUpscaleSize,
  type ImageUpscaleAlgorithm,
  type ImageUpscaleParams,
} from "../image-data";
import { readImageMeta } from "./image-meta";

export type CanvasImageUpscaleParams = ImageUpscaleParams;

const algorithms: ImageUpscaleAlgorithm[] = ["high", "bilinear", "nearest"];
const algorithmLabels = {
  high: ["高清插值", "适合照片和细节图"],
  bilinear: ["双线性", "平滑、速度快"],
  nearest: ["最近邻", "适合像素风格"],
};

const targetOptions = [
  { label: "1K", value: 1024 },
  { label: "2K", value: 2048 },
  { label: "4K", value: MAX_UPSCALE_LONG_EDGE },
];

const defaultParams: CanvasImageUpscaleParams = {
  targetLongEdge: 2048,
  algorithm: "high",
};

export function CanvasNodeUpscaleDialog({
  dataUrl,
  open,
  onClose,
  onConfirm,
  busy = false,
  error,
}: {
  dataUrl: string;
  open: boolean;
  busy?: boolean;
  error?: string;
  onClose: () => void;
  onConfirm: (params: CanvasImageUpscaleParams) => void;
}) {
  const [params, setParams] = useState<CanvasImageUpscaleParams>(defaultParams);
  const [image, setImage] = useState<{ width: number; height: number } | null>(
    null,
  );
  const sourceLongEdge = image ? Math.max(image.width, image.height) : 0;
  const outputSize = useMemo(
    () =>
      image
        ? resolveUpscaleSize(image.width, image.height, params.targetLongEdge)
        : null,
    [image, params.targetLongEdge],
  );
  const canUpscale = Boolean(
    image &&
    sourceLongEdge < params.targetLongEdge &&
    params.targetLongEdge <= MAX_UPSCALE_LONG_EDGE,
  );
  const reachedMax = Boolean(image && sourceLongEdge >= MAX_UPSCALE_LONG_EDGE);

  useEffect(() => {
    if (!open) return;
    // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setParams(defaultParams);
    setImage(null);
  }, [dataUrl, open]);

  useEffect(() => {
    if (!open) return;
    let active = true;
    void readImageMeta(dataUrl)
      .then((value) => {
        if (active) setImage(value);
      })
      .catch(() => {
        if (active) setImage(null);
      });
    return () => {
      active = false;
    };
  }, [dataUrl, open]);

  useEffect(() => {
    if (!image) return;
    const nextTarget =
      targetOptions.find((option) => sourceLongEdge < option.value)?.value ||
      MAX_UPSCALE_LONG_EDGE;
    // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setParams((current) => ({ ...current, targetLongEdge: nextTarget }));
  }, [image, sourceLongEdge]);

  return (
    <Dialog
      open={open && Boolean(dataUrl)}
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault();
        }}
        onPointerDownOutside={(event) => {
          if (busy) event.preventDefault();
        }}
        className={
          "max-h-[calc(100dvh-2rem)] w-full overflow-y-auto sm:max-w-3xl"
        }
      >
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {busy && <p role="status">正在处理图片…</p>}
        <DialogHeader>
          <DialogTitle className="sr-only">图片放大</DialogTitle>
          <DialogDescription className="sr-only">
            选择目标尺寸和插值算法，创建放大后的图片节点。
          </DialogDescription>
        </DialogHeader>
        <div>
          <div className="flex flex-col gap-5">
            <div>
              <h2 className="text-xl font-semibold">{"图片放大"}</h2>
            </div>
            <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-2">
              <Card>
                <CardHeader className="sr-only">
                  <CardTitle>源图预览</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="grid min-h-48 place-items-center rounded-lg bg-black/5">
                    <img
                      src={dataUrl}
                      alt=""
                      className="max-h-[min(50dvh,28rem)] max-w-full rounded-lg object-contain shadow-xl"
                      draggable={false}
                    />
                  </div>
                  <div className="mt-3 flex items-center justify-between text-sm">
                    <span className="opacity-60">{"源图"}</span>
                    <span className="font-semibold">
                      {image ? `${image.width} x ${image.height} px` : "读取中"}
                    </span>
                  </div>
                </CardContent>
              </Card>
              <FieldGroup className="py-2">
                <Field>
                  <FieldLabel>目标像素</FieldLabel>
                  <ToggleGroup
                    type="single"
                    variant="outline"
                    className="w-full flex-wrap"
                    aria-label="目标像素"
                    value={String(params.targetLongEdge)}
                    onValueChange={(value) => {
                      if (value)
                        ((value) =>
                          setParams((current) => ({
                            ...current,
                            targetLongEdge: Number(value),
                          })))(value);
                    }}
                  >
                    {targetOptions
                      .map((option) => ({
                        label: `${option.label} · ${option.value}px`,
                        value: option.value,
                        disabled: Boolean(
                          image && sourceLongEdge >= option.value,
                        ),
                      }))
                      .map((item) => {
                        const option =
                          typeof item === "object"
                            ? item
                            : { value: item, label: item };
                        return (
                          <ToggleGroupItem
                            key={String(option.value)}
                            value={String(option.value)}
                            className="h-auto max-w-full py-2 whitespace-normal"
                            disabled={
                              "disabled" in option
                                ? Boolean(option.disabled)
                                : false
                            }
                          >
                            {option.label}
                          </ToggleGroupItem>
                        );
                      })}
                  </ToggleGroup>
                  {image && !canUpscale ? (
                    <FieldDescription>
                      {reachedMax
                        ? "图片已达到 4K，无需放大"
                        : "图片已达到当前目标像素，无需放大"}
                    </FieldDescription>
                  ) : null}
                </Field>
                <Field>
                  <FieldLabel>放大算法</FieldLabel>
                  <ToggleGroup
                    type="single"
                    variant="outline"
                    className="w-full flex-wrap"
                    value={String(params.algorithm)}
                    aria-label="放大算法"
                    onValueChange={(value) => {
                      if (value)
                        ((value) =>
                          setParams((current) => ({
                            ...current,
                            algorithm: value as ImageUpscaleAlgorithm,
                          })))(value);
                    }}
                  >
                    {algorithms
                      .map((algorithm) => ({
                        value: algorithm,
                        label: (
                          <span className="flex min-h-12 flex-col justify-center text-left leading-5">
                            <span className="font-medium">
                              {algorithmLabels[algorithm][0]}
                            </span>
                            <span className="text-xs opacity-55">
                              {algorithmLabels[algorithm][1]}
                            </span>
                          </span>
                        ),
                      }))
                      .map((item) => {
                        const option =
                          typeof item === "object"
                            ? item
                            : { value: item, label: item };
                        return (
                          <ToggleGroupItem
                            key={String(option.value)}
                            value={String(option.value)}
                            className="h-auto max-w-full py-2 whitespace-normal"
                          >
                            {option.label}
                          </ToggleGroupItem>
                        );
                      })}
                  </ToggleGroup>
                </Field>
                <Card size="sm">
                  <CardHeader>
                    <CardTitle>输出尺寸</CardTitle>
                  </CardHeader>
                  <CardContent>
                    {outputSize
                      ? `${outputSize.width} x ${outputSize.height} px`
                      : "未知"}
                  </CardContent>
                </Card>
              </FieldGroup>
            </div>
            <div className="flex justify-end">
              <Button
                onClick={() => onConfirm(params)}
                type={"button"}
                variant={"default"}
                size="lg"
                disabled={busy || !canUpscale}
              >
                {<ImagePlus data-icon="inline-start" />}
                {"生成放大图"}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
