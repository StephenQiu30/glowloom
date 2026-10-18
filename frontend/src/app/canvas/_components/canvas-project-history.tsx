"use client";

import { useEffect, useState } from "react";
import { listProjectOperations } from "@/api/operations";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { taskLabels } from "@/components/generation/contracts";

export function CanvasProjectHistory({
  projectID,
  onClose,
}: {
  projectID: string;
  onClose: () => void;
}) {
  const [tasks, setTasks] = useState<API.TaskSummary[] | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const abort = new AbortController();
    listProjectOperations({ pid: projectID }, { signal: abort.signal })
      .then((value) => {
        if (abort.signal.aborted) return;
        if (
          !Array.isArray(value.items) ||
          value.items.some(
            (task) =>
              !task.id ||
              !task.status ||
              task.project_id !== projectID ||
              !Object.hasOwn(taskLabels, task.status),
          )
        )
          throw new Error("invalid_history");
        setTasks(value.items);
        setError("");
      })
      .catch(() => {
        if (!abort.signal.aborted) setError("生成历史读取失败，请重试。");
      });
    return () => abort.abort();
  }, [projectID, retry]);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogTitle>生成历史</DialogTitle>
        <DialogDescription>当前项目的真实生成任务</DialogDescription>
        {error ? (
          <div role="alert">
            <p>{error}</p>
            <Button onClick={() => setRetry((v) => v + 1)}>重试</Button>
          </div>
        ) : tasks === null ? (
          <Spinner aria-label="正在读取生成历史" />
        ) : tasks.length === 0 ? (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>暂无生成记录</EmptyTitle>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className="flex max-h-[60dvh] flex-col gap-3 overflow-y-auto">
            {tasks.map((task) => (
              <div
                key={task.id}
                className="flex items-center justify-between gap-3"
              >
                <span>
                  {task.model_name ||
                    (task.capability === "video"
                      ? "视频生成"
                      : task.capability === "image"
                        ? "图片生成"
                        : "生成任务")}
                </span>
                <span>
                  {task.status
                    ? (taskLabels[task.status] ?? task.status)
                    : "状态未知"}
                </span>
              </div>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
