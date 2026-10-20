"use client";
import Image from "next/image";
import { useEffect, useState } from "react";
import { ImageIcon, RotateCw } from "lucide-react";
import { getMediaPreview } from "@/api/media";
import { RequestError } from "@/lib/request";
import { developmentMediaURL } from "@/lib/development-transport";
import { Button } from "@/components/ui/button";
import dynamic from "next/dynamic";
type PreviewProps = {
  projectID: string;
  id: string;
  kind?: string;
  large?: boolean;
};
export function ProjectMediaPreview(props: PreviewProps) {
  return (
    <MediaPreview
      key={props.projectID + ":" + props.id + ":" + props.kind}
      {...props}
    />
  );
}
function MediaPreview({
  projectID,
  id,
  kind = "image",
  large = false,
}: {
  projectID: string;
  id: string;
  kind?: string;
  large?: boolean;
}) {
  const [url, setURL] = useState(""),
    [failed, setFailed] = useState(false),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    getMediaPreview({ pid: projectID, asset_id: id }, { signal: c.signal })
      .then((r) => {
        if (
          r.asset?.id !== id ||
          r.asset?.project_id !== projectID ||
          !r.url ||
          (!/^https?:\/\//.test(r.url) && !developmentMediaURL(r.url)) ||
          !Number.isFinite(Date.parse(r.expires_at ?? "")) ||
          Date.parse(r.expires_at ?? "") <= Date.now()
        )
          throw new RequestError("invalid_receipt");
        if (!c.signal.aborted) {
          setURL(r.url);
          setFailed(false);
        }
      })
      .catch(() => {
        if (!c.signal.aborted) setFailed(true);
      });
    return () => c.abort();
  }, [projectID, id, retry]);
  if (failed)
    return (
      <span className="media-unavailable">
        <ImageIcon />
        <span>预览暂时不可用</span>
        {large && (
          <Button
            variant="ghost"
            onClick={() => {
              setFailed(false);
              setRetry((n) => n + 1);
            }}
          >
            <RotateCw />
            重试
          </Button>
        )}
      </span>
    );
  if (!url)
    return (
      <span className="media-unavailable" role="status">
        正在读取预览…
      </span>
    );
  if (kind === "video")
    return (
      <video
        src={url}
        controls={large}
        preload="metadata"
        className="size-full object-contain"
        onError={() => setFailed(true)}
      />
    );
  if (kind === "audio")
    return (
      <audio
        src={url}
        controls
        preload="metadata"
        onError={() => setFailed(true)}
      />
    );
  return (
    <Image
      src={url}
      alt="素材预览"
      fill
      sizes={large ? "80vw" : "280px"}
      unoptimized
      className={large ? "object-contain" : "object-cover"}
      onError={() => setFailed(true)}
    />
  );
}
const AssetDialog = dynamic(() =>
  import("./project-asset-dialog").then((m) => m.ProjectAssetDialog),
);
export function ProjectMediaPicker({
  projectID,
  onSelect,
  onClose,
  kind = "image",
}: {
  projectID: string;
  kind?: string;
  onSelect: (asset: API.AssetSummary) => void;
  onClose: () => void;
}) {
  return (
    <AssetDialog
      projectID={projectID}
      initialKind={kind}
      reference
      onClose={onClose}
      onAdd={(assets) => {
        for (const asset of assets) onSelect(asset);
      }}
    />
  );
}
