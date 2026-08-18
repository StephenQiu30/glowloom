"use client";

import {
  AlertCircle,
  ArrowRight,
  ChevronRight,
  FileText,
  Layers3,
  LoaderCircle,
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
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
  const readyAssets = snapshot?.episodes[0]?.asset_summary.ready ?? 0;
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
      <LayoutContainer className="py-10">
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
          <div className="min-w-0">
              <PageHeader
              actions={firstEpisode ? (
                <Button asChild>
                  <Link href={`/studio/${firstEpisode.id}/script`}>
                    继续制作<ArrowRight aria-hidden="true" />
                  </Link>
                </Button>
              ) : undefined}
              badges={[
                { label: stageLabels[snapshot.current_stage] },
                { label: project.aspect_ratio },
                { label: project.visual_style ?? "未设视觉风格" },
                ...(project.status === "archived" ? [{ label: "已归档", variant: "secondary" as const }] : []),
              ]}
              breadcrumbs={[{ label: "项目", href: "/projects" }, { label: project.name }]}
              description={project.description || "尚未填写项目简介"}
              note="从已确认事实继续，而不是从头重来。"
              title={project.name}
            />

              <ProductionNextAction
                action={snapshot.next_actions[0]}
                blockingReasons={snapshot.blocking_reasons}
              />

              <MetricGroup
                className="mt-8"
                items={[
                  { label: "项目进度", value: `${snapshot.completion}%` },
                  { label: "活跃单集", value: activeEpisodes.length },
                  { label: "Ready 资产", value: readyAssets },
                  { label: "Ready 分镜", value: readyStoryboards },
                  { label: "进行中任务", value: runningTasks },
                ]}
                label="项目生产摘要"
              />

              <section aria-label="项目内容" className="mt-8 grid gap-4 sm:grid-cols-2">
                <Card className="transition hover:border-foreground/25 hover:shadow-sm">
                  <CardHeader>
                    <CardTitle>项目资产</CardTitle>
                    <CardDescription>角色、场景、道具、服装、声音和风格参考。</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button asChild variant="outline">
                      <Link href={`/projects/${project.id}/assets`}>查看项目资产<ArrowRight aria-hidden="true" /></Link>
                    </Button>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader>
                    <CardTitle>单集生产</CardTitle>
                    <CardDescription>从剧本、资产、分镜到媒体和任务，按单集继续制作。</CardDescription>
                  </CardHeader>
                </Card>
              </section>

              <ScriptDocumentImportCard
                canWrite={canWrite}
                language={project.language}
                projectId={project.id}
                targetDurationMs={project.target_duration_ms}
                workspaceId={project.workspace_id}
              />

              {snapshot.partial_failures.length ? (
                <Alert className="mt-5 border-0 bg-destructive/10" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>部分摘要不可用</AlertTitle><AlertDescription>{snapshot.partial_failures.map((item) => item.summary).join("；")}</AlertDescription></Alert>
              ) : null}

              <div className="mt-8 grid gap-8 xl:grid-cols-[minmax(0,1fr)_360px]">
                <Card>
                <CardHeader>
                  <CardTitle>单集列表</CardTitle>
                  <CardDescription>查看每集当前阶段、阻塞和继续制作入口。</CardDescription>
                </CardHeader>
                <CardContent className="divide-y divide-border p-0">
                  {episodes.length === 0 ? (
                    <div className="px-6 py-16 text-center"><p className="font-medium">尚未生成剧集</p><p className="mt-1 text-sm text-muted-foreground">确认剧本解析与分集方案后，系统将在这里生成项目剧集。</p></div>
                  ) : episodes.map((episode) => {
                    const episodeSnapshot = episodeSnapshots.get(episode.id);
                    return (
                      <Link
                        aria-label={`进入${episode.name}`}
                        className="grid gap-4 px-5 py-4 transition hover:bg-muted/50 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center"
                        href={`/studio/${episode.id}/script`}
                        key={episode.id}
                      >
                        <span className="grid size-11 place-items-center bg-muted font-mono text-sm font-semibold">{String(episode.position).padStart(2, "0")}</span>
                        <span className="min-w-0">
                          <span className="block truncate font-medium">{episode.name}</span>
                          <span className="mt-1 flex flex-wrap gap-3 text-xs text-muted-foreground">
                            <span>{Math.round(episode.target_duration_ms / 1_000)} 秒</span>
                            <span>{episodeSnapshot ? stageLabels[episodeSnapshot.current_stage] : "摘要不可用"}</span>
                            <span>{episodeSnapshot?.completion ?? 0}%</span>
                          </span>
                        </span>
                        <span className="flex items-center gap-3">
                          <Badge variant={episode.status === "active" ? "outline" : "secondary"}>{episode.status === "active" ? "制作中" : "已归档"}</Badge>
                          <ChevronRight className="size-4 text-muted-foreground" aria-hidden="true" />
                        </span>
                      </Link>
                    );
                  })}
                </CardContent>
              </Card>

                <aside className="grid content-start gap-5">
                  <Card>
                    <CardHeader><CardTitle>项目规格</CardTitle></CardHeader>
                    <CardContent className="grid gap-4 text-sm">
                      <div className="flex items-center justify-between gap-4"><span className="flex items-center gap-2 text-muted-foreground"><FileText className="size-4" aria-hidden="true" />语言</span><span>{project.language}</span></div>
                      <div className="flex items-center justify-between gap-4"><span className="flex items-center gap-2 text-muted-foreground"><Layers3 className="size-4" aria-hidden="true" />默认时长</span><span>{Math.round(project.target_duration_ms / 1_000)} 秒</span></div>
                    </CardContent>
                  </Card>
                </aside>
              </div>
            </div>
        )}
      </LayoutContainer>
    </StudioShell>
  );
}
