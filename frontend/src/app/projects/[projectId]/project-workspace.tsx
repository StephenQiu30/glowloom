"use client";

import {
  AlertCircle,
  ArrowRight,
  CheckCircle2,
  ChevronRight,
  CircleDashed,
  FileText,
  Layers3,
  LoaderCircle,
  Plus,
  X,
} from "lucide-react";
import Link from "next/link";
import { type FormEvent, useMemo, useState } from "react";
import { Dialog } from "radix-ui";

import { StudioShell } from "@/components/studio/studio-shell";
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
  appApiErrorMessage,
  useCreateEpisodeMutation,
  useEpisodesQuery,
  useMeQuery,
  useProjectQuery,
  useProjectSnapshotQuery,
} from "@/lib/server-state";

import { EpisodeLifecycleCard } from "./episode-lifecycle-card";
import { ProjectLifecyclePanel } from "./project-lifecycle-panel";

const stageLabels: Record<API.ProjectProductionSnapshot["current_stage"], string> = {
  project_setup: "项目设置",
  script_import: "剧本导入",
  structure_review: "结构确认",
  asset_preparation: "资产准备",
  storyboard_preparation: "分镜准备",
};

const stageSteps: Record<API.ProjectProductionSnapshot["current_stage"], number> = {
  project_setup: 0,
  script_import: 0,
  structure_review: 0,
  asset_preparation: 1,
  storyboard_preparation: 2,
};

function CreateEpisodeDialog({
  defaultDurationMs,
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
}: {
  defaultDurationMs: number;
  isSubmitting: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.EpisodeCreateRequest) => Promise<boolean>;
  open: boolean;
}) {
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const completed = await onSubmit({
      name: String(form.get("name") ?? "").trim(),
      target_duration_ms: Number(form.get("targetDurationSeconds")) * 1_000,
    });
    if (completed) formElement.reset();
  }

  return (
    <Dialog.Root onOpenChange={onOpenChange} open={open}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-slate-200 bg-white p-6 shadow-2xl shadow-slate-950/10">
          <div className="flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-xl font-semibold tracking-tight">创建下一集</Dialog.Title>
              <Dialog.Description className="mt-1 text-sm leading-6 text-slate-500">
                单集按项目顺序创建，进入工作台后再导入固定剧本版本。
              </Dialog.Description>
            </div>
            <Dialog.Close asChild>
              <Button aria-label="关闭" size="icon" variant="ghost"><X aria-hidden="true" /></Button>
            </Dialog.Close>
          </div>
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="episodeName">单集名称</Label>
              <Input id="episodeName" name="name" placeholder="例如：第二集 · 城门旧事" required maxLength={120} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="episodeDuration">目标时长（秒）</Label>
              <Input
                defaultValue={Math.round(defaultDurationMs / 1_000)}
                id="episodeDuration"
                min={1}
                name="targetDurationSeconds"
                required
                type="number"
              />
            </div>
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild><Button type="button" variant="outline">取消</Button></Dialog.Close>
              <Button disabled={isSubmitting} type="submit">
                {isSubmitting ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <Plus aria-hidden="true" />}
                确认创建
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function ProjectWorkspace({ projectId }: { projectId: string }) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const projectQuery = useProjectQuery(projectId, { skip: !authenticated });
  const episodesQuery = useEpisodesQuery(projectId, { skip: !authenticated });
  const snapshotQuery = useProjectSnapshotQuery(projectId, {
    pollingInterval: 5_000,
    skip: !authenticated,
  });
  const [createEpisode, createState] = useCreateEpisodeMutation();
  const [createOpen, setCreateOpen] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const project = projectQuery.data;
  const episodes = episodesQuery.data ?? [];
  const snapshot = snapshotQuery.data;
  const episodeSnapshots = useMemo(
    () => new Map(snapshot?.episodes.map((item) => [item.episode_id, item]) ?? []),
    [snapshot?.episodes],
  );
  const activeEpisodes = episodes.filter((episode) => episode.status === "active");
  const firstEpisode = activeEpisodes[0];
  const readyAssets = snapshot?.episodes.reduce(
    (total, episode) => total + (episode.asset_summary.ready ?? 0),
    0,
  ) ?? 0;
  const runningTasks = snapshot?.episodes.reduce(
    (total, episode) => total + (episode.task_summary.running ?? 0),
    0,
  ) ?? 0;

  async function handleCreate(request: API.EpisodeCreateRequest): Promise<boolean> {
    setActionError(null);
    setNotice(null);
    try {
      const created = await createEpisode({ projectId, body: request }).unwrap();
      setCreateOpen(false);
      setNotice(`第 ${created.position} 集已创建，可以进入剧本工作台。`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  const pageError = me.error ?? projectQuery.error ?? episodesQuery.error ?? snapshotQuery.error;

  if (sessionState === "checking") {
    return (
      <div className="grid min-h-screen place-items-center">
        <LoaderCircle aria-label="正在读取登录状态" className="animate-spin text-[#079db3]" />
      </div>
    );
  }

  return (
    <StudioShell
      active="projects"
      currentStep={snapshot ? stageSteps[snapshot.current_stage] : 0}
      projectName={project?.name}
      topAction={
        firstEpisode ? (
          <Button asChild><Link href={`/studio/${firstEpisode.id}/script`}>继续制作<ArrowRight aria-hidden="true" /></Link></Button>
        ) : (
          <Button onClick={() => setCreateOpen(true)}><Plus aria-hidden="true" />创建第一集</Button>
        )
      }
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      {notice ? (
        <div className="pointer-events-none fixed top-24 right-6 z-50 flex max-w-md items-center gap-2 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg" role="status">
          <CheckCircle2 className="size-4 shrink-0 text-emerald-600" aria-hidden="true" />{notice}
        </div>
      ) : null}
      <div className="mx-auto max-w-[1280px] px-5 py-8 md:px-8">
        {!authenticated ? (
          <Alert className="border-amber-200 bg-amber-50 text-amber-800">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>需要登录</AlertTitle>
            <AlertDescription><Link className="underline" href="/login">登录后查看项目生产事实</Link></AlertDescription>
          </Alert>
        ) : pageError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>项目事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription>
          </Alert>
        ) : !project || !snapshot ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle aria-label="正在加载项目" className="animate-spin text-[#079db3]" /></div>
        ) : (
          <>
            <header className="flex flex-wrap items-start justify-between gap-5">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">{stageLabels[snapshot.current_stage]}</Badge>
                  <Badge variant="outline">{project.aspect_ratio}</Badge>
                  <Badge variant="outline">{project.visual_style ?? "未设视觉风格"}</Badge>
                  {project.status === "archived" ? <Badge variant="secondary">已归档</Badge> : null}
                </div>
                <h1 className="mt-4 text-3xl font-semibold tracking-[-0.035em]">{project.name}</h1>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-500">{project.description || "尚未填写项目简介"}</p>
              </div>
              <Button onClick={() => setCreateOpen(true)} variant="outline"><Plus aria-hidden="true" />创建单集</Button>
            </header>

            <section aria-label="项目生产摘要" className="mt-7 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <Card><CardHeader><CardDescription>项目进度</CardDescription><CardTitle className="text-3xl">{snapshot.completion}%</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>活跃单集</CardDescription><CardTitle className="text-3xl">{activeEpisodes.length}</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>Ready 资产</CardDescription><CardTitle className="text-3xl text-emerald-700">{readyAssets}</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>进行中任务</CardDescription><CardTitle className="text-3xl text-[#087f91]">{runningTasks}</CardTitle></CardHeader></Card>
            </section>

            {actionError ? (
              <Alert className="mt-5" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>操作未完成</AlertTitle><AlertDescription>{actionError}</AlertDescription></Alert>
            ) : null}
            {snapshot.partial_failures.length ? (
              <Alert className="mt-5 border-rose-200 bg-rose-50 text-rose-800"><AlertCircle aria-hidden="true" /><AlertTitle>部分摘要不可用</AlertTitle><AlertDescription>{snapshot.partial_failures.map((item) => item.summary).join("；")}</AlertDescription></Alert>
            ) : null}

            <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
              <Card>
                <CardHeader className="border-b border-slate-100">
                  <CardTitle>单集生产</CardTitle>
                  <CardDescription>阶段、阻塞和入口均来自服务端 ProductionSnapshot。</CardDescription>
                </CardHeader>
                <CardContent className="divide-y divide-slate-100 p-0">
                  {episodes.length === 0 ? (
                    <div className="px-6 py-16 text-center"><p className="font-medium">还没有单集</p><p className="mt-1 text-sm text-slate-500">创建第一集后开始导入剧本。</p></div>
                  ) : episodes.map((episode) => {
                    const episodeSnapshot = episodeSnapshots.get(episode.id);
                    return (
                      <Link
                        aria-label={`进入${episode.name}`}
                        className="grid gap-4 px-5 py-4 transition hover:bg-slate-50 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center"
                        href={`/studio/${episode.id}/script`}
                        key={episode.id}
                      >
                        <span className="grid size-11 place-items-center rounded-xl bg-slate-100 text-sm font-semibold">{String(episode.position).padStart(2, "0")}</span>
                        <span className="min-w-0">
                          <span className="block truncate font-medium">{episode.name}</span>
                          <span className="mt-1 flex flex-wrap gap-3 text-xs text-slate-500">
                            <span>{Math.round(episode.target_duration_ms / 1_000)} 秒</span>
                            <span>{episodeSnapshot ? stageLabels[episodeSnapshot.current_stage] : "摘要不可用"}</span>
                            <span>{episodeSnapshot?.completion ?? 0}%</span>
                          </span>
                        </span>
                        <span className="flex items-center gap-3">
                          <Badge variant={episode.status === "active" ? "outline" : "secondary"}>{episode.status === "active" ? "制作中" : "已归档"}</Badge>
                          <ChevronRight className="size-4 text-slate-400" aria-hidden="true" />
                        </span>
                      </Link>
                    );
                  })}
                </CardContent>
              </Card>

              <aside className="grid content-start gap-5">
                <Card className="border-amber-200 bg-amber-50/70">
                  <CardHeader><CardTitle className="flex items-center gap-2"><CircleDashed className="size-5 text-amber-600" aria-hidden="true" />下一步</CardTitle></CardHeader>
                  <CardContent>
                    <p className="font-medium">{snapshot.next_actions[0]?.label ?? "等待新的生产动作"}</p>
                    {snapshot.blocking_reasons.map((reason) => (
                      <p className="mt-2 text-sm leading-6 text-slate-600" key={`${reason.code}:${reason.resource_id}`}>{reason.summary}</p>
                    ))}
                    {snapshot.next_actions[0] ? (
                      <Button asChild className="mt-5 w-full"><Link href={snapshot.next_actions[0].href}>开始处理<ArrowRight aria-hidden="true" /></Link></Button>
                    ) : null}
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader><CardTitle>项目规格</CardTitle></CardHeader>
                  <CardContent className="grid gap-4 text-sm">
                    <div className="flex items-center justify-between gap-4"><span className="flex items-center gap-2 text-slate-500"><FileText className="size-4" aria-hidden="true" />语言</span><span>{project.language}</span></div>
                    <div className="flex items-center justify-between gap-4"><span className="flex items-center gap-2 text-slate-500"><Layers3 className="size-4" aria-hidden="true" />默认时长</span><span>{Math.round(project.target_duration_ms / 1_000)} 秒</span></div>
                    <div className="flex items-center justify-between gap-4"><span className="text-slate-500">预算上限</span><span>{project.currency} {project.budget_limit}</span></div>
                  </CardContent>
                </Card>
              </aside>
            </div>
            <ProjectLifecyclePanel project={project} />
            <section className="mt-7 grid gap-5" aria-label="单集设置">
              <div>
                <h2 className="text-xl font-semibold">单集设置</h2>
                <p className="mt-1 text-sm text-slate-500">编辑、排序、归档与安全删除均使用服务端 revision。</p>
              </div>
              {episodes.map((episode) => (
                <EpisodeLifecycleCard
                  activeEpisodes={activeEpisodes}
                  episode={episode}
                  episodeSnapshot={episodeSnapshots.get(episode.id)}
                  key={episode.id}
                  project={project}
                />
              ))}
            </section>
          </>
        )}
      </div>
      {project ? (
        <CreateEpisodeDialog
          defaultDurationMs={project.target_duration_ms}
          isSubmitting={createState.isLoading}
          onOpenChange={setCreateOpen}
          onSubmit={handleCreate}
          open={createOpen}
        />
      ) : null}
    </StudioShell>
  );
}
