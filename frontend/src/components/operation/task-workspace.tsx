"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ProjectScope } from "@/components/workbench/project-scope";
import { formatCurrencyMicros } from "@/lib/money";
import {
  OPERATIONS_KEY,
  queryTasks,
  queryTask,
  requestTaskCancel,
  type Task,
} from "./queries";

export const taskStatusLabels: Record<Task["status"], string> = {
  draft: "草稿",
  quoted: "待确认报价",
  expired: "报价过期",
  confirmed: "排队中",
  submitting: "提交中",
  submitted: "生成中",
  unknown: "结果待核对",
  reconciling: "核对中",
  manual: "待人工处理",
  ingesting: "接收素材",
  cancelling: "取消处理中",
  succeeded: "生成成功",
  completed: "已完成",
  failed: "失败",
  cancelled: "已取消",
};
const activeStatuses = new Set([
  "confirmed",
  "submitting",
  "submitted",
  "unknown",
  "reconciling",
  "ingesting",
  "succeeded",
  "cancelling",
]);
export function TaskWorkspace() {
  return (
    <div className="max-w-6xl space-y-8">
      <div>
        <h1 className="text-3xl font-medium tracking-tight">任务</h1>
        <p className="mt-3 text-sm text-muted-foreground">
          查看生成进度、费用和结果，继续尚未完成的创作。
        </p>
        <Button asChild variant="ghost" className="mt-3">
          <Link href="/depths">视频深度任务</Link>
        </Button>
      </div>
      <ProjectScope>
        {(project) => <ProjectTasks projectId={project.id} />}
      </ProjectScope>
    </div>
  );
}
function ProjectTasks({ projectId }: { projectId: string }) {
  const search = useSearchParams();
  const [selected, setSelected] = useState<string | null>(
    search.get("task_id"),
  );
  const [status, setStatus] = useState("");
  const cache = useQueryClient();
  const tasks = useInfiniteQuery({
    queryKey: [...OPERATIONS_KEY, projectId, { status }],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      queryTasks(
        projectId,
        { status: status || undefined, cursor: pageParam },
        signal,
      ),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.pages.some((page) =>
        page.items.some((task) => activeStatuses.has(task.status)),
      )
        ? 5000
        : false,
  });
  const detail = useQuery({
    queryKey: [...OPERATIONS_KEY, projectId, selected],
    enabled: !!selected,
    queryFn: ({ signal }) => queryTask(projectId, selected!, signal),
    retry: false,
    refetchInterval: (query) =>
      query.state.data && activeStatuses.has(query.state.data.status)
        ? 5000
        : false,
  });
  const cancel = useMutation({
    mutationFn: ({ id, key }: { id: string; key: string }) =>
      requestTaskCancel(projectId, id, key),
    retry: false,
    onSuccess: () =>
      cache.invalidateQueries({ queryKey: [...OPERATIONS_KEY, projectId] }),
  });
  const items = tasks.data?.pages.flatMap((page) => page.items) ?? [];
  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <select
          aria-label="任务状态"
          value={status}
          onChange={(event) => setStatus(event.target.value)}
          className="h-10 rounded-lg border bg-background px-3 text-sm"
        >
          <option value="">全部状态</option>
          {Object.entries(taskStatusLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
        <Button
          variant="outline"
          onClick={() => void tasks.refetch()}
          disabled={tasks.isFetching}
        >
          刷新
        </Button>
      </div>
      {tasks.isError ? (
        <p role="alert">任务读取失败，请重新读取。</p>
      ) : tasks.isPending ? (
        <p role="status">正在载入任务…</p>
      ) : items.length === 0 ? (
        <div className="rounded-xl bg-muted/40 p-10 text-center">
          <p className="mb-4 text-muted-foreground">当前项目还没有生成任务。</p>
          <Button asChild>
            <Link href={`/create?project_id=${projectId}`}>开始创作</Link>
          </Button>
        </div>
      ) : (
        <ul className="divide-y">
          {items.map((task) => (
            <li key={task.id}>
              <button
                className="flex w-full flex-wrap items-center justify-between gap-3 py-5 text-left hover:bg-muted/30 focus-visible:outline-2"
                onClick={() => setSelected(task.id)}
              >
                <div>
                  <p className="font-medium">
                    {task.model_name || task.model_key}{" "}
                    <span className="ml-2 text-xs font-normal text-muted-foreground">
                      {task.mode}
                    </span>
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {new Date(task.create_time).toLocaleString("zh-CN")}
                  </p>
                </div>
                <div className="text-right">
                  <p className="text-sm">
                    {task.cancel_requested &&
                    !["cancelled", "completed", "failed"].includes(task.status)
                      ? "已请求取消"
                      : taskStatusLabels[task.status]}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {task.settled_micros === null
                      ? task.quote_micros === null
                        ? "费用待核对"
                        : `报价 ${formatCurrencyMicros(task.quote_micros)}`
                      : formatCurrencyMicros(task.settled_micros)}
                  </p>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
      {tasks.hasNextPage && (
        <Button
          variant="outline"
          disabled={tasks.isFetchingNextPage}
          onClick={() => void tasks.fetchNextPage()}
        >
          加载更多任务
        </Button>
      )}
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null);
            cancel.reset();
          }
        }}
      >
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>任务详情</DialogTitle>
            <DialogDescription>
              {detail.data
                ? taskStatusLabels[detail.data.status]
                : "读取任务与候选结果"}
            </DialogDescription>
          </DialogHeader>
          {detail.isPending ? (
            <p role="status">载入中…</p>
          ) : detail.isError ? (
            <div role="alert">
              <p>详情读取失败。</p>
              <Button variant="outline" onClick={() => void detail.refetch()}>
                重试
              </Button>
            </div>
          ) : detail.data ? (
            <div className="space-y-6">
              <div>
                <p className="text-sm text-muted-foreground">提示词</p>
                <p className="mt-2 break-words whitespace-pre-wrap">
                  {detail.data.inputs.find((input) => input.role === "prompt")
                    ?.text ?? "无文本输入"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">模型与参数</p>
                <p className="mt-2">
                  {detail.data.model_name || detail.data.model_key}
                </p>
                <pre className="mt-2 max-h-40 overflow-auto rounded-lg bg-muted p-3 text-xs">
                  {JSON.stringify(detail.data.params, null, 2)}
                </pre>
              </div>
              {detail.data.failure_code && (
                <p role="status" className="text-sm text-destructive">
                  处理失败：{detail.data.failure_code}
                </p>
              )}
              <div>
                <h2 className="mb-2 text-sm font-medium">候选结果</h2>
                {detail.data.outputs.length ? (
                  <ul className="space-y-2">
                    {detail.data.outputs.map((output) => (
                      <li
                        key={output.id}
                        className="flex items-center justify-between rounded-lg bg-muted/40 p-3 text-sm"
                      >
                        <span>
                          结果 {output.sequence} · {output.kind}
                        </span>
                        {output.media_asset_id &&
                        output.moderation_status === "passed" ? (
                          <Link
                            href={`/assets?project_id=${projectId}&asset_id=${output.media_asset_id}`}
                            className="underline underline-offset-4"
                          >
                            查看素材
                          </Link>
                        ) : (
                          <span className="text-muted-foreground">
                            {output.moderation_status === "rejected"
                              ? "未通过审核"
                              : "等待素材接收与审核"}
                          </span>
                        )}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-muted-foreground">尚无候选。</p>
                )}
              </div>
              <ol
                className="space-y-2 text-xs text-muted-foreground"
                aria-label="处理记录"
              >
                {detail.data.events.map((event) => (
                  <li key={event.id}>
                    {new Date(event.create_time).toLocaleString("zh-CN")} ·{" "}
                    {taskStatusLabels[event.to_status as Task["status"]] ??
                      event.to_status}
                    {event.reason ? ` · ${event.reason}` : ""}
                  </li>
                ))}
              </ol>
              {["confirmed", "submitted"].includes(detail.data.status) && (
                <Button
                  variant="outline"
                  disabled={cancel.isPending || detail.data.cancel_requested}
                  onClick={() =>
                    cancel.mutate({
                      id: detail.data.id,
                      key: crypto.randomUUID(),
                    })
                  }
                >
                  {detail.data.cancel_requested
                    ? "已请求取消"
                    : cancel.isPending
                      ? "提交取消请求…"
                      : "请求取消"}
                </Button>
              )}
              {cancel.isError && (
                <p role="alert" className="text-sm text-destructive">
                  取消请求未确认，请刷新任务后重试。
                </p>
              )}
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
