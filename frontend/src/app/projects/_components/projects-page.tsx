"use client";
import { Suspense, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { PageLayout } from "@/components/layout/page-layout";
import { ProjectList } from "./project-list";
export function ProjectsPage() {
  return (
    <Suspense>
      <ProjectsRoute />
    </Suspense>
  );
}
function ProjectsRoute() {
  const params = useSearchParams();
  const state = params.get("state");
  const query = (params.get("q") ?? "").trim();
  const sort =
    params.get("sort") === "metadata" ? "metadata" : "recent_content";
  const filter = state === "archived" || state === "deleted" ? state : "active";
  return (
    <ProjectsContent
      key={`${filter}:${query}`}
      filter={filter}
      query={query}
      sort={sort}
    />
  );
}
function ProjectsContent({
  filter,
  query,
  sort,
}: {
  filter: "active" | "archived" | "deleted";
  query: string;
  sort: "recent_content" | "metadata";
}) {
  const [search, setSearch] = useState(query);
  const router = useRouter();
  function navigate(value: typeof filter, nextQuery = query, nextSort = sort) {
    const params = new URLSearchParams({ state: value });
    if (nextQuery.trim()) params.set("q", nextQuery.trim());
    if (nextSort === "metadata") params.set("sort", nextSort);
    router.push(`/projects?${params}`);
  }
  return (
    <PageLayout
      screen="projects"
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: (value) => navigate(filter, value),
        label: "搜索画布",
      }}
    >
      <nav className="workspace-tabs">
        <Link href="/assets">生成历史</Link>
        <Link href="/projects" aria-current="page">
          画布
        </Link>
      </nav>
      <ProjectList
        key={`${query}:${filter}:${sort}`}
        query={query}
        view="grid"
        filter={filter}
        onFilterChange={(value) => navigate(value)}
        sort={sort}
        onSortChange={(value) => navigate(filter, query, value)}
      />
    </PageLayout>
  );
}
