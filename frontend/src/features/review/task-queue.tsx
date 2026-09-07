"use client";

import {
  type TaskFilter,
  taskFilterLabels,
  subjectLabel,
  taskStatusLabels,
  shortId,
} from "./review-presentation";
import { Badge } from "@/components/ui/badge";
import { LoaderCircle } from "lucide-react";
import { cn } from "@/lib/class-names";

export function TaskQueue({
  isLoading,
  onFilterChange,
  onSelect,
  selectedTaskId,
  statusFilter,
  tasks,
}: {
  isLoading: boolean;
  onFilterChange: (value: TaskFilter) => void;
  onSelect: (taskId: string) => void;
  selectedTaskId: string;
  statusFilter: TaskFilter;
  tasks: API.HumanTaskListItemResponse[];
}) {
  return (
    <aside aria-label="审核任务队列" className="h-fit border bg-card">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div>
          <h2 className="font-semibold">项目任务</h2>
          <p className="mt-1 text-xs text-muted-foreground">服务端稳定排序，10 秒自动刷新</p>
        </div>
        <Badge variant="outline">{tasks.length}</Badge>
      </div>
      <div className="border-b p-4">
        <label className="grid gap-1.5 text-xs font-medium" htmlFor="review-task-status">
          任务状态筛选
          <select
            className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
            id="review-task-status"
            onChange={(event) => onFilterChange(event.target.value as TaskFilter)}
            value={statusFilter}
          >
            {Object.entries(taskFilterLabels).map(([value, label]) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </label>
      </div>
      {isLoading ? (
        <div className="grid min-h-32 place-items-center">
          <LoaderCircle aria-label="正在加载审核队列" className="size-4 animate-spin" />
        </div>
      ) : tasks.length === 0 ? (
        <p className="p-6 text-center text-sm text-muted-foreground">当前筛选下没有审核任务。</p>
      ) : (
        <ul className="divide-y">
          {tasks.map((task) => (
            <li key={task.id}>
              <button
                aria-pressed={selectedTaskId === task.id}
                className={cn(
                  "w-full p-4 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
                  selectedTaskId === task.id && "bg-muted/70",
                )}
                onClick={() => onSelect(task.id)}
                type="button"
              >
                <span className="flex items-center justify-between gap-3">
                  <span className="truncate text-sm font-medium">{subjectLabel(task.subject_type)}</span>
                  <Badge variant={task.status === "STALE" ? "destructive" : "outline"}>
                    {taskStatusLabels[task.status]}
                  </Badge>
                </span>
                <span className="mt-2 block truncate font-mono text-[11px] text-muted-foreground">
                  {shortId(task.id)} · revision {task.revision}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </aside>
  );
}
