"use client";
import { useEffect, useState } from "react";
import { listAssetCanvases } from "@/api/canvases";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ContentState } from "@/components/layout/content-state";
import { UnifiedSubjectMedia } from "@/components/subjects/unified-media";
import { useSubjectList } from "@/components/subjects/use-subject-list";
import {
  UnifiedSubjectToolbar,
  defaultSubjectFilter,
} from "@/components/subjects/unified-toolbar";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import {
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import { isProjectID } from "@/components/projects/project";
import {
  defaultProjectPickerFilter,
  projectPickerQuery,
} from "./project-picker";
export function ProjectPickerSubjects({
  actorID,
  scopeID,
  selected,
  onSelect,
}: {
  actorID: string;
  scopeID: string;
  selected?: string;
  onSelect: (subject: UnifiedSubject) => void;
}) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState(defaultSubjectFilter);
  const { page, error, loading, hasMore, loadMore, retry } = useSubjectList(
    actorID,
    scopeID,
    query,
    filter,
  );
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <Input
        aria-label="搜索空间主体"
        placeholder="搜索空间主体"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      <UnifiedSubjectToolbar
        value={filter}
        onChange={setFilter}
        disabled={false}
        onBatch={() => {}}
        showBatchAction={false}
      />
      <div className="min-h-0 overflow-auto" data-canvas-wheel-scroll>
        {error && (
          <ContentState
            state="error"
            title="主体暂时无法读取"
            description={error}
            actions={<Button onClick={retry}>重试</Button>}
          />
        )}
        <div className="canvas-material-grid">
          {page?.items.map((subject) => (
            <Button
              key={subject.id}
              variant="ghost"
              className="canvas-material-card h-auto flex-col items-stretch p-2 whitespace-normal"
              aria-label={`选择主体 ${subject.name}`}
              aria-pressed={selected === subject.id}
              onClick={() => onSelect(subject)}
            >
              <div className="canvas-material-preview">
                <UnifiedSubjectMedia
                  label={subject.name}
                  media={{
                    ...subject.primary,
                    bound: {
                      subjectID: subject.id,
                      versionID: subject.version_id,
                      bindingID: subject.primary.binding_id,
                    },
                  }}
                />
              </div>
              <span>{subject.name}</span>
            </Button>
          ))}
        </div>
        {loading && <p role="status">正在读取主体…</p>}
        {!loading && !error && !page?.items.length && (
          <ContentState
            state="empty"
            title="没有匹配的主体"
            description="可更换关键词或筛选条件。"
          />
        )}
        {hasMore && (
          <Button variant="ghost" disabled={loading} onClick={loadMore}>
            加载更多主体
          </Button>
        )}
      </div>
    </div>
  );
}
export function requirePickerCanvases(raw: unknown) {
  const value = raw as API.AssetListResponse | null;
  if (
    !value ||
    !Array.isArray(value.items) ||
    value.items.length > 30 ||
    (value.next_cursor != null &&
      (typeof value.next_cursor !== "string" ||
        !value.next_cursor ||
        value.next_cursor.length > 2048)) ||
    new Set(value.items.map((i) => i.id)).size !== value.items.length ||
    value.items.some(
      (i) =>
        !isProjectID(i.id) ||
        !isProjectID(i.project_id) ||
        i.kind !== "canvas" ||
        i.collection !== "current" ||
        typeof i.name !== "string" ||
        !i.name.trim() ||
        typeof i.favorite !== "boolean" ||
        !/^[1-9]\d*$/.test(i.revision ?? ""),
    )
  )
    throw new RequestError("invalid_receipt");
  return { items: value.items, next: value.next_cursor || undefined };
}
export function ProjectPickerCanvases() {
  const [query, setQuery] = useState("");
  const [favorite, setFavorite] = useState(false);
  const [from, setFrom] = useState("");
  const [through, setThrough] = useState("");
  const key = JSON.stringify([query, favorite, from, through]);
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <Input
        aria-label="搜索空间画布"
        placeholder="搜索空间画布"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      <div className="flex flex-wrap items-center gap-2">
        <Checkbox
          id="picker-canvas-favorite"
          checked={favorite}
          onCheckedChange={(v) => setFavorite(v === true)}
        />
        <Label htmlFor="picker-canvas-favorite">仅收藏</Label>
        <Label htmlFor="picker-canvas-from">开始日期</Label>
        <Input
          className="w-auto"
          id="picker-canvas-from"
          type="date"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        />
        <Label htmlFor="picker-canvas-through">结束日期</Label>
        <Input
          className="w-auto"
          id="picker-canvas-through"
          type="date"
          value={through}
          onChange={(e) => setThrough(e.target.value)}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        当前空间可访问的画布。整张画布添加尚未接入，暂不可选择。
      </p>
      <CanvasResults
        key={key}
        query={query}
        favorite={favorite}
        from={from}
        through={through}
      />
    </div>
  );
}
function CanvasResults({
  query,
  favorite,
  from,
  through,
}: {
  query: string;
  favorite: boolean;
  from: string;
  through: string;
}) {
  const [cursor, setCursor] = useState<string>();
  const [retry, setRetry] = useState(0);
  const [result, setResult] = useState<{
    items: API.AssetCanvas[];
    next?: string;
    error?: string;
    loaded?: string;
  }>({ items: [] });
  const requestKey = `${cursor ?? ""}/${retry}`;
  const loading = result.loaded !== requestKey;
  useEffect(() => {
    const controller = new AbortController();
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    const run = async () => {
      const range = projectPickerQuery("", "image", "", 1, {
        ...defaultProjectPickerFilter,
        from,
        through,
      });
      const response = requirePickerCanvases(
        await listAssetCanvases(
          {
            collection: "current",
            kind: "canvas",
            search: query.trim() || undefined,
            favorite_only: favorite || undefined,
            created_from: range.created_from,
            created_before: range.created_before,
            limit: 30,
            cursor,
          },
          { signal: controller.signal },
        ),
      );
      if (
        controller.signal.aborted ||
        scope !== getScopeGeneration() ||
        session !== getSessionGeneration()
      )
        return;
      if (cursor && response.next === cursor)
        throw new RequestError("invalid_receipt");
      setResult((previous) => ({
        items: [
          ...new Map(
            [...(cursor ? previous.items : []), ...response.items].map((i) => [
              i.id,
              i,
            ]),
          ).values(),
        ],
        next: response.next,
        loaded: requestKey,
      }));
    };
    const timer = setTimeout(
      () => {
        void run().catch(() => {
          if (!controller.signal.aborted)
            setResult((previous) => ({
              ...previous,
              error: "画布暂时无法读取，请重试。",
              loaded: requestKey,
            }));
        });
      },
      query ? 250 : 0,
    );
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [query, favorite, from, through, cursor, retry, requestKey]);
  return (
    <div className="min-h-0 overflow-auto" data-canvas-wheel-scroll>
      {result.items.map((canvas) => (
        <div key={canvas.id} className="rounded-md border p-3">
          <span>{canvas.name}</span>
          {canvas.favorite && (
            <span className="ml-2 text-xs text-muted-foreground">已收藏</span>
          )}
        </div>
      ))}
      {loading && <p role="status">正在读取画布…</p>}
      {result.error && (
        <ContentState
          state="error"
          title={result.error}
          actions={<Button onClick={() => setRetry((v) => v + 1)}>重试</Button>}
        />
      )}
      {!loading && !result.error && !result.items.length && (
        <ContentState state="empty" title="没有匹配的画布" />
      )}
      {result.next && (
        <Button
          disabled={loading}
          variant="ghost"
          onClick={() => setCursor(result.next)}
        >
          加载更多画布
        </Button>
      )}
    </div>
  );
}
