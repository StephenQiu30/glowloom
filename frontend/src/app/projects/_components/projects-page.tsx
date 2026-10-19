"use client";
import { useState } from "react";
import Link from "next/link";
import { PageLayout } from "@/components/layout/page-layout";
import { NewCanvasButton } from "@/components/projects/new-canvas-button";
import { ProjectList } from "./project-list";
export function ProjectsPage() {
  const [search, setSearch] = useState(""),
    [query, setQuery] = useState("");
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
      <div className="workspace-toolbar">
        <span className="text-xs text-muted-foreground">最近修改</span>
        <NewCanvasButton className="ml-auto" />
      </div>
      <ProjectList key={query} query={query} view="grid" />
    </PageLayout>
  );
}
