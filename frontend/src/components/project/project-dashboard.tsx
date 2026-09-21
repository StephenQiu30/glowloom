"use client";

import { AlertCircle, FolderPlus, Plus, Search, SearchX } from "lucide-react";
import { PageLoading } from "@/components/system/page-loading";
import { toast } from "sonner";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useMemo, useState } from "react";

import { LayoutContainer } from "@/layout/layout-container";
import { StudioShell } from "@/components/identity/studio-shell";
import { PageHeader } from "@/components/studio/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { InputGroup, InputGroupInput, InputGroupAddon } from "@/components/ui/input-group";
import { useAuthSessionState } from "@/components/identity/use-auth-session";
import { appApiErrorMessage } from "@/lib/server-state";
import { useCreateProjectMutation, useProjectsQuery } from "@/components/project/endpoints";
import { useMeQuery, useWorkspacesQuery } from "@/components/identity/endpoints";

import { ProjectCreateDialog } from "@/components/project/project-create-dialog";
import { ProjectServerCard } from "@/components/project/project-server-card";

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
  const workspace =
    workspacesQuery.data?.find((item) => item.id === workspaceId) ??
    (meWorkspace?.id === workspaceId ? meWorkspace : undefined);
  const projectsQuery = useProjectsQuery(workspaceId ?? "", { skip: !workspaceId });
  const [createProject, createState] = useCreateProjectMutation();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ProjectFilter>("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const projects = useMemo(() => projectsQuery.data?.items ?? [], [projectsQuery.data?.items]);
  const visibleProjects = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
    return projects.filter((project) => {
      const statusMatches = filter === "all" || project.status === filter;
      const queryMatches =
        !normalizedQuery ||
        [project.name, project.description ?? "", project.visual_style ?? ""].some((value) =>
          value.toLocaleLowerCase("zh-CN").includes(normalizedQuery),
        );
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
    try {
      const created = await createProject(request).unwrap();
      setCreateOpen(false);
      toast.success(`项目已创建：${created.name}`);
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
        <PageLoading label="正在读取登录状态" />
      </StudioShell>
    );
  }

  return (
    <StudioShell
      active="projects"
      viewer={
        me.data
          ? {
              displayName: me.data.user.display_name?.trim() || me.data.user.email,
              workspaceName: me.data.workspace.name,
            }
          : undefined
      }
    >
      <LayoutContainer className="py-8 sm:py-10">
        {!authenticated ? (
          <Alert>
            <AlertCircle aria-hidden="true" />
            <AlertTitle>需要登录</AlertTitle>
            <AlertDescription>登录后管理真实项目与单集。</AlertDescription>
          </Alert>
        ) : pageError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>项目库暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription>
          </Alert>
        ) : !workspace || !projectsQuery.data ? (
          <PageLoading label="正在加载项目库" />
        ) : (
          <>
            <PageHeader
              actions={
                <Button disabled={!workspaceId} onClick={() => setCreateOpen(true)}>
                  <Plus data-icon="inline-start" aria-hidden="true" />
                  创建项目
                </Button>
              }
              description="从一份剧本开始，或打开已有作品，继续分集、人物设定与分镜。"
              eyebrow={workspace.name}
              title="我的作品"
            />

            {actionError && !createOpen ? (
              <Alert className="mt-6" variant="destructive">
                <AlertCircle aria-hidden="true" />
                <AlertTitle>创建失败</AlertTitle>
                <AlertDescription>{actionError}</AlertDescription>
              </Alert>
            ) : null}

            <div className="mt-8 flex flex-wrap items-center gap-2 bg-muted/45 p-1.5">
              <ToggleGroup
                type="single"
                value={filter}
                onValueChange={(value) => {
                  if (value) setFilter(value as ProjectFilter);
                }}
                aria-label="项目状态筛选"
              >
                {filters.map((item) => (
                  <ToggleGroupItem key={item.id} value={item.id}>
                    {item.label}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <InputGroup className="ml-auto w-full sm:w-80">
                <InputGroupAddon>
                  <Search data-icon="inline-start" aria-hidden="true" />
                </InputGroupAddon>
                <InputGroupInput
                  aria-label="搜索项目"
                  placeholder="按名称、简介或风格搜索"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
              </InputGroup>
            </div>

            {visibleProjects.length ? (
              <div className="mt-3 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {visibleProjects.map((project) => (
                  <ProjectServerCard key={project.id} project={project} />
                ))}
              </div>
            ) : (
              <Empty className="mt-3 min-h-80" aria-labelledby="project-empty-title">
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    {hasProjects ? (
                      <SearchX data-icon="inline-start" aria-hidden="true" />
                    ) : (
                      <FolderPlus data-icon="inline-start" aria-hidden="true" />
                    )}
                  </EmptyMedia>
                  <EmptyTitle id="project-empty-title" role="heading" aria-level={2}>
                    {hasProjects ? "没有匹配的项目" : "还没有项目"}
                  </EmptyTitle>
                  <EmptyDescription>
                    {hasProjects
                      ? "试试其他关键词，或清除筛选条件。"
                      : "创建第一个项目，导入剧本开始制作。"}
                  </EmptyDescription>
                </EmptyHeader>
                <EmptyContent>
                  <Button
                    onClick={hasProjects ? clearFilters : () => setCreateOpen(true)}
                    variant="outline"
                  >
                    {hasProjects ? "清除搜索和筛选" : "创建第一个项目"}
                  </Button>
                </EmptyContent>
              </Empty>
            )}
          </>
        )}
      </LayoutContainer>
      {workspaceId ? (
        <ProjectCreateDialog
          errorMessage={actionError ?? undefined}
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
