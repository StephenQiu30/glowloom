"use client";

import { useEffect, useRef, useState } from "react";
import { listMediaLibrary } from "@/api/media";
import { ContentState } from "@/components/layout/content-state";
import {
  mediaError,
  requireLibraryPage,
  type LibraryItem,
} from "@/components/media/library";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import type { MediaKind } from "./catalog";

const titles = { image: "图片", video: "视频" };
const pageSize = 12;

export function ToolAssetPicker({
  media,
  actorID,
  onConfirm,
}: {
  media: readonly MediaKind[];
  actorID: string;
  onConfirm: (asset: LibraryItem, kind: MediaKind) => Promise<void>;
}) {
  const [kind, setKind] = useState(media[0]);
  const [search, setSearch] = useState("");
  const [recent, setRecent] = useState(false);
  const [favorite, setFavorite] = useState(false);
  const [page, setPage] = useState(1);
  const [retry, setRetry] = useState(0);
  const [items, setItems] = useState<LibraryItem[]>([]);
  const [total, setTotal] = useState(0);
  const [selected, setSelected] = useState<LibraryItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [confirming, setConfirming] = useState(false);
  const confirmation = useRef(false);
  const [error, setError] = useState("");
  const [confirmError, setConfirmError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    const current = () =>
      !controller.signal.aborted &&
      scope === getScopeGeneration() &&
      session === getSessionGeneration();
    const timer = setTimeout(
      () => {
        void listMediaLibrary(
          {
            scope: "personal",
            kind,
            search: search.trim() || undefined,
            recent_only: recent || undefined,
            favorite_only: favorite || undefined,
            catalog_state: "active",
            page,
            page_size: pageSize,
          },
          { signal: controller.signal },
        )
          .then((value) => {
            const result = requireLibraryPage(value, actorID);
            if (!current()) return;
            setItems(
              result.items.filter(
                (item) =>
                  item.kind === kind &&
                  item.catalog_state === "active" &&
                  item.asset_id &&
                  item.media,
              ),
            );
            setTotal(result.total);
          })
          .catch((reason) => {
            if (current()) setError(mediaError(reason));
          })
          .finally(() => {
            if (current()) setLoading(false);
          });
      },
      search.trim() ? 250 : 0,
    );
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [actorID, kind, search, recent, favorite, page, retry]);

  function reset(nextPage = 1) {
    setSelected(null);
    setItems([]);
    setError("");
    setConfirmError("");
    setLoading(true);
    setPage(nextPage);
  }

  async function confirm() {
    if (!selected || confirmation.current) return;
    confirmation.current = true;
    setConfirming(true);
    setConfirmError("");
    try {
      await onConfirm(selected, kind);
    } catch (reason) {
      setConfirmError(
        reason instanceof Error ? reason.message : "无法读取所选素材。",
      );
    } finally {
      confirmation.current = false;
      setConfirming(false);
    }
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <Tabs
        value={kind}
        onValueChange={(value) => {
          const next = media.find((kind) => kind === value);
          if (!next || next === kind || confirming) return;
          reset();
          setKind(next);
        }}
      >
        <TabsList variant="pill" aria-label="类型">
          {media.map((value) => (
            <TabsTrigger key={value} value={value} disabled={confirming}>
              {titles[value]}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <div className="flex flex-wrap gap-2">
        <Input
          type="search"
          aria-label="搜索"
          placeholder="搜索"
          value={search}
          disabled={confirming}
          className="min-w-28 flex-1"
          onChange={(event) => {
            reset();
            setSearch(event.target.value);
          }}
        />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="secondary" disabled={confirming}>
              时间
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuGroup>
              <DropdownMenuCheckboxItem
                checked={recent}
                onCheckedChange={(checked) => {
                  reset();
                  setRecent(checked === true);
                }}
              >
                最近 30 天
              </DropdownMenuCheckboxItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="secondary" disabled={confirming}>
              筛选
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuGroup>
              <DropdownMenuCheckboxItem
                checked={favorite}
                onCheckedChange={(checked) => {
                  reset();
                  setFavorite(checked === true);
                }}
              >
                仅收藏
              </DropdownMenuCheckboxItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {loading ? (
        <ContentState state="loading" title="正在读取素材" />
      ) : error ? (
        <ContentState
          state="error"
          title="素材暂时无法读取"
          description={error}
          actions={
            <Button
              size="sm"
              onClick={() => {
                reset(page);
                setRetry((value) => value + 1);
              }}
            >
              重试
            </Button>
          }
        />
      ) : items.length ? (
        <ul className="grid max-h-52 grid-cols-2 gap-2 overflow-y-auto">
          {items.map((item) => (
            <li key={item.id} className="min-w-0">
              <Button
                variant={selected?.id === item.id ? "secondary" : "outline"}
                aria-pressed={selected?.id === item.id}
                disabled={confirming}
                onClick={() => {
                  setSelected(selected?.id === item.id ? null : item);
                  setConfirmError("");
                }}
                title={item.title}
                className="w-full justify-start"
              >
                <span className="truncate">{item.title}</span>
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <ContentState
          state="empty"
          title={search.trim() ? "没有匹配的素材" : `暂无${titles[kind]}素材`}
        />
      )}
      {total > pageSize && !loading && !error && (
        <div className="flex justify-end gap-2">
          <Button
            variant="secondary"
            size="sm"
            disabled={confirming || page === 1}
            onClick={() => reset(page - 1)}
          >
            上一页
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={confirming || page * pageSize >= total}
            onClick={() => reset(page + 1)}
          >
            下一页
          </Button>
        </div>
      )}
      {confirmError && (
        <p role="alert" className="text-sm text-destructive">
          {confirmError}
        </p>
      )}
      <div className="flex items-center justify-between gap-2">
        <span role="status" className="text-sm text-muted-foreground">
          已选择 {selected ? 1 : 0} 个素材
        </span>
        <Button
          disabled={!selected || confirming}
          onClick={() => void confirm()}
        >
          {confirming ? "正在读取" : "确认"}
        </Button>
      </div>
    </div>
  );
}
