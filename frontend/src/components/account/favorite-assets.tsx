"use client";

import { useEffect, useState } from "react";
import { listMediaLibrary, downloadLibraryMediaAsset } from "@/api/media";
import { useSession } from "@/components/auth/session";
import {
  requireLibraryPage,
  type LibraryPage,
  type LibraryItem,
} from "@/components/media/library";
import { AssetPreview } from "@/components/media/asset-preview";
import { DownloadButton } from "@/components/media/download-button";
import {
  DataPagination,
  type PageSize,
} from "@/components/media/data-pagination";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";

export function FavoriteAssets() {
  const { actor } = useSession();
  const [page, setPage] = useState(1);
  const [size, setSize] = useState<PageSize>(12);
  const [result, setResult] = useState<{
    key: string;
    data: LibraryPage;
  } | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [preview, setPreview] = useState<LibraryItem | null>(null);
  const key = `${actor.id}/${page}/${size}/${retry}`;
  const data = result?.key === key ? result.data : null;
  useEffect(() => {
    const controller = new AbortController();
    void listMediaLibrary(
      {
        scope: "personal",
        favorite_only: true,
        catalog_state: "active",
        page,
        page_size: size,
        order: "updated_desc",
      },
      { signal: controller.signal },
    )
      .then((value) => requireLibraryPage(value, actor.id))
      .then((data) => {
        if (!controller.signal.aborted) {
          setResult({ key, data });
          setError("");
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("收藏读取失败");
      });
    return () => controller.abort();
  }, [actor.id, key, page, size]);
  if (error)
    return (
      <div role="alert" className="flex items-center gap-2">
        {error}
        <Button
          variant="ghost"
          onClick={() => {
            setError("");
            setRetry((n) => n + 1);
          }}
        >
          重试
        </Button>
      </div>
    );
  if (!data)
    return (
      <p role="status" className="py-12 text-sm text-muted-foreground">
        正在读取收藏…
      </p>
    );
  return (
    <>
      {data.items.length ? (
        <div className="profile-asset-grid">
          {data.items.map((item) => (
            <Button
              key={item.id}
              variant="ghost"
              className="h-auto min-w-0 flex-col items-stretch gap-2 p-0 text-left"
              onClick={() => setPreview(item)}
            >
              <div className="asset-cover relative overflow-hidden">
                <AssetPreview item={item} />
              </div>
              <span className="truncate">{item.title}</span>
            </Button>
          ))}
        </div>
      ) : (
        <Empty className="min-h-80">
          <EmptyHeader>
            <EmptyTitle>暂无收藏</EmptyTitle>
          </EmptyHeader>
        </Empty>
      )}
      <DataPagination
        label="收藏分页"
        page={page}
        pageSize={size}
        total={data.total}
        onPageChange={setPage}
        onPageSizeChange={(value) => {
          setSize(value);
          setPage(1);
        }}
      />
      <Dialog
        open={!!preview}
        onOpenChange={(open) => {
          if (!open) setPreview(null);
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogTitle>{preview?.title}</DialogTitle>
          <DialogDescription>收藏素材</DialogDescription>
          {preview ? (
            <>
              <div className="relative aspect-video">
                <AssetPreview key={preview.id} item={preview} large />
              </div>
              <DownloadButton
                filename={preview.title}
                load={() =>
                  downloadLibraryMediaAsset(
                    { item_id: preview.id, scope: "personal" },
                    { responseType: "blob" },
                  )
                }
              >
                下载原件
              </DownloadButton>
            </>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}
