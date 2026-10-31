"use client";
import { useEffect, useState } from "react";
import Image from "next/image";
import { getMediaPreview } from "@/api/media";
import { UnifiedSubjectMedia } from "@/components/subjects/unified-media";
import { Button } from "@/components/ui/button";
import { requirePreview } from "@/components/media/library";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import type {
  CanvasSubjectConfig,
  CanvasSubjectMedia,
} from "./subject-node-state";

export function SubjectNodeMedia({
  media,
  config,
  projectID,
  label,
  interactive = false,
}: {
  media: CanvasSubjectMedia;
  config: CanvasSubjectConfig;
  projectID: string;
  label: string;
  interactive?: boolean;
}) {
  if (media.binding_id && config.subject_id && config.version_id)
    return (
      <UnifiedSubjectMedia
        media={{
          ...media,
          bound: {
            subjectID: config.subject_id,
            versionID: config.version_id,
            bindingID: media.binding_id,
          },
        }}
        label={label}
        interactive={interactive}
      />
    );
  return (
    <ProjectSubjectMedia
      key={`${projectID}/${media.asset_id}/${media.asset_revision}/${getSessionGeneration()}/${getScopeGeneration()}`}
      media={media}
      projectID={projectID}
      label={label}
      interactive={interactive}
    />
  );
}
function ProjectSubjectMedia({
  media,
  projectID,
  label,
  interactive,
}: {
  media: CanvasSubjectMedia;
  projectID: string;
  label: string;
  interactive: boolean;
}) {
  const [state, setState] = useState<{ url?: string; error?: string }>({});
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    getMediaPreview(
      { pid: projectID, asset_id: media.asset_id },
      { signal: controller.signal },
    )
      .then((value) => {
        if (
          value.asset?.project_id !== projectID ||
          value.asset.revision !== media.asset_revision ||
          value.asset.kind !== media.kind
        )
          throw new Error("素材版本已变化");
        return requirePreview(value, media.asset_id);
      })
      .then((url) => {
        if (!controller.signal.aborted) setState({ url });
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setState({ error: "素材已失效或无权读取" });
      });
    return () => controller.abort();
  }, [projectID, media.asset_id, media.asset_revision, media.kind, retry]);
  if (state.error)
    return (
      <div role="status">
        {state.error}
        {interactive && (
          <Button
            variant="ghost"
            onClick={() => {
              setState({});
              setRetry((value) => value + 1);
            }}
          >
            重试
          </Button>
        )}
      </div>
    );
  if (!state.url) return <p role="status">正在读取素材…</p>;
  if (media.kind === "audio")
    return (
      <audio
        src={state.url}
        controls
        preload="metadata"
        aria-label={label}
        className="w-full"
      />
    );
  if (media.kind === "video")
    return (
      <video
        src={state.url}
        controls={interactive}
        playsInline
        preload="metadata"
        aria-label={label}
        className="size-full object-contain"
      />
    );
  return (
    <Image
      src={state.url}
      alt={label}
      width={640}
      height={640}
      unoptimized
      className="size-full object-contain"
    />
  );
}
