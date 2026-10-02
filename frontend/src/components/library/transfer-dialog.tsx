"use client";
import { useState } from "react";
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
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { LibraryIdentity } from "@/components/media/library-model";
import type { ProjectListedSummary } from "@/components/project/queries";
import { listTransfers, getTransfer, transferKey } from "./transfer-queries";
import { transferActive, transferControl } from "./transfer-model";
import { TransferForm, type TransferSelection } from "./transfer-form";
import { TransferProgress } from "./transfer-progress";
import { TransferJobList } from "./transfer-job-list";
import { useTransfer } from "./use-transfer";
export function TransferDialog({
  identity,
  selection,
  projects,
  hasMoreProjects,
  projectsLoading,
  onMoreProjects,
  onClose,
  onCloseAutoFocus,
  onChanged,
}: {
  identity: LibraryIdentity;
  selection?: TransferSelection;
  projects: ProjectListedSummary[];
  hasMoreProjects: boolean;
  projectsLoading: boolean;
  onMoreProjects: () => void;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
  onChanged: () => Promise<void>;
}) {
  const cache = useQueryClient(),
    [focused, setFocused] = useState<string>(),
    [accepted, setAccepted] = useState(false);
  const writer = useTransfer(identity, async (receipt) => {
    setFocused(receipt.id);
    setAccepted(true);
    await Promise.all([
      cache.invalidateQueries({ queryKey: transferKey(identity) }),
      onChanged(),
    ]);
  });
  const list = useInfiniteQuery({
    queryKey: [...transferKey(identity), "list"],
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      listTransfers(identity, pageParam, signal),
    getNextPageParam: (page) =>
      page.items.length === page.page_size && page.page < 10000
        ? page.page + 1
        : undefined,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchInterval: (query) =>
      query.state.data?.pages.some((page) => page.items.some(transferActive))
        ? 3000
        : false,
  });
  const jobId =
    writer.intent && writer.intent.action !== "create"
      ? writer.intent.jobId
      : focused;
  const detail = useQuery({
    queryKey: [...transferKey(identity), "job", jobId],
    queryFn: ({ signal }) => getTransfer(identity, jobId!, signal),
    enabled: Boolean(jobId),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchInterval: (query) =>
      query.state.data && transferActive(query.state.data) ? 2000 : false,
  });
  const rows =
      list.isFetchedAfterMount && list.isSuccess
        ? list.data.pages.flatMap((page) => page.items)
        : [],
    duplicate = new Set(rows.map((job) => job.id)).size !== rows.length;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !writer.locked) onClose();
      }}
    >
      <DialogContent
        data-library-dialog
        showCloseButton={!writer.locked}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-5xl"
        onCloseAutoFocus={onCloseAutoFocus}
        onEscapeKeyDown={(event) => {
          if (writer.locked) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (writer.locked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>素材迁移任务</DialogTitle>
          <DialogDescription>
            个人库与项目库之间复制独立原件和衍生物。受理回执不是完成结果；成功行保留，失败行单独重试。
          </DialogDescription>
        </DialogHeader>
        {accepted && (
          <p role="status">
            原请求已受理，正在读取当前任务。永久回执不覆盖当前状态。
          </p>
        )}
        {writer.error && <p role="alert">{writer.error}</p>}
        {writer.storageError && (
          <div role="alert">
            <p>{writer.storageError}</p>
            <Button
              variant="outline"
              disabled={writer.busy}
              onClick={() => void writer.restoreStorage()}
            >
              恢复原迁移存储
            </Button>
          </div>
        )}
        {writer.intent && (
          <section
            aria-label="未确认迁移原意图"
            className="space-y-2 rounded-lg border p-3"
          >
            <p className="break-all">原键 {writer.intent.key}</p>
            <p>
              {writer.intent.action === "create"
                ? `迁移 ${writer.intent.body.items.length} 项，来源库版本 ${writer.intent.body.expected_source_revision}`
                : `任务 ${writer.intent.jobId} · ${writer.intent.action} · 原修订 ${writer.intent.body.revision}`}
            </p>
            <p>保留同一原键和完整原正文，刷新不会自动提交。</p>
            {!writer.rejected ? (
              <Button
                disabled={writer.busy || Boolean(writer.storageError)}
                onClick={() => void writer.replay()}
              >
                使用原键与原正文核验
              </Button>
            ) : (
              <>
                <p>
                  服务端已明确拒绝。重新读取当前事实后可释放此被拒意图，再人工修改表单。
                </p>
                <Button
                  variant="outline"
                  disabled={writer.busy}
                  onClick={() => void writer.readLatest()}
                >
                  读取最新迁移事实
                </Button>
                <Button
                  variant="outline"
                  disabled={writer.busy || !writer.reviewed}
                  onClick={() => void writer.releaseRejected()}
                >
                  审阅后释放被拒意图
                </Button>
              </>
            )}
          </section>
        )}
        {selection && !accepted && (
          <TransferForm
            identity={identity}
            selection={selection}
            projects={projects}
            hasMoreProjects={hasMoreProjects}
            projectsLoading={projectsLoading}
            onMoreProjects={onMoreProjects}
            writer={writer}
          />
        )}
        <div className="grid min-w-0 gap-4 md:grid-cols-[260px_minmax(0,1fr)]">
          <section aria-label="迁移任务分页" className="min-w-0 space-y-2">
            {list.isPending && <p role="status">正在读取迁移任务…</p>}
            {(list.error || duplicate) && (
              <p role="alert">
                {duplicate
                  ? "任务分页身份重复，请重新读取。"
                  : list.error?.message}
              </p>
            )}
            {!list.isPending && !rows.length && !list.error && (
              <p>当前尚无迁移任务。</p>
            )}
            {!duplicate && (
              <TransferJobList
                jobs={rows}
                focused={jobId}
                busy={writer.busy}
                onSelect={setFocused}
              />
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={list.isFetching}
                onClick={() => void list.refetch()}
              >
                刷新迁移任务
              </Button>
              {list.hasNextPage && (
                <Button
                  variant="outline"
                  disabled={list.isFetchingNextPage || duplicate}
                  onClick={() => void list.fetchNextPage()}
                >
                  加载下一页迁移任务
                </Button>
              )}
            </div>
          </section>
          <section className="min-w-0">
            {jobId && !detail.isFetchedAfterMount ? (
              <p role="status">正在读取当前任务事实…</p>
            ) : detail.isError ? (
              <p role="alert">
                {detail.error.message}
                <Button variant="outline" onClick={() => void detail.refetch()}>
                  重新读取当前任务
                </Button>
              </p>
            ) : (
              detail.data &&
              detail.isFetchedAfterMount && (
                <>
                  <TransferProgress
                    job={detail.data}
                    disabled={writer.locked || detail.isFetching}
                    onControl={(action) =>
                      void writer.submit(transferControl(detail.data!, action))
                    }
                  />
                  <Button variant="ghost" onClick={() => void detail.refetch()}>
                    刷新此任务
                  </Button>
                </>
              )
            )}
          </section>
        </div>
        <div className="flex justify-end">
          <Button variant="outline" disabled={writer.locked} onClick={onClose}>
            关闭迁移任务
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
