"use client";

import { cn } from "cn";
import { useEffect, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useQuery } from "@tanstack/react-query";
import { FileText, ImageIcon, Music2, Box, Film, Star } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  useMediaLifecycle,
  useReleaseMediaSource,
} from "@/components/canvas/nodes/media-lifecycle";
import {
  categoryLabels,
  kindLabels,
  type LibraryIdentity,
  type LibraryItem,
} from "./library-model";
import { freshLibrary, libraryKey, previewLibrary } from "./library-queries";

type Props = {
  identity: LibraryIdentity;
  items: LibraryItem[];
  view: string;
  selection: { id: string; revision: number }[];
  disabled: boolean;
  readOnly: boolean;
  onToggle: (id: string, revision: number) => void;
  onView: (id: string) => void;
  onEdit: (id: string) => void;
  onCopyText: (id: string) => void;
};
export function LibraryItems(props: Props) {
  const scroll = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual拥有DOM测量回调；此组件不手动memo。
  const virtual = useVirtualizer({
    count: props.items.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 124,
    getItemKey: (index) => props.items[index].id,
    overscan: 5,
    enabled: props.view === "list" && props.items.length > 100,
  });
  if (props.view === "list" && props.items.length > 100)
    return (
      <div
        ref={scroll}
        className="h-[65dvh] min-h-96 overflow-auto"
        aria-label="素材列表"
      >
        <div className="relative" style={{ height: virtual.getTotalSize() }}>
          {virtual.getVirtualItems().map((row) => (
            <div
              key={row.key}
              data-index={row.index}
              ref={virtual.measureElement}
              className="absolute top-0 left-0 w-full pb-3"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              <ItemCard {...props} item={props.items[row.index]} />
            </div>
          ))}
        </div>
      </div>
    );
  return (
    <div
      className={
        props.view === "list"
          ? "flex flex-col gap-3"
          : "grid grid-cols-2 gap-x-5 gap-y-6 lg:grid-cols-3 xl:grid-cols-4"
      }
      aria-label="素材列表"
    >
      {props.items.map((item) => (
        <ItemCard key={item.id} {...props} item={item} />
      ))}
    </div>
  );
}
function ItemCard({ item, ...props }: Props & { item: LibraryItem }) {
  return (
    <Card size="sm" variant={props.view === "grid" ? "project" : "default"}>
      <CardHeader className={props.view === "grid" ? "order-2" : undefined}>
        <div className="flex min-w-0 items-start gap-3">
          <Checkbox
            aria-label={`选择 ${item.title}`}
            checked={props.selection.some(
              (selected) => selected.id === item.id,
            )}
            disabled={props.disabled}
            onCheckedChange={() => props.onToggle(item.id, item.revision)}
          />
          <CardTitle className="min-w-0 flex-1 break-words">
            {item.title}
          </CardTitle>
          {item.favorite && (
            <Star className="size-4 shrink-0" aria-label="已收藏" />
          )}
        </div>
        <div className="min-w-0 text-sm text-muted-foreground">
          <p>
            {kindLabels[item.kind]} · {categoryLabels[item.category]}
          </p>
          <p className="break-words">{item.tags.join(" · ")}</p>
          {item.media && (
            <p>
              {formatBytes(item.media.byte_size)}
              {item.media.duration_ms
                ? ` · ${(item.media.duration_ms / 1000).toFixed(1)} 秒`
                : ""}
            </p>
          )}
          <p>修改于 {new Date(item.updated_at).toLocaleString("zh-CN")}</p>
        </div>
      </CardHeader>
      <CardContent className={props.view === "grid" ? "order-1" : undefined}>
        <div
          className={
            props.view === "list"
              ? "flex items-center gap-4"
              : "flex flex-col gap-3"
          }
        >
          <ItemThumbnail
            identity={props.identity}
            item={item}
            compact={props.view === "list"}
          />
        </div>
      </CardContent>
      <CardFooter className="order-3 flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={props.disabled}
          onClick={() => props.onView(item.id)}
          aria-label={`查看 ${item.title}`}
        >
          查看
        </Button>
        {!props.readOnly && item.catalog_state === "active" && (
          <Button
            variant="ghost"
            disabled={props.disabled}
            onClick={() => props.onEdit(item.id)}
            aria-label={`编辑 ${item.title}`}
          >
            编辑
          </Button>
        )}
        {item.kind === "text" && (
          <Button
            variant="ghost"
            disabled={props.disabled}
            onClick={() => props.onCopyText(item.id)}
            aria-label={`复制正文 ${item.title}`}
          >
            复制正文
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}
function ItemThumbnail({
  identity,
  item,
  compact,
}: {
  identity: LibraryIdentity;
  item: LibraryItem;
  compact: boolean;
}) {
  const root = useRef<HTMLDivElement>(null),
    image = useRef<HTMLImageElement>(null);
  const visible = useMediaLifecycle(root, true, false);
  const previewable = Boolean(
    item.media && ["image", "video", "audio"].includes(item.kind),
  );
  const preview = useQuery({
    queryKey: [
      ...libraryKey(identity),
      "thumbnail",
      item.id,
      item.media?.revision,
    ],
    queryFn: async ({ signal }) => {
      await freshLibrary(identity, signal);
      return previewLibrary(identity, item.media!, signal);
    },
    enabled: visible && previewable,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const lease = preview.data?.renditions.find((rendition) =>
    ["thumb_256", "poster", "waveform"].includes(rendition.kind),
  );
  const url = visible && lease ? lease.url : "";
  const Icon = {
    image: ImageIcon,
    video: Film,
    audio: Music2,
    model: Box,
    document: FileText,
    text: FileText,
  }[item.kind];
  return (
    <div
      ref={root}
      className={cn(
        "flex items-center justify-center overflow-hidden rounded-lg bg-surface-3",
        compact ? "size-16 shrink-0" : "aspect-square w-full",
      )}
    >
      {url ? (
        <ImageSource
          key={url}
          refObject={image}
          url={url}
          title={item.title}
          expiresAt={lease!.expires_at}
        />
      ) : (
        <Icon
          className="size-8 text-muted-foreground"
          aria-label={kindLabels[item.kind]}
        />
      )}
    </div>
  );
}
function ImageSource({
  refObject,
  url,
  title,
  expiresAt,
}: {
  refObject: React.RefObject<HTMLImageElement | null>;
  url: string;
  title: string;
  expiresAt: string;
}) {
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    const timer = setTimeout(
      () => setExpired(true),
      Math.max(0, Date.parse(expiresAt) - Date.now() - 5000),
    );
    return () => clearTimeout(timer);
  }, [expiresAt]);
  useReleaseMediaSource(refObject, expired ? "" : url);
  if (expired)
    return (
      <ImageIcon
        className="size-8 text-muted-foreground"
        aria-label="缩略图已过期"
      />
    );
  return (
    // eslint-disable-next-line @next/next/no-img-element -- 私有短期授权缩略图不进入公开 Next 图片缓存。
    <img
      ref={refObject}
      src={url}
      alt={title}
      crossOrigin="anonymous"
      loading="lazy"
      onError={() => setExpired(true)}
      decoding="async"
      className="size-full object-contain"
    />
  );
}
export function formatBytes(bytes: number) {
  return bytes < 1024 * 1024
    ? `${(bytes / 1024).toFixed(1)} KiB`
    : `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}
