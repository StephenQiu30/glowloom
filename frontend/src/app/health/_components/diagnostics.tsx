"use client";
import { SelectionToolbar } from "@/components/governance/admin-layout";
import { usePageSelection } from "@/components/governance/use-page-selection";
import { IconAction } from "@/components/ui/icon-action";
import { Eye, RefreshCw } from "lucide-react";
import { DEFAULT_PAGE_SIZE, PAGE_SIZE_OPTIONS } from "@/lib/pagination";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { useState, useRef } from "react";
import {
  getDiagnosticLog,
  getDiagnosticsOverview,
  listDiagnosticLogs,
} from "@/api/diagnostics";
import { SessionGate } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { useInsightRead } from "@/components/insights/use-insight-read";
import {
  Filters,
  dateFields,
  Loading,
  Metric,
  Notice,
  Panel,
  ReadState,
  Snapshot,
  CopyValue,
  useInsightIdentity,
  type FilterField,
} from "@/components/insights/shared";
import {
  numberValue,
  rateValue,
  timestamp,
  statusName,
} from "@/components/insights/contracts";
const fields: FilterField[] = [
  ...dateFields,
  {
    key: "service",
    label: "服务",
    options: [
      ["", "已接入服务"],
      ["backend", "业务后端"],
    ],
  },
  {
    key: "level",
    label: "级别",
    options: [
      ["", "全部级别"],
      ["info", "信息"],
      ["warn", "警告"],
      ["error", "错误"],
    ],
  },
  { key: "request_id", label: "请求 UUID" },
  { key: "trace_id", label: "追踪 ID（小写）" },
];
function Coverage({ value }: { value: API.ObservabilityCoverage }) {
  return (
    <Notice title="部分覆盖 · 当前后端实例">
      <p>仅当前后端实例，不代表全站流量；重启丢失。</p>
      <Collapsible>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="sm">
            查看保留范围
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <p>
            保留 {numberValue(value.retained)} / {numberValue(value.capacity)}{" "}
            条 · 最长 {numberValue(value.retention_seconds / 3600)} 小时 · 淘汰{" "}
            {numberValue(value.evicted)} 条。
          </p>
          <p>
            启动 {timestamp(value.started_at)} · 最早{" "}
            {timestamp(value.oldest_at)}。游标失效时请刷新。
          </p>
        </CollapsibleContent>
      </Collapsible>
    </Notice>
  );
}

function Overview({
  query,
  revision,
}: {
  query: API.getDiagnosticsOverviewParams;
  revision: number;
}) {
  const identity = useInsightIdentity();
  const read = useInsightRead(
    `${identity}:${JSON.stringify(query)}`,
    (signal) => getDiagnosticsOverview(query, { signal }),
    revision,
  );
  const d = read.data;
  return (
    <>
      {read.error ? (
        <ReadState error={read.error} retry={read.refresh} />
      ) : null}
      {!d ? (
        read.pending ? (
          <Loading />
        ) : null
      ) : (
        <>
          <Coverage value={d.coverage} />
          <div className="insights-metrics">
            <Metric label="已观测请求" value={numberValue(d.requests)} />
            <Metric
              label="系统错误率"
              value={rateValue(d.server_error_rate)}
              context="HTTP 5xx 占比"
            />
            <Metric
              label="请求 P95"
              value={
                d.p95_ms === null ? "不适用" : `${numberValue(d.p95_ms)} ms`
              }
            />
            <Metric label="服务覆盖" value="仅后端" />
          </div>
          <div className="insights-panels">
            <Panel title="HTTP 请求结果">
              <dl className="grid grid-cols-2 gap-4">
                <dt>HTTP 5xx</dt>
                <dd>{numberValue(d.server_errors)}</dd>
                <dt>HTTP 4xx</dt>
                <dd>{numberValue(d.client_errors)}</dd>
                <dt>限流请求</dt>
                <dd>{numberValue(d.rate_limited)}</dd>
                <dt>Panic 记录</dt>
                <dd>{numberValue(d.panics)}</dd>
              </dl>
              <p className="insights-note mt-4">Panic 不等同于 HTTP 5xx。</p>
            </Panel>
            <Panel title="请求趋势">
              <ContentState className="min-h-40" title="趋势暂不可用" />
            </Panel>
          </div>
          <Snapshot at={d.as_of} pending={read.pending} />
        </>
      )}
    </>
  );
}
function LogDetail({
  id,
  onClose,
  onForbidden,
  focus,
}: {
  id: string | null;
  onClose: () => void;
  onForbidden: () => void;
  focus: React.RefObject<HTMLButtonElement | null>;
}) {
  const identity = useInsightIdentity();
  const read = useInsightRead(
    `${identity}:${id}`,
    (signal) =>
      id ? getDiagnosticLog({ id }, { signal }) : Promise.resolve(null),
    0,
    onForbidden,
  );
  const d = read.data;
  return (
    <Sheet
      open={Boolean(id)}
      onOpenChange={(open) => {
        if (!open) onClose();
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
          <SheetTitle>日志详情</SheetTitle>
          <SheetDescription className="sr-only">安全诊断字段</SheetDescription>
        </SheetHeader>
        <div className="insights-detail-content">
          {read.error ? (
            <ReadState error={read.error} retry={read.refresh} />
          ) : d ? (
            <>
              <Badge variant="secondary">
                {statusName[d.record.level] ?? d.record.level}
              </Badge>
              <dl>
                <dt>时间（上海）</dt>
                <dd>{timestamp(d.record.timestamp)}</dd>
                <dt>服务 / 角色</dt>
                <dd>
                  {d.record.service} / {d.record.role}
                </dd>
                <dt>请求</dt>
                <dd className="insights-code">
                  {d.record.method} {d.record.route}
                </dd>
                <dt>HTTP 状态 / 耗时</dt>
                <dd>
                  {d.record.status} · {numberValue(d.record.elapsed_ms)} ms
                </dd>
                <dt>事件 / 安全错误分类</dt>
                <dd>
                  {d.record.event} / {d.record.error_class || "—"}
                </dd>
                <dt>Panic</dt>
                <dd>{d.record.panic ? "是" : "否"}</dd>
                <CopyValue label="记录 ID" value={d.record.id} />
                <CopyValue label="请求 ID" value={d.record.request_id} />
                <CopyValue label="追踪 ID" value={d.record.trace_id ?? ""} />
                <CopyValue label="发布标识" value={d.record.release} />
              </dl>
            </>
          ) : (
            <Loading />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
function Logs({
  query,
  revision,
}: {
  query: API.listDiagnosticLogsParams;
  revision: number;
}) {
  const identity = useInsightIdentity();
  const [history, setHistory] = useState({ revision, cursors: [""] });
  const cursors = history.revision === revision ? history.cursors : [""];
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [restarted, setRestarted] = useState(false);
  const cursor = cursors.at(-1);
  const [selection, setSelection] = useState<{
    id: string;
    revision: number;
  } | null>(null);
  const selected = selection?.revision === revision ? selection.id : null;
  const setSelected = (id: string | null) =>
    setSelection(id ? { id, revision } : null);
  const focus = useRef<HTMLButtonElement | null>(null);
  const read = useInsightRead(
    `${identity}:${JSON.stringify(query)}:${pageSize}:${cursor}`,
    (signal) =>
      listDiagnosticLogs(
        { ...query, limit: pageSize, cursor: cursor || undefined },
        { signal },
      ),
    revision,
  );
  const d = read.data;
  const checks = usePageSelection(
    d?.items ?? [],
    JSON.stringify([identity, query, revision, read.error]),
    "日志",
    (row) => row.request_id,
  );
  return (
    <>
      {read.error ? (
        <ReadState
          error={read.error}
          retry={() => {
            setSelected(null);
            setHistory({ revision, cursors: [""] });
            setRestarted(true);
            read.refresh();
          }}
        />
      ) : null}
      {!d ? (
        read.pending ? (
          <Loading />
        ) : null
      ) : (
        <>
          {(restarted || revision > 0) &&
            cursors[0] === "" &&
            cursors.length === 1 &&
            !read.pending &&
            !read.error && (
              <p className="insights-note" role="status">
                已读取首页新快照
              </p>
            )}
          <Coverage value={d.coverage} />
          <div className="min-w-0 shrink-0 overflow-hidden rounded-md border">
            {d.items.length ? (
              <div className="insights-table">
                <Table aria-label="诊断日志">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10">{checks.selectAll}</TableHead>
                      <TableHead>时间 / 级别</TableHead>
                      <TableHead>请求 / 状态</TableHead>
                      <TableHead className="insights-secondary-col">
                        耗时
                      </TableHead>
                      <TableHead className="insights-secondary-col">
                        请求 ID
                      </TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {d.items.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell>{checks.selectRow(item)}</TableCell>
                        <TableCell>
                          <div>{timestamp(item.timestamp)}</div>
                          <Badge variant="outline">
                            {statusName[item.level] ?? item.level}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="insights-code">
                            {item.method} {item.route}
                          </div>
                          <span>
                            {item.status} · {item.service}
                          </span>
                        </TableCell>
                        <TableCell className="insights-secondary-col">
                          {numberValue(item.elapsed_ms)} ms
                        </TableCell>
                        <TableCell className="insights-secondary-col insights-code">
                          {item.request_id}
                        </TableCell>
                        <TableCell className="text-right">
                          <IconAction
                            icon={Eye}
                            label={`查看日志 ${item.request_id}`}
                            onClick={(e) => {
                              focus.current = e.currentTarget;
                              setSelected(item.id);
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
                  Object.keys(query).length ? "没有匹配的日志" : "暂无保留日志"
                }
              />
            )}
          </div>
          {checks.actions}
          <ListPagination
            label="诊断日志分页"
            mode="cursor"
            current={cursors.length}
            pageSize={pageSize}
            pageSizeOptions={PAGE_SIZE_OPTIONS}
            pending={read.pending}
            itemCount={d.items.length}
            hasNextPage={Boolean(d.next_cursor)}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setSelected(null);
              setHistory({ revision, cursors: [""] });
            }}
            onPageChange={(page) => {
              setSelected(null);
              const next =
                page < cursors.length
                  ? cursors.slice(0, -1)
                  : d.next_cursor
                    ? [d.first_cursor, ...cursors.slice(1), d.next_cursor]
                    : cursors;
              setHistory({ revision, cursors: next });
            }}
          />
          <Snapshot at={d.as_of} pending={read.pending} />
        </>
      )}
      <LogDetail
        id={read.error === "forbidden" ? null : selected}
        onForbidden={() => {
          read.invalidate();
          setSelected(null);
        }}
        onClose={() => setSelected(null)}
        focus={focus}
      />
    </>
  );
}
function DiagnosticsContent() {
  const [query, setQuery] = useState<Record<string, string>>({});
  const [tab, setTab] = useState("overview");
  const [revision, setRevision] = useState(0);
  return (
    <PageLayout screen="health" hideSearch>
      <section className="insights">
        <h1 className="sr-only">运行诊断</h1>
        <Tabs value={tab} onValueChange={setTab}>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <TabsList variant="line" aria-label="运行日志视图">
              <TabsTrigger value="overview">概览</TabsTrigger>
              <TabsTrigger value="logs">日志</TabsTrigger>
            </TabsList>
            <SelectionToolbar>
              {" "}
              <div className="flex items-center gap-2">
                <Filters fields={fields} maxDays={7} onApply={setQuery} />
                <IconAction
                  label="刷新"
                  icon={RefreshCw}
                  onClick={() => setRevision((n) => n + 1)}
                />
              </div>
            </SelectionToolbar>
          </div>
          <TabsContent value="overview">
            <div className="flex flex-col gap-5">
              <Overview query={query} revision={revision} />
            </div>
          </TabsContent>
          <TabsContent value="logs">
            <div className="flex flex-col gap-5">
              <Logs
                key={JSON.stringify(query)}
                query={query}
                revision={revision}
              />
            </div>
          </TabsContent>
        </Tabs>
      </section>
    </PageLayout>
  );
}
export function Diagnostics() {
  return (
    <SessionGate admin>
      <DiagnosticsIdentity />
    </SessionGate>
  );
}
function DiagnosticsIdentity() {
  return <DiagnosticsContent key={useInsightIdentity()} />;
}
