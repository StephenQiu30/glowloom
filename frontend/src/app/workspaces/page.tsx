"use client";

import {
  AlertCircle,
  Archive,
  ArrowRight,
  CheckCircle2,
  KeyRound,
  LoaderCircle,
  Plus,
  RotateCcw,
  Save,
  Settings2,
  UserX,
} from "lucide-react";
import Link from "next/link";
import { type FormEvent, useState } from "react";

import { LayoutContainer } from "@/components/layout/layout-container";
import { StudioShell } from "@/components/studio/studio-shell";
import { PageHeader } from "@/components/studio/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Item, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@/components/ui/item";
import { Label } from "@/components/ui/label";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import { clearAccessToken } from "@/lib/auth-session";
import {
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

const roleLabels: Record<API.WorkspaceResponse["role"], string> = {
  owner: "所有者",
  editor: "编辑者",
  viewer: "查看者",
};

export default function WorkspacesPage() {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const workspacesQuery = useWorkspacesQuery(undefined, { skip: !authenticated });
  const [updateProfile, profileState] = useUpdateProfileMutation();
  const [createWorkspace, createState] = useCreateWorkspaceMutation();
  const [updateWorkspace, updateWorkspaceState] = useUpdateWorkspaceMutation();
  const [setWorkspaceArchived, archiveState] = useSetWorkspaceArchivedMutation();
  const [changePassword, passwordState] = useChangePasswordMutation();
  const [deactivateAccount, deactivateState] = useDeactivateAccountMutation();
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  async function runAction(action: () => Promise<string>): Promise<boolean> {
    setNotice(null);
    setActionError(null);
    try {
      setNotice(await action());
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await runAction(async () => {
      await updateProfile({
        display_name: String(form.get("displayName") ?? "").trim() || null,
        avatar_url: me.data?.user.avatar_url ?? null,
      }).unwrap();
      return "个人资料已保存。";
    });
  }

  async function createNewWorkspace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const name = String(form.get("workspaceName") ?? "").trim();
    const completed = await runAction(async () => {
      const created = await createWorkspace({ name }).unwrap();
      return `工作空间“${created.name}”已创建。`;
    });
    if (completed) formElement.reset();
  }

  async function toggleArchived(workspace: API.WorkspaceResponse) {
    await runAction(async () => {
      const archived = workspace.status === "active";
      await setWorkspaceArchived({
        workspaceId: workspace.id,
        expectedRevision: workspace.revision,
        archived,
      }).unwrap();
      return `工作空间“${workspace.name}”已${archived ? "归档" : "恢复"}。`;
    });
  }

  async function renameWorkspace(
    event: FormEvent<HTMLFormElement>,
    workspace: API.WorkspaceResponse,
  ) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const name = String(form.get("workspaceName") ?? "").trim();
    await runAction(async () => {
      await updateWorkspace({
        workspaceId: workspace.id,
        body: { name, expected_revision: workspace.revision },
      }).unwrap();
      return `工作空间“${name}”已保存。`;
    });
  }

  async function submitPasswordChange(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const completed = await runAction(async () => {
      await changePassword({
        current_password: String(form.get("currentPassword") ?? ""),
        new_password: String(form.get("newPassword") ?? ""),
      }).unwrap();
      return "密码已修改，请重新登录。";
    });
    if (completed) {
      clearAccessToken();
      window.location.replace("/login");
    }
  }

  async function submitDeactivation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const completed = await runAction(async () => {
      await deactivateAccount({
        confirmation: String(form.get("confirmation") ?? "") as "DEACTIVATE",
      }).unwrap();
      return "账户已停用。";
    });
    if (completed) {
      clearAccessToken();
      window.location.replace("/login");
    }
  }

  const pageError = me.error ?? workspacesQuery.error;
  const busy =
    profileState.isLoading ||
    createState.isLoading ||
    updateWorkspaceState.isLoading ||
    archiveState.isLoading ||
    passwordState.isLoading ||
    deactivateState.isLoading;
  const currentWorkspaceId = me.data?.workspace.id;

  if (sessionState === "checking") {
    return (
      <StudioShell active="settings">
        <div className="grid min-h-[70dvh] place-items-center">
          <LoaderCircle aria-label="正在读取登录状态" className="animate-spin text-foreground" />
        </div>
      </StudioShell>
    );
  }

  return (
    <StudioShell
      active="settings"
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      <LayoutContainer className="py-9">
        {!authenticated ? (
          <Alert className="border-amber-200 bg-amber-50 text-amber-800"><AlertCircle aria-hidden="true" /><AlertTitle>需要登录</AlertTitle><AlertDescription><Link className="underline" href="/login">登录后管理账户与工作空间</Link></AlertDescription></Alert>
        ) : pageError ? (
          <Alert variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>账户设置暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription></Alert>
        ) : !me.data || !workspacesQuery.data ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle aria-label="正在加载账户设置" className="animate-spin text-foreground" /></div>
        ) : (
          <>
            <PageHeader
              actions={<Button asChild><Link href="/projects">返回项目<ArrowRight aria-hidden="true" /></Link></Button>}
              badges={[{ label: "账户设置" }]}
              description="管理服务端个人资料，以及有权限访问的创作空间。"
              title="账户与工作空间"
            />

            {notice ? <div className="mt-6 flex items-center gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status"><CheckCircle2 className="size-4" aria-hidden="true" />{notice}</div> : null}
            {actionError ? <Alert className="mt-6" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>操作未完成</AlertTitle><AlertDescription>{actionError}</AlertDescription></Alert> : null}

            <div className="mt-7 grid gap-6 lg:grid-cols-[1fr_340px]">
              <Card className="p-6">
                <Item className="p-0">
                  <ItemMedia>
                    <Avatar size="lg"><AvatarFallback>{(me.data.user.display_name || me.data.user.email).slice(0, 1).toUpperCase()}</AvatarFallback></Avatar>
                  </ItemMedia>
                  <ItemContent>
                    <ItemTitle className="text-lg">个人资料</ItemTitle>
                    <ItemDescription>{me.data.user.email}</ItemDescription>
                  </ItemContent>
                </Item>
                <form className="mt-6 grid gap-5" onSubmit={saveProfile}>
                  <div className="grid gap-2"><Label htmlFor="displayName">显示名称</Label><Input defaultValue={me.data.user.display_name ?? ""} id="displayName" name="displayName" maxLength={120} /></div>
                  <div><Button disabled={busy} type="submit"><Save aria-hidden="true" />保存个人资料</Button></div>
                </form>
              </Card>

              <Card className="p-6">
                <Item className="p-0">
                  <ItemMedia variant="icon"><Plus aria-hidden="true" /></ItemMedia>
                  <ItemContent>
                    <ItemTitle>创建工作空间</ItemTitle>
                    <ItemDescription>隔离不同团队与项目</ItemDescription>
                  </ItemContent>
                </Item>
                <form className="mt-6 grid gap-4" onSubmit={createNewWorkspace}>
                  <div className="grid gap-2"><Label htmlFor="workspaceName">空间名称</Label><Input id="workspaceName" name="workspaceName" placeholder="例如：青墨工作室" required maxLength={120} /></div>
                  <Button disabled={busy} type="submit"><Plus aria-hidden="true" />创建工作空间</Button>
                </form>
              </Card>
            </div>

            <section className="mt-7">
              <div className="mb-4 flex items-end justify-between"><div><h2 className="text-xl font-semibold">我的工作空间</h2><p className="mt-1 text-sm text-slate-500">角色与状态均来自当前账号权限。</p></div><span className="text-sm text-slate-400">{workspacesQuery.data.length} 个空间</span></div>
              <div className="grid gap-4">
                {workspacesQuery.data.map((workspace) => {
                  const current = workspace.id === currentWorkspaceId;
                  return (
                    <article className={`flex flex-wrap items-center gap-5 rounded-2xl border bg-card p-5 ${current ? "shadow-sm" : ""}`} key={workspace.id}>
                      <span className="grid size-12 place-items-center rounded-xl bg-slate-100 text-foreground"><Settings2 className="size-5" aria-hidden="true" /></span>
                      <div className="min-w-48 flex-1"><div className="flex flex-wrap items-center gap-2"><h3 className="font-semibold">{workspace.name}</h3>{current ? <Badge className="border-border bg-muted text-foreground" variant="outline">当前空间</Badge> : null}{workspace.status === "archived" ? <Badge variant="secondary">已归档</Badge> : null}</div><p className="mt-1 text-xs text-slate-500">{roleLabels[workspace.role]} · revision {workspace.revision}</p></div>
                      <div className="flex flex-1 flex-wrap items-end justify-end gap-2">
                        {workspace.role === "owner" ? (
                          <form className="flex min-w-64 flex-1 gap-2" onSubmit={(event) => renameWorkspace(event, workspace)}>
                            <Input
                              aria-label={`重命名 ${workspace.name}`}
                              defaultValue={workspace.name}
                              disabled={busy || workspace.status === "archived"}
                              maxLength={120}
                              name="workspaceName"
                              required
                            />
                            <Button
                              aria-label={`保存 ${workspace.name}`}
                              disabled={busy || workspace.status === "archived"}
                              type="submit"
                              variant="outline"
                            >
                              <Save aria-hidden="true" />
                            </Button>
                          </form>
                        ) : null}
                        <Button asChild variant="outline"><Link href={`/projects?workspace=${workspace.id}`}>查看项目</Link></Button>
                        {workspace.role === "owner" && !current ? (
                          <Button
                            aria-label={`${workspace.status === "active" ? "归档" : "恢复"} ${workspace.name}`}
                            disabled={busy}
                            onClick={() => toggleArchived(workspace)}
                            variant="outline"
                          >
                            {workspace.status === "active" ? <Archive aria-hidden="true" /> : <RotateCcw aria-hidden="true" />}
                            {workspace.status === "active" ? "归档" : "恢复"}
                          </Button>
                        ) : null}
                      </div>
                    </article>
                  );
                })}
              </div>
            </section>

            <section className="mt-7 grid gap-5 lg:grid-cols-2" aria-label="账户安全">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2"><KeyRound className="size-5 text-foreground" aria-hidden="true" />修改密码</CardTitle>
                  <CardDescription>修改成功后当前令牌立即失效，需要重新登录。</CardDescription>
                </CardHeader>
                <CardContent>
                  <form className="grid gap-4" onSubmit={submitPasswordChange}>
                    <div className="grid gap-2"><Label htmlFor="currentPassword">当前密码</Label><Input autoComplete="current-password" disabled={busy} id="currentPassword" minLength={12} name="currentPassword" required type="password" /></div>
                    <div className="grid gap-2"><Label htmlFor="newPassword">新密码</Label><Input autoComplete="new-password" disabled={busy} id="newPassword" minLength={12} name="newPassword" required type="password" /></div>
                    <Button disabled={busy} type="submit"><KeyRound aria-hidden="true" />修改密码</Button>
                  </form>
                </CardContent>
              </Card>
              <Card className="border-rose-200">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-rose-700"><UserX className="size-5" aria-hidden="true" />停用账户</CardTitle>
                  <CardDescription>这会撤销当前凭据并禁止再次登录；已有业务事实依据留存规则保留。</CardDescription>
                </CardHeader>
                <CardContent>
                  <form className="grid gap-4" onSubmit={submitDeactivation}>
                    <div className="grid gap-2"><Label htmlFor="deactivateConfirmation">输入 DEACTIVATE 确认</Label><Input disabled={busy} id="deactivateConfirmation" name="confirmation" pattern="DEACTIVATE" required /></div>
                    <Button disabled={busy} type="submit" variant="destructive"><UserX aria-hidden="true" />停用账户</Button>
                  </form>
                </CardContent>
              </Card>
            </section>
          </>
        )}
      </LayoutContainer>
    </StudioShell>
  );
}
