"use client";
/* Authorized expiring URLs must not pass through the image optimization cache. */
/* eslint-disable @next/next/no-img-element */
import { useEffect, useRef, useState } from "react";
import { AudioLines, ImageIcon, VideoIcon } from "lucide-react";
import type { TimelineMedia } from "./timeline-preview";
import { acquireTimelineVideoFrame } from "./timeline-video-frame";
import { acquireTimelineWaveform } from "./timeline-waveform";

function seekThumbnail(video: HTMLVideoElement | null, startMS: number) {
  if (!video || video.readyState < 1) return;
  const target = Math.max(0, startMS / 1000);
  video.currentTime = Number.isFinite(video.duration)
    ? Math.min(target, video.duration)
    : target;
}

export function TimelineThumbnail({
  media,
  kind,
  startMS = 0,
  durationMS,
  strip = false,
}: {
  media?: TimelineMedia;
  kind: string;
  startMS?: number;
  durationMS?: number;
  strip?: boolean;
}) {
  const [wave, setWave] = useState<{ url: string; values: number[] } | null>(
    null,
  );
  const url = media?.url;
  const sourceDuration = media?.asset.duration_ms;
  const retryGeneration = media?.retryGeneration;
  const video = useRef<HTMLVideoElement>(null);
  useEffect(() => {
    if (kind === "video") seekThumbnail(video.current, startMS);
  }, [kind, url, startMS]);
  useEffect(() => {
    if (kind !== "audio" || !url) return;
    let active = true;
    const waveform = acquireTimelineWaveform(url, sourceDuration);
    void waveform.promise
      .then((values) => {
        if (active) setWave({ url, values });
      })
      .catch(() => {});
    return () => {
      active = false;
      waveform.release();
    };
  }, [kind, url, sourceDuration, retryGeneration]);
  if (!url)
    return (
      <span className="timeline-thumbnail-placeholder">
        {kind === "audio" ? (
          <AudioLines />
        ) : kind === "video" ? (
          <VideoIcon />
        ) : (
          <ImageIcon />
        )}
      </span>
    );
  if (kind === "image" && strip) return <RepeatedFrame url={url} />;
  if (kind === "image")
    return (
      <img className="timeline-thumbnail" src={url} alt="" draggable={false} />
    );
  if (kind === "video" && strip)
    return (
      <TimelineVideoStrip
        key={`${url}:${startMS}:${retryGeneration ?? 0}`}
        media={media!}
        startMS={startMS}
      />
    );
  if (kind === "video")
    return (
      <video
        ref={video}
        className="timeline-thumbnail"
        src={url}
        muted
        preload="metadata"
        playsInline
        onLoadedMetadata={(event) =>
          seekThumbnail(event.currentTarget, startMS)
        }
        onLoadedData={(event) => seekThumbnail(event.currentTarget, startMS)}
      />
    );
  const values = wave?.url === url ? wave.values : null;
  if (!values)
    return (
      <span className="timeline-thumbnail-placeholder">
        <AudioLines />
      </span>
    );
  const total = media?.asset.duration_ms || 1;
  const from = Math.floor((startMS / total) * values.length),
    to = Math.max(
      from + 1,
      Math.ceil(((startMS + (durationMS ?? total)) / total) * values.length),
    );
  const visible = values.slice(from, to);
  return (
    <svg
      className="timeline-waveform"
      role="img"
      viewBox={`0 0 ${visible.length * 2} 50`}
      preserveAspectRatio="none"
      aria-label="音频波形"
    >
      {visible.map((v, i) => (
        <path
          key={i}
          d={`M${i * 2 + 1} ${25 - v * 23}V${25 + v * 23}`}
          stroke="currentColor"
          strokeWidth="1"
        />
      ))}
    </svg>
  );
}

function TimelineVideoStrip({
  media,
  startMS,
}: {
  media: TimelineMedia;
  startMS: number;
}) {
  const [frame, setFrame] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    const lease = acquireTimelineVideoFrame(media.url!, startMS);
    void lease.promise
      .then((url) => {
        if (active) setFrame(url);
      })
      .catch(() => {
        if (active) setFailed(true);
      });
    return () => {
      active = false;
      lease.release();
    };
  }, [media.url, startMS]);
  if (failed)
    return <TimelineThumbnail media={media} kind="video" startMS={startMS} />;
  if (!frame)
    return (
      <span className="timeline-thumbnail-placeholder">
        <VideoIcon />
      </span>
    );
  return <RepeatedFrame url={frame} />;
}
function RepeatedFrame({ url }: { url: string }) {
  return (
    <span
      className="timeline-thumbnail"
      data-thumbnail-strip="repeated-frame"
      style={{
        display: "block",
        backgroundImage: `url(${JSON.stringify(url)})`,
        backgroundRepeat: "repeat-x",
        backgroundSize: "auto 100%",
        backgroundPosition: "left center",
      }}
    />
  );
}
