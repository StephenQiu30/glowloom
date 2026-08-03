"use client";

import { Filter, Fingerprint, History, LoaderCircle } from "lucide-react";
import { type FormEvent, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";

export type AuditFilters = {
  action?: string;
  actorId?: string;
  targetId?: string;
  occurredFrom?: string;
  occurredTo?: string;
};

const auditActionLabels: Record<string, string> = {
  "identity.registered": "账户注册",
  "identity.login_succeeded": "登录成功",
  "identity.logged_out": "全局登出",
  "identity.password_changed": "密码修改",
  "identity.profile_updated": "资料更新",
  "identity.account_deactivated": "账户停用",
  "workspace.created": "工作空间创建",
  "workspace.updated": "工作空间更新",
  "workspace.archived": "工作空间归档",
  "workspace.restored": "工作空间恢复",
  "project.created": "项目创建",
  "project.updated": "项目更新",
  "project.budget_updated": "项目预算更新",
  "project.archived": "项目归档",
  "project.restored": "项目恢复",
  "project.deleted": "项目删除",
  "episode.created": "单集创建",
  "episode.updated": "单集更新",
  "episode.reordered": "单集排序",
  "episode.archived": "单集归档",
  "episode.restored": "单集恢复",
  "episode.deleted": "单集删除",
  "script.version_created": "剧本初始版本创建",
  "script.version_published": "剧本版本发布",
  "script.current_changed": "剧本当前版本切换",
  "script.source_archived": "剧本来源归档",
  "script.source_restored": "剧本来源恢复",
  "script.version_deleted": "剧本草稿删除",
  "asset.created": "资产创建",
  "asset.updated": "资产更新",
  "asset.archived": "资产归档",
  "asset.restored": "资产恢复",
  "asset.deleted": "资产删除",
  "asset.version_created": "资产版本创建",
  "asset.current_changed": "资产当前版本切换",
  "shot.spec_version_created": "分镜规格版本创建",
  "shot.current_spec_changed": "分镜当前规格切换",
  "consent.registered": "授权登记",
  "consent.revised": "授权修订",
  "consent.revoked": "授权撤销",
  "media.version_created": "媒体版本创建",
  "media.current_changed": "媒体当前版本切换",
  "media.archived": "媒体归档",
  "media.restored": "媒体恢复",
  "task.created": "任务创建",
  "task.started": "任务开始",
  "task.succeeded": "任务完成",
  "task.failed": "任务失败",
  "task.unknown": "任务结果待对账",
};

function shortId(value: string): string {
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
}

function utcBoundary(value: FormDataEntryValue | null, end: boolean) {
  const date = String(value ?? "").trim();
  if (!date) return undefined;
  return `${date}T${end ? "23:59:59.999" : "00:00:00.000"}Z`;
}

function eventSummary(event: API.AuditEventResponse): string {
  if (Array.isArray(event.metadata.changed_fields)) {
    return `变更字段：${event.metadata.changed_fields.map(String).join("、")}`;
  }
  if (event.metadata.previous_status && event.metadata.status) {
    return `${String(event.metadata.previous_status)} → ${String(event.metadata.status)}`;
  }
  if (event.metadata.previous_token_version && event.metadata.token_version) {
    return `token v${String(event.metadata.previous_token_version)} → v${String(event.metadata.token_version)}`;
  }
  if (event.metadata.token_version) {
    return `token version ${String(event.metadata.token_version)}`;
  }
  if (event.metadata.subject_type) return String(event.metadata.subject_type);
  if (event.metadata.episode_count) {
    return `${String(event.metadata.episode_count)} 个单集`;
  }
  if (event.metadata.position && event.metadata.status) {
    return `位置 ${String(event.metadata.position)} · ${String(event.metadata.status)}`;
  }
  if (event.metadata.version_no) {
    const versionKind = event.action.startsWith("script.")
      ? "剧本"
      : event.action.startsWith("shot.")
        ? "分镜规格"
        : String(event.metadata.kind ?? "媒体");
    return `${versionKind} · v${String(event.metadata.version_no)}`;
  }
  if (event.metadata.task_type) {
    return `${String(event.metadata.task_type)} · ${String(event.metadata.request_type ?? "request")}`;
  }
  if (event.metadata.previous_version_id && event.metadata.current_version_id) {
    return `${shortId(String(event.metadata.previous_version_id))} → ${shortId(String(event.metadata.current_version_id))}`;
  }
  if (event.metadata.current_version_id) {
    return `current ${shortId(String(event.metadata.current_version_id))}`;
  }
  return "最小必要元数据";
}

export function AuditTrail({
  events,
  loading,
  total,
  onFilter,
}: {
  events: API.AuditEventResponse[];
  loading: boolean;
  total: number;
  onFilter: (filters: AuditFilters) => void;
}) {
  const [filtering, setFiltering] = useState(false);

  function submitFilters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const action = String(form.get("action") ?? "").trim();
    const actorId = String(form.get("actorId") ?? "").trim();
    const targetId = String(form.get("targetId") ?? "").trim();
    onFilter({
      action: action || undefined,
      actorId: actorId || undefined,
      targetId: targetId || undefined,
      occurredFrom: utcBoundary(form.get("occurredFrom"), false),
      occurredTo: utcBoundary(form.get("occurredTo"), true),
    });
  }

  return (
    <section aria-label="操作审计" className="mt-8">
      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <History className="size-4 text-[#079db3]" aria-hidden="true" />
                <CardTitle>操作审计</CardTitle>
              </div>
              <CardDescription className="mt-1">
                {total} 条只追加事件；仅展示稳定标识、动作和最小必要元数据。
              </CardDescription>
            </div>
            <Button
              aria-expanded={filtering}
              onClick={() => setFiltering((visible) => !visible)}
              variant="outline"
            >
              <Filter aria-hidden="true" />筛选
            </Button>
          </div>
          {filtering ? (
            <form
              className="mt-4 grid gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 md:grid-cols-2 xl:grid-cols-5"
              onSubmit={submitFilters}
            >
              <div className="grid gap-2">
                <Label htmlFor="auditAction">动作</Label>
                <select
                  className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm"
                  id="auditAction"
                  name="action"
                >
                  <option value="">全部动作</option>
                  {Object.entries(auditActionLabels).map(([value, label]) => (
                    <option key={value} value={value}>{label}</option>
                  ))}
                </select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="auditActor">Actor UUID</Label>
                <input
                  className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm"
                  id="auditActor"
                  name="actorId"
                  placeholder="可选"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="auditTarget">目标 UUID</Label>
                <input
                  className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm"
                  id="auditTarget"
                  name="targetId"
                  placeholder="可选"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="auditFrom">开始日期</Label>
                <input
                  className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm"
                  id="auditFrom"
                  name="occurredFrom"
                  type="date"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="auditTo">结束日期</Label>
                <input
                  className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm"
                  id="auditTo"
                  name="occurredTo"
                  type="date"
                />
              </div>
              <div className="flex justify-end md:col-span-2 xl:col-span-5">
                <Button type="submit">应用审计筛选</Button>
              </div>
            </form>
          ) : null}
        </CardHeader>
        <CardContent className="divide-y divide-slate-100 p-0">
          {loading ? (
            <div className="flex items-center gap-2 px-5 py-8 text-sm text-slate-500">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              正在读取审计事实…
            </div>
          ) : events.length === 0 ? (
            <p className="px-5 py-8 text-sm text-slate-500">当前筛选下没有审计事件。</p>
          ) : (
            events.map((event) => (
              <article className="grid gap-3 px-5 py-4 lg:grid-cols-[minmax(0,1fr)_auto]" key={event.id}>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">
                      {auditActionLabels[event.action] ?? event.action}
                    </Badge>
                    <Badge variant="secondary">
                      revision {String(event.metadata.revision ?? event.metadata.project_revision ?? "—")}
                    </Badge>
                    <time className="text-xs text-slate-400" dateTime={event.occurred_at}>
                      {new Date(event.occurred_at).toLocaleString("zh-CN", { timeZone: "UTC" })}
                    </time>
                  </div>
                  <p className="mt-2 text-sm text-slate-600">
                    Actor {shortId(event.actor_id)} · {event.target_type} {shortId(event.target_id)}
                  </p>
                  <p className="mt-1 text-xs text-slate-400">
                    {eventSummary(event)}
                  </p>
                </div>
                <div className="flex items-center gap-2 font-mono text-xs text-slate-400">
                  <Fingerprint className="size-3.5" aria-hidden="true" />
                  trace {shortId(event.trace_id)}
                </div>
              </article>
            ))
          )}
        </CardContent>
      </Card>
    </section>
  );
}
