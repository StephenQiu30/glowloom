"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import type { FolderScope } from "./folder-intent";
import {
  PROJECT_COVERS_KEY,
  listProjectCoverImages,
} from "./project-cover-queries";
import { ProjectCoverPreview } from "./project-cover-preview";

export function ProjectCoverPicker({
  projectId,
  scope,
  value,
  onClose,
  onSelected,
}: {
  projectId: string;
  scope: FolderScope;
  value: string | null;
  onClose: () => void;
  onSelected: (assetId: string) => void;
}) {
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [page, setPage] = useState(0);
  const images = useQuery({
    queryKey: [
      ...PROJECT_COVERS_KEY,
      scope.origin,
      scope.actorId,
      scope.orgId,
      projectId,
      "picker",
      cursors[page] ?? "first",
    ],
    queryFn: ({ signal }) =>
      listProjectCoverImages(projectId, cursors[page], signal),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const items = images.data?.items ?? [];
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="max-h-[85dvh] overflow-y-auto"
        showCloseButton={false}
      >
        <DialogHeader>
          <DialogTitle>选择项目主图</DialogTitle>
          <DialogDescription>
            只选择当前项目已审核的正式图片；选择后保存项目设置。
          </DialogDescription>
        </DialogHeader>
        {images.isPending && <p role="status">正在读取项目图片…</p>}
        {images.error && (
          <Alert variant="destructive">
            <AlertTitle>项目图片未能读取</AlertTitle>
            <AlertDescription>{images.error.message}</AlertDescription>
            <Button
              onClick={() => void images.refetch()}
              disabled={images.isFetching}
            >
              重试读取图片
            </Button>
          </Alert>
        )}
        {!images.isPending && !images.error && !items.length && (
          <p role="status">已读取的这一页没有可用图片。</p>
        )}
        <ul className="grid grid-cols-2 gap-3">
          {items.map((asset) => (
            <li key={asset.id} className="min-w-0">
              <div className="relative aspect-video overflow-hidden rounded-lg bg-muted">
                <ProjectCoverPreview
                  projectId={projectId}
                  assetId={asset.id}
                  scope={scope}
                />
              </div>
              <Button
                type="button"
                variant={asset.id === value ? "secondary" : "outline"}
                className="mt-2 h-auto w-full min-w-0 break-all whitespace-normal"
                onClick={() => onSelected(asset.id)}
                aria-label={`选择主图 ${asset.file_name} · ${asset.id}`}
              >
                {asset.id === value ? "已选 · " : ""}
                {asset.file_name}
                <span className="sr-only">{asset.id}</span>
              </Button>
            </li>
          ))}
        </ul>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={page === 0 || images.isFetching}
            onClick={() => setPage(page - 1)}
          >
            上一页图片
          </Button>
          <span aria-live="polite" className="text-xs text-muted-foreground">
            第 {page + 1} 页
          </span>
          <Button
            type="button"
            variant="outline"
            disabled={!images.data?.next_cursor || images.isFetching}
            onClick={() => {
              const cursor = images.data?.next_cursor;
              if (!cursor) return;
              setCursors((current) => [...current.slice(0, page + 1), cursor]);
              setPage(page + 1);
            }}
          >
            下一页图片
          </Button>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            取消选择
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
