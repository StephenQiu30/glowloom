"use client";
import { useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  getSourceWrite,
  listSourceWrites,
  reviewScopeKey,
} from "./review-queries";
import { sourceWriteActions } from "./source-write-model";
import { useSourceControl } from "./use-source-control";
import type { ScriptScope } from "./source-intent";

export function SourceWriteDialog({
  scope,
  initialId,
  onClose,
}: {
  scope: ScriptScope;
  initialId?: string;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [selected, setSelected] = useState(initialId);
  const [accepted, setAccepted] = useState(false);
  const [latestReviewed, setLatestReviewed] = useState(false);
  const recovery = useRef<HTMLButtonElement>(null);
  const prefix = [...reviewScopeKey(scope), "source-writes"];
  const control = useSourceControl(scope, async () => {
    setAccepted(true);
    await client.invalidateQueries({ queryKey: prefix });
  });
  const list = useInfiniteQuery({
    queryKey: [...prefix, "list"],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      listSourceWrites(scope, pageParam, signal),
    getNextPageParam: (page) => page.next_after,
    staleTime: 0,
  });
  const items = list.data?.pages.flatMap((page) => page.items) ?? [];
  const duplicates =
    new Set(items.map((item) => item.id)).size !== items.length;
  const id = control.intent?.intentId ?? selected ?? items[0]?.id;
  const detail = useQuery({
    queryKey: [...prefix, "detail", id],
    queryFn: ({ signal }) => getSourceWrite(scope, id!, signal),
    enabled: Boolean(id),
    staleTime: 0,
  });
  const item = detail.data;
  const actions = item
    ? sourceWriteActions(item)
    : { cancel: false, reconcile: false };
  const blocking =
    control.busy || Boolean(control.intent || control.storageError);
  const hasRecovery = Boolean(control.intent || control.storageError);
  useEffect(() => {
    if (!hasRecovery || control.busy) return;
    const active = document.activeElement;
    if (active instanceof HTMLButtonElement && active.disabled)
      recovery.current?.focus();
  }, [control.intent?.key, control.storageError, control.busy, hasRecovery]);
  const status =
    item?.status === "completed"
      ? "已发布"
      : item?.status === "cancelled"
        ? "已取消且清理完成"
        : "保存尚未完成";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !blocking) onClose();
      }}
    >
      <DialogContent
        showCloseButton={!blocking}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl"
        onEscapeKeyDown={(event) => {
          if (blocking) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (blocking) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>正文保存记录与恢复</DialogTitle>
          <DialogDescription>
            只显示当前项目授权的安全元数据。取消和核验控制未发布的原对象，不代替创建者发布旧正文；202
            仅表示控制已受理。
          </DialogDescription>
        </DialogHeader>
        <div className="grid min-w-0 gap-4 sm:grid-cols-[240px_minmax(0,1fr)]">
          <section aria-label="正文保存记录列表" className="min-w-0 space-y-2">
            {list.isPending && <p role="status">正在读取保存记录…</p>}
            {(list.error || duplicates) && (
              <p role="alert">
                {duplicates
                  ? "分页出现重复保存身份，请重新读取完整事实。"
                  : list.error?.message}
              </p>
            )}
            <ul className="max-h-80 space-y-2 overflow-y-auto">
              {!duplicates &&
                items.map((entry) => (
                  <li key={entry.id}>
                    <Button
                      type="button"
                      variant={id === entry.id ? "secondary" : "outline"}
                      className="h-auto w-full flex-col items-start gap-1 py-2 text-left whitespace-normal"
                      disabled={blocking}
                      aria-current={id === entry.id ? "true" : undefined}
                      onClick={() => {
                        setSelected(entry.id);
                        setLatestReviewed(false);
                        setAccepted(false);
                      }}
                    >
                      <span>
                        {entry.action === "create"
                          ? "创建"
                          : entry.action === "update"
                            ? "保存"
                            : entry.action === "delete"
                              ? "移除"
                              : entry.action === "import"
                                ? "导入"
                                : "重排"}{" "}
                        ·{" "}
                        {entry.status === "pending"
                          ? "未完成"
                          : entry.status === "completed"
                            ? "已发布"
                            : "已清理"}
                      </span>
                      <span className="text-xs break-all">{entry.id}</span>
                    </Button>
                  </li>
                ))}
            </ul>
            {list.hasNextPage && (
              <Button
                type="button"
                variant="outline"
                disabled={list.isFetchingNextPage}
                onClick={() => void list.fetchNextPage()}
              >
                读取保存记录下一页
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              disabled={control.busy}
              onClick={() => void list.refetch()}
            >
              重新读取保存记录
            </Button>
          </section>
          <section aria-label="保存记录详情" className="min-w-0 space-y-3">
            {!id && !list.isPending && <p>当前项目尚无正文保存记录。</p>}
            {detail.isPending && id && <p role="status">正在读取原保存事实…</p>}
            {detail.error && <p role="alert">{detail.error.message}</p>}
            {item && (
              <>
                <p className="font-medium">
                  {status} · 修订 {item.revision}
                </p>
                <p className="text-xs break-all">保存身份：{item.id}</p>
                <p>
                  原脚本修订 {item.expected_script_revision}；已核对{" "}
                  {item.confirmed_object_count} / {item.object_count} 个对象。
                </p>
                <p>
                  {item.active_io
                    ? "原 I/O owner 尚存在；请求受理不证明已停止。"
                    : "当前没有持久 I/O owner；停止与对象结果仍以核验事实为准。"}
                </p>
                {item.cancellation_requested && item.status === "pending" && (
                  <p>取消意图已记录，等待实际停止与未发布对象清理。</p>
                )}
                {item.needs_reconciliation && <p>原对象或退出证据仍待核验。</p>}
                {!item.can_control && item.status === "pending" && (
                  <p>
                    当前主体不可控制此保存。可由仍获授权的创建者或当前组织管理员明确恢复。
                  </p>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="destructive"
                    disabled={
                      control.locked ||
                      duplicates ||
                      detail.isError ||
                      !actions.cancel
                    }
                    onClick={() => {
                      setAccepted(false);
                      void control.submit({
                        intentId: item.id,
                        action: "cancel",
                        body: { expected_revision: item.revision },
                      });
                    }}
                  >
                    明确取消未发布保存
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={
                      control.locked ||
                      duplicates ||
                      detail.isError ||
                      !actions.reconcile
                    }
                    onClick={() => {
                      setAccepted(false);
                      void control.submit({
                        intentId: item.id,
                        action: "reconcile",
                        body: { expected_revision: item.revision },
                      });
                    }}
                  >
                    明确核验原保存
                  </Button>
                </div>
              </>
            )}
            {control.conflicted && (
              <>
                <p>
                  控制被确定拒绝，原草稿保留。先人工读取最新修订，再明确提出新的控制意图。
                </p>
                <Button
                  type="button"
                  variant="outline"
                  disabled={detail.isFetching}
                  onClick={async () => {
                    const latest = await detail.refetch();
                    if (latest.isSuccess) setLatestReviewed(true);
                  }}
                >
                  读取最新控制事实
                </Button>
                {latestReviewed && (
                  <Button
                    type="button"
                    onClick={() => {
                      control.acknowledgeLatest();
                      setLatestReviewed(false);
                    }}
                  >
                    确认最新修订并返回控制选择
                  </Button>
                )}
              </>
            )}
            {control.intent && (
              <div className="space-y-2 rounded-lg border border-amber-500 p-3">
                <p>原控制尚未确认。核验保留原修订与原键，不自动重提。</p>
                <p className="text-xs break-all">
                  {control.intent.action} · 原修订{" "}
                  {control.intent.body.expected_revision} · 原键{" "}
                  {control.intent.key}
                </p>
                <pre className="text-xs">
                  {JSON.stringify(control.intent.body)}
                </pre>
              </div>
            )}
            {(control.error || control.storageError) && (
              <p role="alert">{control.storageError ?? control.error}</p>
            )}
            {accepted && (
              <p role="status">
                原控制已受理。当前停止与清理状态来自重新读取，不以受理回执替代。
              </p>
            )}
            {control.storageError ? (
              <Button
                ref={recovery}
                type="button"
                disabled={control.busy}
                onClick={() => void control.restoreStorage()}
              >
                恢复保存控制存储
              </Button>
            ) : (
              control.intent && (
                <Button
                  ref={recovery}
                  type="button"
                  disabled={control.busy}
                  onClick={() => void control.replay()}
                >
                  人工使用原键核验控制
                </Button>
              )
            )}
          </section>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={blocking}
            onClick={onClose}
          >
            关闭保存记录
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
