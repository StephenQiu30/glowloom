"use client";

import { useContext, useState, type ReactNode } from "react";
import useSWRInfinite from "swr/infinite";
import { ChevronDown } from "lucide-react";
import { listProjectOperations } from "@/api/operations";
import {
  activeTask,
  generationStatusLabel,
  requireTask,
} from "@/components/generation/contracts";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { ContentState } from "@/components/layout/content-state";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CanvasMediaContext } from "../lib/canvas-media-context";

const categories = {
  all: "全部",
  image: "图片",
  video: "视频",
  audio: "音频",
} as const;
type Category = keyof typeof categories;
const time = new Intl.DateTimeFormat("zh-CN", {
  hour: "2-digit",
  minute: "2-digit",
});
type Locate = (nodeID: string) => void;

export function CanvasHistoryDialog({
  children,
  onLocate,
}: {
  children: ReactNode;
  onLocate?: Locate;
}) {
  const [open, setOpen] = useState(false);
  const context = useContext(CanvasMediaContext);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={8}
        aria-label="生成历史"
        className="w-sm max-w-[calc(100dvw-2rem)] p-3"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
      >
        {open &&
          (context?.projectId && context.canvasId ? (
            <History
              key={context.projectId + context.canvasId}
              projectID={context.projectId}
              canvasID={String(context.canvasId)}
              onLocate={
                onLocate &&
                ((id) => {
                  onLocate(id);
                  setOpen(false);
                })
              }
            />
          ) : (
            <FeedbackToast message="无法读取生成历史" />
          ))}
      </PopoverContent>
    </Popover>
  );
}
function History({
  projectID,
  canvasID,
  onLocate,
}: {
  projectID: string;
  canvasID: string;
  onLocate?: Locate;
}) {
  const [category, setCategory] = useState<Category>("all");
  return (
    <>
      <h2 className="mb-2 text-sm font-medium">生成历史</h2>
      <Tabs value={category} onValueChange={(v) => setCategory(v as Category)}>
        <TabsList aria-label="生成历史类型">
          {(Object.keys(categories) as Category[]).map((k) => (
            <TabsTrigger key={k} value={k}>
              {categories[k]}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value={category}>
          <HistoryList
            key={category}
            {...{ projectID, canvasID, category, onLocate }}
          />
        </TabsContent>
      </Tabs>
    </>
  );
}
function HistoryList({
  projectID,
  canvasID,
  category,
  onLocate,
}: {
  projectID: string;
  canvasID: string;
  category: Category;
  onLocate?: Locate;
}) {
  const query = useSWRInfinite<API.TaskPageResponse>(
    (index, previous) =>
      index && !previous?.next_cursor
        ? null
        : [
            "canvas-generation-history",
            projectID,
            canvasID,
            category,
            previous?.next_cursor ?? "",
          ],
    async ([, project, canvas, kind, cursor]: [
      string,
      string,
      string,
      Category,
      string,
    ]) => {
      const page = await listProjectOperations({
        pid: project,
        canvas_id: canvas,
        capability: kind === "all" ? undefined : kind + ".generate",
        cursor: cursor || undefined,
        limit: 30,
      });
      if (
        !Array.isArray(page.items) ||
        (page.next_cursor != null &&
          (typeof page.next_cursor !== "string" || page.next_cursor === cursor))
      )
        throw Error("生成历史暂时无法读取");
      for (const item of page.items) {
        requireTask(item, project);
        if (item.source?.canvas_id !== canvas)
          throw Error("生成历史来源不匹配");
      }
      return page;
    },
    {
      shouldRetryOnError: false,
      refreshInterval: (pages) =>
        pages?.some((p) =>
          p.items?.some(
            (t) =>
              activeTask(t.status) ||
              (t.status === "manual" && !t.failure_code),
          ),
        )
          ? 10000
          : 0,
    },
  );
  const items = [
    ...new Map(
      (query.data ?? []).flatMap((p) =>
        (p.items ?? []).map((t) => [t.id, t] as const),
      ),
    ).values(),
  ];
  return (
    <div
      className="max-h-[min(60dvh,var(--radix-popover-content-available-height))] overflow-y-auto"
      data-canvas-wheel-scroll
    >
      {query.error && (
        <FeedbackToast
          message="生成历史暂时无法读取"
          action={{ label: "重新读取", onClick: () => void query.mutate() }}
        />
      )}
      {query.isLoading && <ContentState state="loading" title="正在读取历史" />}
      {!query.isLoading && !query.error && !items.length && (
        <ContentState title="暂无生成历史" />
      )}
      <ul aria-label="生成记录" className="divide-y divide-border">
        {items.map((item) => (
          <li key={item.id}>
            <HistoryItem
              item={item}
              projectID={projectID}
              onLocate={onLocate}
            />
          </li>
        ))}
      </ul>
      {query.data?.at(-1)?.next_cursor && (
        <Button
          variant="ghost"
          size="sm"
          disabled={query.isValidating}
          onClick={() => void query.setSize(query.size + 1)}
        >
          更多记录
        </Button>
      )}
    </div>
  );
}
function HistoryItem({
  item,
  projectID,
  onLocate,
}: {
  item: API.TaskSummary;
  projectID: string;
  onLocate?: Locate;
}) {
  const date = item.create_time && new Date(item.create_time);
  const results = item.canvas_results ?? [];
  const nodeID = item.source?.node_id;
  return (
    <article className="grid gap-2 py-2">
      <Collapsible>
        <header className="flex items-center justify-between gap-2">
          <CollapsibleTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="min-w-0 px-1"
              aria-label={`${item.model_name || "生成任务"}详情`}
            >
              <span className="truncate">{item.model_name || "生成任务"}</span>
              <ChevronDown />
            </Button>
          </CollapsibleTrigger>
          {date && Number.isFinite(date.getTime()) && (
            <time
              className="text-xs text-muted-foreground"
              dateTime={item.create_time}
              title={date.toLocaleString("zh-CN")}
            >
              {time.format(date)}
            </time>
          )}
        </header>
        <CollapsibleContent className="px-1 text-xs break-words whitespace-pre-wrap text-muted-foreground">
          {item.canvas_prompt_preview || generationStatusLabel(item)}
        </CollapsibleContent>
      </Collapsible>
      {results.length ? (
        <div className="grid grid-cols-3 gap-1.5">
          {results.map((out, i) => (
            <Button
              key={out.asset_id + ":" + i}
              variant="ghost"
              disabled={!onLocate || !nodeID}
              onClick={() => nodeID && onLocate?.(nodeID)}
              aria-label={`定位${item.model_name ?? ""}结果 ${i + 1} 的节点`}
              className="relative aspect-square h-auto overflow-hidden bg-muted p-0"
            >
              <ProjectMediaPreview
                projectID={projectID}
                id={out.asset_id!}
                kind={out.kind}
                compact
                interactive={false}
              />
            </Button>
          ))}
        </div>
      ) : (
        <span className="px-1 text-xs text-muted-foreground">
          {generationStatusLabel(item)}
        </span>
      )}
    </article>
  );
}
