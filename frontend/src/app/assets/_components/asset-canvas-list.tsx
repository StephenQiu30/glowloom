"use client";

import { useEffect, useRef, useState } from "react";
import { ListChecks } from "lucide-react";
import { useBatchToolbarFocus } from "./use-batch-toolbar-focus";
import { AssetToolbarButton } from "./asset-toolbar-button";
import { createPortal } from "react-dom";
import Link from "next/link";
import { MoreHorizontal, Pencil, Plus, Star, Trash2, X } from "lucide-react";
import { applyCanvasAssetCommand, listAssetCanvases } from "@/api/canvases";
import { publishProjectInvalidation } from "@/components/projects/project-presentation-store";
import { CanvasDeleteConfirmation } from "@/components/projects/canvas-delete-confirmation";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { useSpace } from "@/components/spaces/space-context";
import { ContentState } from "@/components/layout/content-state";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
import {
  assetDateRange,
  hasAssetFilters,
  requireAssetCanvases,
  type AssetFilters,
  type AssetCanvasItem,
} from "./asset-query";
import { AssetCreateProject } from "./asset-create-project";
import { AssetTemplateList, type AssetTemplate } from "./asset-template-list";
import {
  AssetCardCover,
  AssetCardTitle,
  AssetCardMeta,
} from "./asset-card-parts";
import { formatAssetTime } from "./asset-time";
import { assetActionFeedback } from "./asset-action-feedback";

type Intent = {
  key: string;
  body: Parameters<typeof applyCanvasAssetCommand>[0];
  items: AssetCanvasItem[];
};
export function AssetCanvasList({
  kind,
  collection,
  filters,
  onClear,
  onLock,
  onBatchMode,
  toolbarTarget,
}: {
  kind: "canvas" | "short_drama";
  collection: "current" | "legacy";
  filters: AssetFilters;
  onClear: () => void;
  onLock: (locked: boolean) => void;
  onBatchMode: (batch: boolean) => void;
  toolbarTarget: HTMLElement | null;
}) {
  const [page, setPage] = useState<ReturnType<
    typeof requireAssetCanvases
  > | null>(null);
  const [cursor, setCursor] = useState<string>();
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const [denied, setDenied] = useState(0);
  const [retry, setRetry] = useState(0);
  const [creating, setCreating] = useState(false);
  const [template, setTemplate] = useState<AssetTemplate | undefined>();
  const [batch, setBatch] = useState(false);
  const batchToolbar = useBatchToolbarFocus(batch);
  const [selected, setSelected] = useState<string[]>([]);
  const [deleting, setDeleting] = useState<AssetCanvasItem[] | null>(null);
  const [renaming, setRenaming] = useState<AssetCanvasItem | null>(null);
  const dialogTrigger = useRef<HTMLButtonElement | null>(null);
  function rememberDialogTrigger(event: { currentTarget: HTMLButtonElement }) {
    dialogTrigger.current = event.currentTarget;
  }
  function restoreDialogFocus(event: Event) {
    if (dialogTrigger.current?.isConnected) {
      event.preventDefault();
      dialogTrigger.current.focus();
    }
  }

  const [name, setName] = useState("");
  const [intent, setIntent] = useState<Intent | null>(null);
  const [writing, setWriting] = useState(false);
  const [notice, setNotice] = useState("");
  const [results, setResults] = useState<
    { id: string; title: string; ok: boolean; code: string }[]
  >([]);
  const space = useSpace();
  const busy = useRef(false);
  const flight = useRef<AbortController | null>(null);
  const locked = writing || !!intent;
  const writable = !space.pending && !!space.selected?.writable;
  useEffect(
    () => () => {
      flight.current?.abort();
      onLock(false);
      onBatchMode(false);
    },
    [onLock, onBatchMode],
  );
  useEffect(() => {
    const controller = new AbortController();
    listAssetCanvases(
      {
        kind,
        collection,
        favorite_only: filters.favorite || undefined,
        search: filters.search || undefined,
        ...assetDateRange(filters.from, filters.through),
        sort: filters.sort,
        order: filters.order,
        limit: 24,
        cursor,
      },
      { signal: controller.signal },
    )
      .then((value) => requireAssetCanvases(value, kind, collection))
      .then((result) => {
        if (controller.signal.aborted) return;
        setPage((previous) => ({
          items: cursor
            ? [
                ...(previous?.items ?? []),
                ...result.items.filter(
                  (item) => !previous?.items.some((old) => old.id === item.id),
                ),
              ]
            : result.items,
          next_cursor: result.next_cursor,
        }));
        setError("");
        setDenied(0);
      })
      .catch((reason) => {
        if (controller.signal.aborted) return;
        if (
          reason instanceof RequestError &&
          [401, 403].includes(reason.status)
        ) {
          setDenied(reason.status);
          setPage(null);
          setSelected([]);
        }
        setError(readErrorMessage(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setPending(false);
      });
    return () => controller.abort();
  }, [kind, collection, filters, cursor, retry]);
  function reload() {
    setPending(true);
    setCursor(undefined);
    setRetry((value) => value + 1);
  }
  const selectionLimit = kind === "short_drama" ? 100 : 50;
  const chosen = page?.items.filter((item) => selected.includes(item.id)) ?? [];
  function toggle(id: string) {
    if (!selected.includes(id) && selected.length >= selectionLimit) {
      setNotice(`最多选择${selectionLimit}项内容。`);
      return;
    }
    setSelected((previous) =>
      previous.includes(id)
        ? previous.filter((key) => key !== id)
        : [...previous, id],
    );
  }
  async function command(
    action: "favorite" | "rename" | "delete",
    items: AssetCanvasItem[],
    favorite?: boolean,
  ) {
    if (busy.current || (!intent && (!items.length || items.length > 100)))
      return;
    const op: Intent = intent ?? {
      key: crypto.randomUUID(),
      items,
      body: {
        action,
        favorite,
        name: action === "rename" ? name.trim() : undefined,
        items: items.map((item) => ({
          id: item.id,
          expected_revision: item.revision,
          expected_favorite_revision: item.favorite_revision,
          expected_project_revision: item.is_main
            ? item.project_revision
            : undefined,
        })),
      },
    };
    busy.current = true;
    setWriting(true);
    setIntent(op);
    setNotice("");
    onLock(true);
    const controller = new AbortController();
    flight.current = controller;
    try {
      const response = await applyCanvasAssetCommand(op.body, {
        signal: controller.signal,
        headers: { "Idempotency-Key": op.key },
      });
      if (
        !Array.isArray(response.results) ||
        response.results.length !== op.items.length ||
        !Number.isSafeInteger(response.succeeded) ||
        !Number.isSafeInteger(response.failed) ||
        response.succeeded! + response.failed! !== op.items.length
      )
        throw new RequestError("invalid_receipt");
      const expected = new Set(op.items.map((item) => item.id));
      for (const item of response.results) {
        if (
          !expected.delete(item.id ?? "") ||
          !["succeeded", "failed"].includes(item.status ?? "") ||
          typeof item.code !== "string"
        )
          throw new RequestError("invalid_receipt");
      }
      if (
        response.results.filter((item) => item.status === "succeeded")
          .length !== response.succeeded
      )
        throw new RequestError("invalid_receipt");
      if (controller.signal.aborted) return;
      const outcomes = response.results.map((item) => ({
        id: item.id!,
        title: op.items.find((old) => old.id === item.id)!.name,
        ok: item.status === "succeeded",
        code: item.code!,
      }));
      if (op.body.action === "rename" || op.body.action === "delete") {
        const changedProjects = new Set(
          op.items
            .filter(
              (item) =>
                item.is_main &&
                outcomes.some((result) => result.id === item.id && result.ok),
            )
            .map((item) => item.project_id),
        );
        for (const id of changedProjects) publishProjectInvalidation(id);
      }
      setResults(outcomes);
      setSelected((previous) =>
        previous.filter(
          (id) => !outcomes.some((item) => item.id === id && item.ok),
        ),
      );
      setIntent(null);
      setDeleting(null);
      setRenaming(null);
      onLock(false);
      reload();
    } catch (reason) {
      if (controller.signal.aborted) return;
      if (
        reason instanceof RequestError &&
        reason.status >= 400 &&
        reason.status < 500 &&
        reason.code !== "invalid_receipt"
      ) {
        setIntent(null);
        onLock(false);
        setNotice(readErrorMessage(reason));
      } else setNotice("操作结果尚未确认，请重试原操作。");
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) setWriting(false);
    }
  }
  const toolbar = (
    <div
      ref={batchToolbar}
      data-asset-batch={batch || undefined}
      className="flex flex-wrap items-center gap-2"
      aria-label="画布操作"
    >
      {batch ? (
        <>
          <span
            role="status"
            className="text-sm text-muted-foreground tabular-nums"
          >
            已选择{chosen.length}项内容
          </span>
          <Button
            onPointerDown={rememberDialogTrigger}
            onFocus={rememberDialogTrigger}
            variant="ghost"
            disabled={
              locked || !writable || !chosen.length || chosen.length > 100
            }
            onClick={() => {
              setNotice("");
              setDeleting(chosen);
            }}
          >
            <Trash2 data-icon="inline-start" />
            删除
          </Button>
          <Button
            variant="ghost"
            disabled={locked || !chosen.length || chosen.length > 100}
            onClick={() =>
              void command(
                "favorite",
                chosen,
                !chosen.every((item) => item.favorite),
              )
            }
          >
            <Star data-icon="inline-start" />
            {chosen.length && chosen.every((item) => item.favorite)
              ? "取消收藏"
              : "收藏"}
          </Button>
          <Button
            data-batch-focus
            variant="ghost"
            disabled={locked}
            onClick={() => {
              setBatch(false);
              onBatchMode(false);
              setSelected([]);
            }}
          >
            <X data-icon="inline-start" />
            取消选择
          </Button>
        </>
      ) : (
        <>
          <AssetToolbarButton
            data-batch-focus
            label="批量操作"
            icon={ListChecks}
            variant="ghost"
            disabled={locked || pending || !page || !!error}
            onClick={() => {
              setBatch(true);
              onBatchMode(true);
            }}
          />
        </>
      )}
    </div>
  );
  return (
    <div className="grid min-w-0 gap-6">
      {toolbarTarget ? createPortal(toolbar, toolbarTarget) : toolbar}
      {intent && !deleting && !renaming && (
        <Button
          className="justify-self-start"
          disabled={writing}
          onClick={() =>
            void command(
              intent.body.action!,
              intent.items,
              intent.body.favorite,
            )
          }
        >
          重试原操作
        </Button>
      )}
      {chosen.length > 100 && (
        <p className="text-sm text-muted-foreground">
          本产品每次最多操作100项，请减少选择。
        </p>
      )}
      {error && (
        <ContentState
          state="error"
          title={
            denied === 403
              ? "无权查看此空间的画布"
              : denied === 401
                ? "请重新登录查看画布"
                : kind === "short_drama"
                  ? "暂时无法读取短剧"
                  : "暂时无法读取画布"
          }
          description={error}
          actions={
            <Button disabled={pending || locked} onClick={reload}>
              重试读取
            </Button>
          }
        />
      )}
      {pending && !page && (
        <ContentState state="loading" title="正在读取画布" />
      )}
      {!pending &&
        !error &&
        !page?.items.length &&
        hasAssetFilters(filters) && (
          <ContentState
            variant="workspace"
            title="当前已开启筛选，部分内容可能被隐藏"
            actions={
              <Button variant="ghost" onClick={onClear}>
                清除全部条件
              </Button>
            }
          />
        )}
      <div className="grid min-w-0 grid-cols-2 gap-x-4 gap-y-6 lg:grid-cols-3 xl:grid-cols-4">
        {!!page && !error && !hasAssetFilters(filters) && (
          <Button
            variant="asset-card"
            size="asset-card"
            aria-label={kind === "short_drama" ? "新建短剧画布" : "新建画布"}
            disabled={locked || !writable || pending}
            onClick={() => {
              setTemplate(undefined);
              setCreating(true);
            }}
          >
            <AssetCardCover variant="create">
              <Plus aria-hidden="true" />
            </AssetCardCover>
            <AssetCardTitle>
              {kind === "short_drama" ? "新建短剧画布" : "新建项目"}
            </AssetCardTitle>
          </Button>
        )}
        {page?.items.map((item) => (
          <article
            key={item.id}
            className="group relative min-w-0 [contain-intrinsic-size:auto_16rem] [content-visibility:auto]"
            aria-label={item.name}
          >
            <Button
              variant="asset-card"
              size="asset-card"
              asChild={!batch && !locked}
              disabled={locked}
              aria-pressed={batch ? selected.includes(item.id) : undefined}
              onClick={batch ? () => toggle(item.id) : undefined}
            >
              {batch || locked ? (
                <span className="contents">
                  <CanvasCover item={item} />
                  <CanvasText item={item} sort={filters.sort} />
                </span>
              ) : (
                <Link
                  href={`/canvas?project_id=${item.project_id}&canvas_id=${item.id}`}
                >
                  <CanvasCover item={item} />
                  <CanvasText item={item} sort={filters.sort} />
                </Link>
              )}
            </Button>
            {batch ? (
              <Checkbox
                className="absolute top-3 left-3 after:-inset-3.5"
                checked={selected.includes(item.id)}
                disabled={locked}
                aria-label={`选择${item.name}`}
                onCheckedChange={() => toggle(item.id)}
              />
            ) : (
              <div className="opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 motion-reduce:transition-none max-md:opacity-100">
                <Button
                  variant="ghost"
                  size="icon"
                  className="absolute top-2 right-2 max-md:size-11"
                  disabled={locked}
                  aria-pressed={item.favorite}
                  aria-label={`${item.favorite ? "取消收藏" : "收藏"}${item.name}`}
                  onClick={() =>
                    void command("favorite", [item], !item.favorite)
                  }
                >
                  <Star
                    aria-hidden="true"
                    className={item.favorite ? "fill-current" : undefined}
                  />
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="absolute right-1 bottom-3 max-md:size-11"
                      disabled={locked}
                      onPointerDown={rememberDialogTrigger}
                      onFocus={rememberDialogTrigger}
                      aria-label={`更多 · ${item.name}`}
                    >
                      <MoreHorizontal aria-hidden="true" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="canvas-card-menu">
                    <DropdownMenuGroup>
                      <DropdownMenuItem
                        disabled={!writable}
                        onSelect={() => {
                          setRenaming(item);
                          setName(item.name);
                        }}
                      >
                        <Pencil size={16} aria-hidden="true" />
                        重命名
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() =>
                          void command("favorite", [item], !item.favorite)
                        }
                      >
                        <Star size={16} aria-hidden="true" />
                        {item.favorite ? "取消收藏" : "收藏"}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        disabled={!writable}
                        onSelect={() => {
                          setNotice("");
                          setDeleting([item]);
                        }}
                      >
                        <Trash2 size={16} aria-hidden="true" />
                        删除
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            )}
          </article>
        ))}
        {collection === "current" &&
          !batch &&
          !!page &&
          !error &&
          !hasAssetFilters(filters) && (
            <AssetTemplateList
              kind={kind}
              disabled={locked || !writable || pending}
              onUse={(value) => {
                setTemplate(value);
                setCreating(true);
              }}
            />
          )}
      </div>
      {page?.next_cursor && (
        <Button
          variant="ghost"
          className="justify-self-center"
          disabled={pending || locked}
          onClick={() => {
            setPending(true);
            setCursor(page.next_cursor!);
          }}
        >
          更多画布
        </Button>
      )}
      <section
        aria-label="画布操作结果"
        aria-live="polite"
        className={results.length ? "flex flex-col gap-2" : "sr-only"}
      >
        {results.map((item) => (
          <Alert
            key={item.id}
            variant={item.ok ? "default" : "destructive"}
            role="group"
          >
            <AlertTitle>{item.ok ? "操作完成" : "操作未完成"}</AlertTitle>
            <AlertDescription className="break-words">
              {item.title}：
              {item.ok ? "已完成" : assetActionFeedback(item.code)}
            </AlertDescription>
          </Alert>
        ))}
      </section>
      {creating && (
        <AssetCreateProject
          kind={kind}
          template={template}
          onClose={() => setCreating(false)}
          onLock={onLock}
        />
      )}
      {deleting && (
        <CanvasDeleteConfirmation
          description={
            deleting.some((item) => item.is_main)
              ? "删除主画布将把整个所属项目移入回收站，保留30天，是否确认删除？"
              : `是否确认删除所选${kind === "short_drama" ? "短剧" : "画布"}？`
          }
          locked={locked}
          pending={writing}
          retry={!!intent}
          notice={notice}
          onClose={() => setDeleting(null)}
          onConfirm={() => void command("delete", deleting)}
          onCloseAutoFocus={restoreDialogFocus}
        />
      )}
      {renaming && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !locked) setRenaming(null);
          }}
        >
          <DialogContent showCloseButton={!locked}>
            <DialogTitle>
              重命名{kind === "short_drama" ? "短剧" : "画布"}
            </DialogTitle>
            <DialogDescription>
              {renaming.is_main
                ? "主画布名称将同时更新所属项目名称。"
                : "更新画布名称。"}
            </DialogDescription>
            <form
              className="grid gap-5"
              onSubmit={(event) => {
                event.preventDefault();
                if (name.trim()) void command("rename", [renaming]);
              }}
            >
              <Field>
                <FieldLabel htmlFor="asset-rename-name">名称</FieldLabel>
                <Input
                  id="asset-rename-name"
                  name="name"
                  autoComplete="off"
                  maxLength={50}
                  value={name}
                  disabled={locked}
                  onChange={(event) => setName(event.target.value)}
                />
              </Field>
              <DialogFooter>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={locked}
                  onClick={() => setRenaming(null)}
                >
                  取消
                </Button>
                <Button type="submit" disabled={writing || !name.trim()}>
                  {writing ? "正在保存…" : intent ? "重试原操作" : "保存"}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}
      {notice && !deleting && <FeedbackToast message={notice} />}
    </div>
  );
}
function CanvasCover({ item }: { item: AssetCanvasItem }) {
  return (
    <AssetCardCover>
      {item.cover_asset_id && !item.cover_unavailable ? (
        <ProjectMediaPreview
          projectID={item.project_id}
          id={item.cover_asset_id}
          kind="image"
          compact
          fit="contain"
          interactive={false}
        />
      ) : undefined}
    </AssetCardCover>
  );
}
function CanvasText({
  item,
  sort,
}: {
  item: AssetCanvasItem;
  sort: AssetFilters["sort"];
}) {
  const time = sort === "created" ? item.created_at : item.updated_at;
  return (
    <>
      <AssetCardTitle reserveAction>{item.name}</AssetCardTitle>
      <AssetCardMeta title={new Date(time).toLocaleString("zh-CN")}>
        {item.collection === "legacy" ? "画布 · " : ""}
        {formatAssetTime(time)}
        {sort === "created" ? "创建" : "修改"}
      </AssetCardMeta>
    </>
  );
}
