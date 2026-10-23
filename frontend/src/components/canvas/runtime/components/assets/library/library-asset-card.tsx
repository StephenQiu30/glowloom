"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ImageIcon, Music2, Video, FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { FixedThumbnail } from "@/components/canvas/runtime/components/media/fixed-thumbnail";
import { formatBytes } from "@/components/canvas/runtime/lib/media/image-utils";
import { assetMedia } from "../lib/asset-media";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { assetReviewLabel } from "../lib/asset-review";
import { cn } from "@/lib/utils";
export function LibraryAssetCard({
  asset,
  onOpen,
  href,
  compact = false,
}: {
  asset: CanvasContract.AssetVO;
  onOpen?: () => void;
  href?: string;
  compact?: boolean;
}) {
  const icon =
    asset.kind === "image" ? (
      <ImageIcon />
    ) : asset.kind === "audio" ? (
      <Music2 />
    ) : asset.kind === "video" ? (
      <Video />
    ) : (
      <FileText />
    );
  const preview =
    asset.kind === "text" ? (
      <span className="line-clamp-4 p-4 text-left text-sm whitespace-normal text-muted-foreground">
        {asset.content}
      </span>
    ) : asset.kind === "image" || asset.kind === "video" ? (
      <AssetImage asset={asset} />
    ) : (
      <span className="flex flex-col items-center gap-2 text-muted-foreground">
        {icon}
        <span className="text-xs">
          {formatBytes(Number(assetMedia(asset).size ?? 0))}
        </span>
      </span>
    );
  const className = cn(
    "workspace-media bg-muted/40 h-auto w-full rounded-xl p-0",
    compact && "home-asset-preview",
  );
  const content = (
    <>
      {preview}
      {!compact && asset.review ? (
        <Badge
          variant="outline"
          className="pointer-events-none absolute right-2 bottom-2 max-w-[calc(100%-1rem)] truncate bg-background/90"
        >
          {assetReviewLabel(asset.review)}
        </Badge>
      ) : null}
      {!compact ? (
        <Badge
          variant="outline"
          className="pointer-events-none absolute top-2 left-2 bg-background/90"
        >
          {
            { image: "图片", video: "视频", audio: "音频", text: "文本" }[
              asset.kind ?? "text"
            ]
          }
        </Badge>
      ) : null}
    </>
  );
  return href ? (
    <Button asChild variant="outline" className={className}>
      <Link href={href} aria-label={`查看 ${asset.title}`}>
        {content}
      </Link>
    </Button>
  ) : (
    <Button
      variant="outline"
      className={className}
      onClick={onOpen}
      aria-label={`查看 ${asset.title}`}
    >
      {content}
    </Button>
  );
}
function AssetImage({ asset }: { asset: CanvasContract.AssetVO }) {
  const { account } = useAccount();
  const scope = accountScope(account);
  return (
    <ObservedAssetImage
      key={`${scope}:${asset.fileId}:${asset.fileVersion}`}
      asset={asset}
      scope={scope}
      authenticated={!!account}
    />
  );
}

function ObservedAssetImage({
  asset,
  scope,
  authenticated,
}: {
  asset: CanvasContract.AssetVO;
  scope: string;
  authenticated: boolean;
}) {
  const element = useRef<HTMLSpanElement>(null);
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    if (!element.current || !globalThis.IntersectionObserver) return;
    // The workspace and picker scroll inside their own containers. Expanding only
    // the window viewport would still be clipped by those inner scrollports.
    let root = element.current.parentElement;
    while (
      root &&
      !/(auto|scroll)/.test(window.getComputedStyle(root).overflowY)
    ) {
      root = root.parentElement;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        // Keep this page's loaded element: scrolling back must not fetch it again.
        setVisible(true);
        observer.disconnect();
      },
      {
        root,
        rootMargin: `${root?.clientHeight || window.innerHeight}px 0px`,
      },
    );
    observer.observe(element.current);
    return () => observer.disconnect();
  }, []);
  return (
    <span
      ref={element}
      className="absolute inset-0 flex items-center justify-center"
    >
      {authenticated && visible && asset.fileId && asset.fileVersion ? (
        <VisibleThumbnail
          key={`${scope}:${asset.fileId}:${asset.fileVersion}`}
          asset={asset}
          scope={scope}
        />
      ) : asset.kind === "video" ? (
        <Video className="text-muted-foreground" />
      ) : (
        <ImageIcon className="text-muted-foreground" />
      )}
    </span>
  );
}

function VisibleThumbnail({
  asset,
  scope,
}: {
  asset: CanvasContract.AssetVO;
  scope: string;
}) {
  return (
    <FixedThumbnail
      scope={scope}
      fileId={asset.fileId!}
      version={asset.fileVersion!}
      fallback={
        asset.kind === "video" ? (
          <Video className="text-muted-foreground" />
        ) : (
          <ImageIcon className="text-muted-foreground" />
        )
      }
    />
  );
}
