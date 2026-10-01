"use client";
import { useRef, useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  queryTask,
  requestTaskCancel,
  OPERATIONS_KEY,
  type Task,
} from "@/components/operation/queries";
import { taskStatusLabels } from "@/components/operation/task-workspace";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { getMediaPreview, type MediaAsset } from "./queries";
export function BatchRowTask({
  projectId,
  task,
  disabled,
  enabled,
  onRetry,
  onResults,
}: {
  projectId: string;
  task?: Task;
  disabled: boolean;
  enabled: boolean;
  onRetry: () => void;
  onResults: (assets: MediaAsset[]) => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false),
    [importing, setImporting] = useState(false),
    [error, setError] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const cancellation = useRef<{ id: string; key: string } | null>(null);
  const cache = useQueryClient();
  const detail = useQuery({
    queryKey: [...OPERATIONS_KEY, projectId, task?.id],
    enabled: !!task && open,
    queryFn: ({ signal }) => queryTask(projectId, task!.id, signal),
    retry: false,
    refetchInterval:
      task &&
      !["completed", "failed", "cancelled", "expired"].includes(task.status)
        ? 5000
        : false,
  });
  if (!task)
    return <span className="text-xs text-muted-foreground">未生成</span>;
  const ready =
    detail.data?.outputs.filter(
      (output) =>
        output.media_asset_id && output.moderation_status === "passed",
    ) ?? [];
  return (
    <div className="space-y-1">
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
        {taskStatusLabels[task.status]}
      </Button>
      {enabled && (task.status === "failed" || task.status === "cancelled") ? (
        <Button
          size="sm"
          variant="outline"
          disabled={disabled}
          onClick={onRetry}
        >
          重试此行
        </Button>
      ) : null}
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!importing) setOpen(value);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>批量行任务</DialogTitle>
            <DialogDescription>
              {taskStatusLabels[task.status]}
            </DialogDescription>
          </DialogHeader>
          <Link
            href={`/tasks?project_id=${projectId}&task_id=${task.id}`}
            className="text-sm underline"
          >
            查看完整任务、费用与事件
          </Link>
          {detail.isFetching ? <p role="status">正在读取运行结果…</p> : null}
          {detail.error ? (
            <p role="alert">
              任务读取失败。
              <Button variant="ghost" onClick={() => void detail.refetch()}>
                重试
              </Button>
            </p>
          ) : null}
          {[
            "confirmed",
            "submitting",
            "submitted",
            "unknown",
            "reconciling",
            "ingesting",
            "succeeded",
          ].includes(task.status) ? (
            <Button
              variant="outline"
              disabled={disabled || cancelling}
              onClick={() => {
                if (cancellation.current?.id !== task.id)
                  cancellation.current = {
                    id: task.id,
                    key: crypto.randomUUID(),
                  };
                setCancelling(true);
                setError("");
                void requestTaskCancel(
                  projectId,
                  task.id,
                  cancellation.current.key,
                )
                  .then(() =>
                    cache.invalidateQueries({
                      queryKey: [...OPERATIONS_KEY, projectId],
                    }),
                  )
                  .catch(() => setError("取消尚未确认，请重试读取任务状态。"))
                  .finally(() => setCancelling(false));
              }}
            >
              {cancelling ? "正在请求取消…" : "取消这行任务"}
            </Button>
          ) : null}
          {detail.data ? (
            <div className="space-y-2">
              <p className="text-sm">
                {detail.data.outputs.length} 个输出 · {ready.length} 个可用素材
              </p>
              {detail.data.failure_code ? (
                <p className="text-sm text-destructive">
                  {detail.data.failure_code}
                </p>
              ) : null}
              <Button
                disabled={disabled || importing || !ready.length}
                onClick={() => {
                  setImporting(true);
                  setError("");
                  void Promise.all(
                    ready.map(async (output) => {
                      const preview = await getMediaPreview(
                        projectId,
                        output.media_asset_id!,
                      );
                      if (
                        preview.asset.id !== output.media_asset_id ||
                        preview.asset.project_id !== projectId ||
                        preview.asset.kind !== output.kind
                      )
                        throw new Error("任务输出资产与当前项目不匹配。");
                      return preview.asset;
                    }),
                  )
                    .then(onResults)
                    .then((saved) => {
                      if (saved) setOpen(false);
                      else setError("画布未确认保存，请修复错误后重试。");
                    })
                    .catch((failure: unknown) =>
                      setError(
                        failure instanceof Error
                          ? failure.message
                          : "结果加入画布失败。",
                      ),
                    )
                    .finally(() => setImporting(false));
                }}
              >
                {importing ? "加入画布中…" : "将可用结果加入画布"}
              </Button>
            </div>
          ) : null}
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
