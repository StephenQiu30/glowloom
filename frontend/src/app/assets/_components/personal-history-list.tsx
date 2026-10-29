"use client";

import { useEffect, useRef, useState } from "react";
import { ListChecks } from "lucide-react";
import { useBatchToolbarFocus } from "./use-batch-toolbar-focus";
import { AssetToolbarButton } from "./asset-toolbar-button";
import { createPortal } from "react-dom";
import dynamic from "next/dynamic";
import {
  applyPersonalHistoryCommand,
  getOperation,
  listPersonalOperationHistory,
} from "@/api/operations";
import { getProject } from "@/api/projects";
import { requireProject } from "@/components/projects/project";
import { requireTask, type Task } from "@/components/generation/contracts";
import type { GenerationSeed } from "@/components/generation/generation-dialog";
import { ContentState } from "@/components/layout/content-state";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
import {
  hasAssetFilters,
  historyFilterParams,
  requirePersonalHistory,
  type AssetFilters,
  type AssetKind,
} from "./asset-query";
import {
  canBatchDownload,
  historyCardTargets,
  historyCommandTarget,
  isVideoDubbing,
  hasOriginalDocument,
  hasDocumentBody,
  resultTargetKey,
  type HistoryCardTarget,
} from "./history-results";
import { HistoryResultCard, historyCardTitle } from "./history-result-card";
import { HistoryPublicationDialog } from "./history-publication-dialog";
import { assetActionFeedback } from "./asset-action-feedback";
import {
  downloadHistoryCards,
  type DocumentDownloadFormat,
  type HistoryDownloadResult,
} from "./history-download";

const TaskDetail = dynamic(() =>
  import("@/components/generation/generation-dialog").then((m) => m.TaskDetail),
);
const GenerationDialog = dynamic(() =>
  import("@/components/generation/generation-dialog").then(
    (m) => m.GenerationDialog,
  ),
);
type Command = {
  key: string;
  body: Parameters<typeof applyPersonalHistoryCommand>[0];
  cards: HistoryCardTarget[];
};
type CommandResult = { key: string; title: string; ok: boolean; code: string };

export function PersonalHistoryList({
  kind,
  filters,
  onClear,
  onLock,
  onBatchMode,
  toolbarTarget,
}: {
  kind: AssetKind;
  filters: AssetFilters;
  onClear: () => void;
  onLock: (locked: boolean) => void;
  onBatchMode: (batch: boolean) => void;
  toolbarTarget: HTMLElement | null;
}) {
  const [page, setPage] = useState<ReturnType<
    typeof requirePersonalHistory
  > | null>(null);
  const [cursor, setCursor] = useState<string>();
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const [denied, setDenied] = useState(0);
  const [retry, setRetry] = useState(0);
  const [batch, setBatch] = useState(false);
  const batchToolbar = useBatchToolbarFocus(batch);
  const [chosen, setChosen] = useState<string[]>([]);
  const [detail, setDetail] = useState<HistoryCardTarget | null>(null);
  const [detailAvailable, setDetailAvailable] = useState(false);
  const [reuse, setReuse] = useState<GenerationSeed | null>(null);
  const [deleting, setDeleting] = useState<HistoryCardTarget[] | null>(null);
  const [publishing, setPublishing] = useState<HistoryCardTarget[] | null>(
    null,
  );
  const [intent, setIntent] = useState<Command | null>(null);
  const [writing, setWriting] = useState(false);
  const [downloadFormat, setDownloadFormat] =
    useState<DocumentDownloadFormat>("pdf");
  const [downloadResults, setDownloadResults] = useState<
    HistoryDownloadResult[]
  >([]);
  const [results, setResults] = useState<CommandResult[]>([]);
  const [notice, setNotice] = useState("");
  const [noticeTone, setNoticeTone] = useState<"error" | "info" | "success">(
    "error",
  );
  const busy = useRef(false);
  const actionFlight = useRef<AbortController | null>(null);
  const locked = writing || !!intent;
  useEffect(
    () => () => {
      actionFlight.current?.abort();
      onLock(false);
      onBatchMode(false);
    },
    [onLock, onBatchMode],
  );
  useEffect(() => {
    const controller = new AbortController();
    listPersonalOperationHistory(
      { kind, ...historyFilterParams(filters), limit: 24, cursor },
      { signal: controller.signal },
    )
      .then(requirePersonalHistory)
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
          setDetail(null);
          setChosen([]);
        }
        setError(readErrorMessage(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setPending(false);
      });
    return () => controller.abort();
  }, [kind, filters, cursor, retry]);
  const cards = historyCardTargets(page?.items ?? [], kind);
  const selected = cards.filter((card) => chosen.includes(card.key));
  const selectionLimit = kind === "image" || kind === "document" ? 200 : 50;
  const smallActionLimit = kind === "image" || kind === "document" ? 50 : 100;
  function toggle(card: HistoryCardTarget) {
    if (!chosen.includes(card.key) && chosen.length >= selectionLimit) {
      setNoticeTone("info");
      setNotice(`最多选择${selectionLimit}项内容。`);
      return;
    }
    setChosen((previous) =>
      previous.includes(card.key)
        ? previous.filter((key) => key !== card.key)
        : [...previous, card.key],
    );
  }
  function reload() {
    setPending(true);
    setCursor(undefined);
    setRetry((value) => value + 1);
  }
  async function command(
    action: "favorite" | "delete",
    targets: HistoryCardTarget[],
    favorited?: boolean,
  ) {
    if (busy.current || (!intent && (!targets.length || targets.length > 100)))
      return;
    const op: Command = intent ?? {
      key: crypto.randomUUID(),
      body: { action, favorited, targets: targets.map(historyCommandTarget) },
      cards: targets,
    };
    busy.current = true;
    setWriting(true);
    setIntent(op);
    setNoticeTone("error");
    setNotice("");
    onLock(true);
    const controller = new AbortController();
    actionFlight.current = controller;
    try {
      const response = await applyPersonalHistoryCommand(op.body, {
        headers: { "Idempotency-Key": op.key },
        signal: controller.signal,
      });
      const requested = op.body.targets ?? [];
      if (
        response.request_id !== op.key ||
        response.action !== op.body.action ||
        !Array.isArray(response.results) ||
        response.results.length !== requested.length
      )
        throw new RequestError("invalid_receipt");
      const expected = new Set(
        requested.map((item) =>
          resultTargetKey(item.operation_id!, item.output_id),
        ),
      );
      for (const item of response.results) {
        if (
          !expected.delete(
            resultTargetKey(item.operation_id!, item.output_id),
          ) ||
          typeof item.ok !== "boolean" ||
          !Number.isSafeInteger(item.revision) ||
          !Array.isArray(item.affected_output_ids)
        )
          throw new RequestError("invalid_receipt");
      }
      if (controller.signal.aborted) return;
      const outcomes = response.results.map((item) => {
        const key = resultTargetKey(item.operation_id!, item.output_id);
        return {
          key,
          title: historyCardTitle(op.cards.find((card) => card.key === key)!),
          ok: item.ok!,
          code: item.code ?? "",
        };
      });
      if (
        kind === "audio" &&
        op.body.action === "delete" &&
        outcomes.every((item) => item.ok)
      ) {
        setBatch(false);
        onBatchMode(false);
      }
      setDetail(null);
      setResults(outcomes);
      setChosen((previous) =>
        previous.filter(
          (key) => !outcomes.some((item) => item.key === key && item.ok),
        ),
      );
      setIntent(null);
      setDeleting(null);
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
  async function download(
    targets: HistoryCardTarget[],
    format = downloadFormat,
  ) {
    if (busy.current || locked || !canBatchDownload(targets, kind)) return;
    busy.current = true;
    setWriting(true);
    onLock(true);
    setNotice("");
    setDownloadResults([]);
    setNoticeTone("info");
    const controller = new AbortController();
    actionFlight.current = controller;
    try {
      const outcomes = await downloadHistoryCards(
        targets,
        format,
        controller.signal,
        (done, total) => setNotice(`正在准备下载 ${done}/${total}`),
      );
      if (controller.signal.aborted) return;
      if (
        kind === "audio" &&
        outcomes.length > 0 &&
        outcomes.every((item) => item.ok)
      ) {
        setBatch(false);
        onBatchMode(false);
      }
      setDownloadResults(outcomes);
      setNoticeTone(outcomes.some((item) => !item.ok) ? "error" : "success");
      setNotice(
        `已开始下载${outcomes.filter((item) => item.ok).length}项，失败${outcomes.filter((item) => !item.ok).length}项。`,
      );
      setChosen((previous) =>
        previous.filter(
          (key) =>
            outcomes.some((item) => item.key === key && !item.ok) ||
            !outcomes.some((item) => item.key === key),
        ),
      );
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) {
        setWriting(false);
        onLock(false);
      }
    }
  }
  async function startReuse(card: HistoryCardTarget, task?: Task) {
    if (busy.current || locked) return;
    busy.current = true;
    setWriting(true);
    onLock(true);
    setNotice("");
    setNoticeTone("error");
    const controller = new AbortController();
    actionFlight.current = controller;
    try {
      const [project, source] = await Promise.all([
        getProject(
          { pid: card.task.project_id },
          { signal: controller.signal },
        ).then((value) => requireProject(value, true)),
        task
          ? Promise.resolve(task)
          : getOperation(
              { id: card.task.id, project_id: card.task.project_id },
              { signal: controller.signal },
            ).then((value) => requireTask(value, card.task.project_id)),
      ]);
      if (project.id !== card.task.project_id || source.id !== card.task.id)
        throw new RequestError("invalid_receipt");
      if (!controller.signal.aborted) {
        setDetail(null);
        setReuse({
          prompt:
            source.inputs?.find((input) => input.role === "prompt")?.text ?? "",
          capability: kind === "document" ? "text" : kind,
          project,
          reuse: source,
        });
      }
    } catch (reason) {
      if (!controller.signal.aborted) setNotice(readErrorMessage(reason));
    } finally {
      busy.current = false;
      if (!controller.signal.aborted) {
        setWriting(false);
        onLock(false);
      }
    }
  }
  const canFavorite =
    selected.length > 0 &&
    selected.every((card) => card.canFavorite) &&
    selected.length <= smallActionLimit;
  const canPublish =
    selected.length > 0 &&
    selected.every((card) => card.canPublish) &&
    (kind !== "image" || selected.length <= 7);
  const formatTargets = batch ? selected : cards;
  const originalOnly =
    kind === "document" &&
    formatTargets.length > 0 &&
    formatTargets.every(hasOriginalDocument);
  const canChooseFormat =
    kind === "document" && formatTargets.some(hasDocumentBody);
  const toolbar = (
    <div
      ref={batchToolbar}
      data-asset-batch={batch || undefined}
      className="flex min-w-0 flex-wrap items-center gap-2"
      aria-label="生成历史操作"
    >
      {batch ? (
        <>
          <span
            role="status"
            className="text-sm text-muted-foreground tabular-nums"
          >
            已选择{selected.length}项内容
          </span>
          <Button
            variant="ghost"
            disabled={
              locked || !selected.length || selected.length > smallActionLimit
            }
            onClick={() => {
              setNotice("");
              setDeleting(selected);
            }}
          >
            删除
          </Button>
          {canChooseFormat ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  disabled={locked || !canBatchDownload(selected, kind)}
                >
                  下载
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuGroup>
                  {(["pdf", "md"] as const).map((format) => (
                    <DropdownMenuItem
                      key={format}
                      onSelect={() => void download(selected, format)}
                    >
                      {format === "pdf" ? "下载 PDF" : "下载 Markdown"}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : (
            <Button
              variant="ghost"
              disabled={locked || !canBatchDownload(selected, kind)}
              onClick={() => void download(selected)}
            >
              {originalOnly ? "下载原件" : "下载"}
            </Button>
          )}
          {(kind === "image" || kind === "video") && (
            <Button
              variant="ghost"
              disabled={locked || !canPublish}
              title={
                kind === "image" && selected.length > 7
                  ? "最多选择7项发布"
                  : undefined
              }
              onClick={() => setPublishing(selected)}
            >
              发布
            </Button>
          )}
          <Button
            variant="ghost"
            disabled={locked || !canFavorite}
            title={
              kind === "document" && selected.length > 50
                ? "文档一次最多收藏50项"
                : undefined
            }
            onClick={() =>
              void command(
                "favorite",
                selected,
                !selected.every((card) => card.favorite),
              )
            }
          >
            {selected.length > 0 && selected.every((card) => card.favorite)
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
              setChosen([]);
            }}
          >
            取消选择
          </Button>
        </>
      ) : (
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
      )}
    </div>
  );
  return (
    <div className="grid min-w-0 gap-6">
      {toolbarTarget ? createPortal(toolbar, toolbarTarget) : toolbar}
      {intent && !deleting && (
        <Button
          disabled={writing}
          className="justify-self-start"
          onClick={() =>
            void command(
              intent.body.action as "favorite" | "delete",
              intent.cards,
              intent.body.favorited,
            )
          }
        >
          重试原操作
        </Button>
      )}
      {originalOnly && (
        <p className="text-sm text-muted-foreground">
          所选文档仅提供原件，不支持PDF或Markdown转换。
        </p>
      )}
      {batch &&
        kind === "document" &&
        selected.some(hasOriginalDocument) &&
        selected.some(hasDocumentBody) && (
          <p className="text-sm text-muted-foreground">
            正文按所选格式导出，其余文档下载原件。
          </p>
        )}
      {batch && kind === "document" && selected.length > 50 && (
        <p role="status" className="text-sm text-muted-foreground">
          文档一次最多收藏50项，请减少选择。
        </p>
      )}

      {batch && kind === "image" && selected.length > 7 && (
        <p className="text-sm text-muted-foreground">最多选择7项发布</p>
      )}
      {error && (
        <ContentState
          state="error"
          title={
            denied === 403
              ? "无权查看此空间的生成历史"
              : denied === 401
                ? "请重新登录查看生成历史"
                : "暂时无法读取生成历史"
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
        <ContentState state="loading" title="正在读取生成历史" />
      )}
      {!pending && !error && !cards.length && (
        <ContentState
          variant="workspace"
          title={
            hasAssetFilters(filters)
              ? "当前已开启筛选，部分内容可能被隐藏"
              : "暂无相关资产"
          }
          actions={
            hasAssetFilters(filters) ? (
              <Button variant="ghost" onClick={onClear}>
                清除全部条件
              </Button>
            ) : undefined
          }
        />
      )}
      <div className="grid min-w-0 grid-cols-2 gap-x-4 gap-y-6 lg:grid-cols-3 xl:grid-cols-4">
        {cards.map((card) => (
          <HistoryResultCard
            key={card.key}
            card={card}
            kind={kind}
            batch={batch}
            checked={chosen.includes(card.key)}
            disabled={locked || pending || !!error}
            onOpen={() => {
              setDetailAvailable(false);
              setDetail(card);
            }}
            onToggle={() => toggle(card)}
            onFavorite={() => void command("favorite", [card], !card.favorite)}
            onDelete={() => {
              setNotice("");
              setDeleting([card]);
            }}
            onDownload={() => void download([card])}
            onPublish={() => setPublishing([card])}
            onReuse={() => void startReuse(card)}
          />
        ))}
      </div>
      {page?.next_cursor && (
        <Button
          className="justify-self-center"
          variant="ghost"
          disabled={pending || locked}
          onClick={() => {
            setPending(true);
            setCursor(page.next_cursor!);
          }}
        >
          更多记录
        </Button>
      )}
      <section
        aria-label="操作结果"
        aria-live="polite"
        className={results.length ? "flex flex-col gap-2" : "sr-only"}
      >
        {results.map((item) => (
          <Alert
            key={item.key}
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
      <section
        aria-label="下载结果"
        aria-live="polite"
        className={downloadResults.length ? "grid gap-1 text-sm" : "sr-only"}
      >
        {downloadResults.map((item) => (
          <p key={`${item.key}:${item.outputID}`} className="break-words">
            {item.title} · {item.outputID.slice(0, 8)} ·{" "}
            {item.format === "original"
              ? "原件"
              : item.format === "pdf"
                ? "PDF"
                : "Markdown"}
            ：{item.ok ? "已开始下载" : item.message || "下载失败，请重试。"}
          </p>
        ))}
      </section>
      <Dialog
        open={!!detail}
        onOpenChange={(open) => {
          if (!open && !locked) setDetail(null);
        }}
      >
        <DialogContent
          showCloseButton={!locked}
          className="max-h-[90dvh] overflow-y-auto overscroll-contain sm:max-w-[calc(100vw-3rem)] lg:max-w-6xl"
        >
          <DialogTitle>生成记录</DialogTitle>
          <DialogDescription className="break-words">
            {detail?.task.model_name || "任务详情"}
          </DialogDescription>
          {detail && (
            <TaskDetail
              key={`${detail.key}`}
              projectID={detail.task.project_id}
              id={detail.task.id}
              outputID={detail.output?.id}
              presentation="asset"
              onAvailabilityChange={setDetailAvailable}
              onReuse={(task) => void startReuse(detail, task)}
            />
          )}
          {detail && (
            <div
              className="flex min-w-0 flex-wrap items-center justify-end gap-2 border-t pt-4"
              aria-label="结果管理"
            >
              <Button
                variant="ghost"
                disabled={locked || !detailAvailable || !detail.canFavorite}
                onClick={() =>
                  void command("favorite", [detail], !detail.favorite)
                }
              >
                {detail.favorite ? "取消收藏" : "收藏"}
              </Button>
              {kind === "document" && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      disabled={
                        locked || !detailAvailable || !hasDocumentBody(detail)
                      }
                    >
                      下载格式 ·{" "}
                      {hasOriginalDocument(detail)
                        ? "原件"
                        : downloadFormat === "pdf"
                          ? "PDF"
                          : "Markdown"}
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent>
                    <DropdownMenuGroup>
                      {(["pdf", "md"] as const).map((format) => (
                        <DropdownMenuItem
                          key={format}
                          onSelect={() => setDownloadFormat(format)}
                        >
                          {format === "pdf" ? "PDF" : "Markdown"}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
              {hasOriginalDocument(detail) && (
                <p className="w-full text-sm text-muted-foreground">
                  此文档仅提供原件，不支持PDF或Markdown转换。
                </p>
              )}
              <Button
                variant="ghost"
                disabled={locked || !detailAvailable || !detail.canDownload}
                onClick={() => void download([detail])}
              >
                {hasOriginalDocument(detail) ? "下载原件" : "下载"}
              </Button>
              {(kind === "image" ||
                (kind === "video" && !isVideoDubbing(detail))) && (
                <Button
                  variant="ghost"
                  disabled={locked || !detailAvailable || !detail.canPublish}
                  onClick={() => {
                    setPublishing([detail]);
                    setDetail(null);
                  }}
                >
                  发布
                </Button>
              )}
              <Button
                variant="ghost"
                disabled={locked || !detailAvailable}
                onClick={() => {
                  setNotice("");
                  setDeleting([detail]);
                  setDetail(null);
                }}
              >
                删除
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>
      {deleting && (
        <AlertDialog
          open
          onOpenChange={(open) => {
            if (!open && !locked) setDeleting(null);
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认删除</AlertDialogTitle>
              <AlertDialogDescription>
                删除的历史记录将无法找回。将从你的生成历史中移除
                {deleting.length}
                项，任务执行、计费和画布引用不受影响。
              </AlertDialogDescription>
            </AlertDialogHeader>
            {notice && (
              <Alert variant="destructive">
                <AlertTitle>删除尚未完成</AlertTitle>
                <AlertDescription className="break-words">
                  {notice}
                </AlertDescription>
              </Alert>
            )}
            <AlertDialogFooter>
              <AlertDialogCancel disabled={locked}>取消</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                disabled={writing}
                onClick={(event) => {
                  event.preventDefault();
                  void command("delete", deleting);
                }}
              >
                {writing ? "正在删除…" : intent ? "重试原操作" : "删除"}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
      {publishing && (
        <HistoryPublicationDialog
          cards={publishing}
          onClose={() => setPublishing(null)}
          onLock={onLock}
          onPublished={(failed) => setChosen(failed)}
        />
      )}
      {reuse && (
        <GenerationDialog seed={reuse} onClose={() => setReuse(null)} />
      )}
      {notice && !deleting && (
        <FeedbackToast message={notice} tone={noticeTone} />
      )}
    </div>
  );
}
