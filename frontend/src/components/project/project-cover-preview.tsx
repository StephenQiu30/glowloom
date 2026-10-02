"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import type { FolderScope } from "./folder-intent";
import {
  getProjectCoverPreview,
  projectCoverPreviewKey,
} from "./project-cover-queries";

export function ProjectCoverPreview({
  projectId,
  assetId,
  scope,
  unavailable = false,
  eager = false,
  alt = "",
}: {
  projectId: string;
  assetId: string;
  scope: FolderScope;
  unavailable?: boolean;
  eager?: boolean;
  alt?: string;
}) {
  const container = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(eager);
  const [failedLease, setFailedLease] = useState<number | null>(null);
  useEffect(() => {
    if (eager || !container.current || unavailable) return;
    if (typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        setVisible(true);
        observer.disconnect();
      }
    });
    observer.observe(container.current);
    return () => observer.disconnect();
  }, [eager, unavailable]);
  const preview = useQuery({
    queryKey: projectCoverPreviewKey(scope, projectId, assetId),
    queryFn: ({ signal }) => getProjectCoverPreview(projectId, assetId, signal),
    enabled: visible && !unavailable,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    staleTime: 0,
    gcTime: 0,
  });
  useEffect(() => {
    if (!preview.data) return;
    const timer = window.setTimeout(
      () => setFailedLease(preview.dataUpdatedAt),
      Math.max(
        0,
        Math.min(
          2147483647,
          Date.parse(preview.data.expires_at) - Date.now() - 5000,
        ),
      ),
    );
    return () => window.clearTimeout(timer);
  }, [preview.data, preview.dataUpdatedAt]);
  const failed =
    preview.error || (preview.data && failedLease === preview.dataUpdatedAt);
  return (
    <div
      ref={container}
      className="relative flex h-full min-h-20 w-full items-center justify-center"
    >
      {unavailable ? (
        <span role="status" className="text-xs text-muted-foreground">
          主图不可用
        </span>
      ) : failed ? (
        <div className="flex flex-col items-center gap-2">
          <span role="status" className="text-xs text-muted-foreground">
            主图预览未能读取
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={preview.isFetching}
            onClick={() => void preview.refetch()}
          >
            重新读取主图
          </Button>
        </div>
      ) : preview.data && visible ? (
        <Image
          unoptimized
          fill
          src={preview.data.url}
          alt={alt}
          sizes="(max-width: 640px) 100vw, 320px"
          className="object-cover"
          onError={() => setFailedLease(preview.dataUpdatedAt)}
        />
      ) : (
        <span role="status" className="text-xs text-muted-foreground">
          {preview.isFetching ? "正在读取主图…" : "主图待预览"}
        </span>
      )}
    </div>
  );
}
