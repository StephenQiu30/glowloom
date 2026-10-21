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
  const state = useSearchParams().get("state");
  const filter = state === "archived" || state === "deleted" ? state : "active";
  return <ProjectsContent key={filter} filter={filter} />;
}
function ProjectsContent({
  filter,
}: {
  filter: "active" | "archived" | "deleted";
}) {
  const [search, setSearch] = useState(""),
    [query, setQuery] = useState("");
  const router = useRouter();
  const [sort, setSort] = useState<"recent_content" | "metadata">(
    "recent_content",
  );
  return (
    <PageLayout
      screen="projects"
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: setQuery,
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
        onFilterChange={(value) => router.push(`/projects?state=${value}`)}
        sort={sort}
        onSortChange={setSort}
      />
    </PageLayout>
  );
}
