"use client";
import dayjs from "dayjs";
import Link from "next/link";
import { CanvasProjectCover } from "@/components/canvas/project/canvas-project-cover";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";
export function CanvasProjectCard({
  project,
  compact = false,
  onRename,
  onDelete,
  selected = false,
  onSelectionChange,
  selectionDisabled = false,
}: {
  project: import("../hooks/use-projects").CanvasLibraryItem;
  compact?: boolean;
  onRename: () => void;
  onDelete: () => void;
  selected?: boolean;
  onSelectionChange?: (selected: boolean) => void;
  selectionDisabled?: boolean;
}) {
  const actions = (
    <div className="relative z-10 flex gap-2">
      <Button
        variant="ghost"
        size="sm"
        className="max-md:min-h-11"
        onClick={onRename}
        aria-label={`编辑 ${project.title}`}
      >
        编辑
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="max-md:min-h-11"
        onClick={onDelete}
        aria-label={`删除 ${project.title}`}
      >
        删除
      </Button>
    </div>
  );
  return (
    <Card
      size="sm"
      className={cn(
        "group/card project-card relative h-full",
        compact
          ? "flex-row items-center gap-2 rounded-xl border border-border bg-transparent p-1 ring-0 md:gap-3 md:rounded-2xl"
          : "gap-2 overflow-visible rounded-none border-0 bg-transparent py-0 shadow-none ring-0",
      )}
    >
      <Link
        href={"/canvases/" + project.id}
        className="absolute inset-0 rounded-[inherit] outline-none focus-visible:ring-1 focus-visible:ring-ring"
        aria-label={"打开 " + project.title}
      />
      <CardContent
        className={cn(
          "pointer-events-none relative grid shrink-0 place-items-center overflow-hidden bg-muted p-0",
          compact
            ? "workspace-compact-cover"
            : "workspace-media group-focus-within/card:ring-1 group-focus-within/card:ring-ring",
          selected && "ring-2 ring-ring",
        )}
      >
        <CanvasProjectCover project={project} />
        {onSelectionChange && (
          <div className="project-card-selection pointer-events-auto absolute top-2 right-2 z-10 rounded-md bg-background/90 p-2">
            <Checkbox
              aria-label={"选择画布 " + project.title}
              checked={selected}
              disabled={selectionDisabled}
              onCheckedChange={(checked) => onSelectionChange(checked === true)}
            />
          </div>
        )}
      </CardContent>
      <CardHeader
        className={cn(
          "min-w-0",
          compact
            ? "flex min-w-0 flex-1 flex-col items-stretch gap-1 pr-1 pl-0"
            : "gap-1 px-0",
        )}
      >
        <CardTitle
          className={cn(
            "truncate",
            compact &&
              "group-data-[size=sm]/card:text-workspace-meta md:group-data-[size=sm]/card:text-xs",
          )}
          title={project.title}
        >
          {project.title}
        </CardTitle>
        <CardDescription>
          <time
            dateTime={project.updatedAt}
            className="text-workspace-meta whitespace-nowrap tabular-nums"
          >
            {dayjs(project.updatedAt).format("YYYY-MM-DD")}
          </time>
        </CardDescription>
        {!compact && <CardAction>{actions}</CardAction>}
      </CardHeader>
    </Card>
  );
}
