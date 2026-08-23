"use client";

import { AlertCircle, FolderPlus, LoaderCircle, Plus, Search, SearchX } from "lucide-react";
import { useMemo, useState } from "react";

import { LayoutContainer } from "@/components/layout/layout-container";
import { StudioShell } from "@/components/studio/studio-shell";
import { MetricGroup } from "@/components/studio/metric-group";
import { PageHeader } from "@/components/studio/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
  const hasProjects = projects.length > 0;

  function clearFilters() {
    setQuery("");
    setFilter("all");
  }

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
    return (
      <StudioShell active="projects">
        <div className="grid min-h-[70dvh] place-items-center">
          <LoaderCircle aria-label="正在读取登录状态" className="animate-spin" />
        </div>
      </StudioShell>
    );
  }

  return (
    <StudioShell
      active="projects"
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      {notice ? <div className="pointer-events-none fixed top-24 right-6 z-50 bg-foreground px-4 py-3 text-sm text-background" role="status">{notice}</div> : null}
      <LayoutContainer className="py-12 md:py-14">
        {!authenticated ? (
          <Alert><AlertCircle aria-hidden="true" /><AlertTitle>需要登录</AlertTitle><AlertDescription>登录后管理真实项目与单集。</AlertDescription></Alert>
        ) : pageError ? (
          <Alert variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>项目库暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription></Alert>
        ) : !workspace || !projectsQuery.data ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle aria-label="正在加载项目库" className="animate-spin" /></div>
        ) : (
          <>
            <PageHeader
              actions={<Button disabled={!workspaceId} onClick={() => setCreateOpen(true)}><Plus aria-hidden="true" />创建项目</Button>}
              description="以项目和单集组织生产事实，继续当前阶段，或追踪归档内容。"
              eyebrow={workspace.name}
              title="项目管理"
            />
            <MetricGroup
              className="mt-8"
              columns={3}
              items={[
                { label: "全部", value: projects.length },
                { label: "制作中", value: projects.filter((item) => item.status === "active").length },
                { label: "已归档", value: projects.filter((item) => item.status === "archived").length },
              ]}
              label="项目数量摘要"
            />

            {actionError ? <Alert className="mt-6" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>创建失败</AlertTitle><AlertDescription>{actionError}</AlertDescription></Alert> : null}

            <div className="mt-8 flex flex-wrap items-center gap-2 bg-muted/45 p-1.5">
              <div className="flex items-center gap-1" aria-label="项目状态筛选" role="group">
                {filters.map((item) => (
                  <Button
                    aria-pressed={filter === item.id}
                    className={filter === item.id ? "bg-background hover:bg-background" : undefined}
                    key={item.id}
                    onClick={() => setFilter(item.id)}
                    size="sm"
                    variant="ghost"
                  >
                    {item.label}
                  </Button>
                ))}
              </div>
              <div className="relative ml-auto w-full sm:w-80">
                <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                <Input aria-label="搜索项目" className="h-9 border-0 bg-background pl-9 shadow-none focus-visible:ring-2" placeholder="按名称、简介或风格搜索" value={query} onChange={(event) => setQuery(event.target.value)} />
              </div>
            </div>

            {visibleProjects.length ? (
              <div className="mt-3 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {visibleProjects.map((project) => <ProjectServerCard key={project.id} project={project} />)}
              </div>
            ) : (
              <section className="mt-3 grid min-h-80 place-items-center bg-muted/30 px-6 py-16 text-center" aria-labelledby="project-empty-title">
                <div className="max-w-sm">
                  <div className="mx-auto grid size-11 place-items-center text-muted-foreground">
                    {hasProjects ? <SearchX aria-hidden="true" className="size-5" /> : <FolderPlus aria-hidden="true" className="size-5" />}
                  </div>
                  <h2 className="mt-5 text-lg font-semibold tracking-tight" id="project-empty-title">
                    {hasProjects ? "没有匹配的项目" : "还没有项目"}
                  </h2>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    {hasProjects ? "调整关键词或状态，或恢复全部项目继续浏览。" : "创建第一个项目，把剧本、单集与制作事实组织在一起。"}
                  </p>
                  {hasProjects ? (
                    <Button className="mt-5" onClick={clearFilters} variant="secondary">清除搜索和筛选</Button>
                  ) : (
                    <Button className="mt-5" disabled={!workspaceId} onClick={() => setCreateOpen(true)}><Plus aria-hidden="true" />创建第一个项目</Button>
                  )}
                </div>
              </section>
            )}
          </>
        )}
      </LayoutContainer>
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
