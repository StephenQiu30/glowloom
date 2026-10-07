"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Folder, MoreHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardFooter,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import type { FolderCover, FolderSummary } from "./folder-model";
import type { FolderScope } from "./folder-intent";
import {
  FOLDERS_KEY,
  folderCoverPreviewKey,
  getFolderCoverPreview,
} from "./folder-queries";

export type FolderAction = "rename" | "cover" | "recycle";
export function ProjectFolderCard({
  folder,
  scope,
  onOpen,
  onAction,
}: {
  folder: FolderSummary;
  scope: FolderScope;
  onOpen: (id: string) => void;
  onAction: (folder: FolderSummary, action: FolderAction) => void;
}) {
  const cache = useQueryClient();
  return (
    <Card variant="folder" className="h-full min-w-0">
      <button
        className="relative flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-muted focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={`打开目录 ${folder.name}`}
        onClick={() => onOpen(folder.id)}
      >
        {folder.cover ? (
          <FolderCoverImage
            key={`${folder.cover.project_id}:${folder.cover.asset_id}:${folder.revision}`}
            cover={folder.cover}
            scope={scope}
            alt={folder.name}
          />
        ) : (
          <Folder
            aria-hidden="true"
            className="size-12 text-muted-foreground"
          />
        )}
      </button>
      <CardHeader className="min-w-0 flex-1 p-0">
        <CardTitle className="min-w-0">
          <button
            className="rounded-sm text-left break-words focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => onOpen(folder.id)}
          >
            {folder.name}
          </button>
        </CardTitle>
        <CardDescription>
          {folder.project_count} 个项目
          {folder.cover_unavailable && <span className="ml-2">封面不可用</span>}
        </CardDescription>
        <p className="sr-only">
          <span aria-hidden="true">编号 {folder.id.slice(0, 8)}</span>
          <span className="sr-only">目录编号 {folder.id}</span>
        </p>
      </CardHeader>
      <CardFooter className="p-0">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              data-folder-actions={folder.id}
              aria-label={`${folder.name}的目录操作`}
            >
              <MoreHorizontal data-icon="inline-start" aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuGroup>
              <DropdownMenuItem onSelect={() => onOpen(folder.id)}>
                打开目录
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onAction(folder, "rename")}>
                修改名称
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onAction(folder, "cover")}>
                更换封面
              </DropdownMenuItem>
              {folder.cover && (
                <DropdownMenuItem
                  onSelect={() =>
                    void cache.invalidateQueries({ queryKey: FOLDERS_KEY })
                  }
                >
                  重新读取封面
                </DropdownMenuItem>
              )}
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => onAction(folder, "recycle")}
              >
                回收目录及所有项目
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </CardFooter>
    </Card>
  );
}
export function FolderCoverImage({
  cover,
  scope,
  alt,
}: {
  cover: FolderCover;
  scope: FolderScope;
  alt: string;
}) {
  const [failedLease, setFailedLease] = useState<number | null>(null);
  const preview = useQuery({
    queryKey: folderCoverPreviewKey(scope, cover),
    queryFn: ({ signal }) => getFolderCoverPreview(cover, signal),
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
  if (preview.isFetching)
    return (
      <span role="status" className="text-xs text-muted-foreground">
        正在读取封面…
      </span>
    );
  if (preview.error || failedLease === preview.dataUpdatedAt)
    return (
      <span role="status" className="text-xs text-muted-foreground">
        封面读取失败
      </span>
    );
  return preview.data ? (
    <Image
      unoptimized
      fill
      src={preview.data.url}
      alt={alt}
      sizes="(max-width: 640px) 100vw, 320px"
      className="object-cover"
      onError={() => setFailedLease(preview.dataUpdatedAt)}
    />
  ) : null;
}
