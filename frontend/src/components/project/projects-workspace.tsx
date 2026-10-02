"use client";

import { useId, useRef, useState, useSyncExternalStore } from "react";
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
import { FOLDERS_KEY, findFolder, listFolders } from "./folder-queries";
import { folderUUID, type FolderSummary } from "./folder-model";
import { ProjectFolderCard, type FolderAction } from "./project-folder-card";
import type { FolderSelection } from "./project-folder-dialog";

type Filter = "all" | "active" | "archived" | "deleted";
const ProjectManagementDialog = dynamic(
  () =>
    import("./project-management-dialog").then(
      (module) => module.ProjectManagementDialog,
    ),
  { ssr: false },
);
const ProjectCopyDialog = dynamic(
  () =>
    import("./project-copy-dialog").then((module) => module.ProjectCopyDialog),
  { ssr: false },
);
const ProjectFolderDialog = dynamic(
  () =>
    import("./project-folder-dialog").then(
      (module) => module.ProjectFolderDialog,
    ),
  { ssr: false },
);
const subscribeOrigin = () => () => {};
const browserOrigin = () => window.location.origin;
const serverOrigin = () => "";
export function ProjectsWorkspace() {
  const params = useSearchParams();
  // URL-bound list state remounts on history navigation; unrelated Copy parameters keep their dialog intact.
  const binding = [
    "q",
    "status",
    "deleted",
    "view",
    "folder_id",
    "cursor",
    "page",
    "folder_cursor",
  ]
    .map((key) => params.get(key))
    .join("\0");
  return <ProjectsWorkspaceContent key={binding} />;
}
function ProjectsWorkspaceContent() {
  const router = useRouter();
  const params = useSearchParams();
  const cache = useQueryClient();
  const session = useId();
  const origin = useSyncExternalStore(
    subscribeOrigin,
    browserOrigin,
    serverOrigin,
  );
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
  const [folderId, setFolderId] = useState(
    params.get("folder_id") === "root" ? null : params.get("folder_id"),
  );
  const [cursors, setCursors] = useState<(string | undefined)[]>([
    params.get("cursor") || undefined,
  ]);
  const [page, setPage] = useState(0);
  const [pageNumber, setPageNumber] = useState(() => {
    const value = Number(params.get("page"));
    return Number.isSafeInteger(value) && value > 0 ? value : 1;
  });
  const [folderCursor, setFolderCursor] = useState(
    params.get("folder_cursor") || undefined,
  );
  const [folderSelection, setFolderSelection] =
    useState<FolderSelection | null>(null);
  const [folderNotice, setFolderNotice] = useState("");
  const returnTo = useRef<HTMLElement | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [management, setManagement] = useState<{
    project: ProjectSummary;
    action: ProjectAction;
  } | null>(null);
  const creating = dialogOpen || params.get("create") === "true";
  const copySource = params.get("copy_source");
  const copyJob = params.get("copy_job") || undefined;
  const validFolder = !folderId || folderUUID.safeParse(folderId).success;
  const firstFolders = useQuery({
    queryKey: [...FOLDERS_KEY, origin, "scope", session],
    queryFn: ({ signal }) => listFolders(undefined, signal),
    enabled: Boolean(origin),
    retry: false,
    staleTime: 0,
    refetchOnMount: "always",
    refetchOnWindowFocus: false,
  });
  const actorId = firstFolders.data?.current_actor_id;
  const orgId = firstFolders.data?.current_org_id;
  const scope =
    origin && actorId && orgId ? { origin, actorId, orgId } : undefined;
  const otherFolders = useQuery({
    queryKey: [...FOLDERS_KEY, origin, actorId, orgId, "page", folderCursor],
    queryFn: ({ signal }) => listFolders(folderCursor, signal),
    enabled: Boolean(
      scope && folderCursor && filter !== "deleted" && !folderId,
    ),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const folderPage = folderCursor ? otherFolders : firstFolders;
  const currentFolder = useQuery({
    queryKey: [...FOLDERS_KEY, origin, actorId, orgId, "selected", folderId],
    queryFn: ({ signal }) => findFolder(folderId!, scope!, signal),
    enabled: Boolean(scope && folderId && validFolder && filter !== "deleted"),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const list = useQuery<Awaited<ReturnType<typeof listProjects>>>({
    queryKey: [
      ...PROJECTS_KEY,
      origin,
      session,
      {
        q: search,
        filter,
        folderId: filter === "deleted" ? undefined : (folderId ?? "root"),
        cursor: cursors[page],
      },
    ],
    queryFn: ({ signal }) =>
      listProjects(
        {
          q: search || undefined,
          status:
            filter === "active" || filter === "archived" ? filter : undefined,
          deleted: filter === "deleted",
          cursor: cursors[page],
          limit: 20,
          folder_id: filter === "deleted" ? undefined : (folderId ?? "root"),
        },
        signal,
      ),
    retry: false,
    refetchOnWindowFocus: false,
    enabled: Boolean(origin && (validFolder || filter === "deleted")),
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
    nextFolder: string | null = folderId,
    cursor?: string,
    nextPage = 1,
  ) {
    const next = new URLSearchParams(params.toString());
    for (const key of [
      "q",
      "status",
      "deleted",
      "view",
      "folder_id",
      "cursor",
      "page",
      "folder_cursor",
    ])
      next.delete(key);
    if (nextSearch) next.set("q", nextSearch);
    if (nextFilter === "active" || nextFilter === "archived")
      next.set("status", nextFilter);
    if (nextFilter === "deleted") next.set("deleted", "true");
    if (nextView === "table") next.set("view", "table");
    if (nextFilter !== "deleted" && nextFolder)
      next.set("folder_id", nextFolder);
    if (cursor) {
      next.set("cursor", cursor);
      next.set("page", String(nextPage));
    }
    router.replace(`/projects${next.size ? `?${next}` : ""}`, {
      scroll: false,
    });
  }
  function openFolder(id: string | null) {
    setFolderId(id);
    setCursors([undefined]);
    setPage(0);
    setPageNumber(1);
    setFolderCursor(undefined);
    syncUrl(search, filter === "deleted" ? "all" : filter, view, id);
  }
  function selectFolder(folder: FolderSummary, action: FolderAction) {
    returnTo.current = document.querySelector(
      `[data-folder-actions="${folder.id}"]`,
    );
    setFolderSelection({ action, folder });
  }
  return (
    <div id="projects-main" className="flex min-w-0 flex-col gap-7">
      {scope && (
        <ProjectFolderDialog
          key={`${scope.origin}:${scope.actorId}:${scope.orgId}:${folderSelection?.action}:${folderSelection && "folder" in folderSelection ? folderSelection.folder.id : folderSelection?.action === "move" ? folderSelection.project.id : ""}`}
          scope={scope}
          selection={folderSelection}
          onClose={() => setFolderSelection(null)}
          returnFocus={() => {
            const target = returnTo.current;
            if (target?.isConnected) target.focus();
            else document.getElementById("projects-library-heading")?.focus();
          }}
          onChanged={(intent) => {
            setFolderNotice(
              intent.action === "recycle"
                ? "目录与全部成员项目已回收。"
                : "目录操作已确认，正在读取最新列表。",
            );
            void Promise.all([
              cache.invalidateQueries({ queryKey: PROJECTS_KEY }),
              cache.invalidateQueries({ queryKey: ["canvas", "projects"] }),
            ]);
            if (intent.action === "recycle" && folderId === intent.folderId)
              openFolder(null);
          }}
        />
      )}
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
      {management && scope && (
        <ProjectManagementDialog
          key={`${scope.origin}:${scope.actorId}:${scope.orgId}:${management.project.id}:${management.action}`}
          project={management.project}
          action={management.action}
          scope={scope}
          onClose={() => {
            const id = management.project.id;
            setManagement(null);
            window.requestAnimationFrame(() => {
              const target = document.querySelector<HTMLElement>(
                `[data-project-actions="${id}"]`,
              );
              (
                target ?? document.getElementById("projects-library-heading")
              )?.focus();
            });
          }}
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
          <h1
            id="projects-library-heading"
            tabIndex={-1}
            className="text-2xl font-semibold tracking-tight outline-none"
          >
            我的项目
          </h1>
          <p className="text-sm leading-6 text-muted-foreground">
            每一个故事，都从一张画布开始。
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {filter !== "deleted" && (
            <Button
              variant="outline"
              disabled={!scope || Boolean(firstFolders.error)}
              onClick={(event) => {
                returnTo.current = event.currentTarget;
                setFolderSelection({ action: "create" });
              }}
            >
              新建目录
            </Button>
          )}
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
            presets={
              presets.data?.pages.flatMap((result) => result.items) ?? []
            }
            presetsPending={presets.isPending && presets.isFetching}
            presetsError={presets.error}
            hasMorePresets={presets.hasNextPage}
            loadingMorePresets={presets.isFetchingNextPage}
            onLoadMorePresets={() => void presets.fetchNextPage()}
            onRetryPresets={() => void presets.refetch()}
          />
        </div>
      </div>
      {folderNotice && (
        <p role="status" className="text-sm text-muted-foreground">
          {folderNotice}
        </p>
      )}
      {filter !== "deleted" && (
        <nav
          aria-label="项目目录路径"
          className="flex min-w-0 flex-wrap items-center gap-2 text-sm"
        >
          <Button
            variant="link"
            className="px-0"
            onClick={() => openFolder(null)}
          >
            根目录
          </Button>
          {folderId && (
            <>
              <span aria-hidden="true">/</span>
              <span aria-current="page" className="min-w-0 break-words">
                {currentFolder.data?.name ?? "目录"} · {folderId.slice(0, 8)}
              </span>
              {currentFolder.data && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={(event) => {
                    returnTo.current = event.currentTarget;
                    setFolderSelection({
                      action: "rename",
                      folder: currentFolder.data!,
                    });
                  }}
                >
                  修改当前目录名称
                </Button>
              )}
            </>
          )}
        </nav>
      )}
      {filter !== "deleted" && !validFolder && (
        <Alert variant="destructive">
          <AlertTitle>目录链接无效</AlertTitle>
          <AlertDescription>
            目录编号必须是有效 UUID。请返回根目录。
          </AlertDescription>
        </Alert>
      )}
      {filter !== "deleted" && currentFolder.error && (
        <Alert variant="destructive">
          <AlertTitle>当前目录不可读取</AlertTitle>
          <AlertDescription>{currentFolder.error.message}</AlertDescription>
          <Button
            variant="outline"
            onClick={() => void currentFolder.refetch()}
          >
            重试读取当前目录
          </Button>
        </Alert>
      )}
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
            setPageNumber(1);
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
            setPageNumber(1);
            if (value === "deleted") setFolderId(null);
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
      {filter !== "deleted" && !folderId && (
        <section aria-label="项目目录" className="flex flex-col gap-4">
          <h2 className="text-sm font-medium text-muted-foreground">
            项目目录
          </h2>
          {folderPage.isPending ? (
            <p role="status">正在读取目录…</p>
          ) : folderPage.error ? (
            <Alert variant="destructive">
              <AlertTitle>目录列表未能加载</AlertTitle>
              <AlertDescription>{folderPage.error.message}</AlertDescription>
              <Button
                variant="outline"
                onClick={() => void folderPage.refetch()}
              >
                重试读取目录
              </Button>
            </Alert>
          ) : (
            scope &&
            folderPage.data && (
              <>
                {folderPage.data.current_actor_id !== scope.actorId ||
                folderPage.data.current_org_id !== scope.orgId ? (
                  <p role="alert">目录身份或组织已变化，请重新读取。</p>
                ) : folderPage.data.items.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    当前页没有目录。
                  </p>
                ) : (
                  <ul className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
                    {folderPage.data.items.map((folder) => (
                      <li key={folder.id} className="min-w-0">
                        <ProjectFolderCard
                          folder={folder}
                          scope={scope}
                          onOpen={openFolder}
                          onAction={selectFolder}
                        />
                      </li>
                    ))}
                  </ul>
                )}
                <div className="flex flex-wrap justify-end gap-2">
                  {folderCursor && (
                    <Button
                      variant="ghost"
                      disabled={folderPage.isFetching}
                      onClick={() => {
                        setFolderCursor(undefined);
                        const next = new URLSearchParams(params.toString());
                        next.delete("folder_cursor");
                        router.replace(
                          `/projects${next.size ? `?${next}` : ""}`,
                          { scroll: false },
                        );
                      }}
                    >
                      目录首页
                    </Button>
                  )}
                  {folderPage.data.next_cursor && (
                    <Button
                      variant="ghost"
                      disabled={folderPage.isFetching}
                      onClick={() => {
                        const cursor = folderPage.data!.next_cursor!;
                        setFolderCursor(cursor);
                        const next = new URLSearchParams(params.toString());
                        next.set("folder_cursor", cursor);
                        router.replace(`/projects?${next}`, { scroll: false });
                      }}
                    >
                      下一页目录
                    </Button>
                  )}
                </div>
              </>
            )
          )}
        </section>
      )}
      {validFolder || filter === "deleted" ? (
        list.isPending ? (
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
                  coverAssetId: project.cover_asset_id,
                  coverUnavailable: project.cover_unavailable,
                }))}
                view={view}
                scope={scope}
                onViewChange={(value) => {
                  setView(value);
                  syncUrl(
                    search,
                    filter,
                    value,
                    folderId,
                    cursors[page],
                    pageNumber,
                  );
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
                  if (action === "move") {
                    if (project) {
                      returnTo.current = document.querySelector(
                        `[data-project-actions="${project.id}"]`,
                      );
                      setFolderSelection({ action: "move", project });
                    }
                    return;
                  }
                  if (project) setManagement({ project, action });
                }}
              />
              <Pagination aria-label="项目分页" className="mt-2 justify-end">
                <PaginationContent>
                  <PaginationItem>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={(page === 0 && !cursors[0]) || list.isFetching}
                      onClick={() => {
                        const previous = Math.max(0, page - 1);
                        const cursor =
                          page === 0 ? undefined : cursors[previous];
                        setPage(previous);
                        if (page === 0) setCursors([undefined]);
                        setPageNumber(
                          page === 0 ? 1 : Math.max(1, pageNumber - 1),
                        );
                        syncUrl(
                          search,
                          filter,
                          view,
                          folderId,
                          cursor,
                          page === 0 ? 1 : pageNumber - 1,
                        );
                      }}
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
                      第 {pageNumber} 页
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
                        setPageNumber((current) => current + 1);
                        syncUrl(
                          search,
                          filter,
                          view,
                          folderId,
                          cursor,
                          pageNumber + 1,
                        );
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
        )
      ) : null}
    </div>
  );
}
