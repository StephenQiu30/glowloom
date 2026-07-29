"use client";

import {
  ArrowRight,
  Clapperboard,
  FolderKanban,
  LoaderCircle,
  LogOut,
  Plus,
  Settings,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useState } from "react";
import { useDispatch } from "react-redux";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApi,
  appApiErrorMessage,
  useCreateProjectMutation,
  useLogoutMutation,
  useMeQuery,
  useProjectsQuery,
  useWorkspacesQuery,
} from "@/lib/server-state";
import { clearAccessToken, hasAccessToken } from "@/lib/auth-session";
import type { AppStore } from "@/lib/redux-store";

export function ProjectDashboard({ requestedWorkspaceId }: { requestedWorkspaceId?: string }) {
  const router = useRouter();
  const dispatch = useDispatch<AppStore["dispatch"]>();
  const authState = useAuthSessionState();
  const isAuthenticated = authState === "authenticated";
  const [commandError, setCommandError] = useState<string | null>(null);
  const me = useMeQuery(undefined, { skip: !isAuthenticated });
  const workspaces = useWorkspacesQuery(undefined, { skip: !isAuthenticated });
  const activeWorkspaces = workspaces.data?.filter((workspace) => workspace.status === "active");
  const selectedWorkspace =
    activeWorkspaces?.find((workspace) => workspace.id === requestedWorkspaceId) ??
    activeWorkspaces?.find((workspace) => workspace.id === me.data?.workspace.id) ??
    me.data?.workspace;
  const workspaceId = selectedWorkspace?.id;
  const projects = useProjectsQuery(workspaceId ?? "", { skip: !workspaceId });
  const [createProject, createState] = useCreateProjectMutation();
  const [logout, logoutState] = useLogoutMutation();

  useEffect(() => {
    if (authState === "anonymous") {
      router.replace("/login");
    }
  }, [authState, router]);

  useEffect(() => {
    if (me.isError && !hasAccessToken()) {
      router.replace("/login");
    }
  }, [me.isError, router]);

  async function handleCreateProject(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workspaceId) return;
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    setCommandError(null);
    try {
      await createProject({
        workspace_id: workspaceId,
        name: String(form.get("name")),
        description: String(form.get("description")) || null,
        aspect_ratio: "9:16",
        language: "zh-CN",
        visual_style: null,
        target_duration_ms: 90_000,
      }).unwrap();
      formElement.reset();
    } catch (error: unknown) {
      setCommandError(appApiErrorMessage(error));
    }
  }

  async function handleLogout() {
    try {
      await logout().unwrap();
    } finally {
      clearAccessToken();
      dispatch(appApi.util.resetApiState());
      router.replace("/login");
    }
  }

  if (authState === "checking" || me.isLoading) {
    return (
      <main className="grid min-h-screen place-items-center" aria-live="polite">
        <LoaderCircle className="size-6 animate-spin" aria-hidden="true" />
        <span className="sr-only">正在加载项目工作台</span>
      </main>
    );
  }

  if (authState === "anonymous") return null;

  if (me.isError) {
    return (
      <main className="mx-auto max-w-xl px-6 py-20">
        <Alert variant="destructive">
          <AlertTitle>无法加载工作空间</AlertTitle>
          <AlertDescription>{appApiErrorMessage(me.error)}</AlertDescription>
        </Alert>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-muted/30">
      <header className="border-b bg-background">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
          <Link className="flex items-center gap-3 font-semibold" href="/projects">
            <span className="grid size-9 place-items-center rounded-xl bg-primary text-primary-foreground">
              <Clapperboard className="size-5" aria-hidden="true" />
            </span>
            Lanverse
          </Link>
          <div className="flex items-center gap-3">
            <div className="hidden text-right sm:block">
              <p className="text-sm font-medium">{me.data?.user.display_name}</p>
              <p className="text-xs text-muted-foreground">{me.data?.user.email}</p>
            </div>
            <Button asChild aria-label="账户与工作空间" size="icon" variant="outline">
              <Link href="/workspaces">
                <Settings aria-hidden="true" />
              </Link>
            </Button>
            <Button
              aria-label="退出登录"
              disabled={logoutState.isLoading}
              onClick={handleLogout}
              size="icon"
              variant="outline"
            >
              <LogOut aria-hidden="true" />
            </Button>
          </div>
        </div>
      </header>

      <div className="mx-auto grid max-w-6xl gap-8 px-6 py-10 lg:grid-cols-[1fr_20rem]">
        <section>
          <div className="mb-7 flex items-end justify-between gap-4">
            <div>
              <Badge variant="secondary">{me.data?.workspace.role}</Badge>
              <h1 className="mt-3 text-3xl font-semibold tracking-tight">项目</h1>
              {activeWorkspaces && activeWorkspaces.length > 1 ? (
                <div className="mt-2 grid gap-2">
                  <Label className="sr-only" htmlFor="currentWorkspace">当前工作空间</Label>
                  <select
                    className="h-9 rounded-lg border bg-background px-3 text-sm"
                    id="currentWorkspace"
                    onChange={(event) => router.replace(`/projects?workspace=${event.target.value}`)}
                    value={workspaceId}
                  >
                    {activeWorkspaces.map((workspace) => (
                      <option key={workspace.id} value={workspace.id}>
                        {workspace.name}
                      </option>
                    ))}
                  </select>
                </div>
              ) : (
                <p className="mt-2 text-muted-foreground">{selectedWorkspace?.name}</p>
              )}
            </div>
          </div>

          {projects.isError && (
            <Alert variant="destructive" className="mb-5">
              <AlertTitle>无法加载项目</AlertTitle>
              <AlertDescription>{appApiErrorMessage(projects.error)}</AlertDescription>
            </Alert>
          )}

          {projects.isLoading ? (
            <p className="text-sm text-muted-foreground">正在加载项目…</p>
          ) : projects.data?.items.length ? (
            <div className="grid gap-4 sm:grid-cols-2">
              {projects.data.items.map((project) => (
                <Link aria-label={`打开项目 ${project.name}`} href={`/projects/${project.id}`} key={project.id}>
                  <Card className="h-full transition-transform hover:-translate-y-0.5">
                    <CardHeader>
                      <CardTitle className="flex items-center justify-between gap-3">
                        <span className="flex items-center gap-2">
                          {project.name}
                          {project.status === "archived" ? (
                            <Badge variant="outline">已归档</Badge>
                          ) : null}
                        </span>
                        <ArrowRight className="size-4 text-muted-foreground" aria-hidden="true" />
                      </CardTitle>
                      <CardDescription>{project.description || "尚未添加项目简介"}</CardDescription>
                    </CardHeader>
                    <CardContent className="flex gap-2 text-xs text-muted-foreground">
                      <span>{project.aspect_ratio}</span>
                      <span>·</span>
                      <span>{project.language}</span>
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          ) : (
            <Card className="border-dashed py-12 text-center">
              <CardContent>
                <FolderKanban className="mx-auto size-8 text-muted-foreground" aria-hidden="true" />
                <p className="mt-4 font-medium">还没有项目</p>
                <p className="mt-1 text-sm text-muted-foreground">从右侧创建你的第一部短剧。</p>
              </CardContent>
            </Card>
          )}
        </section>

        <aside>
          <Card>
            <CardHeader>
              <CardTitle>新项目</CardTitle>
              <CardDescription>先建立最小项目，单集与剧本可随后补充。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={handleCreateProject}>
                <div className="grid gap-2">
                  <Label htmlFor="name">项目名称</Label>
                  <Input id="name" name="name" maxLength={120} required />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="description">项目简介</Label>
                  <Input id="description" name="description" maxLength={2000} />
                </div>
                {commandError && (
                  <p className="text-sm text-destructive" role="alert">{commandError}</p>
                )}
                <Button disabled={!workspaceId || createState.isLoading} type="submit">
                  {createState.isLoading ? (
                    <LoaderCircle className="animate-spin" aria-hidden="true" />
                  ) : (
                    <Plus aria-hidden="true" />
                  )}
                  创建项目
                </Button>
              </form>
            </CardContent>
          </Card>
        </aside>
      </div>
    </main>
  );
}
