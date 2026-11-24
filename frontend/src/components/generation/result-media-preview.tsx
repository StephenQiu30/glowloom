"use client";

import { useState, type ReactNode } from "react";
import { Play } from "lucide-react";
import { parseAspectRatio } from "@/components/canvas/runtime/lib/media/media-size";
import { NearViewport } from "@/components/media/near-viewport";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

export function ResultFrame({
  kind,
  ratio,
  children,
}: {
  kind: string;
  ratio: unknown;
  children: ReactNode;
}) {
  const parsed = typeof ratio === "string" && parseAspectRatio(ratio);
  const value = parsed && parsed.width / parsed.height;
  const aspect =
    value && Number.isFinite(value) ? value : kind === "video" ? 16 / 9 : 1;
  return (
    <div
      className="relative flex w-full items-center justify-center overflow-hidden rounded-xl bg-muted"
      style={{ aspectRatio: aspect, maxWidth: `min(36rem, ${52 * aspect}dvh)` }}
    >
      {children}
    </div>
  );
}

export function ResultMediaPreview({
  kind,
  projectID,
  assetID,
}: {
  kind: "image" | "video";
  projectID: string;
  assetID: string;
}) {
  const [play, setPlay] = useState(false);
  if (kind === "video" && !play) {
    return (
      <Button
        type="button"
        variant="secondary"
        size="icon-lg"
        className="min-h-11 min-w-11 rounded-full"
        aria-label="播放生成视频"
        onClick={() => setPlay(true)}
      >
        <Play />
      </Button>
    );
  }
  const preview = (
    <ProjectMediaPreview
      projectID={projectID}
      id={assetID}
      kind={kind}
      large
      playOnOpen={play}
    />
  );
  if (play) return preview;
  return (
    <NearViewport
      className="relative size-full"
      placeholder={<Skeleton className="size-full" aria-hidden />}
    >
      {preview}
    </NearViewport>
  );
}
