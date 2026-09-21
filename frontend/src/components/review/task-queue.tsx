"use client";

import { PageLoading } from "@/components/system/page-loading";
import {
  type TaskFilter,
  taskFilterLabels,
  subjectLabel,
  taskStatusLabels,
  shortId,
} from "@/components/review/review-presentation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Empty, EmptyHeader, EmptyDescription } from "@/components/ui/empty";

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
    <aside aria-label="审核任务队列" className="h-fit bg-transparent">
      <div className="flex items-center justify-between gap-3 p-4">
        <div>
          <h2 className="font-semibold">项目任务</h2>
          <p className="mt-1 text-xs text-muted-foreground">服务端稳定排序，10 秒自动刷新</p>
        </div>
        <Badge variant="outline">{tasks.length}</Badge>
      </div>
      <div className="p-4">
        <Field>
          <FieldLabel htmlFor="review-task-status">任务状态筛选</FieldLabel>
          <Select
            value={statusFilter}
            onValueChange={(value) => onFilterChange(value as TaskFilter)}
          >
            <SelectTrigger id="review-task-status" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {Object.entries(taskFilterLabels).map(([value, label]) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      </div>
      {isLoading ? (
        <PageLoading label="正在加载审核队列" />
      ) : tasks.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyDescription>当前筛选下没有审核任务。</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <ul className="flex flex-col gap-1">
          {tasks.map((task) => (
            <li key={task.id}>
              <Button
                variant="ghost"
                aria-pressed={selectedTaskId === task.id}
                className="h-auto w-full flex-col items-stretch gap-2 p-4 text-left whitespace-normal"
                onClick={() => onSelect(task.id)}
                type="button"
              >
                <span className="flex items-center justify-between gap-3">
                  <span className="truncate text-sm font-medium">
                    {subjectLabel(task.subject_type)}
                  </span>
                  <Badge variant={task.status === "STALE" ? "destructive" : "outline"}>
                    {taskStatusLabels[task.status]}
                  </Badge>
                </span>
                <span className="mt-2 block truncate font-mono text-[11px] text-muted-foreground">
                  {shortId(task.id)} · revision {task.revision}
                </span>
              </Button>
            </li>
          ))}
        </ul>
      )}
    </aside>
  );
}
