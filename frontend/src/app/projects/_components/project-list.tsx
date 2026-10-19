"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { LoadingState } from "@/components/layout/loading-state";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Grid2X2, MoreHorizontal } from "lucide-react";
import { useProjectPresentation } from "@/components/projects/project-presentation";
import { subscribeProjectInvalidation } from "@/components/projects/project-presentation-store";
import { ProjectRenameDialog } from "@/components/projects/project-rename-dialog";
import { useSpace } from "@/components/spaces/space-context";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import {
  DataPagination,
  type PageSize,
} from "@/components/media/data-pagination";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import { listProjects } from "@/api/projects";
import {
  canvasHref,
  projectErrorMessage,
  requireProjectList,
  type Project,
} from "@/components/projects/project";
export function ProjectList({ query, view }: { query: string; view: string }) {
  const [page, setPage] = useState<{
    items: Project[];
    next_cursor: string | null;
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
        order: "recent_content",
        ...(cursor ? { cursor } : {}),
      },
      { signal: controller.signal },
    )
      .then(requireProjectList)
      .then((result) => {
        if (!controller.signal.aborted) setPage(result);
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) setError(projectErrorMessage(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      active.current?.abort();
    };
  }, [query, retry, pageSize, cursor]);

  return (
    <div aria-busy={loading}>
      {loading && !page && (
        <LoadingState className="py-10 text-muted-foreground">
          正在读取画布…
        </LoadingState>
      )}
      {error && (
        <FeedbackToast
          message={error}
          action={
            loading
              ? undefined
              : {
                  label: "重试读取",
                  onClick: () => {
                    setError("");
                    setLoading(true);
                    setRetry((n) => n + 1);
                  },
                }
          }
        />
      )}
      {page?.items.length === 0 && (
        <Empty className="min-h-80">
          <EmptyHeader>
            <EmptyTitle>{query ? "没有匹配的画布" : "还没有画布"}</EmptyTitle>
            <EmptyDescription>
              {query ? "试试其他名称。" : "创建第一个空白画布，进入你的画布。"}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      <div className={view === "grid" ? "canvas-grid" : "flex flex-col gap-3"}>
        {page?.items.map((project) => (
          <ProjectCard
            key={project.id}
            initial={project}
            onCloseRename={reload}
          />
        ))}
      </div>
      {(page || error) && (
        <DataPagination
          label="画布分页"
          page={cursors.length}
          pageSize={pageSize}
          total={
            page?.items.length === 0 && cursors.length === 1 ? 0 : undefined
          }
          hasNext={!!page?.next_cursor}
          disabled={loading}
          onPageChange={(target) => {
            if (target < cursors.length)
              setCursors((value) => value.slice(0, -1));
            else if (page?.next_cursor)
              setCursors((value) => [...value, page.next_cursor!]);
            setPage(null);
            setError("");
            setLoading(true);
          }}
          onPageSizeChange={(size) => {
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
}: {
  initial: Project;
  onCloseRename: () => void;
}) {
  const { project } = useProjectPresentation(initial, false),
    space = useSpace();
  const [rename, setRename] = useState(false);
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
    <article className="canvas-tile">
      {project.canvas_id ? (
        <Link
          href={canvasHref(project)}
          aria-label={`打开画布 · ${project.name}`}
        >
          <div className="canvas-tile-preview">
            <Grid2X2 size={40} strokeWidth={1} />
          </div>
          <h3>{project.name}</h3>
          <p>
            {project.status === "archived" ? "已归档 · 只读 · " : ""}
            {new Date(date).toLocaleDateString("zh-CN")} {label}
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
            aria-label={`画布更多操作 · ${project.name}`}
          >
            <MoreHorizontal />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem
            disabled={!canRename}
            onSelect={() => setRename(true)}
          >
            重命名画布
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
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
