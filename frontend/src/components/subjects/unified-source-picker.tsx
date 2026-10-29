"use client";
import { useEffect, useRef, useState } from "react";
import { Check } from "lucide-react";
import { getSubjectSource } from "@/api/subjects";
import { Button } from "@/components/ui/button";
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
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import { UnifiedSubjectMedia } from "./unified-media";
import { subjectError, type SelectedSubjectMedia } from "./unified-contracts";
import {
  requireSubjectSourceItem,
  type SubjectSourceItem,
} from "./unified-source-contracts";
import { useSubjectSources } from "./use-subject-sources";

export function UnifiedSubjectSourcePicker({
  actorID,
  scopeID,
  allowedKinds,
  onClose,
  onSelect,
  onSelectMany,
  maxSelection = 120,
}: {
  actorID: string;
  scopeID: string;
  allowedKinds: readonly ("image" | "video" | "audio")[];
  onClose: () => void;
  onSelect: (value: SelectedSubjectMedia) => string | void;
  onSelectMany?: (values: SelectedSubjectMedia[]) => string | void;
  // backend/internal/subject/domain/content.go: MaxReferences=120; a primary batch may add one primary.
  maxSelection?: number;
}) {
  const [kind, setKind] = useState(allowedKinds[0]);
  const [group, setGroup] = useState("all");
  const owner = JSON.stringify([actorID, scopeID]);
  const [selection, setSelection] = useState({
    owner,
    kind,
    items: [] as SubjectSourceItem[],
  });
  const selected =
    selection.owner === owner && selection.kind === kind ? selection.items : [];
  const [issue, setIssue] = useState({ owner, message: "" });
  const error = issue.owner === owner ? issue.message : "";
  const [pendingOwner, setPendingOwner] = useState<string | null>(null);
  if (selection.owner !== owner) setSelection({ owner, kind, items: [] });
  if (issue.owner !== owner) setIssue({ owner, message: "" });
  if (pendingOwner !== null && pendingOwner !== owner) setPendingOwner(null);
  const pending = pendingOwner === owner;
  const flight = useRef<AbortController | null>(null);
  const scrollRoot = useRef<HTMLDivElement>(null);
  const sentinel = useRef<HTMLDivElement>(null);
  const sources = useSubjectSources(actorID, scopeID, kind, group);
  const { page, loadMore } = sources;
  const multiple = kind === "image" && !!onSelectMany;
  const limit = multiple
    ? Math.max(0, Math.min(121, Math.floor(maxSelection)))
    : 1;
  const selectedIDs = new Set(selected.map((item) => item.id));

  useEffect(
    () => () => {
      flight.current?.abort();
      flight.current = null;
    },
    [actorID, scopeID],
  );
  useEffect(() => {
    if (
      !scrollRoot.current ||
      !sentinel.current ||
      typeof IntersectionObserver === "undefined"
    )
      return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (!pending && entries.some((entry) => entry.isIntersecting))
          loadMore();
      },
      {
        root: scrollRoot.current,
        rootMargin: `0px 0px ${scrollRoot.current.clientHeight}px 0px`,
      },
    );
    observer.observe(sentinel.current);
    return () => observer.disconnect();
  }, [loadMore, pending, page, group, kind]);

  function changeKind(value: string) {
    if (!allowedKinds.includes(value as typeof kind)) return;
    setKind(value as typeof kind);
    setGroup("all");
    setSelection({ owner, kind: value as typeof kind, items: [] });
    setIssue({ owner, message: "" });
  }
  function select(item: SubjectSourceItem) {
    setIssue({ owner, message: "" });
    setSelection((previous) => {
      const items =
        previous.owner === owner && previous.kind === kind
          ? previous.items
          : [];
      if (items.some((value) => value.id === item.id))
        return {
          owner,
          kind,
          items: items.filter((value) => value.id !== item.id),
        };
      if (!multiple) return { owner, kind, items: [item] };
      return items.length < limit
        ? { owner, kind, items: [...items, item] }
        : previous;
    });
  }
  async function confirm() {
    if (
      !selected.length ||
      selected.length > limit ||
      pending ||
      flight.current
    )
      return;
    setPendingOwner(owner);
    setIssue({ owner, message: "" });
    const controller = new AbortController();
    flight.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    try {
      const values = await Promise.all(
        selected.map(async (item): Promise<SelectedSubjectMedia> => {
          const detail = requireSubjectSourceItem(
            await getSubjectSource(
              { item_id: item.id },
              { signal: controller.signal },
            ),
          );
          const m = detail.media;
          if (
            detail.id !== item.id ||
            detail.revision !== item.revision ||
            detail.source !== item.source ||
            detail.project_id !== item.project_id ||
            m.kind !== item.kind ||
            m.id !== item.media.id ||
            m.revision !== item.media.revision ||
            !allowedKinds.includes(m.kind as typeof kind)
          )
            throw new RequestError("invalid_receipt");
          return {
            asset_id: m.id,
            asset_revision: m.revision,
            kind: detail.kind,
            file_name: m.file_name,
            mime_type: m.mime_type,
            library_item_id: item.id,
            library_source: "subject",
            ...(detail.project_id ? { project_id: detail.project_id } : {}),
          };
        }),
      );
      if (
        !controller.signal.aborted &&
        scope === getScopeGeneration() &&
        session === getSessionGeneration()
      ) {
        const message =
          multiple && onSelectMany ? onSelectMany(values) : onSelect(values[0]);
        if (message) setIssue({ owner, message });
      }
    } catch (reason) {
      if (!controller.signal.aborted)
        setIssue({ owner, message: subjectError(reason) });
    } finally {
      // A failed item cancels any still-running sibling checks; no partial selection is adopted.
      controller.abort();
      if (flight.current === controller) {
        flight.current = null;
        setPendingOwner(null);
      }
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
        onInteractOutside={(event) => event.preventDefault()}
        className="unified-subject-source h-[min(680px,calc(100dvh-32px))] w-[calc(100vw-32px)] overflow-hidden! sm:max-w-[848px]"
      >
        <DialogTitle className="sr-only">从资产添加</DialogTitle>
        <DialogDescription className="sr-only">
          选择参考资产后确认添加。
        </DialogDescription>
        <div className="flex shrink-0 items-center justify-between gap-2">
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
          className="shrink-0"
          onValueChange={(value) => {
            setGroup(value);
            setIssue({ owner, message: "" });
          }}
        >
          <TabsList variant="line" aria-label={`${mediaLabel}分组`}>
            <TabsTrigger value="all" disabled={pending}>
              所有{mediaLabel}
            </TabsTrigger>
            {kind === "image" ? (
              <TabsTrigger value="upscaled" disabled={pending}>
                超清
              </TabsTrigger>
            ) : null}
            <TabsTrigger value="favorite" disabled={pending}>
              收藏
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <div
          key={`${owner}:${kind}:${group}`}
          ref={scrollRoot}
          role="region"
          aria-label="资产列表"
          className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain"
          onScroll={(event) => {
            const element = event.currentTarget;
            if (
              !pending &&
              element.scrollHeight - element.scrollTop <
                2 * element.clientHeight
            )
              loadMore();
          }}
        >
          {error ? (
            <ContentState
              state="error"
              title="暂时无法读取资产"
              description={error}
            />
          ) : null}
          {!page && !sources.error ? (
            <ContentState state="loading" title="正在读取资产" />
          ) : null}
          {page && !page.items.length && !sources.error ? (
            <ContentState title="暂未找到相关内容" />
          ) : null}
          {page?.items.length ? (
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
              {page.items.map((item) => (
                <Button
                  key={item.id}
                  variant="ghost"
                  className="h-auto min-w-0 flex-col items-stretch gap-1 p-0"
                  aria-label={item.title}
                  aria-pressed={selectedIDs.has(item.id)}
                  onClick={() => select(item)}
                  disabled={
                    pending ||
                    (multiple &&
                      !selectedIDs.has(item.id) &&
                      selected.length >= limit)
                  }
                >
                  <span
                    className={`relative block w-full overflow-hidden rounded-lg bg-muted ${kind === "video" ? "aspect-video" : "aspect-square"}`}
                  >
                    <UnifiedSubjectMedia
                      media={{
                        asset_id: item.media.id,
                        asset_revision: item.media.revision,
                        kind: item.kind,
                        file_name: item.media.file_name,
                        mime_type: item.media.mime_type,
                        library_item_id: item.id,
                        library_source: "subject",
                      }}
                      label={item.title}
                    />
                    <span
                      aria-hidden="true"
                      className={`absolute top-2 right-2 flex size-5 items-center justify-center rounded border ${selectedIDs.has(item.id) ? "border-primary bg-primary text-primary-foreground" : "border-white/60 bg-black/30"}`}
                    >
                      {selectedIDs.has(item.id) ? (
                        <Check className="size-4" />
                      ) : null}
                    </span>
                  </span>
                </Button>
              ))}
            </div>
          ) : null}
          {sources.error ? (
            <ContentState
              state="error"
              title="暂时无法读取资产"
              description={sources.error}
              actions={
                <Button
                  variant="outline"
                  disabled={pending}
                  onClick={sources.retry}
                >
                  重试
                </Button>
              }
            />
          ) : null}
          {page && sources.loading ? (
            <p
              role="status"
              className="py-4 text-center text-sm text-muted-foreground"
            >
              正在读取资产…
            </p>
          ) : null}
          <div ref={sentinel} aria-hidden="true" className="h-px" />
        </div>
        <DialogFooter className="shrink-0">
          <span className="mr-auto text-sm text-muted-foreground">
            已选 {selected.length}{" "}
            {kind === "image"
              ? "张图片"
              : kind === "video"
                ? "个视频"
                : "个音频"}
          </span>
          <Button
            disabled={!selected.length || selected.length > limit || pending}
            onClick={() => void confirm()}
          >
            {pending ? "正在核验…" : "确认"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
