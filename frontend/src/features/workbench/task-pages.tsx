"use client";

import Link from "next/link";
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
} from "./poc-components";

const filters = {
  all: "全部",
  succeeded: "已完成",
  failed: "失败",
  unknown: "结果未知",
  partial: "部分失败",
};
export function TaskListPage() {
  const { params, set } = usePreviewQuery();
  const status = params.get("status") ?? "all";
  const q = params.get("q") ?? "";
  const list = tasks.filter(
    (t) =>
      (status === "all" || t.state === status) &&
      `${t.title}${t.project}`.includes(q),
  );
  return (
    <div className="space-y-8">
      <PageHeading
        title="让每次生成都有去向。"
        description="查看任务状态、费用与产物。失败读取不改变执行状态，未知提交先核对，不直接重提。"
      />
      <div className="flex flex-wrap items-center justify-between gap-4">
        <ToggleGroup
          type="single"
          aria-label="任务状态"
          value={status in filters ? status : "all"}
          onValueChange={(v) => {
            if (v) set("status", v);
          }}
          className="flex-wrap"
        >
          {Object.entries(filters).map(([key, label]) => (
            <ToggleGroupItem key={key} value={key}>
              {label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Input
          className="sm:max-w-xs"
          aria-label="搜索任务"
          placeholder="搜索任务或项目…"
          value={q}
          onChange={(e) => set("q", e.target.value)}
        />
      </div>
      {list.length === 0 ? (
        <EmptyMessage title="没有匹配的任务" />
      ) : (
        <DataTable
          caption="任务列表"
          columns={["任务", "项目", "状态", "费用", "时间"]}
          rows={list.map((t) => [
            <Link
              key={t.id}
              className="font-medium hover:underline"
              href={`/tasks/${t.id}`}
            >
              {t.title}
            </Link>,
            t.project,
            <Badge
              key="state"
              variant={t.state === "failed" ? "destructive" : "secondary"}
            >
              {t.label}
            </Badge>,
            t.cost,
            t.time,
          ])}
        />
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
    <div className="space-y-8">
      <PageHeading
        title={task.title}
        description={`${task.project} / ${task.id} · 样例任务，不进行后台轮询或真实取消。`}
        action={
          <Badge
            variant={task.state === "failed" ? "destructive" : "secondary"}
          >
            {task.label}
          </Badge>
        }
      />
      <div className="grid gap-7 xl:grid-cols-[1.4fr_1fr]">
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
            <div className="mt-6 rounded-lg bg-muted p-4 text-sm leading-7">
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
            <Button className="mt-6" variant="secondary" asChild>
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
      <Panel
        title="状态记录"
        description="此时间线只演示已有状态机的表现，不声明真实任务已执行。"
      >
        <ol className="grid gap-5 sm:grid-cols-4">
          {["报价确认", "预留预算", "供应商处理", task.label].map((s, i) => (
            <li key={i}>
              <span className="font-mono text-xs text-muted-foreground">
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
