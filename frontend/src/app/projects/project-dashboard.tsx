"use client";

import { AlertCircle, FolderOpen, LoaderCircle, Plus, Search } from "lucide-react";
import { useMemo, useState } from "react";

import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useCreateProjectMutation,
  useMeQuery,
  useProjectsQuery,
  useWorkspacesQuery,
} from "@/lib/server-state";

import { ProjectCreateDialog } from "./project-create-dialog";
import { ProjectServerCard } from "./project-server-card";

type ProjectFilter = "all" | "active" | "archived";

const filters: Array<{ id: ProjectFilter; label: string }> = [
  { id: "all", label: "全部" },
  { id: "active", label: "制作中" },
  { id: "archived", label: "已归档" },
];

export function ProjectDashboard({ requestedWorkspaceId }: { requestedWorkspaceId?: string }) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const workspacesQuery = useWorkspacesQuery(undefined, { skip: !authenticated });
  const meWorkspace = me.data?.workspace;
  const workspaceId = requestedWorkspaceId ?? meWorkspace?.id;
  const workspace = workspacesQuery.data?.find((item) => item.id === workspaceId) ??
    (meWorkspace?.id === workspaceId ? meWorkspace : undefined);
  const projectsQuery = useProjectsQuery(workspaceId ?? "", { skip: !workspaceId });
  const [createProject, createState] = useCreateProjectMutation();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ProjectFilter>("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const projects = useMemo(() => projectsQuery.data?.items ?? [], [projectsQuery.data?.items]);
  const visibleProjects = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
    return projects.filter((project) => {
      const statusMatches = filter === "all" || project.status === filter;
      const queryMatches = !normalizedQuery || [project.name, project.description ?? "", project.visual_style ?? ""]
        .some((value) => value.toLocaleLowerCase("zh-CN").includes(normalizedQuery));
      return statusMatches && queryMatches;
    });
  }, [filter, projects, query]);

  async function handleCreate(request: API.ProjectCreateRequest): Promise<boolean> {
    setActionError(null);
    setNotice(null);
    try {
      const created = await createProject(request).unwrap();
      setCreateOpen(false);
      setNotice(`项目已创建：${created.name}`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  const pageError = me.error ?? workspacesQuery.error ?? projectsQuery.error;

  if (sessionState === "checking") {
    return <div className="grid min-h-screen place-items-center"><LoaderCircle aria-label="正在读取登录状态" className="animate-spin text-[#079db3]" /></div>;
  }

  return (
    <StudioShell
      active="projects"
      topAction={<Button disabled={!workspaceId} onClick={() => setCreateOpen(true)}><Plus aria-hidden="true" />创建项目</Button>}
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      {notice ? <div className="fixed top-24 right-6 z-50 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg" role="status">{notice}</div> : null}
      <div className="mx-auto max-w-[1280px] px-5 py-9 md:px-8">
        {!authenticated ? (
          <Alert className="border-amber-200 bg-amber-50 text-amber-800"><AlertCircle aria-hidden="true" /><AlertTitle>需要登录</AlertTitle><AlertDescription>登录后管理真实项目与单集。</AlertDescription></Alert>
        ) : pageError ? (
          <Alert variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>项目库暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription></Alert>
        ) : !workspace || !projectsQuery.data ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle aria-label="正在加载项目库" className="animate-spin text-[#079db3]" /></div>
        ) : (
          <>
            <header className="flex flex-wrap items-end justify-between gap-5">
              <div>
                <Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">{workspace.name}</Badge>
                <h1 className="mt-3 text-3xl font-semibold tracking-[-0.035em]">项目库</h1>
                <p className="mt-2 text-sm text-slate-500">管理漫剧项目，并进入每一集的真实生产阶段。</p>
              </div>
              <div className="flex gap-7 text-right">
                <div><p className="text-2xl font-semibold">{projects.length}</p><p className="mt-1 text-xs text-slate-500">项目</p></div>
                <div><p className="text-2xl font-semibold">{projects.filter((item) => item.status === "active").length}</p><p className="mt-1 text-xs text-slate-500">制作中</p></div>
                <div><p className="text-2xl font-semibold">{projects.filter((item) => item.status === "archived").length}</p><p className="mt-1 text-xs text-slate-500">已归档</p></div>
              </div>
            </header>

            {actionError ? <Alert className="mt-5" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>创建失败</AlertTitle><AlertDescription>{actionError}</AlertDescription></Alert> : null}

            <div className="mt-8 flex flex-wrap items-center gap-3 border-b border-slate-200 pb-4">
              <div className="flex gap-1 rounded-xl bg-slate-100 p-1">
                {filters.map((item) => (
                  <button className={`rounded-lg px-3 py-1.5 text-sm transition ${filter === item.id ? "bg-white font-medium text-slate-900 shadow-sm" : "text-slate-500"}`} key={item.id} onClick={() => setFilter(item.id)} type="button">{item.label}</button>
                ))}
              </div>
              <div className="relative ml-auto w-full sm:w-72">
                <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
                <Input aria-label="搜索项目" className="pl-9" placeholder="按名称、简介或风格搜索" value={query} onChange={(event) => setQuery(event.target.value)} />
              </div>
            </div>

            {visibleProjects.length ? (
              <div className="mt-6 grid gap-5 md:grid-cols-2 xl:grid-cols-3">{visibleProjects.map((project) => <ProjectServerCard key={project.id} project={project} />)}</div>
            ) : (
              <div className="mt-16 text-center"><FolderOpen className="mx-auto size-8 text-slate-300" aria-hidden="true" /><p className="mt-3 font-medium">没有匹配的项目</p><p className="mt-1 text-sm text-slate-500">调整搜索或创建第一个漫剧项目。</p></div>
            )}
          </>
        )}
      </div>
      {workspaceId ? (
        <ProjectCreateDialog
          isSubmitting={createState.isLoading}
          onOpenChange={setCreateOpen}
          onSubmit={handleCreate}
          open={createOpen}
          workspaceId={workspaceId}
        />
      ) : null}
    </StudioShell>
  );
}
