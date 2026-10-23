import { useEffect, useRef, useState } from "react";
import useSWR, { useSWRConfig, unstable_serialize } from "swr";
import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { thumbnailQueue } from "@/components/canvas/runtime/components/assets/lib/thumbnail-queue";

/** Lists request visible thumbnails only, never original media or playable videos. */
export function HistoryTaskThumbnail({
  task,
  scope,
}: {
  task: CanvasContract.GenerationTaskVO;
  scope: string;
}) {
  const element = useRef<HTMLSpanElement>(null);
  const [visible, setVisible] = useState(
    () => !globalThis.IntersectionObserver,
  );
  useEffect(() => {
    if (!element.current) return;
    if (!globalThis.IntersectionObserver) return;
    const root = element.current.closest('[data-slot="scroll-area-viewport"]');
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) setVisible(true);
      },
      { root },
    );
    observer.observe(element.current);
    return () => observer.disconnect();
  }, []);
  const output = task.outputs?.[0];
  const fileId = output?.fileId ?? task.fileId;
  const version = output?.fileVersion ?? task.fileVersion;
  return (
    <span ref={element} className="history-task-thumbnail" aria-hidden="true">
      {visible &&
      fileId &&
      version &&
      task.state === "SUCCEEDED" &&
      task.modality !== "text" ? (
        <Thumbnail
          key={`${scope}:${fileId}:${version}`}
          scope={scope}
          fileId={fileId}
          version={version}
        />
      ) : task.modality === "text" ? (
        "Aa"
      ) : (
        "—"
      )}
    </span>
  );
}

function Thumbnail({
  scope,
  fileId,
  version,
}: {
  scope: string;
  fileId: string;
  version: string;
}) {
  const { cache, mutate } = useSWRConfig();
  const queue = thumbnailQueue(cache, scope);
  const key = [
    "asset-preview",
    scope,
    fileId,
    version,
    "thumbnail",
    "webp-frame-v2",
  ];
  const serialized = unstable_serialize(key);
  const [broken, setBroken] = useState(false);
  useEffect(
    () =>
      queue.retain({ fileId, version, variant: "thumbnail" }, () => {
        void mutate(serialized, undefined, { revalidate: false })
          .catch(() => undefined)
          .then(() => cache.delete(serialized));
      }),
    [cache, mutate, queue, serialized, fileId, version],
  );
  const image = useSWR(
    key,
    () => queue.request({ fileId, version, variant: "thumbnail" }),
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      revalidateOnMount: true,
      dedupingInterval: 0,
    },
  );
  return image.data?.status === "READY" && image.data.url && !broken ? (
    <PreviewImage
      src={image.data.url}
      alt=""
      loading="lazy"
      className="size-full object-cover"
      onError={() => setBroken(true)}
    />
  ) : (
    "—"
  );
}
