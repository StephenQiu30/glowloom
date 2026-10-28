"use client";
import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import {
  AudioLines,
  ImageIcon,
  Pause,
  Play,
  RotateCw,
  Video,
} from "lucide-react";
import { previewLibraryMediaAsset } from "@/api/media";
import { getSubjectMedia, previewSubjectSource } from "@/api/subjects";
import { Button } from "@/components/ui/button";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import { requirePreview } from "@/components/media/library";
import type { SelectedSubjectMedia } from "./unified-contracts";

/** Preview access uses formal identities, never a guessed filename or MIME. */
export type SubjectPreviewMedia = Pick<
  SelectedSubjectMedia,
  | "asset_id"
  | "asset_revision"
  | "kind"
  | "bound"
  | "library_item_id"
  | "library_source"
> &
  Partial<Pick<SelectedSubjectMedia, "file_name" | "mime_type">>;

function SubjectAudio({
  url,
  label,
  onError,
}: {
  url: string;
  label: string;
  onError: () => void;
}) {
  const audio = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [starting, setStarting] = useState(false);
  const busy = useRef(false);
  useEffect(() => {
    const element = audio.current;
    return () => element?.pause();
  }, [url]);
  async function toggle() {
    if (!audio.current || busy.current) return;
    if (playing) {
      audio.current.pause();
      return;
    }
    busy.current = true;
    setStarting(true);
    try {
      await audio.current.play();
    } catch {
      onError();
    } finally {
      busy.current = false;
      setStarting(false);
    }
  }
  return (
    <>
      <audio
        ref={audio}
        src={url}
        preload="metadata"
        hidden
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onEnded={() => setPlaying(false)}
        onError={onError}
      />
      <Button
        type="button"
        variant="secondary"
        disabled={starting}
        aria-label={`${playing ? "暂停" : "播放"} ${label}`}
        aria-pressed={playing}
        className="max-w-full gap-2"
        onClick={() => void toggle()}
      >
        {playing ? <Pause /> : <Play />}
        <span className="truncate">{label}</span>
      </Button>
    </>
  );
}

export function UnifiedSubjectMedia({
  media,
  label,
  interactive = false,
}: {
  media: SubjectPreviewMedia;
  label: string;
  interactive?: boolean;
}) {
  const [state, setState] = useState<{
    identity: string;
    url: string;
    error: boolean;
  } | null>(null);
  const [retry, setRetry] = useState(0);
  const subjectID = media.bound?.subjectID;
  const versionID = media.bound?.versionID;
  const bindingID = media.bound?.bindingID;
  const mediaIdentity = subjectID
    ? `${subjectID}/${versionID}/${bindingID}`
    : `${media.library_item_id ?? media.asset_id}/${media.asset_revision}`;
  const identity = `${getSessionGeneration()}/${getScopeGeneration()}/${mediaIdentity}`;
  useEffect(() => {
    const controller = new AbortController();
    let objectURL = "";
    const request =
      subjectID && versionID && bindingID
        ? getSubjectMedia(
            {
              id: subjectID,
              version: versionID,
              binding: bindingID,
            },
            { responseType: "blob", signal: controller.signal },
          ).then((blob) => {
            if (controller.signal.aborted) throw new Error("preview_cancelled");
            if (!(blob instanceof Blob) || !blob.size)
              throw new Error("empty_media");
            objectURL = URL.createObjectURL(blob);
            return objectURL;
          })
        : media.library_source === "subject"
          ? previewSubjectSource(
              { item_id: media.library_item_id ?? media.asset_id },
              { signal: controller.signal },
            ).then((value) => requirePreview(value, media.asset_id))
          : previewLibraryMediaAsset(
              {
                item_id: media.library_item_id ?? media.asset_id,
                scope: "personal",
              },
              { signal: controller.signal },
            ).then((value) => requirePreview(value, media.asset_id));
    request
      .then((url) => {
        if (!controller.signal.aborted)
          setState({ identity, url, error: false });
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setState({ identity, url: "", error: true });
      });
    return () => {
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [
    identity,
    subjectID,
    versionID,
    bindingID,
    media.asset_id,
    media.library_item_id,
    media.library_source,
    retry,
  ]);
  const failed = () => setState({ identity, url: "", error: true });
  const current = state?.identity === identity ? state : null;
  const Icon =
    media.kind === "audio"
      ? AudioLines
      : media.kind === "video"
        ? Video
        : ImageIcon;
  if (!current?.url || current.error)
    return (
      <div
        className="flex size-full min-h-16 flex-col items-center justify-center gap-2 text-xs text-muted-foreground"
        role="status"
      >
        <Icon className="size-6" />
        <span>{current?.error ? "预览暂不可用" : "正在读取预览…"}</span>
        {current?.error && interactive ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setRetry((n) => n + 1)}
          >
            <RotateCw />
            重试预览
          </Button>
        ) : null}
      </div>
    );
  if (media.kind === "audio")
    return (
      <SubjectAudio
        key={current.url}
        url={current.url}
        label={label}
        onError={failed}
      />
    );
  if (media.kind === "video")
    return (
      <video
        src={current.url}
        controls={interactive}
        playsInline
        preload="metadata"
        onError={failed}
        aria-label={label}
        className="size-full object-contain"
      />
    );
  return (
    <Image
      src={current.url}
      alt={label}
      fill
      unoptimized
      sizes={
        interactive
          ? "(max-width:600px) 80vw,512px"
          : "(max-width:600px) 45vw,275px"
      }
      className="object-contain"
      onError={failed}
    />
  );
}
