"use client";
import { useEffect, useState } from "react";
import Image from "next/image";
import { File, ImageIcon, Music, Video, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { previewLibraryMediaAsset } from "./generated/media";
import { requirePreview, type LibraryItem } from "./library";
export function AssetPreview({
  item,
  large = false,
}: {
  item: LibraryItem;
  large?: boolean;
}) {
  const [url, setURL] = useState("");
  const [failed, setFailed] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!["image", "video", "audio"].includes(item.kind)) return;
    const controller = new AbortController();
    previewLibraryMediaAsset(
      { item_id: item.id, scope: "personal" },
      { signal: controller.signal },
    )
      .then((v) => requirePreview(v, item.id))
      .then((u) => {
        if (!controller.signal.aborted) setURL(u);
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true);
      });
    return () => controller.abort();
  }, [item.id, item.kind, retry]);
  const Icon =
    item.kind === "image"
      ? ImageIcon
      : item.kind === "video"
        ? Video
        : item.kind === "audio"
          ? Music
          : File;
  if (!["image", "video", "audio"].includes(item.kind))
    return (
      <div className="asset-placeholder">
        <Icon size={36} strokeWidth={1.2} />
        <span>下载原件以查看</span>
      </div>
    );
  if (failed)
    return (
      <div className="asset-placeholder">
        <Icon />
        <span>预览暂不可用</span>
        {large && (
          <Button
            variant="ghost"
            onClick={() => {
              setFailed(false);
              setURL("");
              setRetry((n) => n + 1);
            }}
          >
            <RotateCw />
            重试预览
          </Button>
        )}
      </div>
    );
  if (url && item.kind === "image")
    return (
      <Image
        src={url}
        alt={item.title}
        fill
        unoptimized
        sizes="(max-width:700px) 50vw,25vw"
        className={large ? "object-contain" : "object-cover"}
        onError={() => setFailed(true)}
      />
    );
  if (url && large && item.kind === "video")
    return (
      <video
        src={url}
        controls
        playsInline
        className="h-full w-full"
        onError={() => setFailed(true)}
      />
    );
  if (url && large && item.kind === "audio")
    return (
      <audio
        src={url}
        controls
        className="w-full"
        onError={() => setFailed(true)}
      />
    );
  return (
    <div className="asset-placeholder">
      <Icon size={36} strokeWidth={1.2} />
      <span>
        {url ? (item.kind === "video" ? "查看视频" : "播放音频") : "加载预览…"}
      </span>
    </div>
  );
}
