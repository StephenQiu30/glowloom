"use client";

import { AlertCircle, ArrowRight, Clapperboard, ClipboardCheck, LoaderCircle } from "lucide-react";
import Link from "next/link";

import { LayoutContainer } from "@/components/layout/layout-container";
import { PageHeader } from "@/components/studio/page-header";
import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useCurrentScriptDocumentQuery,
  useEpisodesQuery,
  useMeQuery,
  useProjectQuery,
} from "@/lib/server-state";

import { ScriptDocumentImportCard } from "./script-document-import-card";

export function ProjectWorkspace({ projectId }: { projectId: string }) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const projectQuery = useProjectQuery(projectId, { skip: !authenticated });
  const episodesQuery = useEpisodesQuery(projectId, { skip: !authenticated });
  const currentScriptQuery = useCurrentScriptDocumentQuery(projectId, {
    skip: !authenticated,
  });
  const project = projectQuery.data;
  const episodes = episodesQuery.data ?? [];
  const canWrite = project?.status === "active" && me.data?.workspace.role !== "viewer";
  const currentScriptError = currentScriptQuery.error as { code?: string } | undefined;
  const error = me.error ?? projectQuery.error ?? episodesQuery.error ?? (
    currentScriptError?.code && currentScriptError.code !== "not_found"
      ? currentScriptQuery.error
      : undefined
  );

  if (sessionState === "checking") {
    return <StudioShell active="projects"><div className="grid min-h-[70dvh] place-items-center"><LoaderCircle aria-label="正在读取登录状态" className="animate-spin" /></div></StudioShell>;
  }

  return (
    <StudioShell
      active="projects"
      projectName={project?.name}
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      <LayoutContainer className="py-8 sm:py-10">
        {!authenticated ? (
          <Alert><AlertCircle aria-hidden="true" /><AlertTitle>需要登录</AlertTitle><AlertDescription><Link className="underline" href="/login">登录后查看项目</Link></AlertDescription></Alert>
        ) : error ? (
          <Alert variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>项目事实暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(error)}</AlertDescription></Alert>
        ) : !project ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle aria-label="正在加载项目" className="animate-spin" /></div>
        ) : (
          <div className="space-y-7">
            <PageHeader
              actions={(
                <div className="flex flex-wrap gap-2">
                  <Button asChild variant="outline">
                    <Link href={`/projects/${project.id}/reviews`}>
                      <ClipboardCheck aria-hidden="true" />
                      审核队列
                    </Link>
                  </Button>
                  {episodes[0] ? (
                    <Button asChild><Link href={`/studio/${episodes[0].id}/script`}>继续制作<ArrowRight aria-hidden="true" /></Link></Button>
                  ) : (
                    <Button asChild><Link href="#script-import">导入剧本<ArrowRight aria-hidden="true" /></Link></Button>
                  )}
                </div>
              )}
              badges={[{ label: project.aspect_ratio }, { label: project.visual_style ?? "未设视觉风格" }]}
              breadcrumbs={[{ label: "项目", href: "/projects" }, { label: project.name }]}
              description={project.description || "从不可变原稿开始，依次确认制作圣经、分集、场景任务和分镜。"}
              note="当前页面只呈现这条 MVP 主链所需的服务端事实。"
              title={project.name}
            />

            <ScriptDocumentImportCard
              canWrite={canWrite}
              currentAnalysis={currentScriptQuery.data}
              language={project.language}
              projectId={project.id}
              targetDurationMs={project.target_duration_ms}
              workspaceId={project.workspace_id}
            />

            <section aria-label="单集工作区" className="border bg-card" id="episodes">
              <div className="flex items-center justify-between border-b p-5">
                <div><h2 className="text-xl font-semibold">单集工作区</h2><p className="mt-1 text-sm text-muted-foreground">每集从已发布剧本进入结构审阅和分镜制作。</p></div>
                <Badge variant="outline">{episodes.length} 集</Badge>
              </div>
              {episodes.length === 0 ? (
                <div className="grid min-h-40 place-items-center p-8 text-center text-sm text-muted-foreground">确认制作圣经和分集计划后，剧集会出现在这里。</div>
              ) : (
                <div className="divide-y">
                  {episodes.map((episode) => (
                    <Link
                      aria-label={`进入${episode.name}`}
                      className="flex items-center justify-between gap-4 p-5 hover:bg-muted/40"
                      href={`/studio/${episode.id}/script`}
                      key={episode.id}
                    >
                      <span className="flex items-center gap-3"><Clapperboard className="size-5" aria-hidden="true" /><span><span className="block font-medium">{episode.name}</span><span className="text-xs text-muted-foreground">第 {episode.position} 集 · {Math.round(episode.target_duration_ms / 1000)} 秒</span></span></span>
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </Link>
                  ))}
                </div>
              )}
            </section>
          </div>
        )}
      </LayoutContainer>
    </StudioShell>
  );
}
