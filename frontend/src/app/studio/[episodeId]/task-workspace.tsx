import { AlertCircle, CheckCircle2, Clock3, RefreshCw } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

import { taskStatusLabels, taskTone } from "./episode-studio-model";

const taskTypeLabels: Record<API.TaskResponse["task_type"], string> = {
  script_extraction: "剧本结构提取",
  media_probe: "媒体探测",
};

export function TaskWorkspace({ tasks }: { tasks: API.TaskResponse[] }) {
  const running = tasks.filter((task) =>
    ["queued", "running", "waiting_provider"].includes(task.status),
  ).length;
  const failed = tasks.filter((task) => ["failed", "unknown"].includes(task.status)).length;

  return (
    <div className="grid gap-6">
      <div className="grid gap-4 sm:grid-cols-3">
        <Card><CardHeader><CardDescription>全部任务</CardDescription><CardTitle className="text-3xl">{tasks.length}</CardTitle></CardHeader></Card>
        <Card><CardHeader><CardDescription>进行中</CardDescription><CardTitle className="text-3xl text-[#087f91]">{running}</CardTitle></CardHeader></Card>
        <Card><CardHeader><CardDescription>需处理</CardDescription><CardTitle className="text-3xl text-rose-700">{failed}</CardTitle></CardHeader></Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>任务时间线</CardTitle>
          <CardDescription>刷新页面后仍从后端 Task 事实恢复，不读取消息队列猜测状态。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {tasks.length === 0 ? (
            <Alert>
              <Clock3 aria-hidden="true" />
              <AlertTitle>还没有任务</AlertTitle>
              <AlertDescription>发布剧本后启动结构提取，或上传媒体开始探测。</AlertDescription>
            </Alert>
          ) : (
            tasks.map((task) => (
              <article className="grid gap-3 rounded-xl border border-slate-200 p-4 md:grid-cols-[1fr_auto] md:items-center" key={task.id}>
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-medium">{taskTypeLabels[task.task_type]}</h3>
                    <Badge className={taskTone(task.status)} variant="outline">
                      {taskStatusLabels[task.status]}
                    </Badge>
                  </div>
                  <p className="mt-1 text-sm text-slate-500">
                    阶段：{task.progress_stage} · revision {task.revision}
                  </p>
                  {task.error ? (
                    <p className="mt-2 text-sm text-rose-700">
                      {task.error.summary} · {task.next_action ?? "等待人工处理"}
                    </p>
                  ) : null}
                </div>
                <div className="flex items-center gap-2 text-sm text-slate-500">
                  {task.status === "succeeded" ? <CheckCircle2 className="size-4 text-emerald-600" aria-hidden="true" /> : task.status === "failed" || task.status === "unknown" ? <AlertCircle className="size-4 text-rose-600" aria-hidden="true" /> : <RefreshCw className="size-4 animate-spin text-[#079db3]" aria-hidden="true" />}
                  {task.request_type === "extraction_batch" ? "提取批次" : "媒体版本"}
                </div>
              </article>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}
