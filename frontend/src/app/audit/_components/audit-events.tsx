"use client";
import { SelectionToolbar } from "@/components/governance/admin-layout";
import { usePageSelection } from "@/components/governance/use-page-selection";
import { IconAction } from "@/components/ui/icon-action";
import { Input } from "@/components/ui/input";
import { Eye, RefreshCw, Search } from "lucide-react";
import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { useState, useRef } from "react";
import { getPlatformSecurityEvents } from "@/api/identity";
import { SessionGate } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
import { ContentState } from "@/components/layout/content-state";
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
  Loading,
  Panel,
  ReadState,
  Snapshot,
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
function compactID(id?: string | null) {
  return id && id.length > 20 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id || "—";
}
function SafeDiff({ value }: { value: API.SecurityEventView["safe_diff"] }) {
  const fields = Object.entries(value ?? {}).filter(
    ([key, v]) =>
      ["status", "platform_role", "role", "governance_blocked"].includes(key) &&
      typeof v === "string",
  );
  return fields.length ? (
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
  ) : (
    <p className="insights-note">暂无变更摘要</p>
  );
}
function Events() {
  const identity = useInsightIdentity();
  const [query, setQuery] = useState<Record<string, string>>({});
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [cursors, setCursors] = useState([""]);
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [restarted, setRestarted] = useState(false);
  const [selected, setSelected] = useState<API.SecurityEventView | null>(null);
  const focus = useRef<HTMLButtonElement | null>(null);
  const read = useInsightRead(
    `${identity}:${JSON.stringify(query)}:${pageSize}:${cursors.at(-1)}`,
    (signal) =>
      getPlatformSecurityEvents(
        { ...query, limit: pageSize, cursor: cursors.at(-1) || undefined },
        { signal },
      ),
  );
  const d = read.data;
  const checks = usePageSelection(
    d?.items ?? [],
    JSON.stringify([identity, query, read.error]),
    "安全事件",
    (row) => row.id,
  );
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
  function applyFilters(value: Record<string, string>) {
    clear();
    setRestarted(false);
    setCursors([""]);
    setQuery(value);
  }
  return (
    <PageLayout screen="audit" hideSearch>
      <section className="insights">
        <h1 className="sr-only">审计日志</h1>
        <div className="flex min-h-11 items-center gap-2">
          <form
            className="mr-auto flex min-w-0 items-center gap-1"
            onSubmit={(event) => {
              event.preventDefault();
              const next: Record<string, string> = {
                ...query,
                action: (draft.action ?? "").trim(),
              };
              if (!next.action) delete next.action;
              applyFilters(next);
            }}
          >
            <Input
              aria-label="搜索操作代码"
              placeholder="搜索操作代码"
              value={draft.action ?? ""}
              onChange={(event) =>
                setDraft({ ...draft, action: event.target.value })
              }
            />
            <IconAction type="submit" icon={Search} label="查询审计事件" />
          </form>
          <SelectionToolbar>
            {" "}
            <Filters
              fields={fields}
              maxDays={31}
              draft={draft}
              onDraftChange={setDraft}
              applied={query}
              onApply={applyFilters}
            />
            <IconAction
              label="刷新"
              icon={RefreshCw}
              disabled={read.pending}
              onClick={restart}
            />
          </SelectionToolbar>
        </div>
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
            <div className="min-w-0 shrink-0 overflow-hidden rounded-md border">
              {d.items.length ? (
                <div className="insights-table">
                  <Table aria-label="安全事件">
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-10">
                          {checks.selectAll}
                        </TableHead>
                        <TableHead>时间</TableHead>
                        <TableHead>操作 / 结果</TableHead>
                        <TableHead className="insights-secondary-col">
                          操作者
                        </TableHead>
                        <TableHead className="insights-secondary-col">
                          目标用户
                        </TableHead>
                        <TableHead className="text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {d.items.map((item) => (
                        <TableRow key={item.id}>
                          <TableCell>{checks.selectRow(item)}</TableCell>
                          <TableCell>{timestamp(item.created_at)}</TableCell>
                          <TableCell>
                            <div title={item.action}>
                              {item.action === "session.refreshed"
                                ? "会话续期"
                                : item.action}
                            </div>
                            <Badge variant="outline">
                              {statusName[item.result] ?? item.result}
                            </Badge>
                          </TableCell>
                          <TableCell className="insights-secondary-col insights-code">
                            <span title={item.actor_id}>
                              {compactID(item.actor_id)}
                            </span>
                          </TableCell>
                          <TableCell className="insights-secondary-col insights-code">
                            <span title={item.target_user_id}>
                              {compactID(item.target_user_id)}
                            </span>
                          </TableCell>
                          <TableCell className="text-right">
                            <IconAction
                              icon={Eye}
                              label={`查看审计 ${item.id}`}
                              onClick={(e) => {
                                focus.current = e.currentTarget;
                                setSelected(item);
                              }}
                            />
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
                />
              )}
            </div>
            {checks.actions}
            <ListPagination
              label="安全事件分页"
              mode="cursor"
              current={cursors.length}
              pageSize={pageSize}
              pending={read.pending}
              itemCount={d.items.length}
              hasNextPage={Boolean(d.next_cursor)}
              onPageSizeChange={(size) => {
                setPageSize(size);
                clear();
                setCursors([""]);
              }}
              onPageChange={(page) => {
                clear();
                const next =
                  page < cursors.length
                    ? cursors.slice(0, -1)
                    : d.next_cursor
                      ? [d.first_cursor, ...cursors.slice(1), d.next_cursor]
                      : cursors;
                setCursors(next);
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
              <SheetDescription className="sr-only">
                安全事件摘要
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
