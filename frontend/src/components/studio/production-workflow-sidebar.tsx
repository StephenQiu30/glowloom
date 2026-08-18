"use client";

import {
  ArrowRight,
  Blocks,
  CheckCircle2,
  ChevronRight,
  Clapperboard,
  FileText,
  FolderKanban,
  ImageIcon,
  ListChecks,
} from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { cn } from "@/lib/class-names";

import { type EpisodePanel, episodePanels } from "@/app/studio/[episodeId]/episode-studio-model";

const projectStageLabels: Record<API.ProjectProductionSnapshot["current_stage"], string> = {
  project_setup: "项目设置",
  script_import: "剧本导入",
  structure_review: "结构确认",
  asset_preparation: "资产准备",
  storyboard_preparation: "分镜准备",
};

const episodeStageLabels: Record<API.EpisodeProductionSnapshot["current_stage"], string> = {
  script_import: "剧本导入",
  structure_review: "结构确认",
  asset_preparation: "资产准备",
  storyboard_preparation: "分镜准备",
};

const panelIcons = {
  script: FileText,
  assets: Blocks,
  storyboard: Clapperboard,
  media: ImageIcon,
  tasks: ListChecks,
} satisfies Record<EpisodePanel, typeof FileText>;

function panelStatus(panel: EpisodePanel, snapshot?: API.EpisodeProductionSnapshot): string | null {
  if (!snapshot) return null;
  if (panel === "script") {
    return snapshot.script_summary.current_version_id ? "已建立版本" : "待导入";
  }
  if (panel === "assets") {
    return `${snapshot.asset_summary.ready}/${snapshot.asset_summary.total} 已就绪`;
  }
  if (panel === "storyboard") {
    return `${snapshot.storyboard_summary.ready ?? 0}/${snapshot.storyboard_summary.total ?? 0} 已就绪`;
  }
  if (panel === "tasks") {
    const running = snapshot.task_summary.running ?? 0;
    const failed = snapshot.task_summary.failed ?? 0;
    if (running > 0) return `${running} 个执行中`;
    if (failed > 0) return `${failed} 个失败`;
    return "状态正常";
  }
  return "媒体管理";
}

function WorkflowLink({
  active = false,
  children,
  description,
  href,
  icon: Icon,
  meta,
}: {
  active?: boolean;
  children: ReactNode;
  description?: string;
  href: string;
  icon: typeof FileText;
  meta?: string | null;
}) {
  return (
    <Link
      aria-current={active ? "page" : undefined}
      className={cn(
        "group flex items-start gap-3 rounded-xl px-3 py-2.5 transition-colors max-xl:min-w-[148px] max-xl:flex-col max-xl:gap-2 max-xl:py-3",
        active
          ? "bg-foreground text-background"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
      href={href}
    >
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 flex-1">
        <span className="flex items-center justify-between gap-2">
          <span className="truncate text-sm font-medium">{children}</span>
          {meta ? (
            <span className={cn("shrink-0 text-[10px] max-xl:hidden", active ? "text-background/70" : "text-muted-foreground")}>
              {meta}
            </span>
          ) : null}
        </span>
        {description ? <span className={cn("mt-0.5 block text-xs max-xl:hidden", active ? "text-background/65" : "text-muted-foreground")}>{description}</span> : null}
      </span>
      <ChevronRight className={cn("mt-0.5 size-3.5 shrink-0 transition-transform group-hover:translate-x-0.5 max-xl:hidden", active ? "text-background/70" : "text-muted-foreground/60")} aria-hidden="true" />
    </Link>
  );
}

export function ProductionWorkflowSidebar({
  activeEpisodeId,
  activePanel,
  activeProjectPanel,
  episodeSnapshot,
  episodes,
  projectId,
  projectName,
  projectSnapshot,
}: {
  activeEpisodeId?: string;
  activePanel?: EpisodePanel;
  activeProjectPanel?: "overview" | "assets";
  episodeSnapshot?: API.EpisodeProductionSnapshot;
  episodes: API.EpisodeResponse[];
  projectId: string;
  projectName: string;
  projectSnapshot?: API.ProjectProductionSnapshot;
}) {
  const activeEpisode = episodes.find((episode) => episode.id === activeEpisodeId);
  const resolvedEpisodeSnapshot = episodeSnapshot ?? projectSnapshot?.episodes.find((item) => item.episode_id === activeEpisodeId);
  const completion = resolvedEpisodeSnapshot?.completion ?? projectSnapshot?.completion;
  const stage = resolvedEpisodeSnapshot
    ? episodeStageLabels[resolvedEpisodeSnapshot.current_stage]
    : projectSnapshot
      ? projectStageLabels[projectSnapshot.current_stage]
      : null;

  function switchEpisode(value: string) {
    if (!value) return;
    window.location.href = `/studio/${value}/${activePanel ?? "script"}`;
  }

  return (
    <aside aria-label="项目制作流程导航" className="xl:sticky xl:top-6">
      <div className="rounded-2xl border bg-card p-3 shadow-sm">
        <div className="rounded-xl bg-muted/60 px-3 py-3">
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <FolderKanban className="size-3.5" aria-hidden="true" />
            制作工作台
          </div>
          <Link className="mt-2 block truncate text-sm font-semibold hover:underline" href={`/projects/${projectId}`}>
            {projectName}
          </Link>
          {stage || typeof completion === "number" ? (
            <div className="mt-3 flex items-center justify-between gap-3 text-xs text-muted-foreground">
              <span>{stage ?? "生产摘要"}</span>
              {typeof completion === "number" ? <span className="font-mono">{completion}%</span> : null}
            </div>
          ) : null}
          {typeof completion === "number" ? (
            <div aria-label={`项目完成度 ${completion}%`} className="mt-2 h-1.5 overflow-hidden rounded-full bg-background" role="progressbar" aria-valuemax={100} aria-valuemin={0} aria-valuenow={completion}>
              <div className="h-full rounded-full bg-foreground transition-[width]" style={{ width: `${Math.min(100, Math.max(0, completion))}%` }} />
            </div>
          ) : null}
        </div>

        <div className="mt-4">
          <label className="mb-2 block px-1 text-xs font-medium text-muted-foreground" htmlFor="production-episode-switcher">
            当前剧集
          </label>
          <select
            aria-label="切换当前剧集"
            className="h-10 w-full rounded-lg border bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            id="production-episode-switcher"
            onChange={(event) => switchEpisode(event.target.value)}
            value={activeEpisodeId ?? ""}
          >
            <option disabled value="">选择一个剧集</option>
            {episodes.map((episode) => (
              <option key={episode.id} value={episode.id}>
                第 {episode.position} 集 · {episode.name}
              </option>
            ))}
          </select>
        </div>

        <nav className="mt-5" aria-label="项目流程">
          <p className="mb-2 px-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">项目</p>
          <WorkflowLink active={!activeEpisodeId && activeProjectPanel === "overview"} description="项目摘要、原稿和分集管理" href={`/projects/${projectId}`} icon={FolderKanban}>
            项目概览
          </WorkflowLink>
          <WorkflowLink active={!activeEpisodeId && activeProjectPanel === "assets"} description="角色、场景、道具和版本" href={`/projects/${projectId}/assets`} icon={Blocks}>
            项目资产
          </WorkflowLink>

          {activeEpisode ? (
            <>
              <div className="my-4 flex items-center gap-2 px-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                <span className="h-px flex-1 bg-border" />
                <span>第 {activeEpisode.position} 集流程</span>
                <span className="h-px flex-1 bg-border" />
              </div>
              <div className="grid gap-1 max-xl:flex max-xl:overflow-x-auto max-xl:pb-1">
                {episodePanels.map((panel) => {
                  const Icon = panelIcons[panel.id];
                  return (
                    <WorkflowLink
                      active={activePanel === panel.id}
                      description={panel.description}
                      href={`/studio/${activeEpisode.id}/${panel.id}`}
                      icon={Icon}
                      key={panel.id}
                      meta={panelStatus(panel.id, resolvedEpisodeSnapshot)}
                    >
                      {panel.label}
                    </WorkflowLink>
                  );
                })}
              </div>
            </>
          ) : (
            <div className="mt-3 rounded-xl border border-dashed px-3 py-4 text-sm text-muted-foreground">
              <p>选择剧集后进入剧本、资产、分镜和任务流程。</p>
              {episodes.length === 0 ? (
                <span className="mt-2 inline-flex items-center gap-1 text-xs"><CheckCircle2 className="size-3.5" aria-hidden="true" />等待创建第一集</span>
              ) : (
                <span className="mt-2 inline-flex items-center gap-1 text-xs">从上方选择剧集<ArrowRight className="size-3.5" aria-hidden="true" /></span>
              )}
            </div>
          )}
        </nav>

      </div>
    </aside>
  );
}
