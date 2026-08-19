"use client";

import {
  AlertCircle,
  ArrowRight,
  Check,
  ChevronRight,
  Clapperboard,
  FileText,
  Layers3,
  ListChecks,
  LoaderCircle,
  UsersRound,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { LayoutContainer } from "@/components/layout/layout-container";
import { StudioShell } from "@/components/studio/studio-shell";
import { MetricGroup } from "@/components/studio/metric-group";
import { PageHeader } from "@/components/studio/page-header";
import { ProductionNextAction } from "@/components/studio/production-next-action";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useEpisodesQuery,
  useMeQuery,
  useProjectQuery,
  useProjectSnapshotQuery,
} from "@/lib/server-state";

import { ScriptDocumentImportCard } from "./script-document-import-card";

const stageLabels: Record<API.ProjectProductionSnapshot["current_stage"], string> = {
  project_setup: "剧本导入",
  script_import: "剧本导入",
  structure_review: "结构确认",
  asset_preparation: "资产准备",
  storyboard_preparation: "分镜准备",
};

const productionStepByStage: Record<
  API.ProjectProductionSnapshot["current_stage"],
  number
> = {
  project_setup: 0,
  script_import: 0,
  structure_review: 0,
  asset_preparation: 1,
  storyboard_preparation: 2,
};

type LifecycleStageId =
  | "script_import"
  | "structure_review"
  | "asset_preparation"
  | "storyboard_preparation"
  | "episode_production";

type LifecycleStage = {
  id: LifecycleStageId;
  label: string;
  detail: string;
  icon: LucideIcon;
};

const lifecycleStages: LifecycleStage[] = [
  {
    id: "script_import",
    label: "剧本导入",
    detail: "原稿预览与固定",
    icon: FileText,
  },
  {
    id: "structure_review",
    label: "结构确认",
    detail: "解析、分集与场景",
    icon: ListChecks,
  },
  {
    id: "asset_preparation",
    label: "资产准备",
    detail: "角色、场景与风格",
    icon: UsersRound,
  },
  {
    id: "storyboard_preparation",
    label: "分镜准备",
    detail: "镜头规格与覆盖",
    icon: Clapperboard,
  },
  {
    id: "episode_production",
    label: "单集工作区",
    detail: "剧本、资产、分镜、媒体与任务",
    icon: Layers3,
  },
];

const lifecycleIndex: Record<API.ProjectProductionSnapshot["current_stage"], number> = {
  project_setup: 0,
  script_import: 0,
  structure_review: 1,
  asset_preparation: 2,
  storyboard_preparation: 3,
};

function lifecycleHref(
  stage: LifecycleStageId,
  projectId: string,
  firstEpisodeId?: string,
): string {
  if (stage === "script_import" || stage === "structure_review") {
    return `/projects/${projectId}#script-import`;
  }
  if (stage === "asset_preparation") return `/projects/${projectId}/assets`;
  if (stage === "storyboard_preparation" && firstEpisodeId) {
    return `/studio/${firstEpisodeId}/storyboard`;
  }
  if (stage === "episode_production" && firstEpisodeId) {
    return `/studio/${firstEpisodeId}/script`;
  }
  return `/projects/${projectId}#episodes`;
}

function ProductionLifecycle({
  currentStage,
  firstEpisodeId,
  projectId,
}: {
  currentStage: API.ProjectProductionSnapshot["current_stage"];
  firstEpisodeId?: string;
  projectId: string;
}) {
  const currentIndex = lifecycleIndex[currentStage];

  return (
    <section
      aria-label="项目制作路径"
      className="rounded-2xl border bg-card p-4 shadow-sm sm:p-5"
    >
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            项目制作路径
          </p>
          <h2 className="mt-2 text-xl font-semibold tracking-tight">
            每一步都从已确认事实继续
          </h2>
        </div>
        <Badge className="shrink-0" variant="outline">
          当前：{stageLabels[currentStage]}
        </Badge>
      </div>

      <ol className="mt-5 flex min-w-full gap-2 overflow-x-auto pb-1">
        {lifecycleStages.map((stage, index) => {
          const state = index < currentIndex ? "complete" : index === currentIndex ? "current" : "upcoming";
          const Icon = stage.icon;
          return (
            <li className="min-w-[160px] flex-1" key={stage.id}>
              <Link
                aria-current={state === "current" ? "step" : undefined}
                className={[
                  "group block h-full rounded-xl border p-3 transition-colors",
                  state === "current"
                    ? "border-foreground bg-foreground text-background"
                    : "border-border bg-background hover:border-foreground/30 hover:bg-muted/45",
                ].join(" ")}
                href={lifecycleHref(stage.id, projectId, firstEpisodeId)}
              >
                <span className="flex items-center justify-between gap-3">
                  <span
                    className={[
                      "grid size-7 place-items-center rounded-full border text-xs",
                      state === "current"
                        ? "border-background/40 bg-background/10"
                        : state === "complete"
                          ? "border-foreground bg-foreground text-background"
                          : "border-border text-muted-foreground",
                    ].join(" ")}
                  >
                    {state === "complete" ? <Check aria-hidden="true" className="size-3.5" /> : <span className="font-mono">0{index + 1}</span>}
                  </span>
                  <Icon aria-hidden="true" className="size-4 opacity-75" />
                </span>
                <span className="mt-3 block text-sm font-medium">{stage.label}</span>
                <span
                  className={[
                    "mt-1 block text-xs",
                    state === "current" ? "text-background/65" : "text-muted-foreground",
                  ].join(" ")}
                >
                  {stage.detail}
                </span>
              </Link>
            </li>
          );
        })}
      </ol>
      <p className="mt-3 text-xs leading-5 text-muted-foreground">
        项目页负责阶段和阻塞摘要；进入单集后，剧本、资产、分镜、媒体与任务在同一个剧集工作区继续。
      </p>
    </section>
  );
}

function ProjectNavigation({
  firstEpisode,
  projectId,
}: {
  firstEpisode?: API.EpisodeResponse;
  projectId: string;
}) {
  return (
    <section aria-label="项目内容" className="grid gap-3 sm:grid-cols-2">
      <Link
        className="group rounded-2xl border bg-card p-5 transition-colors hover:border-foreground/30 hover:bg-muted/35"
        href={`/projects/${projectId}/assets`}
      >
        <span className="flex items-center justify-between gap-4">
          <span className="grid size-9 place-items-center rounded-lg bg-muted">
            <UsersRound aria-hidden="true" className="size-4" />
          </span>
          <ArrowRight aria-hidden="true" className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
        </span>
        <span className="mt-5 block font-medium">查看项目资产</span>
        <span className="mt-1 block text-sm leading-6 text-muted-foreground">
          角色、场景、道具、服装、声音和风格参考。
        </span>
      </Link>
      <Link
        className="group rounded-2xl border bg-card p-5 transition-colors hover:border-foreground/30 hover:bg-muted/35"
        href={firstEpisode ? `/studio/${firstEpisode.id}/script` : "#episodes"}
      >
        <span className="flex items-center justify-between gap-4">
          <span className="grid size-9 place-items-center rounded-lg bg-muted">
            <Clapperboard aria-hidden="true" className="size-4" />
          </span>
          <ArrowRight aria-hidden="true" className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
        </span>
        <span className="mt-5 block font-medium">{firstEpisode ? "进入单集工作区" : "等待生成剧集"}</span>
        <span className="mt-1 block text-sm leading-6 text-muted-foreground">
          {firstEpisode ? "在单集上下文中继续处理剧本、资产、分镜和任务。" : "确认剧本解析与分集方案后，剧集会出现在这里。"}
        </span>
      </Link>
    </section>
  );
}

function EpisodeWorkspaceList({
  episodes,
  episodeSnapshots,
}: {
  episodes: API.EpisodeResponse[];
  episodeSnapshots: Map<string, API.EpisodeProductionSnapshot>;
}) {
  return (
    <section aria-label="单集工作区" className="rounded-2xl border bg-card shadow-sm" id="episodes">
      <div className="flex flex-wrap items-end justify-between gap-3 border-b px-5 py-5 sm:px-6">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            按集管理
          </p>
          <h2 className="mt-2 text-xl font-semibold tracking-tight">单集工作区</h2>
          <p className="mt-1 text-sm text-muted-foreground">从每集的当前状态直接进入下一项制作动作。</p>
        </div>
        <Badge variant="outline">{episodes.length} 集</Badge>
      </div>
      {episodes.length === 0 ? (
        <div className="grid min-h-52 place-items-center px-6 py-12 text-center">
          <div>
            <div className="mx-auto grid size-10 place-items-center rounded-full bg-muted text-muted-foreground">
              <Clapperboard aria-hidden="true" className="size-4" />
            </div>
            <p className="mt-4 font-medium">尚未生成剧集</p>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">确认剧本解析与分集方案后，系统将在这里生成项目剧集。</p>
          </div>
        </div>
      ) : (
        <div className="divide-y divide-border">
          {episodes.map((episode) => {
            const episodeSnapshot = episodeSnapshots.get(episode.id);
            const completion = episodeSnapshot?.completion ?? 0;
            const blockers = episodeSnapshot?.blocking_reasons.length ?? 0;
            return (
              <Link
                aria-label={`进入${episode.name}`}
                className="group grid gap-4 px-5 py-5 transition-colors hover:bg-muted/35 sm:grid-cols-[auto_minmax(0,1fr)_minmax(180px,0.7fr)_auto] sm:items-center sm:px-6"
                href={`/studio/${episode.id}/script`}
                key={episode.id}
              >
                <span className="grid size-11 place-items-center rounded-xl bg-muted font-mono text-sm font-semibold">
                  {String(episode.position).padStart(2, "0")}
                </span>
                <span className="min-w-0">
                  <span className="block truncate font-medium">{episode.name}</span>
                  <span className="mt-1 flex flex-wrap gap-3 text-xs text-muted-foreground">
                    <span>{Math.round(episode.target_duration_ms / 1_000)} 秒</span>
                    <span>{episodeSnapshot ? stageLabels[episodeSnapshot.current_stage] : "摘要暂不可用"}</span>
                    {blockers ? <span className="text-destructive">{blockers} 个阻塞</span> : null}
                  </span>
                </span>
                <span className="grid gap-2">
                  <span className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
                    <span>制作进度</span>
                    <span className="font-mono text-foreground">{completion}%</span>
                  </span>
                  <span aria-label={`${episode.name} 完成度 ${completion}%`} className="h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuemax={100} aria-valuemin={0} aria-valuenow={completion}>
                    <span className="block h-full rounded-full bg-foreground transition-[width]" style={{ width: `${Math.min(100, Math.max(0, completion))}%` }} />
                  </span>
                </span>
                <span className="flex items-center gap-3">
                  <Badge variant={episode.status === "active" ? "outline" : "secondary"}>{episode.status === "active" ? "制作中" : "已归档"}</Badge>
                  <ChevronRight aria-hidden="true" className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
                </span>
              </Link>
            );
          })}
        </div>
      )}
    </section>
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
  const readyStoryboards = snapshot?.episodes.reduce(
    (total, episode) => total + (episode.storyboard_summary.ready ?? 0),
    0,
  ) ?? 0;
  const runningTasks = snapshot?.episodes.reduce(
    (total, episode) => total + (episode.task_summary.running ?? 0),
    0,
  ) ?? 0;
  const canWrite =
    project?.status === "active" && me.data?.workspace.role !== "viewer";

  const pageError = me.error ?? projectQuery.error ?? episodesQuery.error ?? snapshotQuery.error;

  if (sessionState === "checking") {
    return (
      <StudioShell active="projects">
        <div className="grid min-h-[70dvh] place-items-center">
          <LoaderCircle aria-label="正在读取登录状态" className="animate-spin text-foreground" />
        </div>
      </StudioShell>
    );
  }

  return (
    <StudioShell
      active="projects"
      currentStep={snapshot ? productionStepByStage[snapshot.current_stage] : undefined}
      projectName={project?.name}
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      <LayoutContainer className="py-8 sm:py-10">
        {!authenticated ? (
          <Alert className="border-0 bg-muted/50">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>需要登录</AlertTitle>
            <AlertDescription><Link className="underline" href="/login">登录后查看项目</Link></AlertDescription>
          </Alert>
        ) : pageError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>项目事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription>
          </Alert>
        ) : !project || !snapshot ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle aria-label="正在加载项目" className="animate-spin text-foreground" /></div>
        ) : (
          <div className="min-w-0 space-y-7">
            <PageHeader
              actions={firstEpisode ? (
                <Button asChild>
                  <Link href={`/studio/${firstEpisode.id}/script`}>
                    继续制作<ArrowRight aria-hidden="true" />
                  </Link>
                </Button>
              ) : (
                <Button asChild>
                  <Link href="#script-import">
                    导入剧本<ArrowRight aria-hidden="true" />
                  </Link>
                </Button>
              )}
              badges={[
                { label: stageLabels[snapshot.current_stage] },
                { label: project.aspect_ratio },
                { label: project.visual_style ?? "未设视觉风格" },
                ...(project.status === "archived" ? [{ label: "已归档", variant: "secondary" as const }] : []),
              ]}
              breadcrumbs={[{ label: "项目", href: "/projects" }, { label: project.name }]}
              description={project.description || "把剧本、资产、分镜与单集任务放在同一个可恢复的制作上下文中。"}
              note="先完成当前阶段，再进入下一阶段；所有数字都来自对应业务模块的最新事实。"
              title={project.name}
            />

            <ProductionLifecycle
              currentStage={snapshot.current_stage}
              firstEpisodeId={firstEpisode?.id}
              projectId={project.id}
            />

            <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_300px]">
              <div className="min-w-0 space-y-6">
                <ProductionNextAction
                  action={snapshot.next_actions[0]}
                  blockingReasons={snapshot.blocking_reasons}
                />

                <ScriptDocumentImportCard
                  canWrite={canWrite}
                  language={project.language}
                  projectId={project.id}
                  targetDurationMs={project.target_duration_ms}
                  workspaceId={project.workspace_id}
                />

                <ProjectNavigation firstEpisode={firstEpisode} projectId={project.id} />

                {snapshot.partial_failures.length ? (
                  <Alert className="border-0 bg-destructive/10" variant="destructive">
                    <AlertCircle aria-hidden="true" />
                    <AlertTitle>部分摘要不可用</AlertTitle>
                    <AlertDescription>{snapshot.partial_failures.map((item) => item.summary).join("；")}</AlertDescription>
                  </Alert>
                ) : null}

                <EpisodeWorkspaceList episodeSnapshots={episodeSnapshots} episodes={episodes} />
              </div>

              <aside aria-label="项目状态摘要" className="grid content-start gap-5">
                <section className="rounded-2xl border bg-card p-5 shadow-sm" aria-label="当前项目状态">
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">当前项目状态</p>
                      <p className="mt-3 text-lg font-semibold">{stageLabels[snapshot.current_stage]}</p>
                    </div>
                    <span className="font-mono text-2xl font-semibold tracking-tight">{snapshot.completion}%</span>
                  </div>
                  <div aria-label={`项目完成度 ${snapshot.completion}%`} className="mt-5 h-2 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuemax={100} aria-valuemin={0} aria-valuenow={snapshot.completion}>
                    <span className="block h-full rounded-full bg-foreground transition-[width]" style={{ width: `${Math.min(100, Math.max(0, snapshot.completion))}%` }} />
                  </div>
                  <p className="mt-3 text-xs leading-5 text-muted-foreground">摘要更新时间：{new Date(snapshot.computed_at).toLocaleString("zh-CN", { dateStyle: "short", timeStyle: "short" })}</p>
                </section>

                <MetricGroup
                  className="rounded-2xl border bg-card p-5 shadow-sm"
                  columns={3}
                  items={[
                    { label: "活跃单集", value: activeEpisodes.length },
                    { label: "Ready 资产", value: readyAssets },
                    { label: "Ready 分镜", value: readyStoryboards },
                    { label: "进行中任务", value: runningTasks },
                    { label: "项目进度", value: `${snapshot.completion}%` },
                  ]}
                  label="项目生产摘要"
                />

                <section className="rounded-2xl border bg-card p-5 shadow-sm" aria-label="阻塞摘要">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">需要注意</p>
                    <Badge variant={snapshot.blocking_reasons.length ? "destructive" : "outline"}>{snapshot.blocking_reasons.length || "无"}</Badge>
                  </div>
                  {snapshot.blocking_reasons.length ? (
                    <div className="mt-4 grid gap-3">
                      {snapshot.blocking_reasons.slice(0, 3).map((reason) => (
                        <div className="border-l-2 border-destructive/60 pl-3" key={`${reason.code}:${reason.resource_id}`}>
                          <p className="text-sm font-medium leading-5">{reason.summary}</p>
                          <p className="mt-1 text-xs text-muted-foreground">来自 {reason.resource_type === "project" ? "项目" : "单集"} 状态</p>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 text-sm leading-6 text-muted-foreground">当前没有已确认的阻塞项，可以继续推进制作。</p>
                  )}
                </section>

                <section className="rounded-2xl border bg-muted/35 p-5" aria-label="项目规格">
                  <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">项目规格</p>
                  <dl className="mt-4 grid gap-3 text-sm">
                    <div className="flex items-center justify-between gap-4"><dt className="flex items-center gap-2 text-muted-foreground"><FileText aria-hidden="true" className="size-4" />语言</dt><dd>{project.language}</dd></div>
                    <div className="flex items-center justify-between gap-4"><dt className="flex items-center gap-2 text-muted-foreground"><Layers3 aria-hidden="true" className="size-4" />默认时长</dt><dd>{Math.round(project.target_duration_ms / 1_000)} 秒</dd></div>
                  </dl>
                </section>
              </aside>
            </div>
          </div>
        )}
      </LayoutContainer>
    </StudioShell>
  );
}
