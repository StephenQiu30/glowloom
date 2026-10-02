"use client";

import dynamic from "next/dynamic";
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
import { useReleaseMediaSource } from "@/components/canvas/nodes/media-lifecycle";
import {
  categoryLabels,
  kindLabels,
  type LibraryAsset,
  type LibraryDetail,
  type LibraryIdentity,
} from "./library-model";
import {
  downloadLibraryOriginal,
  freshLibrary,
  getLibraryDetail,
  libraryKey,
  previewLibrary,
} from "./library-queries";
import { formatBytes } from "./library-items";
const ModelPreview = dynamic(
  () =>
    import("./library-model-preview").then(
      (module) => module.LibraryModelPreview,
    ),
  { ssr: false },
);

export function LibraryDetailDialog({
  identity,
  id,
  readOnly,
  onClose,
  onCloseAutoFocus,
  onEdit,
}: {
  identity: LibraryIdentity;
  id: string;
  readOnly: boolean;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
  onEdit: (detail: LibraryDetail) => void;
}) {
  const detail = useQuery({
    queryKey: [...libraryKey(identity), "detail", id],
    queryFn: async ({ signal }) => {
      await freshLibrary(identity, signal);
      return getLibraryDetail(identity, id, signal);
    },
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const proven =
    detail.isFetchedAfterMount && detail.isSuccess && !detail.isError;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        data-library-dialog
        onCloseAutoFocus={onCloseAutoFocus}
        className="flex max-h-[92dvh] flex-col overflow-y-auto sm:max-w-5xl"
      >
        <DialogHeader>
          <DialogTitle>{proven ? detail.data.title : "素材详情"}</DialogTitle>
          <DialogDescription>
            按当前账号和素材库范围读取；原件和正文保留独立身份。
          </DialogDescription>
        </DialogHeader>
        {!proven && !detail.isError && (
          <p role="status">正在核验当前身份并读取素材…</p>
        )}
        {detail.isError && (
          <Alert variant="destructive">
            <AlertTitle>素材详情无法读取</AlertTitle>
            <AlertDescription>
              <p>{detail.error.message}</p>
              <Button variant="outline" onClick={() => void detail.refetch()}>
                重新读取素材详情
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {proven && (
          <DetailBody
            key={`${id}:${detail.data.revision}`}
            identity={identity}
            detail={detail.data}
            readOnly={readOnly}
            onEdit={onEdit}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
function DetailBody({
  identity,
  detail,
  readOnly,
  onEdit,
}: {
  identity: LibraryIdentity;
  detail: LibraryDetail;
  readOnly: boolean;
  onEdit: (detail: LibraryDetail) => void;
}) {
  const [notice, setNotice] = useState<string>(),
    [sha, setSHA] = useState<string>(),
    [busy, setBusy] = useState(false);
  const download = useRef<AbortController | null>(null);
  useEffect(() => () => download.current?.abort(), []);
  async function saveOriginal() {
    if (!detail.media || busy) return;
    const controller = new AbortController();
    download.current = controller;
    setBusy(true);
    setNotice(undefined);
    setSHA(undefined);
    try {
      await freshLibrary(identity, controller.signal);
      const result = await downloadLibraryOriginal(
        identity,
        detail.media,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(result.blob);
      try {
        const link = document.createElement("a");
        link.href = url;
        link.download = result.fileName;
        link.click();
        setSHA(result.sha256);
        setNotice("原件下载已交给浏览器；下方是本次收到的完整文件 SHA-256。");
      } finally {
        URL.revokeObjectURL(url);
      }
    } catch (error) {
      if (!controller.signal.aborted)
        setNotice(
          error instanceof Error ? error.message : "原件未能下载，请重新读取。",
        );
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  async function copyText() {
    try {
      if (typeof detail.plain_text !== "string") return;
      await freshLibrary(identity);
      await navigator.clipboard.writeText(detail.plain_text);
      setNotice("已复制完整正文。");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "正文复制未完成。");
    }
  }
  return (
    <div className="space-y-5">
      <dl className="grid min-w-0 gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-muted-foreground">类型和分类</dt>
          <dd>
            {kindLabels[detail.kind]} · {categoryLabels[detail.category]}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">标签</dt>
          <dd>{detail.tags.join(" · ") || "无"}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">来源标签</dt>
          <dd className="break-words">{detail.source_label || "无"}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">素材身份</dt>
          <dd className="break-all">
            {detail.id} · 版本 {detail.revision}
          </dd>
        </div>
        {detail.media && (
          <div>
            <dt className="text-muted-foreground">原件</dt>
            <dd className="break-words">
              {detail.media.file_name} · {formatBytes(detail.media.byte_size)} ·{" "}
              {detail.media.mime_type}
            </dd>
          </div>
        )}
        <div>
          <dt className="text-muted-foreground">备注</dt>
          <dd className="break-words whitespace-pre-wrap">
            {detail.note || "无"}
          </dd>
        </div>
      </dl>
      {detail.media?.mime_type === "image/gif" && (
        <p className="text-sm text-muted-foreground">
          GIF 原件预览保留动画；素材列表缩略图显示首帧。
        </p>
      )}
      {detail.kind === "text" ? (
        <pre
          data-testid="library-plain-text"
          className="max-h-[50dvh] overflow-auto rounded-lg border bg-muted/30 p-4 font-sans text-sm break-words whitespace-pre-wrap"
        >
          {detail.plain_text}
        </pre>
      ) : detail.media && detail.kind !== "document" ? (
        <BinaryPreview
          identity={identity}
          asset={detail.media}
          title={detail.title}
        />
      ) : (
        <p className="text-sm text-muted-foreground">
          文档原件通过正式下载读取，当前不提供在线文档预览。
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {detail.kind === "text" ? (
          <Button variant="outline" onClick={() => void copyText()}>
            复制完整正文
          </Button>
        ) : (
          <Button disabled={busy} onClick={() => void saveOriginal()}>
            {busy ? "正在核验并读取原件…" : "下载素材原件"}
          </Button>
        )}
        {!readOnly && detail.catalog_state === "active" && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => onEdit(detail)}
          >
            编辑素材信息
          </Button>
        )}
      </div>
      {notice && (
        <p role="status" className="text-sm">
          {notice}
        </p>
      )}
      {sha && (
        <div className="space-y-1 text-sm">
          <p>实际下载文件 SHA-256</p>
          <code className="block break-all">{sha}</code>
        </div>
      )}
    </div>
  );
}
function BinaryPreview({
  identity,
  asset,
  title,
}: {
  identity: LibraryIdentity;
  asset: LibraryAsset;
  title: string;
}) {
  const [failed, setFailed] = useState(false);
  const preview = useQuery({
    queryKey: [
      ...libraryKey(identity),
      "full-preview",
      asset.id,
      asset.revision,
    ],
    queryFn: async ({ signal }) => {
      await freshLibrary(identity, signal);
      return previewLibrary(identity, asset, signal);
    },
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  useEffect(() => {
    if (!preview.data) return;
    const timer = setTimeout(
      () => setFailed(true),
      Math.max(0, Date.parse(preview.data.expires_at) - Date.now() - 5000),
    );
    return () => clearTimeout(timer);
  }, [preview.data]);
  if (preview.isFetching) return <p role="status">正在读取私有预览授权…</p>;
  if (failed || preview.isError)
    return (
      <Alert variant="destructive">
        <AlertTitle>预览暂时不可读取</AlertTitle>
        <AlertDescription>
          <p>{preview.error?.message ?? "素材读取失败或授权已过期。"}</p>
          <Button
            variant="outline"
            onClick={() => {
              setFailed(false);
              void preview.refetch();
            }}
          >
            重新读取私有预览
          </Button>
        </AlertDescription>
      </Alert>
    );
  if (!preview.data) return null;
  return (
    <PreviewSource
      key={preview.data.url}
      url={preview.data.url}
      asset={asset}
      title={title}
      onError={() => setFailed(true)}
    />
  );
}
function PreviewSource({
  url,
  asset,
  title,
  onError,
}: {
  url: string;
  asset: LibraryAsset;
  title: string;
  onError: () => void;
}) {
  const image = useRef<HTMLImageElement>(null),
    video = useRef<HTMLVideoElement>(null),
    audio = useRef<HTMLAudioElement>(null);
  useReleaseMediaSource(image, asset.kind === "image" ? url : "");
  useReleaseMediaSource(video, asset.kind === "video" ? url : "");
  useReleaseMediaSource(audio, asset.kind === "audio" ? url : "");
  const [zoom, setZoom] = useState(1),
    [pan, setPan] = useState({ x: 0, y: 0 });
  const drag = useRef<{ x: number; y: number; px: number; py: number } | null>(
    null,
  );
  if (asset.kind === "model")
    return (
      <ModelPreview
        url={url}
        byteSize={asset.byte_size}
        mimeType={asset.mime_type}
        onError={onError}
      />
    );
  if (asset.kind === "video")
    return (
      <video
        ref={video}
        src={url}
        aria-label={title}
        crossOrigin="anonymous"
        controls
        playsInline
        preload="metadata"
        onError={onError}
        className="max-h-[60dvh] w-full"
      />
    );
  if (asset.kind === "audio")
    return (
      <audio
        ref={audio}
        src={url}
        aria-label={title}
        crossOrigin="anonymous"
        controls
        preload="metadata"
        onError={onError}
        className="w-full"
      />
    );
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          aria-label="缩小图片"
          disabled={zoom <= 0.25}
          onClick={() => setZoom(Math.max(0.25, zoom - 0.25))}
        >
          −
        </Button>
        <span className="self-center text-sm">{Math.round(zoom * 100)}%</span>
        <Button
          variant="outline"
          aria-label="放大图片"
          disabled={zoom >= 4}
          onClick={() => setZoom(Math.min(4, zoom + 0.25))}
        >
          +
        </Button>
        <Button
          variant="outline"
          onClick={() => {
            setZoom(1);
            setPan({ x: 0, y: 0 });
          }}
        >
          重置图片视图
        </Button>
      </div>
      <div
        tabIndex={0}
        role="region"
        aria-label="图片预览，可用方向键平移"
        className="flex h-[50dvh] touch-none items-center justify-center overflow-hidden rounded-lg border bg-muted/30 focus-visible:outline-2"
        onKeyDown={(event) => {
          const delta = {
            ArrowLeft: [-20, 0],
            ArrowRight: [20, 0],
            ArrowUp: [0, -20],
            ArrowDown: [0, 20],
          }[event.key];
          if (delta) {
            event.preventDefault();
            setPan((old) => ({ x: old.x + delta[0], y: old.y + delta[1] }));
          }
        }}
        onPointerDown={(event) => {
          if (event.button !== 0) return;
          drag.current = {
            x: event.clientX,
            y: event.clientY,
            px: pan.x,
            py: pan.y,
          };
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          const start = drag.current;
          if (start)
            setPan({
              x: start.px + event.clientX - start.x,
              y: start.py + event.clientY - start.y,
            });
        }}
        onPointerUp={() => {
          drag.current = null;
        }}
        onPointerCancel={() => {
          drag.current = null;
        }}
      >
        {/* eslint-disable-next-line @next/next/no-img-element -- 私有原图使用短期授权，不进入公开图片缓存。 */}
        <img
          ref={image}
          src={url}
          alt={title}
          crossOrigin="anonymous"
          draggable={false}
          onError={onError}
          className="max-h-full max-w-full object-contain"
          style={{
            transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`,
          }}
        />
      </div>
    </div>
  );
}
