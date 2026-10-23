"use client";

import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useState, type ComponentProps } from "react";

type MediaPreviewProps = ComponentProps<"img"> & {
  previewSrc?: string;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

/** Keep lightweight thumbnails in lists and load the original only in the preview. */
export function MediaPreview({
  previewSrc,
  open,
  onOpenChange,
  alt = "图片预览",
  style,
  ...image
}: MediaPreviewProps) {
  const [localOpen, setLocalOpen] = useState(false);
  const visible = open ?? localOpen;
  return (
    <Dialog
      open={visible}
      onOpenChange={(next) => {
        setLocalOpen(next);
        onOpenChange?.(next);
      }}
    >
      {style?.display !== "none" && open === undefined ? (
        <DialogTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            className="h-auto w-full overflow-hidden p-0"
            aria-label={`预览：${alt}`}
          >
            <PreviewImage {...image} alt={alt} style={style} />
          </Button>
        </DialogTrigger>
      ) : null}
      <DialogContent className="dialog-media h-dialog flex flex-col items-center justify-center">
        <DialogTitle className="sr-only">{alt}</DialogTitle>
        <DialogDescription className="sr-only">
          查看媒体预览，按 Escape 关闭。
        </DialogDescription>
        {visible ? (
          <PreviewImage
            src={previewSrc || image.src}
            alt={alt}
            className="min-h-0 max-w-full flex-1 object-contain"
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
