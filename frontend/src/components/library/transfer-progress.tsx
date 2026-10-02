"use client";
import Link from "next/link";
import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { transferActions, type TransferJob } from "./transfer-model";
export const transferStatusLabels: Record<TransferJob["status"], string> = {
  queued: "排队",
  running: "处理中",
  needs_reconciliation: "待核验",
  succeeded: "全部完成",
  partial_failed: "部分失败",
  failed: "失败",
  cancel_requested: "取消处理中",
  cancelled: "已取消",
};
const stages: Record<TransferJob["stage"], string> = {
  frozen: "已冻结来源",
  copying: "复制原件与衍生物",
  registering: "核验并发布目标",
  cleanup: "核验与清理未发布对象",
  completed: "本次尝试结束",
};
const itemStatus = {
  queued: "排队",
  running: "处理中",
  succeeded: "已发布",
  failed: "失败",
  needs_reconciliation: "待核验",
  cancelled: "已取消",
};
const failures: Record<string, string> = {
  source_changed: "来源版本已变化",
  target_folder_changed: "目标目录已变化",
  object_receipt_unknown: "对象回执待核验",
  object_write_unknown: "对象写入待核验",
  object_cleanup_unknown: "对象清理待核验",
  object_absence_unknown: "对象是否存在尚未确认",
  object_mismatch: "对象内容不符",
  source_unavailable: "来源不可读取",
  cancelled: "已取消",
  worker_interrupted: "处理被中断",
};
export function TransferProgress({
  job,
  disabled,
  onControl,
}: {
  job: TransferJob;
  disabled: boolean;
  onControl: (action: "cancel" | "retry" | "reconcile") => void;
}) {
  const gates = transferActions(job),
    succeeded = job.items.filter((item) => item.status === "succeeded").length;
  return (
    <section aria-label="迁移任务详情" className="min-w-0 space-y-4">
      <div className="flex flex-wrap gap-2">
        <Badge>{transferStatusLabels[job.status]}</Badge>
        <span>
          尝试 {job.attempt} · 修订 {job.revision}
        </span>
      </div>
      <p role="status">
        {stages[job.stage]} · 已发布 {succeeded} / {job.items.length} 项
      </p>
      <p className="text-sm text-muted-foreground">
        {job.source.kind === "personal"
          ? "个人素材库"
          : `来源项目 ${job.source.project_id}`}{" "}
        →{" "}
        {job.target.kind === "personal"
          ? "个人素材库"
          : `目标项目 ${job.target.project_id}`}
        。来源保留，已发布的成功条目不会被取消撤回。
      </p>
      {job.execution_unconfirmed && (
        <p role="alert">
          实际执行是否停止尚未确认。任务围栏保留，当前不能重试或重新核验。
        </p>
      )}
      {job.needs_reconciliation && (
        <p role="alert">
          存在需要核验的原对象事实。请显式核验，不能改用新任务重复复制。
        </p>
      )}
      {job.cancellation_requested && (
        <p role="status">
          {job.stage === "completed" &&
          !job.execution_unconfirmed &&
          !job.needs_reconciliation
            ? "取消处理已结束，已发布的成功条目保留。"
            : "取消意图已受理，等待实际停止与未发布对象清理。"}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={disabled || !gates.cancel}
          onClick={() => onControl("cancel")}
        >
          取消未完成迁移
        </Button>
        <Button
          variant="outline"
          disabled={disabled || !gates.retry}
          onClick={() => onControl("retry")}
        >
          重试失败项
        </Button>
        <Button
          variant="outline"
          disabled={disabled || !gates.reconcile}
          onClick={() => onControl("reconcile")}
        >
          核验原对象
        </Button>
      </div>
      <TransferItemResults job={job} disabled={disabled} />
    </section>
  );
}

function TransferItemResults({
  job,
  disabled,
}: {
  job: TransferJob;
  disabled: boolean;
}) {
  const scroll = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual owns DOM measurement; this component is not manually memoized.
  const virtual = useVirtualizer({
    count: job.items.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 140,
    getItemKey: (index) => job.items[index].source_item_id,
    overscan: 4,
    enabled: job.items.length > 100,
  });
  if (job.items.length > 100)
    return (
      <div
        ref={scroll}
        role="list"
        aria-label="逐项迁移结果"
        className="h-96 overflow-auto"
      >
        <div className="relative" style={{ height: virtual.getTotalSize() }}>
          {virtual.getVirtualItems().map((row) => (
            <div
              key={row.key}
              role="listitem"
              data-index={row.index}
              ref={virtual.measureElement}
              className="absolute top-0 left-0 w-full pb-3"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              <TransferItemResult
                job={job}
                disabled={disabled}
                item={job.items[row.index]}
              />
            </div>
          ))}
        </div>
      </div>
    );
  return (
    <ol className="max-h-96 space-y-3 overflow-auto" aria-label="逐项迁移结果">
      {job.items.map((item) => (
        <li key={item.source_item_id}>
          <TransferItemResult job={job} disabled={disabled} item={item} />
        </li>
      ))}
    </ol>
  );
}
function TransferItemResult({
  job,
  item,
  disabled,
}: {
  job: TransferJob;
  item: TransferJob["items"][number];
  disabled: boolean;
}) {
  return (
    <div className="rounded-lg border p-3 text-sm">
      <p>
        第 {item.index + 1} 项 · {itemStatus[item.status]}
      </p>
      <p className="break-all">来源 {item.source_item_id}</p>
      <p className="break-all">
        目标 {item.target_item_id}
        {item.status !== "succeeded" ? "（尚未发布）" : ""}
      </p>
      {item.failure_code && (
        <p>
          {failures[item.failure_code] ?? `服务端原因：${item.failure_code}`}
        </p>
      )}
      {item.status === "succeeded" && !disabled && (
        <Link
          className="underline underline-offset-4"
          href={
            job.target.kind === "project"
              ? `/assets?scope=project&project_id=${job.target.project_id}&asset_id=${item.target_item_id}`
              : `/assets?scope=personal&asset_id=${item.target_item_id}`
          }
        >
          打开已发布目标素材
        </Link>
      )}
    </div>
  );
}
