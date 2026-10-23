"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { listProjectOperations } from "@/api/operations";
import { RequestError } from "@/lib/request";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { ProjectPicker } from "@/components/projects/project-picker";
import type { Project } from "@/components/projects/project";
import {
  requireTask,
  taskLabels,
  flowError,
} from "@/components/generation/contracts";
import { TaskDetail } from "@/components/generation/generation-dialog";
import { Button } from "@/components/ui/button";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";

export function GenerationHistory() {
  const session = useOptionalSession();
  const space = useSpace();
  return <HistoryScope key={`${session?.actor.id}/${space.selected?.id}`} />;
}
function HistoryScope() {
  const [project, setProject] = useState<Project | null>(null);
  return (
    <section aria-label="生成记录" className="grid gap-6">
      <div className="page-toolbar">
        <ProjectPicker value={project} onChange={setProject} compact />
      </div>
      {project ? (
        <HistoryList key={project.id} projectID={project.id} />
      ) : (
        <Empty className="min-h-64">
          <EmptyHeader>
            <EmptyTitle>选择项目，查看生成记录</EmptyTitle>
          </EmptyHeader>
        </Empty>
      )}
    </section>
  );
}
function HistoryList({ projectID }: { projectID: string }) {
  const [items, setItems] = useState<API.TaskSummary[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<API.TaskSummary | null>(null);
  const flight = useRef<AbortController | null>(null);
  const requestedCursor = useRef<string | undefined>(undefined);
  const load = useCallback(
    async (next?: string) => {
      requestedCursor.current = next;
      flight.current?.abort();
      const controller = new AbortController();
      flight.current = controller;
      setPending(true);
      setError("");
      try {
        const result = await listProjectOperations(
          { pid: projectID, limit: 30, cursor: next },
          { signal: controller.signal },
        );
        if (
          !Array.isArray(result.items) ||
          (result.next_cursor != null && typeof result.next_cursor !== "string")
        )
          throw new RequestError("invalid_receipt");
        result.items.forEach((item) => requireTask(item, projectID));
        if (controller.signal.aborted) return;
        setItems((previous) => {
          const merged = new Map((next ? previous : []).map((i) => [i.id, i]));
          result.items!.forEach((i) => merged.set(i.id, i));
          return [...merged.values()];
        });
        setCursor(result.next_cursor || undefined);
      } catch (reason) {
        if (!controller.signal.aborted) setError(flowError(reason));
      } finally {
        if (!controller.signal.aborted) setPending(false);
      }
    },
    [projectID],
  );
  useEffect(() => {
    // Start after mount; abort prevents an old project response entering the new scope.
    let active = true;
    queueMicrotask(() => {
      if (active) void load();
    });
    return () => {
      active = false;
      flight.current?.abort();
    };
  }, [load]);
  return (
    <>
      <div className="flex justify-end">
        <Button variant="ghost" disabled={pending} onClick={() => void load()}>
          刷新记录
        </Button>
      </div>
      {error && (
        <div role="alert" className="flex items-center gap-3 text-sm">
          <span>{error}</span>
          <Button
            variant="secondary"
            onClick={() => void load(requestedCursor.current)}
          >
            重试读取
          </Button>
        </div>
      )}
      {pending && (
        <p role="status" className="text-sm text-muted-foreground">
          正在读取生成记录…
        </p>
      )}
      {!pending && !error && !items.length && (
        <Empty className="min-h-64">
          <EmptyHeader>
            <EmptyTitle>还没有生成记录</EmptyTitle>
          </EmptyHeader>
        </Empty>
      )}
      <div className="grid gap-2">
        {items.map((item) => (
          <Button
            key={item.id}
            variant="ghost"
            className="h-auto min-h-16 justify-between gap-4 px-4 py-3 text-left"
            onClick={() => setSelected(item)}
          >
            <span className="grid gap-1">
              <span>{item.model_name || "生成任务"}</span>
              <span className="text-xs font-normal text-muted-foreground">
                {item.create_time &&
                Number.isFinite(Date.parse(item.create_time))
                  ? new Date(item.create_time).toLocaleString("zh-CN")
                  : ""}
              </span>
            </span>
            <span className="text-muted-foreground">
              {taskLabels[item.status!]}
            </span>
          </Button>
        ))}
      </div>
      {cursor && (
        <Button
          variant="secondary"
          className="justify-self-center"
          disabled={pending}
          onClick={() => void load(cursor)}
        >
          更多记录
        </Button>
      )}
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
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
