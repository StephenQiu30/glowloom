"use client";
import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { purgeStatusLabels, type PurgeJob } from "./library-purge-model";
const failureLabels = {
  in_use: "正在被当前或历史内容引用，已保留原件",
  source_unavailable: "原件暂时不可核验",
  object_mismatch: "原件完整性核验失败",
  object_remove_unknown: "对象清理结果未知",
  worker_interrupted: "执行已中断，需核对原任务",
  cancelled: "此条目清理已取消",
};
function ResultRow({ item }: { item: PurgeJob["items"][number] }) {
  return (
    <div className="min-w-0 rounded-md border p-3 text-sm">
      <p className="break-all">条目 {item.item_id}</p>
      <p>
        {purgeStatusLabels[item.status]}
        {item.failure_code ? ` · ${failureLabels[item.failure_code]}` : ""}
      </p>
      {item.status === "needs_reconciliation" && (
        <p>对象结果尚未确认，保留原清理任务，需明确对账。</p>
      )}
    </div>
  );
}
export function LibraryPurgeResults({ job }: { job: PurgeJob }) {
  const scroll = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual拥有DOM测量回调；此组件不手动memo。
  const virtual = useVirtualizer({
    count: job.items.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 96,
    getItemKey: (index) => job.items[index].item_id,
    overscan: 5,
    enabled: job.items.length > 100,
  });
  return (
    <div className="space-y-3" aria-label="永久清理逐项结果">
      <p role="status">
        {purgeStatusLabels[job.status]} · 尝试 {job.attempt} · 状态版本{" "}
        {job.revision}
      </p>
      <p>
        已永久清理{" "}
        {job.items.filter((item) => item.status === "succeeded").length} /{" "}
        {job.items.length} 项；引用阻断{" "}
        {job.items.filter((item) => item.status === "blocked").length}{" "}
        项；已取消{" "}
        {job.items.filter((item) => item.status === "cancelled").length} 项。
      </p>
      {job.cancellation_requested && (
        <p>
          取消意图已保存；已清理的对象不能恢复，尚未核实停止的任务会继续显示实际状态。
        </p>
      )}
      {job.execution_unconfirmed && (
        <p role="alert">执行是否结束尚未确认，请保留此任务并等待核实。</p>
      )}
      {job.items.length > 100 ? (
        <div
          ref={scroll}
          tabIndex={0}
          className="h-72 overflow-auto"
          aria-label="清理结果滚动列表"
        >
          <ul className="relative" style={{ height: virtual.getTotalSize() }}>
            {virtual.getVirtualItems().map((row) => (
              <li
                key={row.key}
                ref={virtual.measureElement}
                data-index={row.index}
                className="absolute top-0 left-0 w-full pb-2"
                style={{ transform: `translateY(${row.start}px)` }}
              >
                <ResultRow item={job.items[row.index]} />
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <ul className="space-y-2">
          {job.items.map((item) => (
            <li key={item.item_id}>
              <ResultRow item={item} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
