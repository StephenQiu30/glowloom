"use client";

import { useRef, useState, useContext, useLayoutEffect } from "react";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { CanvasPreviewRenewal } from "../hooks/use-canvas-file-previews";
import { Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { CanvasNodeData } from "../types/canvas";

export function CanvasVideoPlayer({ node }: { node: CanvasNodeData }) {
  const source = JSON.stringify(
    node.documentData?.fileReferences ??
      node.metadata?.generationOutput ??
      node.metadata?.content,
  );
  return <VideoPlayer key={`${node.id}/${source}`} node={node} />;
}

function VideoPlayer({ node }: { node: CanvasNodeData }) {
  const player = useRef<HTMLVideoElement>(null);
  const [started, setStarted] = useState(false);
  const [error, setError] = useState("");
  const resume = useRef<{
    source: string;
    time: number;
    paused: boolean;
  } | null>(null);
  const renew = useContext(CanvasPreviewRenewal);
  const position = useRef(0);
  const output = JSON.stringify(node.metadata?.generationOutput);
  const src = node.metadata?.content;
  useLayoutEffect(() => {
    const video = player.current;
    if (!video || !src || video.getAttribute("src") === src) return;
    if (video.hasAttribute("src") && !resume.current) {
      resume.current = {
        source: output,
        time: video.currentTime || position.current,
        paused: video.paused || video.ended,
      };
    }
    video.src = src;
  }, [src, output]);
  return (
    <div
      className="canvas-video-player bg-media-backdrop relative h-full w-full overflow-hidden rounded-xl"
      data-canvas-no-zoom
    >
      <video
        ref={player}
        controls={started}
        playsInline
        preload="metadata"
        aria-label={node.title || "视频节点"}
        className="h-full w-full object-contain"
        data-canvas-video={node.id}
        onTimeUpdate={(event) => {
          if (event.currentTarget.readyState >= 2)
            position.current = event.currentTarget.currentTime;
        }}
        onPlay={() => {
          setStarted(true);
          setError("");
        }}
        onEnded={() => setStarted(false)}
        onPointerDown={started ? (event) => event.stopPropagation() : undefined}
        onMouseDown={started ? (event) => event.stopPropagation() : undefined}
        onDoubleClick={started ? (event) => event.stopPropagation() : undefined}
        onKeyDown={(event) => event.stopPropagation()}
        onError={(event) => {
          resume.current = {
            source: output,
            time: event.currentTarget.currentTime || position.current,
            paused: event.currentTarget.paused,
          };
          setError("视频暂时无法播放，请重试");
          setStarted(false);
        }}
        onLoadedMetadata={(event) => {
          setError("");
          const previous = resume.current;
          resume.current = null;
          if (!previous || previous.source !== output) return;
          const video = event.currentTarget;
          if (Number.isFinite(video.duration))
            video.currentTime = Math.min(previous.time, video.duration);
          if (!previous.paused)
            void video.play().catch(() => setStarted(false));
        }}
      />
      {!started && (
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
          <Button
            variant="ghost"
            size="icon-lg"
            className="rounded-full bg-black/30 text-white hover:bg-black/50"
            aria-label={error ? "重试播放视频" : "播放视频"}
            onPointerDown={(event) => event.stopPropagation()}
            onMouseDown={(event) => event.stopPropagation()}
            onKeyDown={(event) => event.stopPropagation()}
            onDoubleClick={(event) => event.stopPropagation()}
            onClick={(event) => {
              event.stopPropagation();
              if (error && renew && node.documentData?.fileReferences?.length) {
                resume.current = {
                  source: output,
                  time: resume.current?.time ?? position.current,
                  paused: false,
                };
                renew(node);
                return;
              }
              if (error) player.current?.load();
              void player.current?.play().catch((reason: unknown) => {
                // Pausing or replacing a loading video cancels play() normally.
                if (
                  reason instanceof DOMException &&
                  reason.name === "AbortError"
                )
                  return;
                setError("视频暂时无法播放，请重试");
              });
            }}
          >
            <Play />
          </Button>
        </div>
      )}
      <FeedbackToast message={error} />
    </div>
  );
}
