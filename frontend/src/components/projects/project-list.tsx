"use client";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Grid2X2, MoreHorizontal, PanelsTopLeft } from "lucide-react";
import { useProjectPresentation } from "@/components/projects/project-presentation";
import { subscribeProjectInvalidation } from "@/components/projects/project-presentation-store";
import { ProjectRenameDialog } from "@/components/projects/project-rename-dialog";
import { ProjectCopyDialog } from "@/components/projects/project-copy-dialog";
import { useSpace } from "@/components/spaces/space-context";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuGroup,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import {
  DataPagination,
  type PageSize,
} from "@/components/media/data-pagination";

import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  SelectGroup,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { ProjectActionsDialog } from "@/components/projects/project-actions-dialog";
import {
  canManageProject,
  projectActions,
  type ProjectAction,
} from "@/components/projects/project-management";
import { listProjects } from "@/api/projects";
import {
  canvasHref,
  requireProjectList,
  type Project,
} from "@/components/projects/project";
export function ProjectList({
  query,
  view,
  filter = "active",
  onFilterChange,
  sort = "recent_content",
  onSortChange,
}: {
  sort?: "recent_content" | "metadata";
  onSortChange?: (sort: "recent_content" | "metadata") => void;
  query: string;
  view: string;
  filter?: "active" | "archived" | "deleted";
  onFilterChange?: (filter: "active" | "archived" | "deleted") => void;
}) {
  const space = useSpace();
  const [managing, setManaging] = useState(false);
  const toolbar = useRef<HTMLDivElement>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [operation, setOperation] = useState<{
    projects: Project[];
    action: ProjectAction;
  } | null>(null);
  const writable = space.selected?.writable === true;
  function toggle(id: string) {
    setSelected((value) =>
      value.includes(id) ? value.filter((v) => v !== id) : [...value, id],
    );
  }
  const [page, setPage] = useState<{
    items: Project[];
    next_cursor: string | null;
    server_time?: string;
  } | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [retry, setRetry] = useState(0);
  const active = useRef<AbortController | null>(null);
  const [pageSize, setPageSize] = useState<PageSize>(12);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const cursor = cursors.at(-1);
  function reload() {
    setCursors([undefined]);
    setPage(null);
    setLoading(true);
    setError("");
    setRetry((n) => n + 1);
  }
  useEffect(() => {
    const refresh = () => {
      if (
        document.visibilityState !== "hidden" &&
        !document.querySelector('[role="dialog"][data-state="open"]')
      )
        reload();
    };
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
    const unsubscribe = subscribeProjectInvalidation(refresh);
    return () => {
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", refresh);
      unsubscribe();
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    active.current = controller;
    listProjects(
      {
        q: query,
        limit: pageSize,
        ...(sort === "recent_content"
          ? { order: "recent_content" as const }
          : {}),
        ...(filter === "deleted" ? { deleted: true } : { status: filter }),
        ...(cursor ? { cursor } : {}),
      },
      { signal: controller.signal },
    )
      .then(requireProjectList)
      .then((result) => {
        if (!controller.signal.aborted) setPage(result);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) setError(readErrorMessage(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      active.current?.abort();
    };
  }, [query, retry, pageSize, cursor, filter, sort]);

  useEffect(() => {
    if (filter !== "deleted" || operation || !page?.server_time) return;
    const now = Date.parse(page.server_time);
    const nearest = Math.min(
      ...page.items.flatMap((p) =>
        p.purge_after ? [Date.parse(p.purge_after) - now] : [],
      ),
    );
    if (!Number.isFinite(nearest)) return;
    const timer = window.setTimeout(
      reload,
      Math.min(Math.max(nearest + 50, 50), 2_147_483_647),
    );
    return () => window.clearTimeout(timer);
  }, [filter, operation, page]);
  const items = page?.items ?? [];
  const selectable = items.filter((p) => p.status !== "copying" && writable);
  const chosen = items.filter((p) => selected.includes(p.id));
  const actions: ProjectAction[] =
    filter === "deleted"
      ? ["restore"]
      : filter === "archived"
        ? ["unarchive", "recycle"]
        : ["archive", "recycle"];
  return (
    <div aria-busy={loading}>
      <div
        ref={toolbar}
        role="group"
        aria-label="画布管理工具栏"
        tabIndex={-1}
        className="workspace-toolbar project-management-toolbar outline-none focus-visible:ring-1 focus-visible:ring-ring/50"
      >
        {managing ? (
          <>
            <Label className="flex items-center gap-2 text-sm">
              <Checkbox
                aria-label="选择本页"
                checked={
                  chosen.length === 0
                    ? false
                    : chosen.length === selectable.length
                      ? true
                      : "indeterminate"
                }
                disabled={loading || !!operation || selectable.length === 0}
                onCheckedChange={(checked) =>
                  setSelected(checked ? selectable.map((p) => p.id) : [])
                }
              />
              选择本页
            </Label>
            <span role="status" className="text-sm text-muted-foreground">
              已选择 {chosen.length} 项
            </span>
            <div className="ml-auto flex flex-wrap gap-2">
              {actions.map((action) => (
                <Button
                  key={action}
                  variant="outline"
                  size="default"
                  disabled={
                    loading ||
                    !!operation ||
                    chosen.length === 0 ||
                    !chosen.every((p) => canManageProject(p, action))
                  }
                  onClick={() => setOperation({ projects: chosen, action })}
                >
                  {action === "recycle"
                    ? "移入回收站"
                    : `${projectActions[action].label}所选`}
                </Button>
              ))}
              <Button
                variant="ghost"
                size="default"
                disabled={!!operation}
                onClick={() => {
                  setManaging(false);
                  setSelected([]);
                }}
              >
                取消
              </Button>
            </div>
          </>
        ) : (
          <>
            {(
              [
                ["active", "画布"],
                ["archived", "已归档"],
                ["deleted", "回收站"],
              ] as const
            ).map(([value, label]) => (
              <Button
                key={value}
                variant={filter === value ? "secondary" : "ghost"}
                size="default"
                aria-pressed={filter === value}
                onClick={() => onFilterChange?.(value)}
              >
                {label}
              </Button>
            ))}
            <Select
              value={sort}
              onValueChange={(value) =>
                onSortChange?.(value as "recent_content" | "metadata")
              }
            >
              <SelectTrigger
                aria-label="画布排序"
                variant="quiet"
                className="w-32"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="recent_content">最近修改</SelectItem>
                  <SelectItem value="metadata">项目更新</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <Button
              className="ml-auto"
              variant="outline"
              size="default"
              disabled={!writable || loading || selectable.length === 0}
              onClick={() => setManaging(true)}
            >
              批量管理
            </Button>
          </>
        )}
      </div>
      {loading && !page && (
        <ContentState state="loading" title="正在读取画布" />
      )}
      {error && (
        <ContentState
          state="error"
          title="暂时无法读取画布"
          description={error}
          actions={
            <Button
              disabled={loading}
              onClick={() => {
                setError("");
                setLoading(true);
                setRetry((n) => n + 1);
              }}
            >
              重试读取
            </Button>
          }
        />
      )}
      {!error && !loading && page?.items.length === 0 && (
        <ContentState
          icon={PanelsTopLeft}
          title={
            query
              ? "没有匹配的画布"
              : filter === "archived"
                ? "没有已归档的画布"
                : filter === "deleted"
                  ? "回收站为空"
                  : "还没有画布"
          }
          description={
            query
              ? "试试其他名称。"
              : filter === "active"
                ? "创建第一个空白画布，进入你的画布。"
                : "画布状态变化后会显示在这里。"
          }
        />
      )}
      <div className={view === "grid" ? "canvas-grid" : "flex flex-col gap-3"}>
        {page?.items.map((project) => (
          <ProjectCard
            key={project.id}
            initial={project}
            onCloseRename={reload}
            managing={managing}
            selected={selected.includes(project.id)}
            onToggle={() => {
              setManaging(true);
              toggle(project.id);
            }}
            onAction={(action) => setOperation({ projects: [project], action })}
            actions={actions}
          />
        ))}
      </div>
      {operation && (
        <ProjectActionsDialog
          projects={operation.projects}
          action={operation.action}
          onResult={(_success, failed) => setSelected(failed)}
          onRestoreFocus={() => toolbar.current?.focus()}
          onClose={() => {
            setOperation(null);
            reload();
          }}
        />
      )}
      {(page || error) && (
        <DataPagination
          label="画布分页"
          page={cursors.length}
          pageSize={pageSize}
          total={
            page?.items.length === 0 && cursors.length === 1 ? 0 : undefined
          }
          hasNext={!!page?.next_cursor}
          disabled={loading || !!operation}
          onPageChange={(target) => {
            setSelected([]);
            if (target < cursors.length)
              setCursors((value) => value.slice(0, -1));
            else if (page?.next_cursor)
              setCursors((value) => [...value, page.next_cursor!]);
            setPage(null);
            setError("");
            setLoading(true);
          }}
          onPageSizeChange={(size) => {
            setSelected([]);
            setPageSize(size);
            reload();
          }}
        />
      )}
    </div>
  );
}

function ProjectCard({
  initial,
  onCloseRename,
  managing,
  selected,
  onToggle,
  onAction,
  actions,
}: {
  initial: Project;
  onCloseRename: () => void;
  managing: boolean;
  selected: boolean;
  onToggle: () => void;
  onAction: (action: ProjectAction) => void;
  actions: ProjectAction[];
}) {
  const { project, namespace } = useProjectPresentation(initial, false),
    space = useSpace();
  const [rename, setRename] = useState(false);
  const [copy, setCopy] = useState(false);
  const canRename =
    project.status === "active" &&
    project.is_delete === false &&
    space.selected?.writable === true;
  const date = project.last_modified_at ?? project.update_time;
  const label =
    !project.last_modified_at ||
    project.last_modified_basis === "legacy_metadata"
      ? "项目更新"
      : project.last_modified_basis === "created"
        ? "创建"
        : "更新";
  return (
    <article
      className="canvas-tile"
      data-selected={selected}
      data-managing={managing}
    >
      {
        <Checkbox
          className="canvas-tile-check"
          aria-label={`选择画布 · ${project.name}`}
          checked={selected}
          disabled={project.status === "copying" || !space.selected?.writable}
          onCheckedChange={onToggle}
        />
      }
      {project.canvas_id ? (
        <Link
          href={canvasHref(project)}
          onClick={(event) => {
            if (managing || project.is_delete) {
              event.preventDefault();
              if (
                managing &&
                project.status !== "copying" &&
                space.selected?.writable
              )
                onToggle();
            }
          }}
          aria-disabled={project.is_delete === true}
          tabIndex={managing ? -1 : undefined}
          aria-label={`打开画布 · ${project.name}`}
        >
          <div className="canvas-tile-preview">
            <Grid2X2 size={28} strokeWidth={1} />
          </div>
          <h3>{project.name}</h3>
          <p>
            {project.is_delete
              ? "回收站 · "
              : project.status === "archived"
                ? "已归档 · 只读 · "
                : ""}
            {project.is_delete ? (
              <>
                {project.delete_time
                  ? new Date(project.delete_time).toLocaleString("zh-CN")
                  : "删除时间不可用"}{" "}
                删除
                <br />
                {project.purge_after
                  ? `${new Date(project.purge_after).toLocaleString("zh-CN")} 前可恢复`
                  : "恢复期限不可用"}
                <br />
                恢复到{project.status === "archived" ? "已归档" : "画布"}
              </>
            ) : (
              `${new Date(date).toLocaleDateString("zh-CN")} ${label}`
            )}
          </p>
        </Link>
      ) : (
        <>
          <h3>{project.name}</h3>
          <p>主画布关联不可用</p>
        </>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="canvas-tile-action"
            disabled={managing}
            aria-label={`画布更多操作 · ${project.name}`}
          >
            <MoreHorizontal />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuGroup>
            <DropdownMenuItem
              disabled={!canRename}
              onSelect={() => setRename(true)}
            >
              重命名画布
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={!canRename}
              onSelect={() => setCopy(true)}
            >
              复制画布
            </DropdownMenuItem>
            {actions.map((action) => (
              <DropdownMenuItem
                key={action}
                disabled={
                  !space.selected?.writable ||
                  !canManageProject(project, action)
                }
                onSelect={() => onAction(action)}
              >
                {action === "recycle"
                  ? "移入回收站"
                  : `${projectActions[action].label}画布`}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {copy && namespace && (
        <ProjectCopyDialog
          key={namespace}
          project={project}
          onClose={() => {
            setCopy(false);
            onCloseRename();
          }}
        />
      )}
      <ProjectRenameDialog
        project={project}
        open={rename}
        onOpenChange={(open) => {
          setRename(open);
          if (!open) onCloseRename();
        }}
      />
    </article>
  );
}
