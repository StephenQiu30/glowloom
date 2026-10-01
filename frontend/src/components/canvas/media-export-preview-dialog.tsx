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
  const [waveformReady, setWaveformReady] = useState(false);
  const [reviewed, setReviewed] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [expiredLease, setExpiredLease] = useState<string>();
  const attempt = useRef<string | null>(null);
  const active = useRef(false);
  const video = useRef<HTMLVideoElement | HTMLAudioElement>(null);
  const waveform = useRef<HTMLImageElement>(null);
  const isAudio = job.output_kind === "audio";
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
  const waveformURL = url ? (preview.data?.waveform?.url ?? "") : "";
  useReleaseMediaSource(waveform, waveformURL);
  const canReview = ready && (!isAudio || waveformReady);
  const refetch = preview.refetch;
  useEffect(() => {
    if (!preview.data) return;
    const remaining =
      Math.min(
        Date.parse(preview.data.expires_at),
        preview.data.waveform
          ? Date.parse(preview.data.waveform.expires_at)
          : Infinity,
      ) -
      Date.now() -
      5000;
    const timer = setTimeout(
      () => {
        setExpiredLease(preview.data!.expires_at);
        setReady(false);
        setWaveformReady(false);
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
      !canReview ||
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
            {isAudio
              ? "播放并检查实际音频的声音、裁切、音量和淡入淡出，再确认审核。"
              : "检查实际成片的画面、裁切、字幕与声音，再确认审核。"}
            输出来自画布修订 {job.source.revision}。
          </DialogDescription>
        </DialogHeader>
        {waveformURL && isAudio && preview.data?.waveform ? (
          // eslint-disable-next-line @next/next/no-img-element -- 正式job授权的短期私有波形，不交给公开图片缓存。
          <img
            ref={waveform}
            src={waveformURL}
            crossOrigin="anonymous"
            alt="实际导出音频波形"
            className="max-h-40 w-full rounded bg-black object-contain"
            onLoad={(event) => {
              const actual = event.currentTarget,
                expected = preview.data?.waveform;
              if (
                expected &&
                actual.naturalWidth === expected.width &&
                actual.naturalHeight === expected.height
              )
                setWaveformReady(true);
              else {
                setWaveformReady(false);
                setError("音频波形解码尺寸与实际输出不一致，请重新读取。");
              }
            }}
            onError={() => {
              setWaveformReady(false);
              setError("实际音频波形读取失败，无法确认审核。");
            }}
          />
        ) : null}
        {url && isAudio ? (
          <audio
            ref={video as React.RefObject<HTMLAudioElement>}
            controls
            preload="metadata"
            crossOrigin="anonymous"
            src={url}
            aria-label="实际导出音频"
            className="w-full"
            onLoadedData={(event) => {
              const media = event.currentTarget,
                asset = preview.data?.asset;
              if (
                !asset ||
                !Number.isFinite(media.duration) ||
                media.duration <= 0 ||
                Math.abs(media.duration * 1000 - (asset.duration_ms ?? 0)) >
                  1000
              ) {
                setReady(false);
                setWaveformReady(false);
                setError("音频解码时长与导出素材不一致，请重新读取。");
              } else {
                setReady(true);
                setError("");
              }
            }}
            onError={() => {
              setReady(false);
              setError("实际音频读取失败，无法确认审核。");
            }}
          />
        ) : url ? (
          <video
            ref={video as React.RefObject<HTMLVideoElement>}
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
          <p role="status">
            {isAudio ? "正在读取实际导出音频…" : "正在读取实际导出成片…"}
          </p>
        )}
        {preview.error || error ? (
          <div role="alert" className="text-sm text-destructive">
            {error || "无法取得输出授权，请重新读取。"}
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
              {isAudio ? "重新读取音频" : "重新读取成片"}
            </Button>
          </div>
        ) : null}
        {preview.data ? (
          <p className="text-xs break-all text-muted-foreground">
            {!isAudio
              ? `${preview.data.asset.width} × ${preview.data.asset.height} · `
              : "M4A · "}
            {((preview.data.asset.duration_ms ?? 0) / 1000).toFixed(2)} s · 输出
            SHA256 {preview.data.sha256}
          </p>
        ) : null}
        {job.status === "review_required" ? (
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={reviewed}
              disabled={!canReview || pending || readOnly}
              aria-label={
                isAudio
                  ? "我已检查实际音频并确认可用"
                  : "我已检查实际成片并确认可用"
              }
              onCheckedChange={(checked) => setReviewed(checked === true)}
            />
            {isAudio
              ? "我已播放并检查实际音频，确认可以加入项目素材。"
              : "我已检查实际成片，确认可以加入项目素材。"}
          </label>
        ) : null}
        <DialogFooter>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            关闭
          </Button>
          {job.status === "review_required" ? (
            <Button
              disabled={!canReview || !reviewed || pending || readOnly}
              onClick={() => {
                void review();
              }}
            >
              {pending
                ? "确认审核中…"
                : isAudio
                  ? "确认审核音频"
                  : "确认审核成片"}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
