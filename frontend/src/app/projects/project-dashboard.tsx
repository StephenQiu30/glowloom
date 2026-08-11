"use client";

import { AlertCircle, ArrowRight, LoaderCircle, MoreHorizontal, Plus, Search } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { StudioShell } from "@/components/studio/studio-shell";
import { MetricGroup } from "@/components/studio/metric-group";
import { PageHeader } from "@/components/studio/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useCreateProjectMutation,
  useMeQuery,
  useProjectsQuery,
  useWorkspacesQuery,
} from "@/lib/server-state";

import { ProjectCreateDialog } from "./project-create-dialog";

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
    return <div className="grid min-h-screen place-items-center"><LoaderCircle aria-label="正在读取登录状态" className="animate-spin" /></div>;
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
      {notice ? <div className="pointer-events-none fixed top-24 right-6 z-50 bg-foreground px-4 py-3 text-sm text-background" role="status">{notice}</div> : null}
      <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
        {!authenticated ? (
          <Alert><AlertCircle aria-hidden="true" /><AlertTitle>需要登录</AlertTitle><AlertDescription>登录后管理真实项目与单集。</AlertDescription></Alert>
        ) : pageError ? (
          <Alert variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>项目库暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription></Alert>
        ) : !workspace || !projectsQuery.data ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle aria-label="正在加载项目库" className="animate-spin" /></div>
        ) : (
          <>
            <PageHeader
              description="以项目和单集组织生产事实，继续当前阶段，或追踪归档内容。"
              eyebrow={workspace.name}
              title="项目管理"
            />
            <MetricGroup
              className="mt-6"
              columns={3}
              items={[
                { label: "全部", value: projects.length },
                { label: "制作中", value: projects.filter((item) => item.status === "active").length },
                { label: "已归档", value: projects.filter((item) => item.status === "archived").length },
              ]}
              label="项目数量摘要"
            />

            {actionError ? <Alert className="mt-6" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>创建失败</AlertTitle><AlertDescription>{actionError}</AlertDescription></Alert> : null}

            <div className="flex flex-wrap items-center gap-2 py-5">
              {filters.map((item) => (
                <Button key={item.id} onClick={() => setFilter(item.id)} size="sm" variant={filter === item.id ? "secondary" : "ghost"}>{item.label}</Button>
              ))}
              <div className="relative ml-auto w-full sm:w-80">
                <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                <Input aria-label="搜索项目" className="pl-9" placeholder="按名称、简介或风格搜索" value={query} onChange={(event) => setQuery(event.target.value)} />
              </div>
            </div>
            <Separator />

            {visibleProjects.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>项目</TableHead>
                    <TableHead>规格</TableHead>
                    <TableHead>视觉风格</TableHead>
                    <TableHead>事实状态</TableHead>
                    <TableHead>版本</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleProjects.map((project) => (
                    <TableRow key={project.id}>
                      <TableCell className="min-w-72 whitespace-normal py-4">
                        <p className="font-medium">{project.name}</p>
                        <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">{project.description || "尚未填写项目简介"}</p>
                      </TableCell>
                      <TableCell className="font-mono text-xs">{project.aspect_ratio} · {Math.round(project.target_duration_ms / 1_000)}s</TableCell>
                      <TableCell>{project.visual_style || "未设置"}</TableCell>
                      <TableCell><Badge variant={project.status === "active" ? "secondary" : "outline"}>{project.status === "active" ? "制作中" : "已归档"}</Badge></TableCell>
                      <TableCell className="font-mono text-xs">r{project.revision}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button asChild size="sm" variant="ghost"><Link aria-label={`打开项目 ${project.name}`} href={`/projects/${project.id}`}>打开<ArrowRight aria-hidden="true" /></Link></Button>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild><Button aria-label={`${project.name} 更多操作`} size="icon-sm" variant="ghost"><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger>
                            <DropdownMenuContent align="end"><DropdownMenuItem asChild><Link href={`/projects/${project.id}`}>查看项目事实</Link></DropdownMenuItem></DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <div className="py-24 text-center"><p className="font-medium">没有匹配的项目</p><p className="mt-2 text-sm text-muted-foreground">调整搜索或创建第一个漫剧项目。</p></div>
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
