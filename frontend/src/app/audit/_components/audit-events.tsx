"use client";
import { useState, useRef } from "react";
import { getPlatformSecurityEvents } from "@/api/identity";
import { SessionGate } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  Filters,
  dateFields,
  InsightsHeading,
  Loading,
  Panel,
  ReadState,
  Snapshot,
  Pager,
  CopyValue,
  useInsightIdentity,
  type FilterField,
} from "@/components/insights/shared";
import { useInsightRead } from "@/components/insights/use-insight-read";
import { timestamp, statusName } from "@/components/insights/contracts";
const fields: FilterField[] = [
  ...dateFields,
  { key: "action", label: "操作（精确匹配）" },
  { key: "actor_id", label: "操作者 UUID" },
  { key: "target_user_id", label: "目标用户 UUID" },
  { key: "team_id", label: "团队 UUID" },
];
function SafeDiff({ value }: { value: API.SecurityEventView["safe_diff"] }) {
  const fields = Object.entries(value ?? {}).filter(
    ([key, v]) =>
      ["status", "platform_role", "role", "governance_blocked"].includes(key) &&
      typeof v === "string",
  );
  return fields.length ? (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>字段</TableHead>
            <TableHead>安全摘要</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {fields.map(([key, v]) => (
            <TableRow key={key}>
              <TableCell>{key}</TableCell>
              <TableCell className="insights-code">{String(v)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <p className="insights-note">
        来源仅提供安全字段摘要，未提供独立变更前后值。
      </p>
    </>
  ) : (
    <p className="insights-note">此记录没有可展示的安全变更摘要。</p>
  );
}
function Events() {
  const identity = useInsightIdentity();
  const [query, setQuery] = useState<Record<string, string>>({});
  const [cursors, setCursors] = useState([""]);
  const [restarted, setRestarted] = useState(false);
  const [selected, setSelected] = useState<API.SecurityEventView | null>(null);
  const focus = useRef<HTMLButtonElement | null>(null);
  const read = useInsightRead(
    `${identity}:${JSON.stringify(query)}:${cursors.at(-1)}`,
    (signal) =>
      getPlatformSecurityEvents(
        { ...query, limit: 25, cursor: cursors.at(-1) || undefined },
        { signal },
      ),
  );
  const d = read.data;
  const show = read.error === "forbidden" ? null : selected;
  function restart() {
    clear();
    setCursors([""]);
    setRestarted(true);
    read.refresh();
  }
  function clear() {
    setSelected(null);
  }
  return (
    <PageLayout screen="audit" hideSearch>
      <section className="insights">
        <InsightsHeading
          active="audit"
          title="审计日志"
          description="平台安全事件 · 当前管理员授权范围"
          pending={read.pending}
          refresh={restart}
        />
        <Filters
          fields={fields}
          maxDays={31}
          onApply={(v) => {
            clear();
            setRestarted(false);
            setCursors([""]);
            setQuery(v);
          }}
        />
        {read.error ? <ReadState error={read.error} retry={restart} /> : null}
        {!d ? (
          read.pending ? (
            <Loading />
          ) : null
        ) : (
          <>
            {restarted &&
              cursors[0] === "" &&
              cursors.length === 1 &&
              !read.pending &&
              !read.error && (
                <p className="insights-note" role="status">
                  已读取首页新快照
                </p>
              )}
            <Panel
              title="安全事件"
              description="安全摘要及脱敏变更；不包含组织业务审计"
            >
              {d.items.length ? (
                <div className="insights-table">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>时间</TableHead>
                        <TableHead>操作 / 结果</TableHead>
                        <TableHead className="insights-secondary-col">
                          操作者
                        </TableHead>
                        <TableHead className="insights-secondary-col">
                          目标用户
                        </TableHead>
                        <TableHead>详情</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {d.items.map((item) => (
                        <TableRow key={item.id}>
                          <TableCell>{timestamp(item.created_at)}</TableCell>
                          <TableCell>
                            <div>{item.action}</div>
                            <Badge variant="outline">
                              {statusName[item.result] ?? item.result}
                            </Badge>
                          </TableCell>
                          <TableCell className="insights-secondary-col insights-code">
                            {item.actor_id || "—"}
                          </TableCell>
                          <TableCell className="insights-secondary-col insights-code">
                            {item.target_user_id || "—"}
                          </TableCell>
                          <TableCell>
                            <Button
                              variant="ghost"
                              aria-label={`查看审计 ${item.id}`}
                              onClick={(e) => {
                                focus.current = e.currentTarget;
                                setSelected(item);
                              }}
                            >
                              查看
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              ) : (
                <ContentState
                  title={
                    Object.keys(query).length
                      ? "没有匹配的安全事件"
                      : "暂无安全事件"
                  }
                  description="可调整时间、操作或账号筛选条件。"
                />
              )}
            </Panel>
            <Pager
              back={cursors.length > 1 && !read.pending}
              next={Boolean(d.next_cursor) && !read.pending}
              onBack={() => {
                clear();
                setCursors((v) => v.slice(0, -1));
              }}
              onNext={() => {
                clear();
                if (d.next_cursor)
                  setCursors([
                    d.first_cursor,
                    ...cursors.slice(1),
                    d.next_cursor,
                  ]);
              }}
            />
            <Snapshot at={d.as_of} pending={read.pending} />
          </>
        )}
        <Sheet
          open={Boolean(show)}
          onOpenChange={(open) => {
            if (!open) clear();
          }}
        >
          <SheetContent
            className="insights-detail"
            onCloseAutoFocus={(e) => {
              e.preventDefault();
              focus.current?.focus();
            }}
          >
            <SheetHeader>
              <SheetTitle>审计详情</SheetTitle>
              <SheetDescription>
                当前列表已授权的安全摘要 · 只读
              </SheetDescription>
            </SheetHeader>
            {show ? (
              <div className="insights-detail-content">
                <dl>
                  <dt>时间（上海）</dt>
                  <dd>{timestamp(show.created_at)}</dd>
                  <dt>操作 / 结果</dt>
                  <dd>
                    {show.action} · {statusName[show.result] ?? show.result}
                  </dd>
                  <dt>原因</dt>
                  <dd>{show.reason || "—"}</dd>
                  <CopyValue label="操作者" value={show.actor_id} />
                  <CopyValue label="目标用户" value={show.target_user_id} />
                  <CopyValue label="团队" value={show.team_id} />
                  <CopyValue label="请求 ID" value={show.request_id} />
                  <CopyValue label="事件 ID" value={show.id} />
                </dl>
                <Panel title="安全变更摘要">
                  <SafeDiff value={show.safe_diff} />
                </Panel>
              </div>
            ) : null}
          </SheetContent>
        </Sheet>
      </section>
    </PageLayout>
  );
}
export function AuditEvents() {
  return (
    <SessionGate admin>
      <AuditIdentity />
    </SessionGate>
  );
}
function AuditIdentity() {
  return <Events key={useInsightIdentity()} />;
}
