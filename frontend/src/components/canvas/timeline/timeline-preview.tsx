"use client";
/* Timeline sources are short-lived authorized media URLs; Next image optimization must not cache them. */
/* eslint-disable @next/next/no-img-element */
import { useCallback, useEffect, useRef, useState } from "react";
import { type Timeline, type TimelineClip } from "./timeline-model";
import { timelinePreviewDimensions } from "./timeline-aspect-ratio";
export type TimelineMedia = {
  asset: API.AssetSummary;
  url: string;
  retryGeneration?: number;
};
function PreviewMedia({
  clip,
  media,
  playhead,
  playing,
  muted,
  volume,
  onError,
}: {
  clip: TimelineClip;
  media: TimelineMedia;
  playhead: number;
  playing: boolean;
  muted: boolean;
  volume: number;
  onError: (message: string, source?: TimelineMedia) => void;
}) {
  const element = useRef<HTMLMediaElement | null>(null);
  const [failedURL, setFailedURL] = useState<string | null>(null);
  const reportedURL = useRef<string | null>(null);
  const failed = failedURL === media.url;
  const report = useCallback(() => {
    if (reportedURL.current === media.url) return;
    reportedURL.current = media.url;
    element.current?.pause();
    setFailedURL(media.url);
    onError(`无法播放：${clip.title}`, media);
  }, [clip.title, media, onError]);
  useEffect(() => {
    const value = element.current;
    if (!value) return;
    const time = (clip.source_start_ms + playhead - clip.start_ms) / 1000;
    if (!playing || Math.abs(value.currentTime - time) > 0.15)
      value.currentTime = time;
    const elapsed = playhead - clip.start_ms,
      remaining = clip.duration_ms - elapsed;
    const gain = Math.min(
      clip.fade_in_ms ? elapsed / clip.fade_in_ms : 1,
      clip.fade_out_ms ? remaining / clip.fade_out_ms : 1,
      1,
    );
    value.volume = Math.max(0, Math.min(1, volume * clip.volume * gain));
    value.muted = muted;
  }, [clip, playhead, playing, muted, volume, onError, media.url]);
  useEffect(() => {
    const value = element.current;
    if (!value) return;
    let active = true;
    if (playing && !failed) {
      void value.play().catch(() => {
        if (active) report();
      });
    } else value.pause();
    return () => {
      active = false;
      value.pause();
    };
  }, [playing, media.url, failed, report]);
  useEffect(() => {
    const value = element.current;
    return () => {
      value?.pause();
      // React owns src. StrictMode replays cleanup without removing this DOM node.
    };
  }, []);
  const crop = clip.crop;
  const style =
    crop && media.asset.width && media.asset.height
      ? {
          position: "absolute" as const,
          width: `${(media.asset.width / crop.width!) * 100}%`,
          height: `${(media.asset.height / crop.height!) * 100}%`,
          maxWidth: "none",
          left: `${(-crop.x! / crop.width!) * 100}%`,
          top: `${(-crop.y! / crop.height!) * 100}%`,
        }
      : { width: "100%", height: "100%", objectFit: "contain" as const };
  if (clip.kind === "audio")
    return (
      <audio
        ref={(node) => {
          element.current = node;
        }}
        src={media.url}
        preload="auto"
        onError={report}
      />
    );
  const mediaRatio = crop
    ? crop.width! / crop.height!
    : media.asset.width && media.asset.height
      ? media.asset.width / media.asset.height
      : 1;
  return (
    <div
      className="relative flex max-h-full max-w-full items-center justify-center overflow-hidden"
      style={{
        aspectRatio: mediaRatio,
        width: `min(100cqw, calc(100cqh * ${mediaRatio}))`,
        height: `min(100cqh, calc(100cqw / ${mediaRatio}))`,
      }}
    >
      {clip.kind === "image" ? (
        <img src={media.url} alt={clip.title} style={style} onError={report} />
      ) : (
        <video
          ref={(node) => {
            element.current = node;
          }}
          src={media.url}
          playsInline
          preload="auto"
          style={style}
          onError={report}
        />
      )}
    </div>
  );
}
export function TimelinePreview({
  timeline,
  media,
  playhead,
  playing,
  muted,
  volume,
  onError,
}: {
  timeline: Timeline;
  media: Record<string, TimelineMedia>;
  playhead: number;
  playing: boolean;
  muted: boolean;
  volume: number;
  onError: (message: string, source?: TimelineMedia) => void;
}) {
  const active = timeline.tracks.flatMap((track) =>
    !track.visible
      ? []
      : timeline.clips
          .filter(
            (clip) =>
              clip.track_id === track.id &&
              clip.start_ms <= playhead &&
              playhead < clip.start_ms + clip.duration_ms,
          )
          .map((clip) => ({ clip, track })),
  );
  const visible = active.some(({ clip }) => clip.kind !== "audio");
  const firstVisual = [...timeline.clips]
    .filter((clip) => clip.kind === "image" || clip.kind === "video")
    .sort((a, b) => a.start_ms - b.start_ms)[0];
  const firstSource =
    firstVisual &&
    media[firstVisual.asset_id ?? firstVisual.node_id ?? ""]?.asset;
  const dimensions = timelinePreviewDimensions(
    timeline.aspect_ratio,
    firstSource?.width && firstSource.height
      ? { width: firstSource.width, height: firstSource.height }
      : undefined,
  );
  const ratio = dimensions ? dimensions.width / dimensions.height : 16 / 9;
  return (
    <div
      aria-label="时间线预览画面"
      className={`relative flex items-center justify-center overflow-hidden ${timeline.clips.length ? "bg-black" : ""}`}
      style={{
        containerType: "size",
        aspectRatio: ratio ?? undefined,
        width: ratio ? `min(100cqw, calc(100cqh * ${ratio}))` : "100%",
        height: ratio ? `min(100cqh, calc(100cqw / ${ratio}))` : "100%",
      }}
    >
      {!visible && !timeline.clips.length && (
        <p className="text-[13px] text-foreground/30">没有媒体可供预览</p>
      )}
      {active.map(({ clip, track }) => {
        if (clip.kind === "text" || clip.kind === "subtitle")
          return (
            <div
              key={clip.id}
              className="pointer-events-none absolute inset-x-0 z-10 px-6 text-center"
              style={{
                color: timeline.subtitle_style.color,
                fontSize: timeline.subtitle_style.font_size,
                ...(timeline.subtitle_style.position === "top"
                  ? { top: "8%" }
                  : timeline.subtitle_style.position === "center"
                    ? { top: "50%" }
                    : { bottom: "8%" }),
              }}
            >
              {clip.text}
            </div>
          );
        const source = media[clip.asset_id ?? clip.node_id ?? ""];
        return source ? (
          <div
            key={`${clip.id}:${source.retryGeneration ?? 0}`}
            className={
              clip.kind === "audio"
                ? "hidden"
                : "absolute inset-0 flex items-center justify-center"
            }
          >
            <PreviewMedia
              clip={clip}
              media={source}
              playhead={playhead}
              playing={playing}
              muted={muted || track.muted}
              volume={volume}
              onError={onError}
            />
          </div>
        ) : (
          <p key={clip.id} className="absolute text-xs text-foreground/40">
            正在读取 {clip.title}
          </p>
        );
      })}
    </div>
  );
}
