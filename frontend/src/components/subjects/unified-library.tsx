"use client";
import { useEffect, useRef, useState, type SetStateAction } from "react";
import dynamic from "next/dynamic";
import {
  Download,
  Star,
  LoaderCircle,
  MoreHorizontal,
  Plus,
  Trash2,
} from "lucide-react";
import { batchSubjects, exportSubjects } from "@/api/subjects";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { ContentState } from "@/components/layout/content-state";
import {
  RequestError,
  getScopeGeneration,
  getSessionGeneration,
} from "@/lib/request";
import { UnifiedSubjectMedia } from "./unified-media";
import {
  AppliedSubjectTimeRange,
  UnifiedSubjectToolbar,
  defaultSubjectFilter,
  type SubjectFilter,
} from "./unified-toolbar";
import {
  requireSubjectBatchResult,
  selectedSubjectMedia,
  subjectError,
  subjectBatchError,
  subjectRelativeTime,
  type UnifiedSubject,
  type SubjectSelection,
} from "./unified-contracts";
import "./unified-subjects.css";
import { useSubjectList } from "./use-subject-list";
import { PageToolbar } from "@/components/layout/page-toolbar";
const Editor = dynamic(() =>
  import("./unified-editor").then((m) => m.UnifiedSubjectEditor),
);
type Mutation = {
  key: string;
  body: { action: string; items: SubjectSelection[] };
  subjects: UnifiedSubject[];
};
export function UnifiedSubjectLibrary({
  actorID,
  scopeID,
  query = "",
  onSelect,
  toolbarInHeader = false,
}: {
  toolbarInHeader?: boolean;
  actorID: string;
  scopeID: string;
  query?: string;
  onSelect?: (value: UnifiedSubject) => void;
}) {
  const [filter, setFilter] = useState<SubjectFilter>(defaultSubjectFilter);
  const { page, error, loading, hasMore, loadMore, retry, refresh } =
    useSubjectList(actorID, scopeID, query, filter);
  const tail = useRef<HTMLDivElement>(null);
  const [batch, setBatch] = useState(false);
  const [selectionState, setSelectionState] = useState<{
    query: string;
    items: UnifiedSubject[];
  }>({ query, items: [] });
  const selected = selectionState.query === query ? selectionState.items : [];
  function setSelected(value: SetStateAction<UnifiedSubject[]>) {
    setSelectionState((previous) => ({
      query,
      items:
        typeof value === "function"
          ? value(previous.query === query ? previous.items : [])
          : value,
    }));
  }
  const [editing, setEditing] = useState<UnifiedSubject | "new" | null>(null);
  const [deleting, setDeleting] = useState<UnifiedSubject[] | null>(null);
  const [pending, setPending] = useState(false);
  const [writeError, setWriteError] = useState("");
  const [uncertain, setUncertain] = useState(false);
  const operation = useRef<Mutation | null>(null);
  const busy = useRef(false);
  const flight = useRef<AbortController | null>(null);
  const [exporting, setExporting] = useState(false);
  const exportBusy = useRef(false);
  const locked = pending || uncertain || exporting;
  useEffect(() => {
    if (!hasMore || loading || error || locked || !tail.current) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (
          entries.some(
            (entry) => entry.isIntersecting && entry.intersectionRatio >= 0.2,
          )
        )
          loadMore();
      },
      { threshold: 0.2 },
    );
    observer.observe(tail.current);
    return () => observer.disconnect();
  }, [hasMore, loading, error, locked, loadMore]);
  useEffect(() => () => flight.current?.abort(), []);
  async function mutate(action?: string, subjects?: UnifiedSubject[]) {
    if (busy.current || exportBusy.current) return;
    const op =
      operation.current ??
      (action && subjects?.length
        ? {
            key: crypto.randomUUID(),
            subjects,
            body: {
              action,
              items: subjects.map((s) => ({
                id: s.id,
                expected_revision: s.revision,
              })),
            },
          }
        : null);
    if (!op) return;
    operation.current = op;
    busy.current = true;
    setPending(true);
    setWriteError("");
    const controller = new AbortController();
    flight.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    try {
      const response = requireSubjectBatchResult(
        await batchSubjects(op.body, {
          headers: { "Idempotency-Key": op.key },
          signal: controller.signal,
        }),
        op.body.items,
      );
      if (
        controller.signal.aborted ||
        scope !== getScopeGeneration() ||
        session !== getSessionGeneration()
      )
        return;
      const failures = response.items.filter((i) => i.status !== "succeeded");
      const failedIDs = new Set(failures.map((i) => i.id));
      setSelected(op.subjects.filter((s) => failedIDs.has(s.id)));
      setWriteError(
        failures.length
          ? `${failures.length} 项未完成：${failures.map((i) => `${op.subjects.find((s) => s.id === i.id)?.name ?? "主体"}（${subjectBatchError(i.code)}）`).join("、")}`
          : "",
      );
      operation.current = null;
      setUncertain(false);
      setDeleting(null);
      refresh();
    } catch (e) {
      if (!controller.signal.aborted) {
        setWriteError(subjectError(e));
        const unknown = !(
          e instanceof RequestError &&
          e.status >= 400 &&
          e.status < 500
        );
        setUncertain(unknown);
        if (!unknown) operation.current = null;
      }
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  async function download(subjects: UnifiedSubject[]) {
    if (!subjects.length || locked || exportBusy.current || busy.current)
      return;
    exportBusy.current = true;
    setExporting(true);
    setWriteError("");
    const controller = new AbortController();
    flight.current = controller;
    const scope = getScopeGeneration(),
      session = getSessionGeneration();
    let url = "";
    try {
      const blob = await exportSubjects(
        {
          items: subjects.map((s) => ({ id: s.id, version_id: s.version_id })),
        },
        { responseType: "blob", signal: controller.signal },
      );
      if (
        controller.signal.aborted ||
        scope !== getScopeGeneration() ||
        session !== getSessionGeneration()
      )
        return;
      if (!(blob instanceof Blob) || !blob.size)
        throw new Error("empty_export");
      url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download =
        subjects.length === 1 ? `${subjects[0].name}.zip` : "主体.zip";
      document.body.append(link);
      link.click();
      link.remove();
    } catch (e) {
      if (!controller.signal.aborted) setWriteError(subjectError(e));
    } finally {
      exportBusy.current = false;
      if (url) setTimeout(() => URL.revokeObjectURL(url), 0);
      if (!controller.signal.aborted) setExporting(false);
    }
  }
  function toggle(subject: UnifiedSubject) {
    if (
      !selected.some((item) => item.id === subject.id) &&
      selected.length >= 120
    ) {
      setWriteError("每次最多选择 120 个主体。");
      return;
    }
    setSelected((items) =>
      items.some((i) => i.id === subject.id)
        ? items.filter((i) => i.id !== subject.id)
        : [...items, subject],
    );
  }
  function openDelete(subjects: UnifiedSubject[]) {
    setWriteError("");
    setDeleting(subjects);
  }
  function changeFilter(value: SubjectFilter) {
    setFilter(value);
    setSelected([]);
  }
  const batchTrigger = useRef<HTMLButtonElement>(null);
  const cancelBatchTrigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (batch) cancelBatchTrigger.current?.focus();
  }, [batch]);
  const allFavorites = selected.length > 0 && selected.every((s) => s.favorite);
  return (
    <section
      className="unified-subject-library flex min-w-0 flex-col gap-6"
      aria-label="主体"
    >
      <PageToolbar inHeader={toolbarInHeader}>
        {toolbarInHeader || !batch ? (
          <UnifiedSubjectToolbar
            batchTriggerRef={batchTrigger}
            showBatchAction={!toolbarInHeader}
            showAppliedRange={!toolbarInHeader}
            value={filter}
            disabled={locked}
            onChange={changeFilter}
            onBatch={() => {
              setBatch(true);
              setSelected([]);
            }}
          />
        ) : null}
      </PageToolbar>
      <PageToolbar
        inHeader={toolbarInHeader}
        slot="actions"
        presentation={batch ? "replace-base" : "adjacent"}
      >
        {batch ? (
          <div
            className="unified-subject-toolbar flex min-w-0 flex-1 flex-wrap items-center gap-2"
            aria-label="主体批量操作"
          >
            <span className="mr-auto text-sm" aria-live="polite">
              已选择 {selected.length} 项内容
            </span>
            <Button
              variant="ghost"
              disabled={locked || !selected.length}
              onClick={() => openDelete(selected)}
            >
              <Trash2 />
              删除
            </Button>
            <Button
              variant="ghost"
              disabled={locked || !selected.length}
              onClick={() => void download(selected)}
            >
              <Download />
              下载
            </Button>
            <Button
              variant="ghost"
              disabled={locked || !selected.length}
              onClick={() =>
                void mutate(allFavorites ? "unfavorite" : "favorite", selected)
              }
            >
              <Star />
              {allFavorites ? "取消收藏" : "收藏"}
            </Button>
            <Button
              variant="ghost"
              disabled={locked}
              ref={cancelBatchTrigger}
              onClick={() => {
                setBatch(false);
                setSelected([]);
                requestAnimationFrame(() => batchTrigger.current?.focus());
              }}
            >
              取消选择
            </Button>
          </div>
        ) : toolbarInHeader ? (
          <Button
            ref={batchTrigger}
            variant="ghost"
            disabled={locked}
            onClick={() => {
              setBatch(true);
              setSelected([]);
            }}
          >
            批量操作
          </Button>
        ) : null}
      </PageToolbar>
      {toolbarInHeader ? (
        <AppliedSubjectTimeRange
          value={filter}
          disabled={locked}
          onChange={changeFilter}
        />
      ) : null}
      {writeError ? (
        <div
          role="alert"
          className="rounded-lg border border-destructive/30 p-3 text-sm"
        >
          <p>{writeError}</p>
          {uncertain ? (
            <Button
              className="mt-2"
              variant="outline"
              disabled={pending}
              onClick={() => void mutate()}
            >
              重试原操作
            </Button>
          ) : null}
        </div>
      ) : null}
      {!page && loading ? (
        <div className="unified-subject-grid" aria-label="正在读取主体">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-60 w-full" />
          ))}
        </div>
      ) : !page && error ? (
        <ContentState
          state="error"
          title="暂时无法读取主体"
          description={error}
          actions={
            <Button variant="outline" onClick={retry}>
              重试读取
            </Button>
          }
        />
      ) : (
        <>
          <div className="unified-subject-grid">
            {!query && !filter.favorite && !onSelect ? (
              <Button
                variant="ghost"
                className="unified-subject-new h-auto min-w-0 flex-col items-stretch justify-start text-left"
                disabled={locked}
                onClick={() => setEditing("new")}
              >
                <span className="unified-subject-cover unified-subject-create">
                  <Plus className="size-6 text-muted-foreground" />
                </span>
                <span className="unified-subject-info">
                  <span className="block">新建主体</span>
                  <span
                    className="unified-subject-meta-placeholder block h-5"
                    aria-hidden="true"
                  />
                </span>
              </Button>
            ) : null}
            {page?.items.map((subject) => (
              <article
                key={subject.id}
                className="unified-subject-card group relative min-w-0"
              >
                <div className="relative">
                  <Button
                    variant="ghost"
                    className="unified-subject-open h-auto w-full p-0"
                    disabled={locked}
                    onClick={() =>
                      batch
                        ? toggle(subject)
                        : onSelect
                          ? onSelect(subject)
                          : setEditing(subject)
                    }
                    aria-label={`${batch ? "选择" : onSelect ? "引用" : "设置"}主体 ${subject.name}`}
                  >
                    <span className="unified-subject-cover">
                      <UnifiedSubjectMedia
                        media={selectedSubjectMedia(subject, subject.primary)}
                        label={subject.name}
                      />
                    </span>
                  </Button>
                  {batch ? (
                    <Checkbox
                      className="absolute top-2 left-2"
                      aria-label={`选择主体 ${subject.name}`}
                      checked={selected.some((s) => s.id === subject.id)}
                      disabled={locked}
                      onCheckedChange={() => toggle(subject)}
                    />
                  ) : (
                    <Button
                      variant="secondary"
                      size="icon"
                      className={`absolute top-2 right-2 transition-opacity group-hover:opacity-100 focus-visible:opacity-100 max-md:opacity-100 ${subject.favorite ? "text-[var(--favorite-foreground)] opacity-100" : "opacity-0"}`}
                      aria-label={`${subject.favorite ? "取消收藏" : "收藏"} ${subject.name}`}
                      aria-pressed={subject.favorite}
                      disabled={locked}
                      onClick={() =>
                        void mutate(
                          subject.favorite ? "unfavorite" : "favorite",
                          [subject],
                        )
                      }
                    >
                      <Star
                        className={
                          subject.favorite ? "fill-current" : undefined
                        }
                      />
                    </Button>
                  )}
                </div>
                <div className="unified-subject-info pr-12">
                  <p className="truncate">{subject.name}</p>
                  <p className="text-muted-foreground" suppressHydrationWarning>
                    主体 ·{" "}
                    {subjectRelativeTime(
                      filter.order.startsWith("created")
                        ? subject.created_at
                        : subject.updated_at,
                      filter.order.startsWith("created")
                        ? "created"
                        : "updated",
                    )}
                  </p>
                </div>
                {!batch && !onSelect ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="secondary"
                        size="icon"
                        className="absolute right-3 bottom-3 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100 max-md:opacity-100"
                        aria-label={`更多 ${subject.name}`}
                        disabled={locked}
                      >
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent
                      align="end"
                      className="unified-subject-menu"
                    >
                      <DropdownMenuItem onSelect={() => setEditing(subject)}>
                        设置主体
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() => void download([subject])}
                      >
                        下载
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() =>
                          void mutate(
                            subject.favorite ? "unfavorite" : "favorite",
                            [subject],
                          )
                        }
                      >
                        {subject.favorite ? "取消收藏" : "收藏"}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onSelect={() => openDelete([subject])}
                      >
                        删除
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : null}
              </article>
            ))}
          </div>
          {!page?.items.length &&
          (query || filter.favorite || filter.from || filter.through) ? (
            <ContentState
              title={
                query
                  ? "没有匹配的主体"
                  : filter.favorite
                    ? "还没有收藏的主体"
                    : "此时间范围内没有主体"
              }
              description="调整筛选条件后重试。"
            />
          ) : null}
          {loading ? (
            <div
              role="status"
              aria-label="正在加载更多主体"
              className="flex justify-center py-4"
            >
              <LoaderCircle
                className="size-5 animate-spin"
                aria-hidden="true"
              />
            </div>
          ) : error ? (
            <div
              className="flex items-center justify-center gap-3 py-4"
              role="alert"
            >
              <span className="text-sm text-muted-foreground">{error}</span>
              <Button variant="secondary" onClick={retry}>
                重试加载
              </Button>
            </div>
          ) : hasMore ? (
            <div ref={tail} className="h-4" aria-hidden="true" />
          ) : null}
        </>
      )}
      {editing ? (
        <Editor
          actorID={actorID}
          scopeID={scopeID}
          initial={editing === "new" ? undefined : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            refresh();
          }}
        />
      ) : null}
      {deleting ? (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !locked) setDeleting(null);
          }}
        >
          <DialogContent
            role="alertdialog"
            className="unified-subject-source"
            showCloseButton={!locked}
          >
            <DialogTitle>确认删除主体</DialogTitle>
            <DialogDescription>
              主体删除后无法恢复，关联的历史作品将无法重新编辑。
            </DialogDescription>
            <p className="text-sm break-words">
              {deleting.map((s) => s.name).join("、")}
            </p>
            {writeError ? (
              <p role="alert" className="text-sm text-destructive">
                {writeError}
              </p>
            ) : null}
            <DialogFooter>
              <Button
                variant="outline"
                disabled={locked}
                onClick={() => setDeleting(null)}
              >
                取消
              </Button>
              <Button
                variant="destructive"
                disabled={pending || exporting}
                onClick={() => void mutate("delete", deleting)}
              >
                {pending ? "正在删除…" : uncertain ? "重试原删除" : "删除"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}
    </section>
  );
}
