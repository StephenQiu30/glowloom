"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { useRouter, useSearchParams } from "next/navigation";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@/components/ui/pagination";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ApiError } from "@/lib/request";
import { CreateProjectDialog } from "./create-project-dialog";
import {
  ProjectList,
  type ProjectListView,
  type ProjectAction,
} from "./project-list";
import {
  PROJECTS_KEY,
  createProject,
  listProjects,
  listStylePresets,
  type ProjectSummary,
} from "./queries";
import type { CreationBody } from "./creation";

type Filter = "all" | "active" | "archived" | "deleted";
const ProjectManagementDialog = dynamic(() =>
  import("./project-management-dialog").then(
    (module) => module.ProjectManagementDialog,
  ),
);
const ProjectCopyDialog = dynamic(
  () =>
    import("./project-copy-dialog").then((module) => module.ProjectCopyDialog),
  { ssr: false },
);
export function ProjectsWorkspace() {
  const router = useRouter();
  const params = useSearchParams();
  const cache = useQueryClient();
  const [q, setQ] = useState(params.get("q") ?? "");
  const [search, setSearch] = useState(q.trim());
  const [filter, setFilter] = useState<Filter>(
    params.get("deleted") === "true"
      ? "deleted"
      : params.get("status") === "active"
        ? "active"
        : params.get("status") === "archived"
          ? "archived"
          : "all",
  );
  const [view, setView] = useState<ProjectListView>(
    params.get("view") === "table" ? "table" : "cards",
  );
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [page, setPage] = useState(0);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [management, setManagement] = useState<{
    project: ProjectSummary;
    action: ProjectAction;
  } | null>(null);
  const creating = dialogOpen || params.get("create") === "true";
  const copySource = params.get("copy_source");
  const copyJob = params.get("copy_job") || undefined;
  const list = useQuery<Awaited<ReturnType<typeof listProjects>>>({
    queryKey: [...PROJECTS_KEY, { q: search, filter, cursor: cursors[page] }],
    queryFn: ({ signal }) =>
      listProjects(
        {
          q: search || undefined,
          status:
            filter === "active" || filter === "archived" ? filter : undefined,
          deleted: filter === "deleted",
          cursor: cursors[page],
          limit: 20,
        },
        signal,
      ),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const presets = useInfiniteQuery({
    queryKey: [...PROJECTS_KEY, "style-presets"],
    enabled: creating,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => listStylePresets(pageParam, signal),
    getNextPageParam: (result) => result.next_cursor ?? undefined,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const creation = useMutation({
    mutationFn: ({ body, key }: { body: CreationBody; key: string }) =>
      createProject(body, key),
    retry: false,
  });
  function syncUrl(
    nextSearch: string,
    nextFilter: Filter,
    nextView: ProjectListView,
  ) {
    const next = new URLSearchParams(params.toString());
    for (const key of ["q", "status", "deleted", "view"]) next.delete(key);
    if (nextSearch) next.set("q", nextSearch);
    if (nextFilter === "active" || nextFilter === "archived")
      next.set("status", nextFilter);
    if (nextFilter === "deleted") next.set("deleted", "true");
    if (nextView === "table") next.set("view", "table");
    router.replace(`/projects${next.size ? `?${next}` : ""}`, {
      scroll: false,
    });
  }
  return (
    <div id="projects-main" className="flex min-w-0 flex-col gap-7">
      {copySource ? (
        <ProjectCopyDialog
          key={`${copySource}:${copyJob ?? ""}`}
          sourceId={copySource}
          jobId={copyJob}
          onClose={() => {
            const next = new URLSearchParams(params.toString());
            next.delete("copy_source");
            next.delete("copy_job");
            router.replace(`/projects${next.size ? `?${next}` : ""}`, {
              scroll: false,
            });
          }}
          onJobSelected={(id) => {
            const next = new URLSearchParams(params.toString());
            next.set("copy_job", id);
            router.replace(`/projects?${next}`, { scroll: false });
          }}
        />
      ) : copyJob ? (
        <Alert variant="destructive" className="border-0 bg-muted/40">
          <AlertTitle>复制链接缺少来源项目</AlertTitle>
          <AlertDescription>
            请从来源项目的“复制任务”入口打开。
          </AlertDescription>
        </Alert>
      ) : null}
      {management && (
        <ProjectManagementDialog
          key={`${management.project.id}:${management.action}`}
          project={management.project}
          action={management.action}
          onClose={() => setManagement(null)}
          onChanged={() => {
            void Promise.all([
              cache.invalidateQueries({ queryKey: PROJECTS_KEY }),
              cache.invalidateQueries({ queryKey: ["canvas", "projects"] }),
            ]);
          }}
        />
      )}
      <div className="flex flex-wrap items-start justify-between gap-5">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">我的项目</h1>
          <p className="text-sm leading-6 text-muted-foreground">
            每一个故事，都从一张画布开始。
          </p>
        </div>
        <CreateProjectDialog
          open={creating}
          onSubmit={(body, key) => creation.mutateAsync({ body, key })}
          onCreated={(id) => {
            void Promise.all([
              cache.invalidateQueries({ queryKey: PROJECTS_KEY }),
              cache.invalidateQueries({ queryKey: ["canvas", "projects"] }),
            ]).then(() => router.push(`/projects/${id}/canvas`));
          }}
          onOpenChange={(open) => {
            setDialogOpen(open);
            if (!open && params.has("create")) {
              const next = new URLSearchParams(params.toString());
              next.delete("create");
              router.replace(`/projects${next.size ? `?${next}` : ""}`, {
                scroll: false,
              });
            }
          }}
          presets={presets.data?.pages.flatMap((result) => result.items) ?? []}
          presetsPending={presets.isPending && presets.isFetching}
          presetsError={presets.error}
          hasMorePresets={presets.hasNextPage}
          loadingMorePresets={presets.isFetchingNextPage}
          onLoadMorePresets={() => void presets.fetchNextPage()}
          onRetryPresets={() => void presets.refetch()}
        />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <form
          aria-label="项目搜索"
          className="order-2 flex w-full items-center gap-2 sm:w-auto sm:min-w-72 lg:order-none"
          onSubmit={(event) => {
            event.preventDefault();
            const next = q.trim();
            setSearch(next);
            setCursors([undefined]);
            setPage(0);
            syncUrl(next, filter, view);
          }}
        >
          <div className="relative min-w-0 flex-1">
            <Search
              aria-hidden="true"
              className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              aria-label="搜索项目"
              placeholder="按项目名称搜索…"
              value={q}
              onChange={(event) => setQ(event.target.value)}
              className="rounded-full border-0 bg-muted/60 pl-9 shadow-none"
            />
          </div>
          <Button type="submit" variant="ghost" className="rounded-full">
            搜索
          </Button>
        </form>
        <ToggleGroup
          type="single"
          value={filter}
          onValueChange={(value) => {
            if (
              value !== "all" &&
              value !== "active" &&
              value !== "archived" &&
              value !== "deleted"
            )
              return;
            setFilter(value);
            setCursors([undefined]);
            setPage(0);
            syncUrl(search, value, view);
          }}
          aria-label="项目状态筛选"
          className="order-1 flex-wrap gap-1 lg:order-none"
        >
          <ToggleGroupItem value="all" className="rounded-full px-4">
            全部
          </ToggleGroupItem>
          <ToggleGroupItem value="active" className="rounded-full px-4">
            进行中
          </ToggleGroupItem>
          <ToggleGroupItem value="archived" className="rounded-full px-4">
            已归档
          </ToggleGroupItem>
          <ToggleGroupItem value="deleted" className="rounded-full px-4">
            回收站
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
      {filter === "deleted" && (
        <p className="text-sm text-muted-foreground">
          回收中的项目保留 30 天，恢复后保留原有画布、素材和归档状态。
        </p>
      )}
      {list.isPending ? (
        <div
          role="status"
          aria-label="正在加载项目"
          className="grid gap-6 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4"
        >
          {[0, 1, 2].map((index) => (
            <Skeleton key={index} className="aspect-video rounded-2xl" />
          ))}
        </div>
      ) : list.error ? (
        <Alert variant="destructive" className="border-0 bg-muted/40">
          <AlertTitle>项目列表未能加载</AlertTitle>
          <AlertDescription>
            {list.error.message}
            {list.error instanceof ApiError && list.error.requestId && (
              <p>请求编号：{list.error.requestId}</p>
            )}
          </AlertDescription>
          <Button
            variant="ghost"
            onClick={() => void list.refetch()}
            disabled={list.isFetching}
          >
            重试读取项目
          </Button>
        </Alert>
      ) : (
        list.data && (
          <>
            <ProjectList
              projects={list.data.items.map((project) => ({
                id: project.id,
                name: project.name,
                aspectRatio: project.aspect_ratio,
                styleType: project.style_type,
                status: project.status,
                isDeleted: project.is_delete,
              }))}
              view={view}
              onViewChange={(value) => {
                setView(value);
                syncUrl(search, filter, value);
              }}
              onAction={(item, action) => {
                if (action === "copy" || action === "copies") {
                  const next = new URLSearchParams(params.toString());
                  next.set("copy_source", item.id);
                  next.delete("copy_job");
                  router.replace(`/projects?${next}`, { scroll: false });
                  return;
                }
                const project = list.data.items.find(
                  (entry) => entry.id === item.id,
                );
                if (project) setManagement({ project, action });
              }}
            />
            <Pagination aria-label="项目分页" className="mt-2 justify-end">
              <PaginationContent>
                <PaginationItem>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={page === 0 || list.isFetching}
                    onClick={() => setPage((current) => current - 1)}
                  >
                    <ChevronLeft aria-hidden="true" className="size-4" />
                    上一页
                  </Button>
                </PaginationItem>
                <PaginationItem>
                  <span
                    aria-live="polite"
                    className="px-3 text-sm tabular-nums"
                  >
                    第 {page + 1} 页
                  </span>
                </PaginationItem>
                <PaginationItem>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!list.data.next_cursor || list.isFetching}
                    onClick={() => {
                      const cursor = list.data?.next_cursor;
                      if (!cursor) return;
                      setCursors((current) => [
                        ...current.slice(0, page + 1),
                        cursor,
                      ]);
                      setPage((current) => current + 1);
                    }}
                  >
                    下一页
                    <ChevronRight aria-hidden="true" className="size-4" />
                  </Button>
                </PaginationItem>
              </PaginationContent>
            </Pagination>
          </>
        )
      )}
    </div>
  );
}
