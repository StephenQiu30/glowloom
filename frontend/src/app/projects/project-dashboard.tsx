"use client";

import { Filter, Grid2X2, List, Plus, Search, Sparkles } from "lucide-react";
import { useMemo, useState } from "react";

import { ProjectCard } from "@/components/studio/project-card";
import { StudioShell } from "@/components/studio/studio-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { mockProjects } from "@/lib/mock-studio-data";

const filters = ["全部", "制作中", "待审核", "草稿"];

export function ProjectDashboard({ requestedWorkspaceId }: { requestedWorkspaceId?: string }) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("全部");
  const [created, setCreated] = useState(false);
  const visibleProjects = useMemo(() => {
    const statusMatch = (status: string) => filter === "全部" || (filter === "制作中" && status === "active") || (filter === "待审核" && status === "review") || (filter === "草稿" && status === "draft");
    return mockProjects.filter((project) => project.name.includes(query) && statusMatch(project.status));
  }, [filter, query]);

  return (
    <StudioShell
      active="projects"
      topAction={<Button className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]" onClick={() => setCreated(true)}><Plus aria-hidden="true" />创建项目</Button>}
    >
      <div className="mx-auto max-w-[1280px] px-5 py-9 md:px-8">
        {created ? <div className="mb-5 flex items-center gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status"><Sparkles className="size-4" aria-hidden="true" />新的 Mock 项目已加入草稿区。</div> : null}
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div>
            <Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">
              {requestedWorkspaceId ? `工作空间 ${requestedWorkspaceId}` : "Stephen 的创作空间"}
            </Badge>
            <h1 className="mt-3 text-3xl font-semibold tracking-[-0.035em]">项目库</h1>
            <p className="mt-2 text-sm text-slate-500">管理所有漫剧，从概念草稿推进到最终交付。</p>
          </div>
          <div className="flex gap-6 text-right">
            <div><p className="text-2xl font-semibold">3</p><p className="mt-1 text-xs text-slate-500">项目</p></div>
            <div><p className="text-2xl font-semibold">58</p><p className="mt-1 text-xs text-slate-500">计划集数</p></div>
            <div><p className="text-2xl font-semibold">21</p><p className="mt-1 text-xs text-slate-500">已完成</p></div>
          </div>
        </div>

        <div className="mt-8 flex flex-wrap items-center gap-3 border-b border-slate-200 pb-4">
          <div className="flex gap-1 rounded-xl bg-slate-100 p-1">
            {filters.map((item) => <button className={`rounded-lg px-3 py-1.5 text-sm transition ${filter === item ? "bg-white font-medium text-slate-900 shadow-sm" : "text-slate-500"}`} key={item} onClick={() => setFilter(item)} type="button">{item}</button>)}
          </div>
          <div className="relative ml-auto w-full sm:w-64"><Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-slate-400" aria-hidden="true" /><Input aria-label="搜索项目" className="pl-9" placeholder="搜索项目" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
          <Button aria-label="筛选项目" size="icon" variant="outline"><Filter aria-hidden="true" /></Button>
          <div className="hidden gap-1 rounded-lg border border-slate-200 p-1 sm:flex"><button className="grid size-7 place-items-center rounded-md bg-slate-100" aria-label="网格视图" type="button"><Grid2X2 className="size-4" aria-hidden="true" /></button><button className="grid size-7 place-items-center text-slate-400" aria-label="列表视图" type="button"><List className="size-4" aria-hidden="true" /></button></div>
        </div>

        {visibleProjects.length ? <div className="mt-6 grid gap-5 md:grid-cols-2 xl:grid-cols-3">{visibleProjects.map((project) => <ProjectCard key={project.id} project={project} />)}</div> : <div className="mt-16 text-center"><p className="font-medium">没有找到匹配的项目</p><p className="mt-1 text-sm text-slate-500">调整筛选条件或创建一个新项目。</p></div>}
      </div>
    </StudioShell>
  );
}
