"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useReleaseMediaSource } from "./nodes/media-lifecycle";
import {
  MEDIA_EXPORTS_KEY,
  previewTimelineExport,
  reviewTimelineExport,
} from "./media-export-queries";
import type { ExportJob } from "./media-export-model";

export function MediaExportPreviewDialog({
  job,
  readOnly,
  onClose,
  onReviewed,
  onBusy,
}: {
  job: ExportJob;
  readOnly: boolean;
  onClose: () => void;
  onReviewed: () => void;
  onBusy: (busy: boolean) => void;
}) {
  const [ready, setReady] = useState(false);
  const [reviewed, setReviewed] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [expiredLease, setExpiredLease] = useState<string>();
  const attempt = useRef<string | null>(null);
  const active = useRef(false);
  const video = useRef<HTMLVideoElement>(null);
  const cache = useQueryClient();
  const preview = useQuery({
    queryKey: [
      ...MEDIA_EXPORTS_KEY,
      "preview",
      job.project_id,
      job.id,
      job.revision,
    ],
    queryFn: ({ signal }) => previewTimelineExport(job, signal),
    retry: false,
    staleTime: 60000,
    gcTime: 0,
  });
  const url =
    !preview.error && preview.data && expiredLease !== preview.data.expires_at
      ? preview.data.url
      : "";
  useReleaseMediaSource(video, url);
  const refetch = preview.refetch;
  useEffect(() => {
    if (!preview.data) return;
    const remaining = Date.parse(preview.data.expires_at) - Date.now() - 5000;
    const timer = setTimeout(
      () => {
        setExpiredLease(preview.data!.expires_at);
        setReady(false);
        setReviewed(false);
        void refetch();
      },
      Math.max(1000, remaining),
    );
    return () => clearTimeout(timer);
  }, [preview.data, refetch]);
  async function review() {
    if (
      active.current ||
      !preview.data ||
      !ready ||
      !reviewed ||
      pending ||
      readOnly ||
      job.status !== "review_required"
    )
      return;
    active.current = true;
    setPending(true);
    onBusy(true);
    setError("");
    attempt.current ??= crypto.randomUUID();
    try {
      await reviewTimelineExport(job, preview.data, attempt.current);
      await Promise.all([
        cache.invalidateQueries({
          queryKey: [...MEDIA_EXPORTS_KEY, job.project_id],
        }),
        cache.invalidateQueries({
          queryKey: ["media", "assets", job.project_id],
        }),
        cache.invalidateQueries({
          queryKey: ["canvas", "media", job.project_id],
        }),
      ]);
      onReviewed();
    } catch (failure) {
      setError(
        failure instanceof Error
          ? failure.message
          : "输出审核未确认，请重试原请求。",
      );
    } finally {
      active.current = false;
      setPending(false);
      onBusy(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent
        className="flex max-h-[92dvh] flex-col overflow-y-auto sm:max-w-4xl [&>*]:shrink-0"
        onEscapeKeyDown={(event) => {
          if (pending) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>检查导出结果</DialogTitle>
          <DialogDescription>
            检查实际成片的画面、裁切、字幕与声音，再确认审核。输出来自画布修订{" "}
            {job.source.revision}。
          </DialogDescription>
        </DialogHeader>
        {url ? (
          <video
            ref={video}
            controls
            playsInline
            preload="metadata"
            crossOrigin="anonymous"
            src={url}
            aria-label="实际导出成片"
            className="max-h-[58dvh] w-full rounded bg-black"
            onLoadedData={(event) => {
              const media = event.currentTarget,
                asset = preview.data?.asset;
              if (
                !asset ||
                media.videoWidth < 2 ||
                media.videoHeight < 2 ||
                !Number.isFinite(media.duration) ||
                Math.abs(media.duration * 1000 - (asset.duration_ms ?? 0)) >
                  1000 ||
                (asset.width && asset.width !== media.videoWidth) ||
                (asset.height && asset.height !== media.videoHeight)
              ) {
                setReady(false);
                setError("成片解码尺寸或时长与导出素材不一致，请重新读取。");
              } else {
                setReady(true);
                setError("");
              }
            }}
            onError={() => {
              setReady(false);
              setError("实际成片读取失败，无法确认审核。");
            }}
          />
        ) : (
          <p role="status">正在读取实际导出成片…</p>
        )}
        {preview.error || error ? (
          <div role="alert" className="text-sm text-destructive">
            {error || "无法取得成片授权，请重新读取。"}
            <Button
              variant="ghost"
              disabled={pending}
              onClick={() => {
                setReady(false);
                setReviewed(false);
                setError("");
                void preview.refetch();
              }}
            >
              重新读取成片
            </Button>
          </div>
        ) : null}
        {preview.data ? (
          <p className="text-xs break-all text-muted-foreground">
            {preview.data.asset.width} × {preview.data.asset.height} ·{" "}
            {((preview.data.asset.duration_ms ?? 0) / 1000).toFixed(2)} s · 输出
            SHA256 {preview.data.sha256}
          </p>
        ) : null}
        {job.status === "review_required" ? (
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={reviewed}
              disabled={!ready || pending || readOnly}
              aria-label="我已检查实际成片并确认可用"
              onCheckedChange={(checked) => setReviewed(checked === true)}
            />
            我已检查实际成片，确认可以加入项目素材。
          </label>
        ) : null}
        <DialogFooter>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            关闭
          </Button>
          {job.status === "review_required" ? (
            <Button
              disabled={!ready || !reviewed || pending || readOnly}
              onClick={() => {
                void review();
              }}
            >
              {pending ? "确认审核中…" : "确认审核成片"}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
