"use client";
import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { libraryKey } from "./library-queries";
import {
  getLibraryPurge,
  listLibraryPurges,
  reviewLibraryPurge,
} from "./library-purge-query";
import {
  purgeStatusLabels,
  purgeTerminal,
  type PurgeJob,
} from "./library-purge-model";
import { LibraryPurgeResults } from "./library-purge-results";
import type { LibraryIdentity } from "./library-model";
import type { PurgeWriter } from "./library-purge-writer";
export type PurgeFrame =
  | { mode: "selected"; items: { id: string; revision: number }[] }
  | { mode: "all" }
  | { mode: "history" };
export function LibraryPurgeDialog({
  identity,
  frame,
  writer,
  accepted,
  readOnly,
  onClose,
  onCloseAutoFocus,
  onObserved,
}: {
  identity: LibraryIdentity;
  frame?: PurgeFrame;
  writer: PurgeWriter;
  accepted?: PurgeJob;
  readOnly: boolean;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
  onObserved?: () => Promise<void>;
}) {
  const [confirmed, setConfirmed] = useState(false),
    [page, setPage] = useState(1),
    [chosen, setChosen] = useState<string>(),
    [control, setControl] = useState<{
      job: PurgeJob;
      action: "cancel" | "reconcile";
    }>();
  const recovery = Boolean(writer.intent || writer.storageError),
    canReview =
      frame &&
      frame.mode !== "history" &&
      !recovery &&
      !accepted &&
      !writer.plan;
  const review = useQuery({
    queryKey: [...libraryKey(identity), "purge-review", frame],
    queryFn: ({ signal }) =>
      reviewLibraryPurge(
        identity,
        frame?.mode === "selected" ? frame.items : undefined,
        signal,
      ),
    enabled: Boolean(canReview),
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const history = useQuery({
    queryKey: [...libraryKey(identity), "purges", page],
    queryFn: ({ signal }) => listLibraryPurges(identity, page, signal),
    enabled: !canReview && !recovery,
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  const jobId = chosen ?? accepted?.id;
  const current = useQuery({
    queryKey: [...libraryKey(identity), "purge-job", jobId],
    queryFn: ({ signal }) => getLibraryPurge(identity, jobId!, signal),
    enabled: Boolean(jobId) && !recovery,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchInterval: (query) =>
      query.state.data && !purgeTerminal(query.state.data) ? 2500 : false,
  });
  const job =
    !current.isError && current.isFetchedAfterMount ? current.data : undefined;
  const observed = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!job || !purgeTerminal(job) || !onObserved) return;
    const version = `${job.id}:${job.revision}`;
    if (observed.current === version) return;
    observed.current = version;
    void onObserved().catch(() => {
      observed.current = undefined;
    });
  }, [job, onObserved]);
  const locked = writer.busy || writer.running || recovery,
    closeLocked = locked || Boolean(writer.plan);
  const refreshReview = () => {
    setConfirmed(false);
    void review.refetch();
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !closeLocked) onClose();
      }}
    >
      <DialogContent
        data-library-dialog="purge"
        className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl"
        showCloseButton={!closeLocked}
        onCloseAutoFocus={onCloseAutoFocus}
        onEscapeKeyDown={(event) => {
          if (closeLocked) event.preventDefault();
        }}
        onPointerDownOutside={(event) => {
          if (closeLocked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>永久清理与任务记录</DialogTitle>
          <DialogDescription>
            范围：
            {identity.scope.kind === "personal"
              ? "当前个人素材库"
              : `项目 ${identity.scope.project_id}`}
            。只有实际对象清理确证后才报告成功和释放容量。
          </DialogDescription>
        </DialogHeader>
        {writer.error && (
          <Alert variant="destructive">
            <AlertTitle>原清理操作尚未完成</AlertTitle>
            <AlertDescription>{writer.error}</AlertDescription>
          </Alert>
        )}
        {writer.storageError && (
          <Alert variant="destructive">
            <AlertTitle>原清理存储需要恢复</AlertTitle>
            <AlertDescription>
              <p>{writer.storageError}</p>
              <Button
                variant="outline"
                disabled={writer.busy}
                onClick={() => void writer.restoreStorage()}
              >
                恢复原键存储
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {writer.intent && (
          <section className="space-y-3" aria-label="原永久清理意图">
            <p className="break-all">原键 {writer.intent.key}</p>
            <p>
              原动作：
              {writer.intent.action === "create"
                ? `永久清理 ${writer.intent.body.items.length} 项`
                : writer.intent.action === "cancel"
                  ? "请求取消"
                  : "对账未知对象结果"}
              。原范围与完整正文保持不变。
            </p>
            <Textarea
              aria-label="原清理完整正文"
              readOnly
              value={JSON.stringify(writer.intent.body, null, 2)}
              className="max-h-64"
            />
            {writer.rejected ? (
              <>
                <p>
                  服务器已明确拒绝原请求；需要重新读取当前事实并重新确认，旧版本不会自动替换。
                </p>
                <Button
                  variant="outline"
                  disabled={writer.busy}
                  onClick={() =>
                    void writer.releaseRejected().then((released) => {
                      if (released) onClose();
                    })
                  }
                >
                  确认结束被拒绝的原意图并重新审阅
                </Button>
              </>
            ) : (
              <>
                <p>结果尚未确认，不会自动发送。请明确使用原键核验原动作。</p>
                <Button
                  disabled={writer.busy || Boolean(writer.storageError)}
                  onClick={() => void writer.replay()}
                >
                  使用原键明确恢复原操作
                </Button>
              </>
            )}
          </section>
        )}
        {canReview && (
          <section className="space-y-4">
            <Alert variant="destructive">
              <AlertTitle>
                {frame.mode === "all"
                  ? "清空整个回收站"
                  : "永久删除全部所选素材"}
              </AlertTitle>
              <AlertDescription>
                永久清理不能撤销。当前或历史内容仍在引用的素材会保留并逐项报告；取消无法恢复已经清理的文件。清空全部包括其他分页及当前筛选之外的条目。
              </AlertDescription>
            </Alert>
            {review.isFetching ? (
              <p role="status">正在读取全部范围并核对版本…</p>
            ) : review.isError ? (
              <Alert variant="destructive">
                <AlertTitle>清理审阅已停止</AlertTitle>
                <AlertDescription>
                  <p>{review.error.message}</p>
                  <Button variant="outline" onClick={refreshReview}>
                    重新读取完整范围
                  </Button>
                </AlertDescription>
              </Alert>
            ) : review.data ? (
              <>
                <p>
                  完整审阅 {review.data.items.length} 项 · 素材库版本{" "}
                  {review.data.revision}
                  {review.data.projectRevision
                    ? ` · 项目版本 ${review.data.projectRevision}`
                    : ""}
                  。
                </p>
                <Textarea
                  aria-label="全部永久清理条目及冻结版本"
                  readOnly
                  value={review.data.items
                    .map(
                      (item) =>
                        `${item.title} · ${item.id} · 条目版本 ${item.revision}`,
                    )
                    .join("\n")}
                  className="max-h-60"
                />
                <label className="flex items-start gap-3">
                  <Checkbox
                    aria-label="我已审阅完整范围并确认永久删除不可恢复"
                    checked={confirmed}
                    disabled={readOnly || writer.locked}
                    onCheckedChange={(value) => setConfirmed(value === true)}
                  />
                  <span>我已审阅完整范围并确认永久删除不可恢复</span>
                </label>
                {frame.mode === "all" && (
                  <p>
                    完整计划最多2000项，每子批最多200项。当前{" "}
                    {Math.ceil(review.data.items.length / 200)}{" "}
                    个子批会在本次确认的前台会话串行完成；页面转后台、卸载或未知结果时立即停止，刷新后不会自动继续。引用阻断与已取消条目会保留并报告。
                  </p>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="destructive"
                    disabled={
                      !confirmed ||
                      readOnly ||
                      writer.locked ||
                      review.isFetching
                    }
                    onClick={() => {
                      const value = review.data!;
                      setConfirmed(false);
                      if (frame.mode === "all") void writer.startPlan(value);
                      else
                        void writer.submit({
                          action: "create",
                          body: {
                            scope: identity.scope,
                            items: value.items.map(({ id, revision }) => ({
                              id,
                              revision,
                            })),
                            expected_revision: value.revision,
                            expected_project_revision: value.projectRevision,
                            permanent_delete_confirmed: true,
                          },
                        });
                    }}
                  >
                    确认永久删除 {review.data.items.length} 项
                  </Button>
                  <Button
                    variant="outline"
                    disabled={writer.locked}
                    onClick={refreshReview}
                  >
                    重新读取并审阅
                  </Button>
                </div>
              </>
            ) : null}
          </section>
        )}
        {writer.plan && (
          <section
            className="space-y-3 rounded-lg border p-3"
            aria-label="完整回收站清理计划"
          >
            <p className="break-all">
              完整计划 {writer.plan.id} · 冻结 {writer.plan.targets.length} 项 /{" "}
              {writer.plan.batches.length} 子批
            </p>
            <p>
              全部目标、条目版本与子键已保存。每次继续会核对当前身份、整个回收站及所有原任务；任何新条目、正文或外部版本变化都停止。
            </p>
            <Textarea
              aria-label="完整计划冻结UUID与版本"
              readOnly
              value={writer.plan.targets
                .map((item) => `${item.id} · 条目版本 ${item.revision}`)
                .join("\n")}
              className="max-h-48"
            />
            <ul className="space-y-2">
              {writer.plan.batches.map((batch, index) => (
                <li key={batch.key} className="break-all">
                  子批 {index + 1} · 原键 {batch.key} ·{" "}
                  {batch.job
                    ? purgeStatusLabels[batch.job.status]
                    : batch.input
                      ? "原受理结果待核验"
                      : "尚未开始"}
                  {batch.job && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setChosen(batch.job!.id);
                        setControl(undefined);
                        setConfirmed(false);
                      }}
                    >
                      核对第{index + 1}子批结果
                    </Button>
                  )}
                </li>
              ))}
            </ul>
            {writer.running && (
              <p role="status">
                正在前台串行处理已确认完整计划，未开始子批不会在后台继续。
              </p>
            )}
            {writer.running && (
              <Button variant="outline" onClick={writer.stopPlan}>
                停止前台计划并保留原子键
              </Button>
            )}
            {writer.planComplete ? (
              <p role="status">
                全部子批已处理。引用阻断、取消或未完成条目保留，未自动重试；逐项结果以原任务为准。
              </p>
            ) : (
              <Button
                disabled={writer.busy || writer.running || recovery || readOnly}
                onClick={() => void writer.continuePlan()}
              >
                明确继续并核验下一子批
              </Button>
            )}
            <Button
              variant="outline"
              disabled={writer.busy || writer.running || recovery}
              onClick={() =>
                void writer.endPlan().then((ended) => {
                  if (ended) onClose();
                })
              }
            >
              结束原计划并重新审阅全部
            </Button>
          </section>
        )}
        {!canReview && !recovery && (
          <section className="space-y-4" aria-label="永久清理记录">
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={history.isFetching || writer.busy}
                onClick={() => void history.refetch()}
              >
                刷新清理记录
              </Button>
              <Button
                variant="outline"
                disabled={page <= 1 || writer.busy}
                onClick={() => setPage((old) => old - 1)}
              >
                上一页记录
              </Button>
              <Button
                variant="outline"
                disabled={
                  history.isFetching ||
                  history.isError ||
                  history.data?.length !== 20 ||
                  writer.busy
                }
                onClick={() => setPage((old) => old + 1)}
              >
                下一页记录
              </Button>
              <span>第 {page} 页</span>
            </div>
            {history.isFetching ? (
              <p role="status">正在读取清理记录…</p>
            ) : history.isError ? (
              <p role="alert">清理记录暂不可读取：{history.error.message}</p>
            ) : history.data?.length ? (
              <ul className="space-y-2">
                {history.data.map((record) => (
                  <li key={record.id}>
                    <Button
                      variant="outline"
                      className="h-auto w-full justify-start text-left whitespace-normal"
                      onClick={() => {
                        setChosen(record.id);
                        setControl(undefined);
                        setConfirmed(false);
                      }}
                    >
                      {new Date(record.created_at).toLocaleString("zh-CN")} ·{" "}
                      {purgeStatusLabels[record.status]} · {record.items.length}{" "}
                      项
                    </Button>
                  </li>
                ))}
              </ul>
            ) : (
              <p>此页没有清理记录。</p>
            )}
            {jobId && (
              <>
                <p className="break-all">任务 {jobId}</p>
                <Button
                  variant="outline"
                  disabled={current.isFetching}
                  onClick={() => {
                    setControl(undefined);
                    setConfirmed(false);
                    void current.refetch();
                  }}
                >
                  核对任务实际状态
                </Button>
                {current.isPending ? (
                  <p role="status">正在读取逐项实际结果…</p>
                ) : current.isError ? (
                  <p role="alert">当前任务不能读取：{current.error.message}</p>
                ) : job ? (
                  <>
                    <LibraryPurgeResults job={job} />
                    {!purgeTerminal(job) && !readOnly && (
                      <div className="flex flex-wrap gap-2">
                        <Button
                          variant="outline"
                          disabled={
                            writer.busy ||
                            writer.running ||
                            job.cancellation_requested
                          }
                          onClick={() => {
                            setControl({ job, action: "cancel" });
                            setConfirmed(false);
                          }}
                        >
                          审阅取消清理
                        </Button>
                        <Button
                          variant="outline"
                          disabled={
                            writer.busy ||
                            writer.running ||
                            ![
                              "needs_reconciliation",
                              "cancel_requested",
                              "running",
                            ].includes(job.status) ||
                            job.execution_unconfirmed
                          }
                          onClick={() => {
                            setControl({ job, action: "reconcile" });
                            setConfirmed(false);
                          }}
                        >
                          {job.needs_reconciliation
                            ? "审阅对账未知结果"
                            : "审阅执行状态与对账"}
                        </Button>
                      </div>
                    )}
                  </>
                ) : null}
              </>
            )}
            {control && (
              <section
                className="space-y-3 rounded-md border p-3"
                aria-label="清理控制确认"
              >
                <p>
                  {control.action === "cancel"
                    ? "请求停止尚未开始的清理；已经永久删除的原件不能恢复。"
                    : "核验原任务并对账对象结果，取消意图继续保留。服务器会先核验执行已结束与当前权限；仍在执行或状态变化时停止，不自动重试。"}{" "}
                  本次使用已观察状态版本 {control.job.revision}
                  ，发生变化时停止。
                </p>
                <label className="flex items-center gap-3">
                  <Checkbox
                    aria-label="明确确认此次任务控制动作"
                    checked={confirmed}
                    disabled={locked || writer.running || readOnly}
                    onCheckedChange={(value) => setConfirmed(value === true)}
                  />
                  明确确认此次任务控制动作
                </label>
                <Button
                  disabled={!confirmed || locked || writer.running || readOnly}
                  onClick={() => {
                    const original = control;
                    setControl(undefined);
                    setConfirmed(false);
                    void writer.submit({
                      action: original.action,
                      jobId: original.job.id,
                      body: { revision: original.job.revision },
                    });
                  }}
                >
                  {control.action === "cancel"
                    ? "确认请求取消"
                    : "确认对账原任务"}
                </Button>
              </section>
            )}
          </section>
        )}
        <Button variant="outline" disabled={closeLocked} onClick={onClose}>
          关闭清理窗口
        </Button>
      </DialogContent>
    </Dialog>
  );
}
