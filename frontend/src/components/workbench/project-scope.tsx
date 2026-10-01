"use client";

import { useEffect, type ReactNode } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { PROJECTS_KEY, listProjects } from "@/components/project/queries";

export function ProjectScope({
  children,
}: {
  children: (project: { id: string; name: string }) => ReactNode;
}) {
  const parameters = useSearchParams();
  const router = useRouter(),
    pathname = usePathname();
  const chosen = parameters.get("project_id") ?? "";
  const query = useInfiniteQuery({
    queryKey: [...PROJECTS_KEY, "workbench-scope"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listProjects({ status: "active", limit: 100, cursor: pageParam }, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  const projects = query.data?.pages.flatMap((page) => page.items) ?? [];
  const project =
    projects.find((value) => value.id === chosen) ??
    (chosen ? undefined : projects[0]);
  const { hasNextPage, isFetchingNextPage, isError, fetchNextPage } = query;
  useEffect(() => {
    if (chosen && !project && hasNextPage && !isFetchingNextPage && !isError)
      void fetchNextPage();
  }, [
    chosen,
    project,
    hasNextPage,
    isFetchingNextPage,
    isError,
    fetchNextPage,
  ]);
  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-center gap-3">
        <Label htmlFor="active-project">当前项目</Label>
        <select
          id="active-project"
          value={project?.id ?? chosen}
          onChange={(event) => {
            const next = new URLSearchParams(parameters);
            next.set("project_id", event.target.value);
            for (const key of ["canvas_id", "node_id", "task_id", "asset_id"])
              next.delete(key);
            router.replace(`${pathname}?${next}`, { scroll: false });
          }}
          className="h-10 max-w-full min-w-48 rounded-lg border bg-background px-3 text-sm"
          disabled={query.isPending || query.isError}
        >
          <option value="" disabled>
            请选择项目
          </option>
          {projects.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}
            </option>
          ))}
        </select>
        {query.hasNextPage && (
          <Button
            variant="ghost"
            disabled={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            加载更多项目
          </Button>
        )}
      </div>
      {query.isPending ||
      (chosen && !project && query.hasNextPage && !query.isError) ? (
        <p role="status" className="text-muted-foreground">
          正在载入项目…
        </p>
      ) : query.isError ? (
        <div role="alert" className="space-y-3">
          <p>项目暂时无法读取。</p>
          <Button variant="outline" onClick={() => void query.refetch()}>
            重新读取
          </Button>
        </div>
      ) : !project ? (
        <div className="rounded-xl bg-muted/40 px-6 py-12 text-center">
          <p className="mb-5">
            {chosen
              ? "此项目未在当前工作区中找到。"
              : "新建一个项目，开始组织素材与创作。"}
          </p>
          <Button asChild>
            <Link href="/projects?create=true">新建项目</Link>
          </Button>
        </div>
      ) : (
        <div key={project.id}>{children(project)}</div>
      )}
    </div>
  );
}
