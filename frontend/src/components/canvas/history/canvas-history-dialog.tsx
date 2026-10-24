"use client";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";

import { useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { X } from "lucide-react";
import { listProjectOperations } from "@/api/operations";
import { requireTask, taskLabels } from "@/components/generation/contracts";
import { TaskDetail } from "@/components/generation/generation-dialog";
import { CanvasMediaContext } from "../lib/canvas-media-context";
import { isProjectID } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

const labels = {
  all: "全部",
  image: "图片",
  video: "视频",
  audio: "音频",
  text: "文字",
};
type Modality = keyof typeof labels;

export function CanvasHistoryDialog({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const context = useContext(CanvasMediaContext);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        align="end"
        side="bottom"
        sideOffset={12}
        className="w-[440px] max-w-[calc(100dvw-2rem)] p-4"
        aria-label="生成历史"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
      >
        {open &&
          (context &&
          isProjectID(context.projectId) &&
          isProjectID(String(context.canvasId)) ? (
            <HistoryContent
              key={`${context.projectId}/${context.canvasId}`}
              projectID={context.projectId}
              canvasID={String(context.canvasId)}
              close={() => setOpen(false)}
            />
          ) : (
            <ContentState
              className="min-h-40"
              state="error"
              title="无法读取生成历史"
              description="请重新打开画布后重试。"
            />
          ))}
      </PopoverContent>
    </Popover>
  );
}

function HistoryContent({
  projectID,
  canvasID,
  close,
}: {
  projectID: string;
  canvasID: string;
  close: () => void;
}) {
  const [modality, setModality] = useState<Modality>("all");
  const [selected, setSelected] = useState<API.TaskSummary | null>(null);
  return (
    <>
      <div className="mb-4 flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">生成历史</h2>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="关闭生成历史"
          onClick={close}
        >
          <X />
        </Button>
      </div>
      <Tabs
        value={modality}
        onValueChange={(value) => setModality(value as Modality)}
      >
        <TabsList aria-label="生成历史类型">
          {(Object.keys(labels) as Modality[]).map((kind) => (
            <TabsTrigger key={kind} value={kind}>
              {labels[kind]}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <HistoryList
        key={`${projectID}/${canvasID}/${modality}`}
        projectID={projectID}
        canvasID={canvasID}
        modality={modality}
        onSelect={setSelected}
      />
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      >
        <DialogContent className="max-h-dialog overflow-auto sm:max-w-2xl">
          <DialogTitle>生成记录</DialogTitle>
          <DialogDescription>
            {selected?.model_name || "任务详情"}
          </DialogDescription>
          {selected?.id && (
            <TaskDetail
              key={selected.id}
              projectID={projectID}
              id={selected.id}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function HistoryList({
  projectID,
  canvasID,
  modality,
  onSelect,
}: {
  projectID: string;
  canvasID: string;
  modality: Modality;
  onSelect: (task: API.TaskSummary) => void;
}) {
  const [items, setItems] = useState<API.TaskSummary[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  const requestedCursor = useRef<string | undefined>(undefined);
  async function load(next?: string) {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    requestedCursor.current = next;
    setPending(true);
    setError("");
    try {
      const page = await listProjectOperations(
        {
          pid: projectID,
          canvas_id: canvasID,
          capability: modality === "all" ? undefined : `${modality}.generate`,
          cursor: next,
          limit: 30,
        },
        { signal: controller.signal },
      );
      if (
        !Array.isArray(page.items) ||
        (page.next_cursor != null && typeof page.next_cursor !== "string") ||
        (next && page.next_cursor === next)
      )
        throw new Error("生成历史返回无效分页");
      page.items.forEach((item) => requireTask(item, projectID));
      if (controller.signal.aborted) return;
      setItems((previous) => [
        ...new Map(
          [...(next ? previous : []), ...page.items!].map((item) => [
            item.id,
            item,
          ]),
        ).values(),
      ]);
      setCursor(page.next_cursor || undefined);
    } catch (cause) {
      if (!controller.signal.aborted) setError(readErrorMessage(cause));
    } finally {
      if (!controller.signal.aborted) setPending(false);
    }
  }
  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (active) void load();
    });
    return () => {
      active = false;
      request.current?.abort();
    };
    // The list remounts for every project, canvas and modality.
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="mt-3 flex min-h-40 flex-col gap-3">
      <div
        className="max-h-[min(24rem,50dvh)] overflow-auto"
        data-canvas-wheel-scroll
      >
        {pending && (
          <ContentState
            className="min-h-40"
            state="loading"
            title="正在读取历史"
          />
        )}
        {error && (
          <ContentState
            className="min-h-40"
            state="error"
            title="生成历史暂时无法读取"
            description={error}
          />
        )}
        {!pending && !error && !items.length && (
          <ContentState className="min-h-40" title="暂无生成历史" />
        )}
        {items.map((item) => (
          <Button
            key={item.id}
            variant="ghost"
            className="h-auto w-full justify-between gap-4 py-3 text-left"
            onClick={() => onSelect(item)}
          >
            <span className="min-w-0 truncate">
              {item.model_name || "生成任务"}
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {taskLabels[item.status!]}
            </span>
          </Button>
        ))}
      </div>
      <div className="mt-auto flex justify-end gap-2">
        {cursor && (
          <Button
            variant="ghost"
            size="sm"
            disabled={pending}
            onClick={() => void load(cursor)}
          >
            更多记录
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          disabled={pending}
          onClick={() => void load(error ? requestedCursor.current : undefined)}
        >
          {error ? "重新读取" : "刷新"}
        </Button>
      </div>
    </div>
  );
}
