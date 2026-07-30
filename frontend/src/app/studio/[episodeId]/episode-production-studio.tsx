"use client";

import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  LoaderCircle,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useAssetsQuery,
  useCompleteMediaUploadMutation,
  useConfirmStructureMutation,
  useDecideExtractionCandidateMutation,
  useEpisodeQuery,
  useEpisodeSnapshotQuery,
  useEpisodesQuery,
  useExtractionBatchQuery,
  useExtractionCandidatesQuery,
  useImportScriptMutation,
  useInitializeMediaUploadMutation,
  useMeQuery,
  useMediaVersionsQuery,
  useProjectQuery,
  usePublishScriptVersionMutation,
  useRetryMediaProbeMutation,
  useScriptSourcesQuery,
  useScriptVersionQuery,
  useScriptVersionsQuery,
  useSetCurrentScriptVersionMutation,
  useStartExtractionMutation,
  useTasksQuery,
} from "@/lib/server-state";

import { EpisodeAssetOverview } from "./episode-asset-overview";
import {
  type EpisodePanel,
  episodePanels,
  sha256File,
  stageLabels,
} from "./episode-studio-model";
import { MediaWorkspace } from "./media-workspace";
import { ScriptWorkspace } from "./script-workspace";
import { TaskWorkspace } from "./task-workspace";

const stageSteps: Record<API.EpisodeProductionSnapshot["current_stage"], number> = {
  script_import: 0,
  structure_review: 0,
  asset_preparation: 1,
  storyboard_preparation: 2,
};

export function EpisodeProductionStudio({
  episodeId,
  initialPanel,
}: {
  episodeId: string;
  initialPanel: EpisodePanel;
}) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const workspaceId = me.data?.workspace.id;
  const episodeQuery = useEpisodeQuery(episodeId, { skip: !authenticated });
  const episode = episodeQuery.data;
  const projectQuery = useProjectQuery(episode?.project_id ?? "", { skip: !episode });
  const project = projectQuery.data;
  const episodesQuery = useEpisodesQuery(episode?.project_id ?? "", {
    skip: !episode,
  });
  const snapshotQuery = useEpisodeSnapshotQuery(episodeId, {
    pollingInterval: 5_000,
    skip: !authenticated,
  });
  const snapshot = snapshotQuery.data;
  const sourcesQuery = useScriptSourcesQuery(episodeId, { skip: !authenticated });
  const currentVersionQuery = useScriptVersionQuery(
    episode?.current_script_version_id ?? "",
    { skip: !episode?.current_script_version_id },
  );
  const sources = sourcesQuery.data?.items ?? [];
  const activeSource =
    sources.find((source) => source.id === currentVersionQuery.data?.source_id) ??
    sources[0];
  const versionsQuery = useScriptVersionsQuery(activeSource?.id ?? "", {
    skip: !activeSource,
  });
  const versions = versionsQuery.data?.items ?? [];
  const editableVersion =
    currentVersionQuery.data ?? versions.at(-1);
  const tasksQuery = useTasksQuery(workspaceId ?? "", {
    pollingInterval: 4_000,
    skip: !workspaceId,
  });
  const episodeTasks = useMemo(
    () =>
      (tasksQuery.data?.items ?? []).filter(
        (task) => task.scope.episode_id === episodeId,
      ),
    [episodeId, tasksQuery.data?.items],
  );
  const [startedBatchId, setStartedBatchId] = useState<string | null>(null);
  const taskBatchId = episodeTasks.find(
    (task) => task.task_type === "script_extraction",
  )?.request_id;
  const batchId =
    startedBatchId ?? snapshot?.script_summary.extraction_batch_id ?? taskBatchId;
  const batchQuery = useExtractionBatchQuery(batchId ?? "", {
    pollingInterval: 4_000,
    skip: !batchId,
  });
  const candidatesQuery = useExtractionCandidatesQuery(batchId ?? "", {
    pollingInterval: batchQuery.data?.status === "running" ? 4_000 : 0,
    skip: !batchId,
  });
  const assetsQuery = useAssetsQuery(episode?.project_id ?? "", { skip: !episode });
  const mediaQuery = useMediaVersionsQuery(workspaceId ?? "", { skip: !workspaceId });

  const [importScript, importState] = useImportScriptMutation();
  const [publishVersion, publishState] = usePublishScriptVersionMutation();
  const [startExtraction, extractionState] = useStartExtractionMutation();
  const [decideCandidate, decisionState] = useDecideExtractionCandidateMutation();
  const [confirmStructure, confirmationState] = useConfirmStructureMutation();
  const [setCurrentVersion, currentState] = useSetCurrentScriptVersionMutation();
  const [initializeUpload, initializationState] = useInitializeMediaUploadMutation();
  const [completeUpload, completionState] = useCompleteMediaUploadMutation();
  const [retryProbe, retryState] = useRetryMediaProbeMutation();
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const busy = [
    importState,
    publishState,
    extractionState,
    decisionState,
    confirmationState,
    currentState,
    initializationState,
    completionState,
    retryState,
  ].some((state) => state.isLoading);

  async function runAction(action: () => Promise<string>) {
    setActionError(null);
    setNotice(null);
    try {
      setNotice(await action());
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function handleImport(request: API.ScriptImportRequest) {
    await runAction(async () => {
      const result = await importScript({ episodeId, body: request }).unwrap();
      return `剧本《${result.source.title}》已导入为 v${result.version.version_no} 草稿。`;
    });
  }

  async function handlePublish(body: string) {
    if (!activeSource || !episode) return;
    await runAction(async () => {
      const result = await publishVersion({
        episodeId,
        sourceId: activeSource.id,
        body: {
          body,
          expected_current_version_id: episode.current_script_version_id,
        },
      }).unwrap();
      setStartedBatchId(null);
      return `剧本 v${result.version.version_no} 已发布并设为当前版本。`;
    });
  }

  async function handleStartExtraction() {
    const versionId = episode?.current_script_version_id;
    if (!versionId || !workspaceId) return;
    await runAction(async () => {
      const result = await startExtraction({
        episodeId,
        workspaceId,
        versionId,
        body: {
          scope: "full",
          idempotency_key: `studio-extraction:${versionId}:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setStartedBatchId(result.id);
      return "提取任务已创建，可以离开页面后再回来查看状态。";
    });
  }

  async function handleDecision(
    candidate: API.ExtractionCandidateResponse,
    decision: API.CandidateDecisionRequest["decision"],
  ): Promise<boolean> {
    if (!batchId || !episode?.project_id) return false;
    let succeeded = false;
    await runAction(async () => {
      await decideCandidate({
        candidateId: candidate.id,
        batchId,
        episodeId,
        projectId: episode.project_id,
        body: {
          decision_key: `studio-decision:${candidate.id}:${crypto.randomUUID()}`,
          expected_revision: candidate.revision,
          decision,
        },
      }).unwrap();
      succeeded = true;
      return `候选“${candidate.candidate_key}”已完成决议。`;
    });
    return succeeded;
  }

  async function handleConfirm() {
    if (!batchId) return;
    await runAction(async () => {
      const result = await confirmStructure({ batchId, episodeId }).unwrap();
      return `结构已确认，生成剧本 v${result.confirmed_version.version_no}。`;
    });
  }

  async function handleSetCurrent() {
    const confirmedVersionId = batchQuery.data?.confirmed_script_version_id;
    if (!confirmedVersionId || !episode) return;
    await runAction(async () => {
      await setCurrentVersion({
        episodeId,
        body: {
          version_id: confirmedVersionId,
          expected_current_version_id: episode.current_script_version_id,
        },
      }).unwrap();
      setStartedBatchId(null);
      return "已确认结构的剧本版本已设为当前入口。";
    });
  }

  async function handleUpload(
    file: File,
    kind: API.UploadDeclaration["kind"],
  ): Promise<boolean> {
    if (!workspaceId) return false;
    let succeeded = false;
    await runAction(async () => {
      const sha256 = await sha256File(file);
      const initialized = await initializeUpload({
        workspace_id: workspaceId,
        kind,
        filename: file.name,
        size_bytes: file.size,
        mime_type: file.type || "application/octet-stream",
        sha256,
        idempotency_key: `studio-upload:${sha256}:${file.name}`,
      }).unwrap();
      if (!initialized.upload.url || !initialized.upload.method) {
        throw new Error("对象存储未返回有效的上传地址");
      }
      const uploaded = await fetch(initialized.upload.url, {
        method: initialized.upload.method,
        headers: initialized.upload.headers as HeadersInit,
        body: file,
      });
      if (!uploaded.ok) throw new Error(`对象存储返回 ${uploaded.status}`);
      const result = await completeUpload({
        uploadSessionId: initialized.upload_session.id,
        workspaceId,
      }).unwrap();
      succeeded = true;
      return `${result.version.filename} 已上传，媒体探测任务已创建。`;
    });
    return succeeded;
  }

  async function handleRetry(version: API.MediaVersionResponse) {
    if (!workspaceId) return;
    await runAction(async () => {
      await retryProbe({
        versionId: version.id,
        workspaceId,
        body: { idempotency_key: `studio-probe-retry:${version.id}:${version.probe_attempt + 1}` },
      }).unwrap();
      return `${version.filename} 已重新进入探测队列。`;
    });
  }

  const pageError =
    me.error ??
    episodeQuery.error ??
    projectQuery.error ??
    episodesQuery.error ??
    snapshotQuery.error ??
    sourcesQuery.error ??
    currentVersionQuery.error ??
    versionsQuery.error ??
    tasksQuery.error ??
    batchQuery.error ??
    candidatesQuery.error ??
    assetsQuery.error ??
    mediaQuery.error;

  if (sessionState === "checking") {
    return <div className="grid min-h-screen place-items-center"><LoaderCircle className="animate-spin text-[#079db3]" aria-label="正在读取登录状态" /></div>;
  }

  return (
    <StudioShell
      active="assets"
      currentStep={snapshot ? stageSteps[snapshot.current_stage] : 0}
      projectName={project?.name}
      viewer={
        me.data
          ? {
              displayName: me.data.user.display_name?.trim() || me.data.user.email,
              workspaceName: me.data.workspace.name,
            }
          : undefined
      }
      topAction={
        episode ? (
          <Button asChild variant="outline">
            <Link href={`/projects/${episode.project_id}`}>项目概览</Link>
          </Button>
        ) : undefined
      }
    >
      {notice ? (
        <div className="fixed top-24 right-6 z-50 flex max-w-md items-center gap-2 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg" role="status">
          <CheckCircle2 className="size-4 shrink-0 text-emerald-600" aria-hidden="true" />
          {notice}
        </div>
      ) : null}
      <div className="mx-auto max-w-[1420px] px-5 py-8 md:px-8">
        {!authenticated ? (
          <Alert className="border-amber-200 bg-amber-50 text-amber-800">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>需要登录</AlertTitle>
            <AlertDescription><Link className="underline" href="/login">登录后进入单集生产工作台</Link></AlertDescription>
          </Alert>
        ) : pageError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>生产事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription>
          </Alert>
        ) : !episode || !project || !snapshot ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle className="animate-spin text-[#079db3]" aria-label="正在加载生产工作台" /></div>
        ) : (
          <>
            <header className="flex flex-wrap items-start justify-between gap-5">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">
                    {stageLabels[snapshot.current_stage]}
                  </Badge>
                  <Badge variant="outline">{project.aspect_ratio}</Badge>
                  <Badge variant="outline">{project.visual_style ?? "未设视觉风格"}</Badge>
                </div>
                <h1 className="mt-3 text-3xl font-semibold tracking-[-0.035em]">{episode.name}</h1>
                <p className="mt-2 text-sm text-slate-500">
                  第 {episode.position} 集 · 服务端计算 {snapshot.completion}% · revision {episode.revision}
                </p>
              </div>
              <div className="relative">
                <select
                  aria-label="切换单集"
                  className="h-10 min-w-48 appearance-none rounded-lg border border-slate-200 bg-white pr-10 pl-3 text-sm"
                  value={episode.id}
                  onChange={(event) => {
                    window.location.href = `/studio/${event.target.value}/${initialPanel}`;
                  }}
                >
                  {(episodesQuery.data ?? []).map((item) => (
                    <option key={item.id} value={item.id}>第 {item.position} 集 · {item.name}</option>
                  ))}
                </select>
                <ChevronDown className="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
              </div>
            </header>

            <section className="mt-7 grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="生产摘要">
              <Card><CardHeader><CardDescription>当前阶段</CardDescription><CardTitle>{stageLabels[snapshot.current_stage]}</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>剧本状态</CardDescription><CardTitle>v{editableVersion?.version_no ?? "-"}</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>Ready 资产</CardDescription><CardTitle>{snapshot.asset_summary.ready} / {snapshot.asset_summary.total}</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>进行中任务</CardDescription><CardTitle>{snapshot.task_summary.running}</CardTitle></CardHeader></Card>
            </section>

            {snapshot.partial_failures.length ? (
              <Alert className="mt-5 border-rose-200 bg-rose-50 text-rose-800">
                <AlertCircle aria-hidden="true" />
                <AlertTitle>部分摘要不可用</AlertTitle>
                <AlertDescription>{snapshot.partial_failures.map((failure) => failure.summary).join("；")}</AlertDescription>
              </Alert>
            ) : null}
            {actionError ? (
              <Alert className="mt-5" variant="destructive">
                <AlertCircle aria-hidden="true" />
                <AlertTitle>操作未完成</AlertTitle>
                <AlertDescription>{actionError}</AlertDescription>
              </Alert>
            ) : null}

            <nav className="mt-7 grid gap-2 rounded-2xl border border-slate-200 bg-white p-2 sm:grid-cols-2 xl:grid-cols-4" aria-label="单集制作模块">
              {episodePanels.map((panel) => (
                <Link
                  aria-current={panel.id === initialPanel ? "page" : undefined}
                  className={`flex items-center gap-3 rounded-xl px-4 py-3 transition ${panel.id === initialPanel ? "bg-slate-100 text-[#078fa5]" : "text-slate-600 hover:bg-slate-50"}`}
                  href={`/studio/${episode.id}/${panel.id}`}
                  key={panel.id}
                >
                  <panel.icon className="size-5 shrink-0" aria-hidden="true" />
                  <span><span className="block text-sm font-medium">{panel.label}</span><span className="mt-0.5 block text-xs text-slate-400">{panel.description}</span></span>
                </Link>
              ))}
            </nav>

            <section className="mt-6">
              {initialPanel === "script" ? (
                <ScriptWorkspace
                  assets={assetsQuery.data?.items ?? []}
                  batch={batchQuery.data}
                  busy={busy}
                  candidates={candidatesQuery.data?.items ?? []}
                  editableVersion={editableVersion}
                  episode={episode}
                  key={editableVersion?.id ?? activeSource?.id ?? "script-import"}
                  snapshot={snapshot}
                  source={activeSource}
                  versions={versions}
                  onConfirm={handleConfirm}
                  onDecide={handleDecision}
                  onImport={handleImport}
                  onPublish={handlePublish}
                  onSetCurrent={handleSetCurrent}
                  onStartExtraction={handleStartExtraction}
                />
              ) : initialPanel === "assets" ? (
                <EpisodeAssetOverview assets={assetsQuery.data?.items ?? []} summary={snapshot.asset_summary} />
              ) : initialPanel === "media" ? (
                <MediaWorkspace busy={busy} media={mediaQuery.data?.items ?? []} onRetry={handleRetry} onUpload={handleUpload} />
              ) : (
                <TaskWorkspace tasks={episodeTasks} />
              )}
            </section>
          </>
        )}
      </div>
    </StudioShell>
  );
}
