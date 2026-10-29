"use client";
import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { getMediaPreview } from "@/api/media";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import { developmentMediaURL } from "@/lib/development-transport";
import { Button } from "@/components/ui/button";

export function DirectorAssetPreview({
  thumbnail,
  name,
}: {
  thumbnail: string;
  name: string;
}) {
  const [failed, setFailed] = useState(false);
  return (
    <div className="director-library-preview">
      {failed ? (
        <span>预览暂不可用</span>
      ) : (
        <Image
          src={thumbnail}
          alt={name}
          fill
          sizes="140px"
          unoptimized
          className="object-contain"
          onError={() => setFailed(true)}
        />
      )}
    </div>
  );
}

export function DirectorProjectAssetPreview({
  projectID,
  asset,
}: {
  projectID: string;
  asset: API.AssetSummary;
}) {
  const element = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  const [url, setURL] = useState("");
  const [error, setError] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!element.current) return;
    if (!globalThis.IntersectionObserver) {
      const frame = requestAnimationFrame(() => setVisible(true));
      return () => cancelAnimationFrame(frame);
    }
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setVisible(true);
          observer.disconnect();
        }
      },
      { rootMargin: "100px" },
    );
    observer.observe(element.current);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!visible || !asset.id) return;
    const controller = new AbortController(),
      scope = getScopeGeneration(),
      session = getSessionGeneration();
    const current = () =>
      !controller.signal.aborted &&
      scope === getScopeGeneration() &&
      session === getSessionGeneration();
    async function load() {
      try {
        const result = await getMediaPreview(
          { pid: projectID, asset_id: asset.id! },
          { signal: controller.signal },
        );
        if (!current()) return;
        if (
          result.asset?.id !== asset.id ||
          result.asset?.project_id !== projectID ||
          result.asset?.kind !== asset.kind ||
          result.asset?.revision !== asset.revision ||
          !result.url ||
          (!/^https?:\/\//.test(result.url) &&
            !developmentMediaURL(result.url)) ||
          Date.parse(result.expires_at ?? "") <= Date.now() ||
          !Number.isFinite(Date.parse(result.expires_at ?? ""))
        )
          throw new Error("预览不可用");
        const value =
          asset.kind === "model"
            ? await (
                await import("./director-asset-preview-renderer")
              ).renderDirectorAssetThumbnail(result.url, controller.signal)
            : result.url;
        if (current()) {
          setURL(value);
          setError(false);
        }
      } catch {
        if (current()) setError(true);
      }
    }
    void load();
    return () => controller.abort();
  }, [visible, projectID, asset.id, asset.kind, asset.revision, retry]);
  return (
    <div ref={element} className="director-library-preview">
      {error ? (
        <Button
          size="sm"
          variant="ghost"
          onClick={(event) => {
            event.stopPropagation();
            setError(false);
            setRetry((v) => v + 1);
          }}
        >
          重试预览
        </Button>
      ) : url ? (
        <Image
          src={url}
          alt={asset.file_name || "素材预览"}
          fill
          sizes="140px"
          unoptimized
          className="object-contain"
          onError={() => setError(true)}
        />
      ) : (
        <span role="status">{visible ? "正在加载预览…" : "素材预览"}</span>
      )}
    </div>
  );
}
