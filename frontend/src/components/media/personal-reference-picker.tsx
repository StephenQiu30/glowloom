"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import Image from "next/image";
import { useEffect, useState } from "react";
import { FileText, ImageIcon, Search, Upload, Video } from "lucide-react";
import { listMediaLibrary, previewLibraryMediaAsset } from "@/api/media";
import { requireLibraryPage, mediaError, type LibraryItem } from "./library";
import { UploadDialog } from "./upload-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldLabel } from "@/components/ui/field";
import { ContentState } from "@/components/layout/content-state";
import { isProjectID } from "@/components/projects/project";

export type ReferenceOwner = { actorId: string; scopeId: string };
export type PersonalReferenceItem = LibraryItem & {
  asset_id: string;
  media: API.LibraryAssetSummary & { id: string; revision: number };
};
export function isPersonalReferenceItem(
  item: LibraryItem,
): item is PersonalReferenceItem {
  return (
    item.catalog_state === "active" &&
    isProjectID(item.id) &&
    Number.isSafeInteger(item.revision) &&
    item.revision > 0 &&
    isProjectID(item.asset_id) &&
    item.media?.id === item.asset_id &&
    item.media.kind === item.kind &&
    Number.isSafeInteger(item.media.revision) &&
    item.media.revision! > 0 &&
    ["image", "video", "document"].includes(item.kind)
  );
}

/** Both IDs are explicit: library item IDs and media IDs are not interchangeable. */
export function PersonalMediaPreview({
  itemID,
  assetID,
  kind = "image",
  title = "参考素材",
  large = false,
}: {
  itemID: string;
  assetID?: string;
  kind?: string;
  title?: string;
  large?: boolean;
}) {
  return (
    <PersonalPreview
      key={`${itemID}/${assetID}/${kind}/${large}`}
      {...{ itemID, assetID, kind, title, large }}
    />
  );
}
function PersonalPreview({
  itemID,
  assetID,
  kind,
  title,
  large,
}: Required<Omit<Parameters<typeof PersonalMediaPreview>[0], "assetID">> & {
  assetID?: string;
}) {
  const [url, setURL] = useState("");
  const [failed, setFailed] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!["image", "video"].includes(kind)) return;
    const c = new AbortController();
    previewLibraryMediaAsset(
      { item_id: itemID, scope: "personal" },
      { signal: c.signal },
    )
      .then((r) => {
        if (
          !r.asset ||
          (assetID && r.asset.id !== assetID) ||
          !r.url ||
          !/^https?:\/\//.test(r.url) ||
          !Number.isFinite(Date.parse(r.expires_at ?? "")) ||
          Date.parse(r.expires_at!) <= Date.now()
        )
          throw new Error("预览回执无效");
        if (!c.signal.aborted) setURL(r.url);
      })
      .catch(() => {
        if (!c.signal.aborted) setFailed(true);
      });
    return () => c.abort();
  }, [itemID, assetID, kind, retry]);
  const Icon =
    kind === "video" ? Video : kind === "document" ? FileText : ImageIcon;
  if (failed)
    return (
      <div className="flex size-full items-center justify-center gap-1 text-xs text-muted-foreground">
        <Icon className="size-4" />
        <span>预览不可用</span>
        {large && (
          <Button
            variant="ghost"
            onClick={() => {
              setFailed(false);
              setURL("");
              setRetry((n) => n + 1);
            }}
          >
            重试
          </Button>
        )}
      </div>
    );
  if (!url)
    return (
      <div className="flex size-full flex-col items-center justify-center gap-1 text-xs text-muted-foreground">
        <Icon className="size-6" />
        <span>{kind === "document" ? title : "正在读取预览…"}</span>
      </div>
    );
  return kind === "video" ? (
    <video
      src={url}
      aria-label={title}
      controls={large}
      playsInline
      preload="metadata"
      className="size-full object-contain"
      onError={() => setFailed(true)}
    />
  ) : (
    <Image
      src={url}
      alt={title}
      fill
      unoptimized
      sizes={large ? "80vw" : "240px"}
      className={large ? "object-contain" : "object-cover"}
      onError={() => setFailed(true)}
    />
  );
}

const kindLabels: Record<string, string> = {
  image: "图片",
  video: "视频",
  document: "小说 / 文档",
};
export function PersonalLibraryPicker(props: {
  owner: ReferenceOwner;
  allowedKinds?: readonly string[];
  maxSelection?: number;
  onSelect: (items: PersonalReferenceItem[]) => void | Promise<void>;
  onClose: () => void;
}) {
  return (
    <PersonalPicker
      key={`${props.owner.actorId}/${props.owner.scopeId}`}
      {...props}
    />
  );
}
function PersonalPicker({
  owner,
  allowedKinds = ["image", "video", "document"],
  maxSelection = 30,
  onSelect,
  onClose,
}: Parameters<typeof PersonalLibraryPicker>[0]) {
  const [kind, setKind] = useState(allowedKinds[0]!);
  const [filter, setFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [revision, setRevision] = useState(0);
  const [selection, setSelection] = useState<PersonalReferenceItem[]>([]);
  const [upload, setUpload] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{
    key: string;
    items: LibraryItem[];
    total: number;
    error: string;
  }>();
  const key = `${owner.actorId}/${owner.scopeId}/${kind}/${filter}/${search}/${page}/${revision}`;
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      void listMediaLibrary(
        {
          scope: "personal",
          kind,
          search: search || undefined,
          favorite_only: filter === "favorite" || undefined,
          resolutions:
            filter === "hd" && kind === "image" ? "2048,4096,8192" : undefined,
          catalog_state: "active",
          order: "updated_desc",
          page,
          page_size: 24,
        },
        { signal: controller.signal },
      )
        .then((r) => {
          const p = requireLibraryPage(r, owner.actorId);
          if (!controller.signal.aborted)
            setResult({ key, items: p.items, total: p.total, error: "" });
        })
        .catch((e) => {
          if (!controller.signal.aborted)
            setResult({ key, items: [], total: 0, error: mediaError(e) });
        });
    }, 200);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [owner.actorId, owner.scopeId, kind, search, filter, page, revision, key]);
  const ready = result?.key === key;
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !confirming) onClose();
        }}
      >
        <DialogContent
          className="flex max-h-[90dvh] min-h-96 flex-col sm:max-w-4xl"
          showCloseButton={!confirming}
        >
          <DialogHeader>
            <DialogTitle>资产选取</DialogTitle>
          </DialogHeader>
          <Tabs
            value={kind}
            onValueChange={(value) => {
              setKind(value);
              setFilter("all");
              setPage(1);
            }}
          >
            <TabsList>
              {allowedKinds.map((k) => (
                <TabsTrigger key={k} value={k}>
                  {kindLabels[k] ?? k}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <div className="flex flex-wrap gap-2">
            <Button
              variant={filter === "all" ? "secondary" : "ghost"}
              onClick={() => {
                setFilter("all");
                setPage(1);
              }}
            >
              所有{kindLabels[kind]}
            </Button>
            {kind === "image" && (
              <Button
                variant={filter === "hd" ? "secondary" : "ghost"}
                onClick={() => {
                  setFilter("hd");
                  setPage(1);
                }}
              >
                超清
              </Button>
            )}
            <Button
              variant={filter === "favorite" ? "secondary" : "ghost"}
              onClick={() => {
                setFilter("favorite");
                setPage(1);
              }}
            >
              收藏
            </Button>
            <Button
              className="ml-auto"
              variant="outline"
              onClick={() => setUpload(true)}
            >
              <Upload />
              从设备上传
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <Search aria-hidden="true" className="size-4" />
            <Input
              aria-label="搜索参考素材"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(1);
              }}
            />
          </div>
          <div className="min-h-52 flex-1 overflow-y-auto">
            {!ready ? (
              <ContentState state="loading" title="正在读取素材" />
            ) : result.error ? (
              <ContentState
                state="error"
                title={result.error}
                actions={
                  <Button onClick={() => setRevision((n) => n + 1)}>
                    重试
                  </Button>
                }
              />
            ) : !result.items.length ? (
              <ContentState title="没有符合条件的素材" />
            ) : (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                {result.items.filter(isPersonalReferenceItem).map((item) => {
                  const checked = selection.some((i) => i.id === item.id);
                  return (
                    <FieldLabel
                      key={item.id}
                      className="relative flex cursor-pointer flex-col gap-2 rounded-xl border p-2 has-[[data-state=checked]]:border-primary"
                    >
                      <div className="relative aspect-square overflow-hidden rounded-lg bg-muted">
                        <PersonalMediaPreview
                          itemID={item.id}
                          assetID={item.asset_id}
                          title={item.title}
                          kind={item.kind}
                        />
                      </div>
                      <span className="truncate text-sm">{item.title}</span>
                      <Checkbox
                        aria-label={`选择 · ${item.title}`}
                        checked={checked}
                        disabled={!checked && selection.length >= maxSelection}
                        className="absolute top-3 left-3"
                        onCheckedChange={(value) =>
                          setSelection((items) =>
                            value === true
                              ? [...items.filter((i) => i.id !== item.id), item]
                              : items.filter((i) => i.id !== item.id),
                          )
                        }
                      />
                    </FieldLabel>
                  );
                })}
              </div>
            )}
          </div>
          {error && <FeedbackToast message={error} />}
          <div className="flex items-center justify-between text-sm">
            <Button
              variant="ghost"
              disabled={page === 1 || !ready}
              onClick={() => setPage((p) => p - 1)}
            >
              上一页
            </Button>
            <span>第{page}页</span>
            <Button
              variant="ghost"
              disabled={!ready || page * 24 >= (result?.total ?? 0)}
              onClick={() => setPage((p) => p + 1)}
            >
              下一页
            </Button>
          </div>
          <DialogFooter>
            <span
              role="status"
              className="mr-auto text-sm text-muted-foreground"
            >
              已选 {selection.length} 项，最多 {maxSelection} 项
            </span>
            <Button variant="secondary" disabled={confirming} onClick={onClose}>
              取消
            </Button>
            <Button
              disabled={!selection.length || confirming}
              onClick={async () => {
                setConfirming(true);
                setError("");
                try {
                  await onSelect(selection);
                  onClose();
                } catch (e) {
                  setError(mediaError(e));
                } finally {
                  setConfirming(false);
                }
              }}
            >
              确认
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {upload && (
        <UploadDialog
          acceptedExtensions={allowedKinds.flatMap((k) =>
            k === "image"
              ? ["png", "jpg", "jpeg", "webp"]
              : k === "video"
                ? ["mp4", "mov", "webm"]
                : k === "document"
                  ? ["txt", "docx"]
                  : [],
          )}
          onClose={() => setUpload(false)}
          onUploaded={(asset) => {
            if (!asset || !allowedKinds.includes(asset.kind ?? ""))
              throw new Error("文件类型不符合本次参考要求。");
            setKind(asset.kind!);
            setFilter("all");
            setSearch(asset.file_name ?? "");
            setPage(1);
            setRevision((n) => n + 1);
            setUpload(false);
          }}
        />
      )}
    </>
  );
}
