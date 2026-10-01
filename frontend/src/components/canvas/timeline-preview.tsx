"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { CanvasNodeData } from "./model";
import { getMediaPreview } from "./queries";
import { useReleaseMediaSource } from "./nodes/media-lifecycle";
import type { TimelineClip, TimelineProject } from "./timeline";
import { timelineMediaLayout } from "./timeline-media-layout";

export function TimelinePreview({
  timeline,
  nodes,
  projectId,
  timeMs,
  playing,
}: {
  timeline: TimelineProject;
  nodes: CanvasNodeData[];
  projectId: string;
  timeMs: number;
  playing: boolean;
}) {
  const tracks = new Map(
    timeline.tracks.map((track, index) => [
      track.id,
      { ...track, order: index },
    ]),
  );
  const active = timeline.clips.filter(
    (clip) =>
      tracks.get(clip.trackId)?.visible &&
      timeMs >= clip.startMs &&
      timeMs < clip.startMs + clip.durationMs,
  );
  const videoIds = new Set(
    active
      .filter((clip) => clip.kind === "video")
      .slice(0, 3)
      .map((clip) => clip.id),
  );
  return (
    <div
      className="relative mx-auto w-full max-w-[650px] overflow-hidden rounded border bg-black"
      style={{ aspectRatio: timeline.aspectRatio.replace(":", "/") }}
      aria-label="时间线预览"
    >
      {active.map((clip) => {
        const track = tracks.get(clip.trackId)!;
        if (clip.kind === "text" || clip.kind === "subtitle")
          return (
            <div
              key={clip.id}
              className="pointer-events-none absolute inset-x-0 px-6 text-center whitespace-pre-wrap"
              style={{
                zIndex: 100 + track.order,
                color: timeline.subtitleStyle.color,
                fontSize: timeline.subtitleStyle.fontSize,
                textShadow: "0 1px 4px black",
                top:
                  timeline.subtitleStyle.position === "top"
                    ? "8%"
                    : timeline.subtitleStyle.position === "center"
                      ? "45%"
                      : undefined,
                bottom:
                  timeline.subtitleStyle.position === "bottom"
                    ? "8%"
                    : undefined,
              }}
            >
              {clip.text}
            </div>
          );
        if (clip.kind === "video" && !videoIds.has(clip.id)) return null;
        const assetId =
          clip.assetId ??
          nodes.find((node) => node.id === clip.nodeId)?.assetId;
        if (!assetId)
          return (
            <p
              key={clip.id}
              role="alert"
              className="absolute inset-0 z-50 grid place-items-center bg-black/80 p-6 text-sm text-white"
            >
              {clip.title}：请选择有效素材。
            </p>
          );
        return (
          <TimelineMedia
            key={`${clip.id}:${assetId}`}
            clip={clip}
            assetId={assetId}
            projectId={projectId}
            timeMs={timeMs}
            playing={playing}
            muted={track.muted}
            order={track.order}
            aspectRatio={timeline.aspectRatio}
          />
        );
      })}
      {!active.length ? (
        <span className="absolute inset-0 grid place-items-center text-xs text-neutral-500">
          当前时间没有可见片段
        </span>
      ) : null}
      {active.filter((clip) => clip.kind === "video").length > 3 ? (
        <p
          role="status"
          className="absolute top-0 right-0 z-50 bg-black/80 p-2 text-xs text-white"
        >
          预览最多同时解码三个视频轨道。
        </p>
      ) : null}
    </div>
  );
}

function TimelineMedia({
  clip,
  assetId,
  projectId,
  timeMs,
  playing,
  muted,
  order,
  aspectRatio,
}: {
  clip: TimelineClip;
  assetId: string;
  projectId: string;
  timeMs: number;
  playing: boolean;
  muted: boolean;
  order: number;
  aspectRatio: TimelineProject["aspectRatio"];
}) {
  const ref = useRef<HTMLMediaElement | HTMLImageElement>(null);
  const [error, setError] = useState("");
  const preview = useQuery({
    queryKey: ["canvas", "timeline-preview", projectId, assetId],
    queryFn: async ({ signal }) => {
      const result = await getMediaPreview(projectId, assetId, signal);
      const url = new URL(result.url),
        expires = Date.parse(result.expires_at);
      if (
        result.asset.id !== assetId ||
        result.asset.project_id !== projectId ||
        result.asset.kind !== clip.kind ||
        !["http:", "https:"].includes(url.protocol) ||
        url.username ||
        url.password ||
        !Number.isFinite(expires) ||
        expires <= Date.now() + 5000
      )
        throw new Error("时间线素材身份或预览授权无效。");
      return result;
    },
    retry: false,
    staleTime: 60000,
    gcTime: 60000,
  });
  useReleaseMediaSource(ref, preview.data?.url ?? "");
  const url = preview.data?.url;
  const elapsed = timeMs - clip.startMs;
  const fade = Math.min(
    1,
    clip.fadeInMs ? elapsed / clip.fadeInMs : 1,
    clip.fadeOutMs ? (clip.durationMs - elapsed) / clip.fadeOutMs : 1,
  );
  const volume = muted ? 0 : Math.min(1, clip.volume * Math.max(0, fade));
  useEffect(() => {
    const media = ref.current;
    if (!url || !(media instanceof HTMLMediaElement)) return;
    const target = (clip.sourceStartMs + timeMs - clip.startMs) / 1000;
    const synchronize = () => {
      if (Math.abs(media.currentTime - target) > 0.25)
        media.currentTime = target;
      media.volume = volume;
      if (playing && media.paused)
        void media
          .play()
          .catch(() => setError("浏览器未允许播放，请点击媒体播放按钮。"));
      else if (!playing) media.pause();
    };
    synchronize();
    media.addEventListener("loadedmetadata", synchronize, { once: true });
    return () => media.removeEventListener("loadedmetadata", synchronize);
  }, [url, clip.sourceStartMs, clip.startMs, timeMs, playing, volume]);
  const refetch = preview.refetch;
  useEffect(() => {
    if (!preview.data) return;
    const remaining = Date.parse(preview.data.expires_at) - Date.now() - 5000;
    const timer = setTimeout(
      () => {
        void refetch();
      },
      Number.isFinite(remaining) ? Math.max(1000, remaining) : 1000,
    );
    return () => clearTimeout(timer);
  }, [preview.data, refetch]);
  let layout: ReturnType<typeof timelineMediaLayout> | undefined;
  let layoutError = "";
  if (preview.data && (clip.kind === "video" || clip.kind === "image")) {
    try {
      layout = timelineMediaLayout(
        preview.data.asset.width ?? 0,
        preview.data.asset.height ?? 0,
        aspectRatio,
        clip.crop,
      );
    } catch (failure) {
      layoutError =
        failure instanceof Error ? failure.message : "裁切预览失败。";
    }
  }
  if (preview.error || error || layoutError)
    return (
      <p
        role="alert"
        className="absolute inset-x-0 bottom-0 z-50 bg-black/80 p-3 text-xs text-white"
      >
        {error || layoutError || `无法读取 ${clip.title}。`}
        <button
          className="ml-2 underline"
          onClick={() => {
            setError("");
            void preview.refetch();
          }}
        >
          重试
        </button>
      </p>
    );
  if (!url)
    return (
      <span
        role="status"
        className="absolute inset-0 grid place-items-center text-xs text-white"
      >
        正在读取 {clip.title}…
      </span>
    );
  const mediaStyle = layout
    ? {
        position: "absolute" as const,
        width: `${layout.mediaWidthPercent}%`,
        height: `${layout.mediaHeightPercent}%`,
        left: `${layout.leftPercent}%`,
        top: `${layout.topPercent}%`,
        maxWidth: "none",
      }
    : undefined;
  if (clip.kind === "image" || clip.kind === "video")
    return (
      <div
        className="absolute inset-0 grid place-items-center"
        style={{ zIndex: order, opacity: Math.max(0, fade) }}
      >
        <div
          className="relative overflow-hidden"
          style={{
            width: `${layout!.frameWidthPercent}%`,
            height: `${layout!.frameHeightPercent}%`,
          }}
        >
          {clip.kind === "image" ? (
            <>
              {/* eslint-disable-next-line @next/next/no-img-element -- 项目授权URL不能经过共享优化缓存。 */}
              <img
                ref={ref as React.RefObject<HTMLImageElement>}
                src={url}
                crossOrigin="anonymous"
                alt={clip.title}
                style={mediaStyle}
                onError={() => setError("图片读取失败。")}
              />
            </>
          ) : (
            <video
              ref={ref as React.RefObject<HTMLVideoElement>}
              src={url}
              crossOrigin="anonymous"
              aria-label={clip.title}
              controls={!playing}
              playsInline
              preload="metadata"
              style={mediaStyle}
              onError={() => setError("视频播放失败。")}
            />
          )}
        </div>
      </div>
    );
  return (
    <audio
      ref={ref as React.RefObject<HTMLAudioElement>}
      src={url}
      crossOrigin="anonymous"
      aria-label={clip.title}
      preload="metadata"
      onError={() => setError("音频播放失败。")}
    />
  );
}
