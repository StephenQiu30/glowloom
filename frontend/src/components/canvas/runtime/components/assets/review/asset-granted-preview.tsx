"use client";

import { useEffect } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import { assetReviewRead } from "@/components/canvas/runtime/api/assetReviewBffController";
import { readGrantedText } from "@/components/canvas/runtime/lib/media/granted-transfer";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

export function AssetGrantedPreview({
  scope,
  id,
  revision,
  kind,
  title,
  onViewed,
}: {
  scope: "REVIEW" | "PUBLIC";
  id: string;
  revision: string;
  kind: string;
  title: string;
  onViewed?: () => void;
}) {
  const { account } = useAccount();
  const preview = useSWR(
    account
      ? ["asset-granted-preview", String(account.id), scope, id, revision]
      : null,
    async (_key, options) => {
      const grant = unwrap(
        await assetReviewRead(
          { scope, id, revision, purpose: "PREVIEW" },
          options,
        ),
      );
      if (!grant.url?.startsWith("/api/file/content?ticket="))
        throw new Error("原件读取授权无效");
      if (kind === "text") {
        const text = await readGrantedText(
          grant.url,
          options.signal instanceof AbortSignal ? options.signal : undefined,
        );
        return { url: grant.url, text };
      }
      return { url: grant.url, text: undefined };
    },
    { shouldRetryOnError: false },
  );
  useEffect(() => {
    if (preview.data?.text != null) onViewed?.();
  }, [preview.data?.text, onViewed]);
  return (
    <div className="flex min-h-40 min-w-0 flex-col items-center justify-center gap-3 rounded-xl bg-muted/30 p-3">
      {preview.isLoading ? (
        <Spinner aria-label="正在读取固定原件" />
      ) : preview.error ? (
        <p className="text-sm text-muted-foreground" role="alert">
          原件暂不可读取，许可或读取凭据可能已变化。
        </p>
      ) : preview.data ? (
        kind === "text" ? (
          <p className="max-h-64 w-full overflow-auto text-sm break-words whitespace-pre-wrap">
            {preview.data.text}
          </p>
        ) : kind === "image" ? (
          <PreviewImage
            src={preview.data.url}
            alt={title}
            className="max-h-80 max-w-full object-contain"
            onLoad={onViewed}
          />
        ) : kind === "video" ? (
          <video
            src={preview.data.url}
            aria-label={title}
            controls
            controlsList="nodownload"
            preload="metadata"
            className="max-h-80 w-full"
            onEnded={onViewed}
          />
        ) : (
          <audio
            src={preview.data.url}
            aria-label={title}
            controls
            controlsList="nodownload"
            preload="metadata"
            className="w-full"
            onEnded={onViewed}
          />
        )
      ) : null}
      <Button
        size="sm"
        variant="ghost"
        disabled={preview.isLoading}
        onClick={() => void preview.mutate()}
      >
        重新读取固定原件
      </Button>
    </div>
  );
}
