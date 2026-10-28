"use client";
import { useEffect, useRef, useState } from "react";
import { listSubjectSources, getSubjectSource } from "@/api/subjects";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ContentState } from "@/components/layout/content-state";
import {
  DataPagination,
  type PageSize,
} from "@/components/media/data-pagination";
import {
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import { UnifiedSubjectMedia } from "./unified-media";
import { subjectError, type SelectedSubjectMedia } from "./unified-contracts";
import {
  requireSubjectSourcePage,
  requireSubjectSourceItem,
  type SubjectSourcePage,
  type SubjectSourceItem,
} from "./unified-source-contracts";

export function UnifiedSubjectSourcePicker({
  actorID,
  scopeID,
  allowedKinds,
  onClose,
  onSelect,
}: {
  actorID: string;
  scopeID: string;
  allowedKinds: readonly ("image" | "video" | "audio")[];
  onClose: () => void;
  onSelect: (value: SelectedSubjectMedia) => string | void;
}) {
  const [kind, setKind] = useState(allowedKinds[0]);
  const [group, setGroup] = useState("all");
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(1);
  const [pageSize, setPageSize] = useState<PageSize>(12);
  const [page, setPage] = useState<SubjectSourcePage | null>(null);
  const [selected, setSelected] = useState<SubjectSourceItem | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [retry, setRetry] = useState(0);
  const flight = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    listSubjectSources(
      {
        kind,
        search: query,
        favorite_only: group === "favorite" || undefined,
        quality: group === "upscaled" ? "upscaled" : undefined,
        page: index,
        page_size: pageSize,
      },
      { signal: controller.signal },
    )
      .then((v) => requireSubjectSourcePage(v, actorID, scopeID, kind))
      .then((v) => {
        if (!controller.signal.aborted) {
          setPage(v);
          setError("");
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(subjectError(e));
      });
    return () => controller.abort();
  }, [actorID, scopeID, kind, group, query, index, pageSize, retry]);
  useEffect(() => () => flight.current?.abort(), []);
  function reset() {
    setIndex(1);
    setPage(null);
    setSelected(null);
    setError("");
  }
  function changeKind(value: string) {
    if (!allowedKinds.includes(value as typeof kind)) return;
    setKind(value as typeof kind);
    setGroup("all");
    reset();
  }
  async function confirm() {
    if (!selected || pending) return;
    setPending(true);
    setError("");
    const controller = new AbortController();
    flight.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    try {
      const detail = requireSubjectSourceItem(
        await getSubjectSource(
          { item_id: selected.id },
          { signal: controller.signal },
        ),
      );
      const m = detail.media;
      if (
        detail.id !== selected.id ||
        detail.revision !== selected.revision ||
        detail.source !== selected.source ||
        detail.project_id !== selected.project_id ||
        m?.kind !== selected.kind ||
        m.id !== selected.media.id ||
        m.revision !== selected.media.revision ||
        !allowedKinds.includes(m!.kind as typeof kind) ||
        !m?.mime_type ||
        typeof m.file_name !== "string"
      )
        throw new RequestError("invalid_receipt");
      if (
        !controller.signal.aborted &&
        scope === getScopeGeneration() &&
        session === getSessionGeneration()
      ) {
        const issue = onSelect({
          asset_id: m.id!,
          asset_revision: m.revision!,
          kind: m.kind as typeof kind,
          file_name: m.file_name,
          mime_type: m.mime_type,
          library_item_id: selected.id,
          library_source: "subject",
          ...(detail.project_id ? { project_id: detail.project_id } : {}),
        });
        if (issue) setError(issue);
      }
    } catch (e) {
      if (!controller.signal.aborted) setError(subjectError(e));
    } finally {
      if (!controller.signal.aborted) setPending(false);
    }
  }
  const mediaLabel =
    kind === "image" ? "图片" : kind === "video" ? "视频" : "音频";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="unified-subject-source max-h-[calc(100dvh-32px)] w-[calc(100vw-32px)] overflow-hidden sm:max-w-[800px]"
      >
        <DialogTitle className="sr-only">从资产添加</DialogTitle>
        <DialogDescription className="sr-only">
          选择本人有权使用的参考资产；无需切换当前创作空间。
        </DialogDescription>
        <div className="flex items-center justify-between gap-2">
          <Tabs value={kind} onValueChange={changeKind}>
            <TabsList variant="line" aria-label="资产类型">
              {allowedKinds.map((k) => (
                <TabsTrigger
                  key={k}
                  value={k}
                  disabled={pending}
                  className="min-w-20"
                >
                  {k === "image" ? "图片" : k === "video" ? "视频" : "音频"}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <Button
            type="button"
            variant="ghost"
            disabled={pending}
            onClick={onClose}
          >
            关闭
          </Button>
        </div>
        <Tabs
          value={group}
          onValueChange={(value) => {
            setGroup(value);
            reset();
          }}
        >
          <TabsList variant="line" aria-label={`${mediaLabel}分组`}>
            <TabsTrigger value="all" disabled={pending}>
              所有{mediaLabel}
            </TabsTrigger>
            {kind === "image" ? (
              <TabsTrigger
                value="upscaled"
                disabled={pending || !page?.capabilities.upscaled_filter}
              >
                超清
              </TabsTrigger>
            ) : null}
            <TabsTrigger value="favorite" disabled={pending}>
              收藏
            </TabsTrigger>
          </TabsList>
        </Tabs>
        {kind === "image" && page && !page.capabilities.upscaled_filter ? (
          <p className="text-xs text-muted-foreground">暂无可用的超清来源。</p>
        ) : null}
        <Input
          aria-label="搜索资产"
          placeholder="搜索资产"
          value={query}
          disabled={pending}
          onChange={(e) => {
            setQuery(e.target.value);
            reset();
          }}
        />
        <div className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain">
          {error ? (
            <ContentState
              state="error"
              title="暂时无法读取资产"
              description={error}
              actions={
                <Button
                  variant="outline"
                  onClick={() => {
                    setPage(null);
                    setError("");
                    setRetry((n) => n + 1);
                  }}
                >
                  重试
                </Button>
              }
            />
          ) : !page ? (
            <ContentState state="loading" title="正在读取资产" />
          ) : !page.items.length ? (
            <ContentState title="暂未找到相关内容" />
          ) : (
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
              {page.items.map((item) => (
                <Button
                  key={item.id}
                  variant="ghost"
                  className="h-auto min-w-0 flex-col items-stretch p-0"
                  aria-pressed={selected?.id === item.id}
                  onClick={() => setSelected(item)}
                  disabled={pending}
                >
                  <span className="relative block aspect-[275/180] w-full overflow-hidden rounded-lg bg-muted">
                    <UnifiedSubjectMedia
                      media={{
                        asset_id: item.media.id,
                        asset_revision: item.media.revision,
                        kind: item.kind as SelectedSubjectMedia["kind"],
                        file_name: item.media.file_name,
                        mime_type: item.media.mime_type,
                        library_item_id: item.id,
                        library_source: "subject",
                      }}
                      label={item.title}
                    />
                  </span>
                  <span className="w-full truncate px-1 pb-2 text-left">
                    {item.title}
                  </span>
                </Button>
              ))}
            </div>
          )}
        </div>
        {page && page.total > pageSize ? (
          <DataPagination
            label="主体资产分页"
            page={index}
            pageSize={pageSize}
            total={page.total}
            disabled={pending}
            onPageChange={(n) => {
              setIndex(n);
              setPage(null);
              setSelected(null);
            }}
            onPageSizeChange={(size) => {
              setPageSize(size);
              reset();
            }}
          />
        ) : null}
        <DialogFooter>
          <span className="mr-auto text-sm text-muted-foreground">
            已选 {selected ? 1 : 0}{" "}
            {kind === "image"
              ? "张图片"
              : kind === "video"
                ? "个视频"
                : "个音频"}
          </span>
          <Button
            disabled={!selected || pending}
            onClick={() => void confirm()}
          >
            {pending ? "正在核验…" : "确认"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
