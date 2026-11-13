"use client";
import { lazy, Suspense, useState } from "react";
import {
  getCreationAnalytics,
  getBusinessAnalyticsCapability,
} from "@/api/analytics";
import { SessionGate, useSession } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import {
  Filters,
  dateFields,
  InsightsHeading,
  Loading,
  Metric,
  Notice,
  Panel,
  ReadState,
  Snapshot,
  useInsightIdentity,
  type FilterField,
} from "@/components/insights/shared";
import {
  numberValue,
  rateValue,
  timestamp,
  originName,
  statusName,
} from "@/components/insights/contracts";
import { useInsightRead } from "@/components/insights/use-insight-read";
const CreationChart = lazy(
  () => import("@/components/insights/creation-chart"),
);
function Breakdown({
  title,
  rows,
}: {
  title: string;
  rows: { name: string; count: number }[];
}) {
  return (
    <Panel title={title} description="同一接受时间窗口 · 按操作计数">
      {rows.length ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>分类</TableHead>
              <TableHead>操作数</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r, i) => (
              <TableRow key={`${r.name}-${i}`}>
                <TableCell className="break-all">{r.name}</TableCell>
                <TableCell>{numberValue(r.count)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <ContentState className="min-h-40" title="暂无记录" />
      )}
    </Panel>
  );
}
function Adoption() {
  return (
    <Panel
      title="产物采用与留存"
      description="需要独立历史事实与成熟生成同期群"
    >
      <ContentState
        className="min-h-44"
        title="未接入"
        description="尚无采用、编辑、访问与交付历史事实，无法计算采用率、用户转化或留存。"
      />
      <p className="insights-note">
        7日观察窗内的同期群应单列为未成熟；当前来源不支持判定同期群成熟度，不将其记为零或失败。
      </p>
    </Panel>
  );
}
function CreationView({
  query,
  revision,
}: {
  query: API.getCreationAnalyticsParams;
  revision: number;
}) {
  const identity = useInsightIdentity();
  const read = useInsightRead(
    `${identity}:${JSON.stringify(query)}`,
    (signal) => getCreationAnalytics(query, { signal }),
    revision,
  );
  const [view, setView] = useState("overview");
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
          {d.coverage.truncated ? (
            <Notice title="部分覆盖 · 统计达到读取上限">
              当前范围超过 {numberValue(d.coverage.max_operations)}{" "}
              条操作；以下所有指标、趋势与分类仅覆盖已读取部分。请缩小时间或筛选范围。
            </Notice>
          ) : null}
          <p className="insights-note">
            {d.scope === "platform" ? "平台汇总" : "个人范围"} ·{" "}
            {timestamp(d.from)} 至 {timestamp(d.to)}（不含） · 时区 {d.timezone}{" "}
            · 按接受时间归属，展示当前执行状态
          </p>
          <Tabs value={view} onValueChange={setView}>
            <TabsList variant="line">
              <TabsTrigger value="overview">创作概览</TabsTrigger>
              <TabsTrigger value="retention">转化与留存</TabsTrigger>
            </TabsList>
            <TabsContent value="overview">
              <div className="flex flex-col gap-5">
                <div className="insights-metrics">
                  <Metric
                    label="已受理任务"
                    value={numberValue(d.accepted_operations)}
                    context="已被系统接受的创作操作"
                  />
                  <Metric
                    label="已完成任务"
                    value={numberValue(d.completed_operations)}
                    context="同一接受窗口 · 当前已完成"
                  />
                  <Metric
                    label="已登记产物"
                    value={numberValue(d.outputs)}
                    context="已登记输出数，不等于采用或交付"
                  />
                  <Metric
                    label="终态成功率"
                    value={rateValue(d.terminal_success_rate)}
                    context="剔除部分完成后：已完成 /（已完成 + 失败 + 取消）；待核对与等待中不参与"
                  />
                </div>
                <div className="insights-panels">
                  <Panel
                    title="创作趋势"
                    description="上海自然日 · 已接受与已完成任务分别计数"
                  >
                    {d.daily.length && d.accepted_operations ? (
                      <>
                        <Suspense fallback={<Skeleton className="h-52" />}>
                          <CreationChart daily={d.daily} />
                        </Suspense>
                        <Collapsible>
                          <CollapsibleTrigger asChild>
                            <Button variant="ghost">查看趋势数据表</Button>
                          </CollapsibleTrigger>
                          <CollapsibleContent>
                            <Table>
                              <TableHeader>
                                <TableRow>
                                  <TableHead>日期</TableHead>
                                  <TableHead>已接受</TableHead>
                                  <TableHead>已完成</TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {d.daily.map((day) => (
                                  <TableRow key={day.date}>
                                    <TableCell>{day.date}</TableCell>
                                    <TableCell>
                                      {numberValue(day.accepted)}
                                    </TableCell>
                                    <TableCell>
                                      {numberValue(day.completed)}
                                    </TableCell>
                                  </TableRow>
                                ))}
                              </TableBody>
                            </Table>
                          </CollapsibleContent>
                        </Collapsible>
                      </>
                    ) : (
                      <ContentState
                        className="min-h-52"
                        title={
                          Object.keys(query).length > 1
                            ? "没有匹配的创作记录"
                            : "暂无创作记录"
                        }
                        description="系统接受创作操作后才计入统计。"
                      />
                    )}
                  </Panel>
                  <Adoption />
                </div>
                <div className="insights-panels">
                  <Breakdown
                    title="模型分布"
                    rows={d.models.map((m) => ({
                      name: m.model_name ?? m.model_id ?? "未关联模型",
                      count: m.operations,
                    }))}
                  />
                  <Breakdown
                    title="来源分布"
                    rows={d.entries.map((e) => ({
                      name: originName[e.key] ?? e.key,
                      count: e.operations,
                    }))}
                  />
                </div>
                <div className="insights-panels">
                  <Breakdown
                    title="执行状态"
                    rows={d.statuses.map((s) => ({
                      name: statusName[s.key] ?? s.key,
                      count: s.operations,
                    }))}
                  />
                  <Panel
                    title="来源与计数边界"
                    description="操作、尝试、产物与用户分别计数"
                  >
                    <dl className="grid grid-cols-2 gap-4">
                      <dt>调用尝试</dt>
                      <dd>{numberValue(d.attempts)}</dd>
                      <dt>失败操作</dt>
                      <dd>{numberValue(d.failed_operations)}</dd>
                      <dt>取消操作</dt>
                      <dd>{numberValue(d.cancelled_operations)}</dd>
                      <dt>部分完成</dt>
                      <dd>{numberValue(d.partial_operations)}</dd>
                      <dt>待核对操作</dt>
                      <dd>{numberValue(d.unknown_operations)}</dd>
                    </dl>
                    <p className="insights-note mt-4">
                      有效创作用户、完成交付与用户留存未接入；操作数不代表人数、下载或访问。
                    </p>
                  </Panel>
                </div>
              </div>
            </TabsContent>
            <TabsContent value="retention">
              <div className="flex flex-col gap-5">
                <div className="insights-metrics">
                  <Metric
                    label="有效创作用户"
                    value="未接入"
                    context="缺少用户行为事实"
                  />
                  <Metric
                    label="产物采用率"
                    value="不适用"
                    context="缺少采用事实与同期群"
                  />
                  <Metric
                    label="完成交付"
                    value="未接入"
                    context="缺少交付历史事实"
                  />
                  <Metric
                    label="7日留存"
                    value="不适用"
                    context="缺少历史访问与成熟同期群"
                  />
                </div>
                <Adoption />
              </div>
            </TabsContent>
          </Tabs>
          <Snapshot at={d.as_of} pending={read.pending} />
        </>
      )}
    </>
  );
}
function BusinessView({ revision }: { revision: number }) {
  const identity = useInsightIdentity();
  const read = useInsightRead(
    identity,
    (signal) => getBusinessAnalyticsCapability({ signal }),
    revision,
  );
  return (
    <>
      {read.error ? (
        <ReadState error={read.error} retry={read.refresh} />
      ) : !read.data ? (
        <Loading />
      ) : (
        <>
          <Notice title="经营数据未接入">
            财务授权与支付分摊来源尚未连接，金额暂不可用。管理员身份不代表经营核查授权。
          </Notice>
          <div className="insights-metrics">
            <Metric
              label="现金实收"
              value="未接入"
              context="需要核验支付事实"
            />
            <Metric
              label="现金退款"
              value="未接入"
              context="需要核验退款事实"
            />
            <Metric
              label="可变成本"
              value="未知"
              context="未连接完整成本来源"
            />
            <Metric
              label="贡献利润"
              value="不适用"
              context="收入与成本不完整"
            />
          </div>
          <div className="insights-panels">
            <Panel
              title="现金实收趋势"
              description="已核验收款，与确认收入分开"
            >
              <ContentState
                className="min-h-52"
                title="暂无可用财务来源"
                description="支付、退款与核销分摊接入并通过经营授权后开放。"
              />
            </Panel>
            <Panel title="收入、成本与利润" description="缺失来源不计为零">
              <ContentState
                className="min-h-52"
                title="利润暂不可计算"
                description="采购、支付手续费及其他可变成本尚未形成完整口径。"
              />
            </Panel>
          </div>
          <Panel title="报表导出" description="导出服务尚未开放">
            <div className="flex items-center gap-3">
              <Button disabled variant="secondary">
                导出不可用
              </Button>
              <p className="insights-note">报表导出尚未开放。</p>
            </div>
          </Panel>
          <Snapshot at={read.data.as_of} pending={read.pending} />
        </>
      )}
    </>
  );
}
function AnalyticsContent() {
  const admin = useSession().actor.platform_role === "platform_admin";
  const [query, setQuery] = useState<Record<string, string>>({
    scope: "personal",
  });
  const [filterDraft, setFilterDraft] = useState<Record<string, string>>({
    scope: "personal",
  });
  const [tab, setTab] = useState("creation");
  const [revision, setRevision] = useState(0);
  const fields: FilterField[] = [
    ...dateFields,
    ...(admin
      ? [
          {
            key: "scope",
            label: "分析范围",
            options: [
              ["personal", "我的创作"],
              ["platform", "平台汇总"],
            ],
          } as FilterField,
        ]
      : []),
    {
      key: "origin",
      label: "创作来源",
      options: [["", "全部来源"], ...Object.entries(originName)],
    },
    { key: "model_id", label: "模型版本 UUID" },
    { key: "project_id", label: "项目 UUID" },
  ];
  return (
    <PageLayout screen="analytics" hideSearch>
      <section className="insights">
        <InsightsHeading
          active="analytics"
          title={tab === "business" ? "经营分析" : "创作分析"}
          description={
            tab === "business"
              ? "经营核查能力 · 财务来源尚未接入"
              : query.scope === "platform"
                ? "平台汇总 · 已受理任务口径"
                : "我的创作 · 仅当前账号已受理任务"
          }
          refresh={() => setRevision((n) => n + 1)}
        />
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="creation">创作</TabsTrigger>
            {admin ? <TabsTrigger value="business">经营</TabsTrigger> : null}
          </TabsList>
          <TabsContent value="creation">
            <div className="flex flex-col gap-5">
              <Filters
                fields={fields}
                maxDays={31}
                defaults={{ scope: "personal" }}
                draft={filterDraft}
                onDraftChange={setFilterDraft}
                applied={query}
                onApply={(v) =>
                  setQuery({
                    ...v,
                    scope: admin ? (v.scope ?? "personal") : "personal",
                  })
                }
              />
              <CreationView query={query} revision={revision} />
              {admin && query.scope === "platform" ? (
                <p className="insights-note">
                  管理导出暂不可用：导出服务尚未开放。
                </p>
              ) : null}
            </div>
          </TabsContent>
          {admin ? (
            <TabsContent value="business">
              <div className="flex flex-col gap-5">
                <BusinessView revision={revision} />
              </div>
            </TabsContent>
          ) : null}
        </Tabs>
      </section>
    </PageLayout>
  );
}
export function Analytics() {
  return (
    <SessionGate>
      <AnalyticsIdentity />
    </SessionGate>
  );
}
function AnalyticsIdentity() {
  return <AnalyticsContent key={useInsightIdentity()} />;
}
