"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
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
import { ThemeToggle } from "@/components/theme-toggle";
import {
  CURRENT_SESSION_KEY,
  getCurrentSession,
} from "@/features/auth/queries";
import { ApiError } from "@/lib/request";
import { CreateProjectDialog } from "./create-project-dialog";
import { ProjectList, type ProjectListView } from "./project-list";
import {
  PROJECTS_KEY,
  createProject,
  listProjects,
  listStylePresets,
} from "./queries";
import type { CreationBody } from "./creation";

type Filter = "all" | "active" | "archived" | "deleted";
function isAuthenticationError(error: unknown) {
  return (
    error instanceof ApiError &&
    (error.status === 401 || error.code === "must_change_password")
  );
}
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
  const [creating, setCreating] = useState(false);
  const session = useQuery({
    queryKey: CURRENT_SESSION_KEY,
    queryFn: ({ signal }) => getCurrentSession(signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const identityAllowed = Boolean(
    session.isSuccess &&
    !session.isFetching &&
    !session.error &&
    session.data &&
    !session.data.user.must_change_password &&
    !session.data.must_change_password,
  );
  const list = useQuery<Awaited<ReturnType<typeof listProjects>>>({
    queryKey: [
      ...PROJECTS_KEY,
      session.data?.user.id,
      { q: search, filter, cursor: cursors[page] },
    ],
    enabled: (query) =>
      identityAllowed && !isAuthenticationError(query.state.error),
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
  const canReadPresets = identityAllowed && !isAuthenticationError(list.error);
  const presets = useInfiniteQuery({
    queryKey: [...PROJECTS_KEY, "style-presets", session.data?.user.id],
    enabled: canReadPresets && creating,
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
  const authError = [
    session.error,
    list.error,
    presets.error,
    creation.error,
  ].find(
    (error) =>
      error instanceof ApiError &&
      (error.status === 401 || error.code === "must_change_password"),
  );
  const mustChangePassword = Boolean(
    session.data?.must_change_password ||
    session.data?.user.must_change_password,
  );
  const allowed = identityAllowed && !authError;
  useEffect(() => {
    if (authError || mustChangePassword) {
      void cache.cancelQueries({ queryKey: PROJECTS_KEY });
      if (creating) return;
      const suffix = params.toString();
      router.replace(
        `/login?returnTo=${encodeURIComponent(`/projects${suffix ? `?${suffix}` : ""}`)}`,
      );
    }
  }, [authError, mustChangePassword, creating, params, router, cache]);
  function syncUrl(
    nextSearch: string,
    nextFilter: Filter,
    nextView: ProjectListView,
  ) {
    const next = new URLSearchParams();
    if (nextSearch) next.set("q", nextSearch);
    if (nextFilter === "active" || nextFilter === "archived")
      next.set("status", nextFilter);
    if (nextFilter === "deleted") next.set("deleted", "true");
    if (nextView === "table") next.set("view", "table");
    router.replace(`/projects${next.size ? `?${next}` : ""}`, {
      scroll: false,
    });
  }
  const sessionFailure = session.error;
  if (!allowed && !creating)
    return (
      <main className="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-5 p-6">
        {sessionFailure && !authError ? (
          <Alert variant="destructive" className="border-0 bg-muted/40">
            <AlertTitle>无法打开项目工作区</AlertTitle>
            <AlertDescription>{sessionFailure.message}</AlertDescription>
            <Button variant="ghost" onClick={() => void session.refetch()}>
              重试确认身份
            </Button>
          </Alert>
        ) : (
          <p role="status">正在确认身份…</p>
        )}
      </main>
    );
  return (
    <div className="min-h-screen bg-background text-foreground">
      <a
        href="#projects-main"
        className="sr-only rounded bg-background p-3 focus:not-sr-only focus:absolute"
      >
        跳转到主要内容
      </a>
      <header className="mx-auto flex max-w-[1600px] flex-wrap items-center justify-between gap-4 px-6 py-5 lg:px-9">
        <Link
          href="/projects"
          className="inline-flex items-center gap-2.5 rounded-md font-semibold tracking-tight focus-visible:ring-2 focus-visible:ring-ring"
          aria-label="Lanverse，返回项目列表"
        >
          <span
            aria-hidden="true"
            className="flex size-8 items-center justify-center rounded-xl bg-foreground text-background"
          >
            L
          </span>
          Lanverse
        </Link>
        <div className="flex items-center gap-3">
          <Link
            href="/canvas"
            className="rounded-md text-sm hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            画布工作区
          </Link>
          {allowed && (
            <span className="hidden text-sm text-muted-foreground sm:block">
              {session.data?.user.display_name}
            </span>
          )}
          <ThemeToggle />
        </div>
      </header>
      <main
        id="projects-main"
        className="mx-auto flex max-w-[1600px] flex-col gap-8 px-6 pt-8 pb-16 lg:px-9"
      >
        <div className="flex flex-wrap items-start justify-between gap-5">
          <div className="flex flex-col gap-3">
            <p className="text-xs tracking-widest text-muted-foreground">
              LANVERSE / WORKSPACE
            </p>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              把故事，变成画面。
            </h1>
            <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
              创建作品项目，确定画幅与风格，进入画布继续创作。
            </p>
          </div>
          <CreateProjectDialog
            authenticationRequired={!allowed}
            onSubmit={(body, key) => creation.mutateAsync({ body, key })}
            onCreated={(id) => {
              void Promise.all([
                cache.invalidateQueries({ queryKey: PROJECTS_KEY }),
                cache.invalidateQueries({ queryKey: ["canvas", "projects"] }),
              ]).then(() => router.push(`/projects/${id}/canvas`));
            }}
            onOpenChange={setCreating}
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
        <div className="flex flex-wrap items-center justify-between gap-4">
          <form
            aria-label="项目搜索"
            className="flex w-full max-w-md items-center gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              const next = q.trim();
              setSearch(next);
              setCursors([undefined]);
              setPage(0);
              syncUrl(next, filter, view);
            }}
          >
            <Input
              aria-label="搜索项目"
              placeholder="按项目名称搜索…"
              value={q}
              onChange={(event) => setQ(event.target.value)}
            />
            <Button type="submit" variant="secondary">
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
            className="flex-wrap"
          >
            <ToggleGroupItem value="all">全部</ToggleGroupItem>
            <ToggleGroupItem value="active">进行中</ToggleGroupItem>
            <ToggleGroupItem value="archived">已归档</ToggleGroupItem>
            <ToggleGroupItem value="deleted">回收站</ToggleGroupItem>
          </ToggleGroup>
        </div>
        {filter === "deleted" && (
          <p className="text-sm text-muted-foreground">
            回收中的项目保留 30 天。
          </p>
        )}
        {!allowed ? (
          <Alert variant="destructive" className="border-0 bg-muted/40">
            <AlertTitle>请重新确认身份</AlertTitle>
            <AlertDescription>
              项目读取和创建已暂停，请重新登录后继续。
            </AlertDescription>
          </Alert>
        ) : list.isPending ? (
          <div
            role="status"
            aria-label="正在加载项目"
            className="grid gap-6 md:grid-cols-2 xl:grid-cols-3"
          >
            {[0, 1, 2].map((index) => (
              <Skeleton key={index} className="h-48 rounded-xl" />
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
              />
              <Pagination aria-label="项目分页">
                <PaginationContent>
                  <PaginationItem>
                    <Button
                      variant="ghost"
                      disabled={page === 0 || list.isFetching}
                      onClick={() => setPage((current) => current - 1)}
                    >
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
                    </Button>
                  </PaginationItem>
                </PaginationContent>
              </Pagination>
            </>
          )
        )}
      </main>
    </div>
  );
}
