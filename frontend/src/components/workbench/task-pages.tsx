"use client";

import Link from "next/link";
import {
  ArrowUpRight,
  CheckCircle2,
  CircleAlert,
  Clock3,
  ListVideo,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { tasks } from "./data";
import {
  PageHeading,
  Panel,
  DataTable,
  EmptyMessage,
  usePreviewQuery,
} from "./workbench-components";

const filters = {
  all: "全部",
  succeeded: "已完成",
  failed: "失败",
  unknown: "结果未知",
  partial: "部分失败",
};
const taskIcons = {
  succeeded: CheckCircle2,
  failed: CircleAlert,
  unknown: Clock3,
  partial: ListVideo,
};
export function TaskListPage() {
  const { params, set } = usePreviewQuery();
  const requestedStatus = params.get("status") ?? "all";
  const status = Object.keys(filters).includes(requestedStatus)
    ? requestedStatus
    : "all";
  const q = params.get("q") ?? "";
  const search = q.trim().toLocaleLowerCase();
  const list = tasks.filter(
    (t) =>
      (status === "all" || t.state === status) &&
      `${t.title} ${t.project} ${t.id}`.toLocaleLowerCase().includes(search),
  );
  return (
    <div className="space-y-6">
      <PageHeading
        title="任务中心"
        description="查看生成进度、结果与费用。提交结果未知时先核对，再决定后续操作。"
      />
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <ToggleGroup
          type="single"
          aria-label="任务状态"
          value={status}
          onValueChange={(v) => {
            if (v) set("status", v);
          }}
          className="max-w-full flex-wrap gap-1"
        >
          {Object.entries(filters).map(([key, label]) => (
            <ToggleGroupItem
              key={key}
              value={key}
              className="data-[state=on]:bg-primary/10 data-[state=on]:text-primary"
            >
              {label}
              <span className="ml-1 text-xs text-muted-foreground">
                {key === "all"
                  ? tasks.length
                  : tasks.filter((task) => task.state === key).length}
              </span>
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className="relative w-full lg:max-w-xs">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
          />
          <Input
            className="h-9 rounded-full border-0 bg-muted/50 pl-9"
            aria-label="搜索任务"
            placeholder="搜索任务或项目…"
            value={q}
            onChange={(e) => set("q", e.target.value)}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {list.length} 个任务 · 样例记录
      </p>
      {list.length === 0 ? (
        <EmptyMessage
          title="没有匹配的任务"
          description="尝试其他状态或搜索词。"
        />
      ) : (
        <ul aria-label="任务列表" className="space-y-2">
          {list.map((task) => {
            const Icon = taskIcons[task.state as keyof typeof taskIcons];
            return (
              <li key={task.id}>
                <Link
                  href={`/tasks/${task.id}`}
                  className="group @container flex items-start gap-4 rounded-xl bg-card p-4 transition-colors outline-none hover:bg-muted/70 focus-visible:ring-2 focus-visible:ring-ring sm:items-center sm:p-5"
                >
                  <span
                    className={`flex size-11 shrink-0 items-center justify-center rounded-xl ${task.state === "failed" ? "bg-destructive/10 text-destructive" : "bg-muted text-primary"}`}
                  >
                    <Icon aria-hidden="true" className="size-5" />
                  </span>
                  <div className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto] gap-3 @2xl:grid-cols-[minmax(160px,1fr)_140px_115px_90px] @2xl:items-center">
                    <div className="col-span-2 min-w-0 @2xl:col-span-1">
                      <h2 className="truncate text-sm font-medium">
                        {task.title}
                      </h2>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {task.project} · {task.id}
                      </p>
                    </div>
                    <Badge
                      variant={
                        task.state === "failed" ? "destructive" : "secondary"
                      }
                      className={
                        task.state === "succeeded"
                          ? "bg-primary/10 text-primary"
                          : ""
                      }
                    >
                      {task.label}
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {task.cost}
                    </span>
                    <time className="col-span-2 text-xs text-muted-foreground @2xl:col-span-1">
                      {task.time}
                    </time>
                  </div>
                  <ArrowUpRight
                    aria-hidden="true"
                    className="mt-1 size-4 shrink-0 text-muted-foreground transition-colors group-hover:text-primary sm:mt-0"
                  />
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
export function TaskDetailPage({ taskId }: { taskId: string }) {
  const task = tasks.find((t) => t.id === taskId)!;
  const href =
    task.state === "partial"
      ? "/projects/harbor/episodes/ep-01/audio"
      : `/projects/harbor/episodes/ep-01/shots/${task.state === "failed" ? "shot-02" : "shot-01"}`;
  return (
    <div className="space-y-6">
      <PageHeading
        eyebrow="任务详情"
        title={task.title}
        description={`${task.project} · ${task.id} · 样例任务`}
        action={
          <Badge
            variant={task.state === "failed" ? "destructive" : "secondary"}
          >
            {task.label}
          </Badge>
        }
      />
      <div className="grid gap-4 xl:grid-cols-[1.4fr_1fr]">
        <Panel
          title={
            task.state === "unknown"
              ? "需要核对提交结果"
              : task.state === "failed"
                ? "失败原因与下一步"
                : "任务结果"
          }
        >
          <p className="text-sm leading-8">{task.detail}</p>
          {task.state === "unknown" ? (
            <div className="mt-5 rounded-xl bg-muted/60 p-4 text-sm leading-7">
              <h3 className="font-medium">核对步骤</h3>
              <ol className="mt-3 list-inside list-decimal space-y-2">
                <li>检查供应商是否已有任务与产物。</li>
                <li>取回已有结果，或确认未受理后重新报价。</li>
                <li>核对预留与实际费用，再结束未决状态。</li>
              </ol>
              <p className="mt-3 text-xs text-muted-foreground">
                真实核对服务尚未接入，当前无法确定是否收费。
              </p>
            </div>
          ) : (
            <Button className="mt-5" variant="secondary" asChild>
              <Link href={href}>{task.action}</Link>
            </Button>
          )}
          {task.state === "partial" && (
            <DataTable
              caption="配音批次明细"
              columns={["台词", "状态", "下一步"]}
              rows={[
                ["01 / 林夏", "已完成", "保留产物"],
                ["02 / 老陈", "已完成", "保留产物"],
                ["03 / 林夏", "授权待核对", "核对后只重新报价此项"],
              ]}
            />
          )}
        </Panel>
        <Panel title="模型与费用">
          <dl className="space-y-5 text-sm">
            {[
              ["模型", task.model],
              ["费用", task.cost],
              ["时间", task.time],
              ["业务范围", "第一集 / 样例任务"],
              ["状态来源", "前端固定样例"],
            ].map(([label, value]) => (
              <div key={label} className="flex justify-between gap-4">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="text-right">{value}</dd>
              </div>
            ))}
          </dl>
          <Button disabled variant="outline" className="mt-6">
            取消与核对服务待接入
          </Button>
        </Panel>
      </div>
      <Panel title="状态记录" description="样例操作记录，不代表实际任务执行。">
        <ol className="grid gap-5 sm:grid-cols-4">
          {["报价确认", "预留预算", "供应商处理", task.label].map((s, i) => (
            <li key={i}>
              <span className="flex size-8 items-center justify-center rounded-lg bg-muted text-xs text-muted-foreground">
                0{i + 1}
              </span>
              <p className="mt-2 text-sm font-medium">{s}</p>
            </li>
          ))}
        </ol>
      </Panel>
    </div>
  );
}
