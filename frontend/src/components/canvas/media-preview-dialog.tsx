"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { getMediaPreview } from "./queries";
import { ApiError } from "@/lib/request";
import { useReleaseMediaSource } from "./nodes/media-lifecycle";
import type { CanvasNodeData } from "./model";

export function MediaPreviewDialog({
  node,
  projectId,
  onClose,
}: {
  node: CanvasNodeData;
  projectId: string;
  onClose: () => void;
}) {
  const [sourceFailed, setSourceFailed] = useState(false);
  const preview = useQuery({
    queryKey: ["canvas", "full-media-preview", projectId, node.assetId],
    queryFn: async ({ signal }) => {
      const value = await getMediaPreview(projectId, node.assetId!, signal);
      const url = new URL(value.url);
      if (
        value.asset.id !== node.assetId ||
        value.asset.project_id !== projectId ||
        value.asset.kind !== node.type ||
        !["http:", "https:"].includes(url.protocol) ||
        url.username ||
        url.password
      )
        throw new ApiError(502, "invalid_response");
      const expires = Date.parse(value.expires_at);
      if (!Number.isFinite(expires) || expires <= Date.now() + 5000)
        throw new Error("素材预览授权已过期，请重新读取。");
      return value;
    },
    enabled: Boolean(node.assetId),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    staleTime: 0,
    gcTime: 0,
  });
  useEffect(() => {
    if (!preview.data) return;
    const remaining = Date.parse(preview.data.expires_at) - Date.now() - 5000;
    const timer = window.setTimeout(
      () => setSourceFailed(true),
      Math.max(0, Math.min(remaining, 2_147_483_647)),
    );
    return () => window.clearTimeout(timer);
  }, [preview.data]);
  const failed = Boolean(preview.error || sourceFailed);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="flex max-h-[90dvh] flex-col sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>{node.title}</DialogTitle>
          <DialogDescription>当前项目素材预览</DialogDescription>
        </DialogHeader>
        {preview.isFetching && <p role="status">正在读取素材…</p>}
        {failed && !preview.isFetching && (
          <Alert variant="destructive">
            <AlertTitle>素材读取失败</AlertTitle>
            <AlertDescription>
              <p>
                {preview.error?.message ??
                  "素材加载失败或授权已过期，请重新读取。"}
              </p>
              <Button
                variant="secondary"
                onClick={() => {
                  setSourceFailed(false);
                  void preview.refetch();
                }}
              >
                重试预览
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {preview.data && !failed && !preview.isFetching && (
          <PreviewSource
            key={preview.data.url}
            node={node}
            url={preview.data.url}
            onError={() => setSourceFailed(true)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
function PreviewSource({
  node,
  url,
  onError,
}: {
  node: CanvasNodeData;
  url: string;
  onError: () => void;
}) {
  const source = useRef<HTMLImageElement | HTMLMediaElement>(null);
  useReleaseMediaSource(source, url);
  if (node.type === "image")
    return (
      // eslint-disable-next-line @next/next/no-img-element -- 短期授权原图直接读取，不交给公开图片缓存。
      <img
        ref={source as React.RefObject<HTMLImageElement>}
        src={url}
        alt={node.title}
        crossOrigin="anonymous"
        onError={onError}
        className="max-h-[70dvh] w-full object-contain"
      />
    );
  if (node.type === "video")
    return (
      <video
        ref={source as React.RefObject<HTMLVideoElement>}
        src={url}
        aria-label={node.title}
        crossOrigin="anonymous"
        onError={onError}
        controls
        playsInline
        preload="metadata"
        className="max-h-[70dvh] w-full"
      />
    );
  return (
    <audio
      ref={source as React.RefObject<HTMLAudioElement>}
      src={url}
      aria-label={node.title}
      crossOrigin="anonymous"
      onError={onError}
      controls
      preload="metadata"
      className="w-full"
    />
  );
}
