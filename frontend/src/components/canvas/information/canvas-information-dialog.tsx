"use client";
import { useEffect, useState } from "react";
import "./information.css";
import { getCanvasInformation } from "@/api/canvases";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import { flowError } from "@/components/generation/contracts";
import { ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import type { CanvasNodeData } from "../types/canvas";
import { canvasNodeStatistics, formatCanvasCharge } from "./node-statistics";
function requiredInformation(
  value: API.CanvasInformation,
  canvasID: string,
  projectID: string,
) {
  if (
    value.id !== canvasID ||
    value.project_id !== projectID ||
    !value.created_at ||
    !value.updated_at ||
    !Number.isFinite(Date.parse(value.created_at)) ||
    !Number.isFinite(Date.parse(value.updated_at)) ||
    value.usage?.currency !== "CNY" ||
    !/^\d+$/.test(value.usage.settled_micros ?? "") ||
    !/^\d+$/.test(value.usage.task_count ?? "") ||
    !Array.isArray(value.usage.entries)
  )
    throw new Error("项目信息数据无效");
  for (const entry of value.usage.entries)
    if (
      !entry.id ||
      !entry.operation_id ||
      !/^\d+$/.test(entry.amount_micros ?? "") ||
      !entry.created_at ||
      !Number.isFinite(Date.parse(entry.created_at))
    )
      throw new Error("费用明细数据无效");
  return value as API.CanvasInformation & {
    created_at: string;
    updated_at: string;
    usage: API.CanvasUsage & {
      settled_micros: string;
      task_count: string;
      entries: API.CanvasCharge[];
    };
  };
}
type Information = ReturnType<typeof requiredInformation>;
const time = (value: string) =>
  new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(value));
export function CanvasInformationDialog({
  canvasID,
  projectID,
  nodes,
  onClose,
}: {
  canvasID: string;
  projectID: string;
  nodes: CanvasNodeData[];
  onClose: () => void;
}) {
  const [tab, setTab] = useState("basic"),
    [page, setPage] = useState(1),
    [retry, setRetry] = useState(0),
    [details, setDetails] = useState(false);
  const requestKey = `${canvasID}:${projectID}:${page}:${retry}`;
  const [completion, setCompletion] = useState<{
    key: string;
    data?: Information;
    error?: string;
  } | null>(null);
  const current = completion?.key === requestKey ? completion : null;
  const data = current?.data,
    error = current?.error,
    loading = !current;
  useEffect(() => {
    const controller = new AbortController(),
      scope = getScopeGeneration(),
      session = getSessionGeneration();
    let active = true;
    getCanvasInformation({ id: canvasID, page }, { signal: controller.signal })
      .then((value) => {
        if (
          !active ||
          controller.signal.aborted ||
          scope !== getScopeGeneration() ||
          session !== getSessionGeneration()
        )
          return;
        setCompletion({
          key: requestKey,
          data: requiredInformation(value, canvasID, projectID),
        });
      })
      .catch((reason: unknown) => {
        if (
          active &&
          !controller.signal.aborted &&
          scope === getScopeGeneration() &&
          session === getSessionGeneration()
        ) {
          setCompletion({ key: requestKey, error: flowError(reason) });
        }
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [canvasID, projectID, page, requestKey]);
  const all = canvasNodeStatistics(nodes);
  const distribution = [
    ...all.slice(0, 6),
    { type: "other", label: "其他", count: all[6].count + all[7].count },
  ];
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="canvas-information-surface flex min-h-[546px] flex-col gap-4 rounded-2xl sm:max-w-[800px]"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onPointerDown={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <DialogHeader>
          <DialogTitle>项目信息</DialogTitle>
          <DialogDescription className="sr-only">
            当前画布的基本信息和费用记录
          </DialogDescription>
        </DialogHeader>
        <Tabs
          className="min-h-0 flex-1 gap-4"
          value={tab}
          onValueChange={(value) => {
            setTab(value);
            setDetails(false);
          }}
        >
          <TabsList variant="line">
            <TabsTrigger value="basic">基础信息</TabsTrigger>
            <TabsTrigger value="usage">积分消耗</TabsTrigger>
          </TabsList>
          {loading && (
            <p role="status" className="py-10 text-center">
              正在读取项目信息…
            </p>
          )}
          {error && (
            <div role="alert" className="py-6">
              <p>{error}</p>
              <Button variant="outline" onClick={() => setRetry((v) => v + 1)}>
                重试
              </Button>
            </div>
          )}
          {data && (
            <>
              <TabsContent value="basic">
                <dl className="grid grid-cols-1 gap-4 py-2 sm:grid-cols-3">
                  {[
                    ["所有者", data.owner_name || "所有者信息不可用"],
                    ["创建时间", time(data.created_at)],
                    ["最新修改", time(data.updated_at)],
                  ].map(([label, value]) => (
                    <div key={label} className="space-y-4">
                      <dt className="text-muted-foreground">{label}</dt>
                      <dd>{value}</dd>
                    </div>
                  ))}
                </dl>
                <div className="pt-7">
                  <h3 className="mb-2 text-sm text-muted-foreground">
                    节点分布
                  </h3>
                  <dl className="grid grid-cols-2 gap-1 sm:grid-cols-4">
                    <div className="rounded-lg bg-muted p-6">
                      <dt className="text-xs text-muted-foreground">
                        全部节点
                      </dt>
                      <dd className="mt-3 text-xl tabular-nums">
                        {nodes.length}
                      </dd>
                    </div>
                    {distribution.map((item) => (
                      <div
                        key={item.type}
                        className="rounded-lg bg-[var(--canvas-reference-text-start)] p-6"
                      >
                        <dt className="text-muted-foreground">{item.label}</dt>
                        <dd className="mt-3 text-xl tabular-nums">
                          {item.count}
                        </dd>
                      </div>
                    ))}
                  </dl>
                </div>
              </TabsContent>
              <TabsContent value="usage">
                <dl className="flex gap-12 py-5">
                  <div>
                    <dt className="text-muted-foreground">
                      实际费用（人民币）
                    </dt>
                    <dd className="mt-2 text-2xl tabular-nums">
                      {formatCanvasCharge(data.usage.settled_micros)}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">任务数</dt>
                    <dd className="mt-2 text-2xl tabular-nums">
                      {data.usage.task_count}
                    </dd>
                  </div>
                </dl>
                <p className="text-xs text-muted-foreground">
                  当前按人民币结算，未配置积分兑换。仅统计本画布已确认的生成任务；AI
                  对话费用暂未归入。
                </p>
                <section aria-label="积分消耗" className="mt-4 min-h-32">
                  {data.usage.entries.length === 0 ? (
                    <p className="py-10 text-center text-muted-foreground">
                      暂无数据
                    </p>
                  ) : details ? (
                    <>
                      <ul className="max-h-60 divide-y overflow-y-auto">
                        {data.usage.entries.map((entry) => (
                          <li
                            key={entry.id}
                            className="flex justify-between gap-4 py-3 text-sm"
                          >
                            <div>
                              <p>{entry.model_key || "生成任务"}</p>
                              <p className="text-xs text-muted-foreground">
                                {time(entry.created_at!)} · {entry.operation_id}
                              </p>
                            </div>
                            <span className="shrink-0 tabular-nums">
                              {formatCanvasCharge(entry.amount_micros!)}
                            </span>
                          </li>
                        ))}
                      </ul>
                      <div className="mt-3 flex items-center justify-between">
                        <Button
                          variant="ghost"
                          disabled={page === 1}
                          onClick={() => setPage((v) => v - 1)}
                        >
                          上一页
                        </Button>
                        <span className="text-sm">第 {page} 页</span>
                        <Button
                          variant="ghost"
                          disabled={!data.usage.next_page}
                          onClick={() => setPage(data.usage.next_page!)}
                        >
                          下一页
                        </Button>
                      </div>
                    </>
                  ) : (
                    <p className="py-8 text-center text-muted-foreground">
                      已有费用记录，可查看明细
                    </p>
                  )}
                </section>
              </TabsContent>
            </>
          )}
        </Tabs>
        <div className="mt-auto flex justify-end pt-3">
          <Button
            variant="ghost"
            onClick={() => {
              setTab("usage");
              setDetails(true);
            }}
          >
            查看积分明细
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
