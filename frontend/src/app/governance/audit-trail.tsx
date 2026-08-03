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
  "consent.registered": "授权登记",
  "consent.revised": "授权修订",
  "consent.revoked": "授权撤销",
  "media.version_created": "媒体版本创建",
  "media.current_changed": "媒体当前版本切换",
  "media.archived": "媒体归档",
  "media.restored": "媒体恢复",
  "task.created": "任务创建",
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
  if (event.metadata.subject_type) return String(event.metadata.subject_type);
  if (event.metadata.version_no) {
    return `${String(event.metadata.kind ?? "媒体")} · v${String(event.metadata.version_no)}`;
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
                    <Badge variant="secondary">revision {String(event.metadata.revision ?? "—")}</Badge>
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
