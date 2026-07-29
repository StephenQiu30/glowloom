"use client";

import {
  Archive,
  ArrowLeft,
  Clapperboard,
  KeyRound,
  LoaderCircle,
  Plus,
  RotateCcw,
  Save,
  UserRoundX,
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
  useChangePasswordMutation,
  useCreateWorkspaceMutation,
  useDeactivateAccountMutation,
  useMeQuery,
  useSetWorkspaceArchivedMutation,
  useUpdateProfileMutation,
  useUpdateWorkspaceMutation,
  useWorkspacesQuery,
} from "@/lib/server-state";
import { clearAccessToken } from "@/lib/auth-session";
import type { AppStore } from "@/lib/redux-store";

export default function WorkspacesPage() {
  const router = useRouter();
  const dispatch = useDispatch<AppStore["dispatch"]>();
  const authState = useAuthSessionState();
  const isAuthenticated = authState === "authenticated";
  const me = useMeQuery(undefined, { skip: !isAuthenticated });
  const workspaces = useWorkspacesQuery(undefined, { skip: !isAuthenticated });
  const [updateProfile, profileState] = useUpdateProfileMutation();
  const [createWorkspace, createState] = useCreateWorkspaceMutation();
  const [updateWorkspace, workspaceUpdateState] = useUpdateWorkspaceMutation();
  const [setWorkspaceArchived, archiveState] = useSetWorkspaceArchivedMutation();
  const [changePassword, passwordState] = useChangePasswordMutation();
  const [deactivateAccount, deactivateState] = useDeactivateAccountMutation();
  const [message, setMessage] = useState<string | null>(null);
  const [commandError, setCommandError] = useState<string | null>(null);

  useEffect(() => {
    if (authState === "anonymous") router.replace("/login");
  }, [authState, router]);

  async function runCommand(command: () => Promise<unknown>, success: string) {
    setCommandError(null);
    setMessage(null);
    try {
      await command();
      setMessage(success);
      return true;
    } catch (error: unknown) {
      setCommandError(appApiErrorMessage(error));
      return false;
    }
  }

  async function handleProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runCommand(
      () =>
        updateProfile({
          display_name: String(form.get("displayName")),
          avatar_url: String(form.get("avatarUrl")) || null,
        }).unwrap(),
      "个人资料已更新。",
    );
  }

  async function handleCreateWorkspace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const succeeded = await runCommand(
      () => createWorkspace({ name: String(form.get("workspaceName")) }).unwrap(),
      "工作空间已创建。",
    );
    if (succeeded) formElement.reset();
  }

  async function handleRenameWorkspace(
    event: FormEvent<HTMLFormElement>,
    workspace: API.WorkspaceResponse,
  ) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runCommand(
      () =>
        updateWorkspace({
          workspaceId: workspace.id,
          body: {
            name: String(form.get("name")),
            expected_revision: workspace.revision,
          },
        }).unwrap(),
      "工作空间名称已更新。",
    );
  }

  async function handleWorkspaceState(workspace: API.WorkspaceResponse) {
    await runCommand(
      () =>
        setWorkspaceArchived({
          workspaceId: workspace.id,
          expectedRevision: workspace.revision,
          archived: workspace.status === "active",
        }).unwrap(),
      workspace.status === "active" ? "工作空间已归档。" : "工作空间已恢复。",
    );
  }

  async function handleChangePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const succeeded = await runCommand(
      () =>
        changePassword({
          current_password: String(form.get("currentPassword")),
          new_password: String(form.get("newPassword")),
        }).unwrap(),
      "密码已修改，请重新登录。",
    );
    if (succeeded) endSession();
  }

  async function handleDeactivate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    if (String(form.get("confirmation")) !== "DEACTIVATE") {
      setMessage(null);
      setCommandError("请输入 DEACTIVATE 以确认停用。");
      return;
    }
    const succeeded = await runCommand(
      () => deactivateAccount({ confirmation: "DEACTIVATE" }).unwrap(),
      "账户已停用。",
    );
    if (succeeded) endSession();
  }

  function endSession() {
    clearAccessToken();
    dispatch(appApi.util.resetApiState());
    router.replace("/login");
  }

  if (authState === "checking" || me.isLoading) {
    return (
      <main className="grid min-h-screen place-items-center" aria-live="polite">
        <LoaderCircle className="size-6 animate-spin" aria-hidden="true" />
        <span className="sr-only">正在加载账户设置</span>
      </main>
    );
  }

  if (authState === "anonymous") return null;

  return (
    <main className="min-h-screen bg-muted/30">
      <header className="border-b bg-background">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-5">
          <Link className="flex items-center gap-3 font-semibold" href="/projects">
            <span className="grid size-9 place-items-center rounded-xl bg-primary text-primary-foreground">
              <Clapperboard className="size-5" aria-hidden="true" />
            </span>
            Lanverse
          </Link>
          <Button asChild variant="ghost">
            <Link href="/projects">
              <ArrowLeft aria-hidden="true" />
              返回项目
            </Link>
          </Button>
        </div>
      </header>

      <div className="mx-auto grid max-w-5xl gap-7 px-6 py-10">
        <div>
          <Badge variant="secondary">账户设置</Badge>
          <h1 className="mt-3 text-3xl font-semibold tracking-tight">账户与工作空间</h1>
          <p className="mt-2 text-muted-foreground">管理个人资料、创作空间和登录安全。</p>
        </div>

        {commandError ? (
          <Alert variant="destructive">
            <AlertTitle>操作未完成</AlertTitle>
            <AlertDescription>{commandError}</AlertDescription>
          </Alert>
        ) : null}
        {message ? (
          <Alert>
            <AlertTitle>操作成功</AlertTitle>
            <AlertDescription>{message}</AlertDescription>
          </Alert>
        ) : null}

        <div className="grid gap-7 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>个人资料</CardTitle>
              <CardDescription>{me.data?.user.email}</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={handleProfile}>
                <div className="grid gap-2">
                  <Label htmlFor="displayName">显示名称</Label>
                  <Input
                    defaultValue={me.data?.user.display_name}
                    id="displayName"
                    maxLength={80}
                    name="displayName"
                    required
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="avatarUrl">头像地址</Label>
                  <Input
                    defaultValue={me.data?.user.avatar_url ?? ""}
                    id="avatarUrl"
                    name="avatarUrl"
                    placeholder="https://"
                    type="url"
                  />
                </div>
                <Button disabled={profileState.isLoading} type="submit">
                  <Save aria-hidden="true" />
                  保存个人资料
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>创建工作空间</CardTitle>
              <CardDescription>项目和媒体按工作空间隔离。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={handleCreateWorkspace}>
                <div className="grid gap-2">
                  <Label htmlFor="workspaceName">工作空间名称</Label>
                  <Input id="workspaceName" maxLength={120} name="workspaceName" required />
                </div>
                <Button disabled={createState.isLoading} type="submit">
                  <Plus aria-hidden="true" />
                  创建工作空间
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>

        <section>
          <h2 className="text-xl font-semibold">我的工作空间</h2>
          <p className="mt-1 text-sm text-muted-foreground">归档后停止新的业务写入，历史仍可读取。</p>
          {workspaces.isError ? (
            <Alert className="mt-4" variant="destructive">
              <AlertTitle>无法加载工作空间</AlertTitle>
              <AlertDescription>{appApiErrorMessage(workspaces.error)}</AlertDescription>
            </Alert>
          ) : null}
          <div className="mt-4 grid gap-4">
            {workspaces.data?.map((workspace) => (
              <Card key={workspace.id}>
                <CardContent className="grid gap-4 pt-6 md:grid-cols-[1fr_auto] md:items-end">
                  <form
                    className="grid gap-2"
                    key={`${workspace.id}-${workspace.revision}`}
                    onSubmit={(event) => handleRenameWorkspace(event, workspace)}
                  >
                    <div className="flex items-center gap-2">
                      <p className="font-medium">{workspace.name}</p>
                      <Badge variant={workspace.status === "active" ? "secondary" : "outline"}>
                        {workspace.status === "active" ? "使用中" : "已归档"}
                      </Badge>
                    </div>
                    <Label className="sr-only" htmlFor={`workspace-${workspace.id}`}>
                      重命名 {workspace.name}
                    </Label>
                    <div className="flex gap-2">
                      <Input
                        defaultValue={workspace.name}
                        disabled={workspace.status === "archived"}
                        id={`workspace-${workspace.id}`}
                        maxLength={120}
                        name="name"
                        required
                      />
                      <Button
                        aria-label={`保存 ${workspace.name}`}
                        disabled={workspace.status === "archived" || workspaceUpdateState.isLoading}
                        size="icon"
                        type="submit"
                        variant="outline"
                      >
                        <Save aria-hidden="true" />
                      </Button>
                    </div>
                  </form>
                  <div className="flex gap-2">
                    {workspace.status === "active" ? (
                      <Button asChild variant="outline">
                        <Link href={`/projects?workspace=${workspace.id}`}>打开项目</Link>
                      </Button>
                    ) : null}
                    <Button
                      aria-label={`${workspace.status === "active" ? "归档" : "恢复"} ${workspace.name}`}
                      disabled={archiveState.isLoading}
                      onClick={() => handleWorkspaceState(workspace)}
                      variant={workspace.status === "active" ? "destructive" : "outline"}
                    >
                      {workspace.status === "active" ? (
                        <Archive aria-hidden="true" />
                      ) : (
                        <RotateCcw aria-hidden="true" />
                      )}
                      {workspace.status === "active" ? "归档" : "恢复"}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </section>

        <div className="grid gap-7 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>修改密码</CardTitle>
              <CardDescription>修改后所有已有登录令牌立即失效。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={handleChangePassword}>
                <div className="grid gap-2">
                  <Label htmlFor="currentPassword">当前密码</Label>
                  <Input id="currentPassword" minLength={12} name="currentPassword" required type="password" />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="newPassword">新密码</Label>
                  <Input id="newPassword" minLength={12} name="newPassword" required type="password" />
                </div>
                <Button disabled={passwordState.isLoading} type="submit" variant="outline">
                  <KeyRound aria-hidden="true" />
                  修改密码
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card className="border-destructive/30">
            <CardHeader>
              <CardTitle>停用账户</CardTitle>
              <CardDescription>历史项目不会删除；停用后需通过运营恢复流程重新启用。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4" onSubmit={handleDeactivate}>
                <div className="grid gap-2">
                  <Label htmlFor="confirmation">输入 DEACTIVATE 确认</Label>
                  <Input autoComplete="off" id="confirmation" name="confirmation" required />
                </div>
                <Button disabled={deactivateState.isLoading} type="submit" variant="destructive">
                  <UserRoundX aria-hidden="true" />
                  停用账户
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>
      </div>
    </main>
  );
}
