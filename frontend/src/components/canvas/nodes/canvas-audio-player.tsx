"use client";
import { useEffect, useRef, useState } from "react";
import { AudioLines, Play, Pause } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { toast } from "@/components/feedback/notification";
import { acquireTimelineWaveform } from "../timeline/timeline-waveform";

function timestamp(seconds: number) {
  return [
    Math.floor(seconds / 3600),
    Math.floor(seconds / 60) % 60,
    Math.floor(seconds) % 60,
  ]
    .map((v) => String(v).padStart(2, "0"))
    .join(":");
}

export function CanvasAudioPlayer({
  src,
  title,
}: {
  src: string;
  title: string;
}) {
  const player = useRef<HTMLAudioElement>(null);
  const [duration, setDuration] = useState(0),
    [position, setPosition] = useState(0),
    [playing, setPlaying] = useState(false),
    [peaks, setPeaks] = useState<number[]>([]);
  useEffect(() => {
    const waveform = acquireTimelineWaveform(src);
    let active = true;
    waveform.promise
      .then((values) => {
        if (active) setPeaks(values);
      })
      .catch(() => {});
    return () => {
      active = false;
      waveform.release();
    };
  }, [src]);
  const progress = duration ? position / duration : 0;
  return (
    <div
      className="group/audio relative flex size-full items-center justify-center px-6 pb-6"
      data-canvas-no-zoom
      data-canvas-shortcuts-ignore
    >
      <audio
        ref={player}
        src={src}
        preload="metadata"
        aria-label={title}
        onLoadedMetadata={(e) => {
          const length = e.currentTarget.duration;
          setDuration(Number.isFinite(length) ? length : 0);
        }}
        onTimeUpdate={(e) => setPosition(e.currentTarget.currentTime)}
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onError={() => toast.error("音频暂时无法播放")}
      />
      <div className="relative h-20 w-full">
        {peaks.length ? (
          <svg
            aria-hidden
            viewBox="0 0 256 80"
            preserveAspectRatio="none"
            className="size-full text-muted-foreground"
          >
            {peaks.map((v, i) => {
              const height = Math.max(1, v * 76);
              return (
                <rect
                  key={i}
                  x={i * 2}
                  y={(80 - height) / 2}
                  width="1.4"
                  height={height}
                  fill="currentColor"
                  opacity={i / peaks.length < progress ? 1 : 0.5}
                />
              );
            })}
          </svg>
        ) : (
          <AudioLines
            className="mx-auto h-full text-muted-foreground"
            aria-hidden
          />
        )}
        <Slider
          aria-label="播放位置"
          className="absolute inset-0 opacity-0 focus-within:opacity-100"
          disabled={!duration}
          min={0}
          max={duration || 1}
          step={0.05}
          value={[position]}
          onPointerDown={(e) => e.stopPropagation()}
          onValueChange={([seconds]) => {
            if (player.current) player.current.currentTime = seconds;
            setPosition(seconds);
          }}
        />
      </div>
      <span className="absolute bottom-4 left-5 text-sm tabular-nums">
        {timestamp(playing ? position : duration)}
      </span>
      <Button
        size="icon-sm"
        variant="secondary"
        className="absolute right-4 bottom-4 opacity-0 group-hover/audio:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100"
        aria-label={playing ? "暂停音频" : "播放音频"}
        onPointerDown={(e) => e.stopPropagation()}
        onClick={(e) => {
          e.stopPropagation();
          const audio = player.current;
          if (!audio) return;
          if (playing) audio.pause();
          else void audio.play().catch(() => toast.error("音频暂时无法播放"));
        }}
      >
        {playing ? <Pause /> : <Play />}
      </Button>
    </div>
  );
}
