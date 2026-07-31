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
  useAppendShotSpecMutation,
  useArchivedShotsQuery,
  useCompleteMediaUploadMutation,
  useConfirmStructureMutation,
  useConfirmedStructureQuery,
  useCopyShotMutation,
  useCreateShotMutation,
  useCreateShotFromCandidateMutation,
  useDeleteScriptVersionMutation,
  useDeleteShotMutation,
  useDecideExtractionCandidateMutation,
  useEpisodeQuery,
  useEpisodeSnapshotQuery,
  useEpisodesQuery,
  useExtractionBatchQuery,
  useExtractionCandidatesQuery,
  useImportScriptMutation,
  useInitializeMediaUploadMutation,
  useLazyShotSpecVersionQuery,
  useLazyScriptVersionDiffQuery,
  useMeQuery,
  useMergeShotsMutation,
  useMergeShotsPreflightMutation,
  useMediaVersionsQuery,
  useProjectQuery,
  usePublishScriptVersionMutation,
  useRetryMediaProbeMutation,
  useReorderShotsMutation,
  useScriptSourcesQuery,
  useScriptVersionQuery,
  useScriptVersionsQuery,
  useSetCurrentScriptVersionMutation,
  useSetScriptSourceArchivedMutation,
  useSetCurrentShotSpecMutation,
  useSetShotArchivedMutation,
  useShotDeletePreflightMutation,
  useShotOrderQuery,
  useShotReadinessQuery,
  useShotSpecVersionsQuery,
  useSplitShotMutation,
  useSplitShotPreflightMutation,
  useStartExtractionMutation,
  useTasksQuery,
  useUpdateShotMutation,
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
import { type MergePreparation } from "./storyboard-shot-operations";
import { StoryboardWorkspace } from "./storyboard-workspace";
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
  const storyboardActive = initialPanel === "storyboard";
  const shotOrderQuery = useShotOrderQuery(episodeId, {
    skip: !authenticated || !storyboardActive,
  });
  const archivedShotsQuery = useArchivedShotsQuery(episodeId, {
    skip: !authenticated || !storyboardActive,
  });
  const shotReadinessQuery = useShotReadinessQuery(episodeId, {
    pollingInterval: 5_000,
    skip: !authenticated || !storyboardActive,
  });
  const confirmedVersionId =
    snapshot?.script_summary.status === "confirmed"
      ? episode?.current_script_version_id
      : null;
  const structureQuery = useConfirmedStructureQuery(confirmedVersionId ?? "", {
    skip: !confirmedVersionId || !storyboardActive,
  });
  const [selectedShotId, setSelectedShotId] = useState<string | null>(null);
  const selectedShot =
    shotOrderQuery.data?.items.find((shot) => shot.id === selectedShotId) ??
    shotOrderQuery.data?.items[0];
  const confirmedShotCandidates = useMemo(() => {
    const usedCandidateIds = new Set(
      [
        ...(shotOrderQuery.data?.items ?? []),
        ...(archivedShotsQuery.data ?? []),
      ].flatMap((shot) =>
        shot.source_candidate_id ? [shot.source_candidate_id] : [],
      ),
    );
    return (candidatesQuery.data?.items ?? []).filter(
      (candidate) =>
        candidate.kind === "shot" &&
        candidate.proposal.kind === "shot" &&
        candidate.status === "accepted" &&
        !usedCandidateIds.has(candidate.id),
    );
  }, [
    archivedShotsQuery.data,
    candidatesQuery.data?.items,
    shotOrderQuery.data?.items,
  ]);
  const shotSpecVersionsQuery = useShotSpecVersionsQuery(selectedShot?.id ?? "", {
    skip: !selectedShot || !storyboardActive,
  });
  const storyboardLoading =
    storyboardActive &&
    (shotOrderQuery.isLoading ||
      archivedShotsQuery.isLoading ||
      shotReadinessQuery.isLoading ||
      structureQuery.isLoading);

  const [importScript, importState] = useImportScriptMutation();
  const [publishVersion, publishState] = usePublishScriptVersionMutation();
  const [startExtraction, extractionState] = useStartExtractionMutation();
  const [decideCandidate, decisionState] = useDecideExtractionCandidateMutation();
  const [confirmStructure, confirmationState] = useConfirmStructureMutation();
  const [setCurrentVersion, currentState] = useSetCurrentScriptVersionMutation();
  const [loadScriptVersionDiff, scriptDiffState] =
    useLazyScriptVersionDiffQuery();
  const [setScriptSourceArchived, scriptSourceState] =
    useSetScriptSourceArchivedMutation();
  const [deleteScriptVersion, scriptDeleteState] =
    useDeleteScriptVersionMutation();
  const [initializeUpload, initializationState] = useInitializeMediaUploadMutation();
  const [completeUpload, completionState] = useCompleteMediaUploadMutation();
  const [retryProbe, retryState] = useRetryMediaProbeMutation();
  const [createShot, createShotState] = useCreateShotMutation();
  const [createShotFromCandidate, createShotFromCandidateState] =
    useCreateShotFromCandidateMutation();
  const [updateShot, updateShotState] = useUpdateShotMutation();
  const [appendShotSpec, appendShotSpecState] = useAppendShotSpecMutation();
  const [reorderShots, reorderShotsState] = useReorderShotsMutation();
  const [copyShot, copyShotState] = useCopyShotMutation();
  const [setShotArchived, shotArchiveState] = useSetShotArchivedMutation();
  const [setCurrentShotSpec, currentShotSpecState] =
    useSetCurrentShotSpecMutation();
  const [splitShotPreflight, splitPreflightState] =
    useSplitShotPreflightMutation();
  const [splitShot, splitShotState] = useSplitShotMutation();
  const [mergeShotsPreflight, mergePreflightState] =
    useMergeShotsPreflightMutation();
  const [mergeShots, mergeShotsState] = useMergeShotsMutation();
  const [shotDeletePreflight, shotDeletePreflightState] =
    useShotDeletePreflightMutation();
  const [deleteShot, deleteShotState] = useDeleteShotMutation();
  const [loadShotSpecVersion, shotSpecLookupState] =
    useLazyShotSpecVersionQuery();
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const busy = [
    importState,
    publishState,
    extractionState,
    decisionState,
    confirmationState,
    currentState,
    scriptDiffState,
    scriptSourceState,
    scriptDeleteState,
    initializationState,
    completionState,
    retryState,
    createShotState,
    createShotFromCandidateState,
    updateShotState,
    appendShotSpecState,
    reorderShotsState,
    copyShotState,
    shotArchiveState,
    currentShotSpecState,
    splitPreflightState,
    splitShotState,
    mergePreflightState,
    mergeShotsState,
    shotDeletePreflightState,
    deleteShotState,
    shotSpecLookupState,
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

  async function handleSetCurrent(
    versionId: string,
  ): Promise<API.CurrentScriptVersionResponse | undefined> {
    if (!episode) return undefined;
    let result: API.CurrentScriptVersionResponse | undefined;
    await runAction(async () => {
      result = await setCurrentVersion({
        episodeId,
        body: {
          version_id: versionId,
          expected_current_version_id: episode.current_script_version_id,
        },
      }).unwrap();
      setStartedBatchId(null);
      const affected = result.impact.affected_shot_ids.length;
      return affected
        ? `剧本版本已切换；${affected} 个镜头仍引用其他版本。`
        : "剧本版本已切换；现有镜头均引用该版本。";
    });
    return result;
  }

  async function handleCompareVersions(
    versionId: string,
    otherVersionId: string,
  ): Promise<API.ScriptVersionDiffResponse | undefined> {
    let result: API.ScriptVersionDiffResponse | undefined;
    await runAction(async () => {
      result = await loadScriptVersionDiff(
        { versionId, otherVersionId },
        true,
      ).unwrap();
      return "剧本版本差异已加载。";
    });
    return result;
  }

  async function handleSetScriptSourceArchived(
    source: API.ScriptSourceResponse,
  ): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      const archived = source.status === "active";
      await setScriptSourceArchived({
        episodeId,
        sourceId: source.id,
        expectedRevision: source.revision,
        archived,
      }).unwrap();
      succeeded = true;
      return archived ? "剧本来源已归档，历史版本仍可读取。" : "剧本来源已恢复。";
    });
    return succeeded;
  }

  async function handleDeleteScriptDraft(
    version: API.ScriptVersionResponse,
  ): Promise<boolean> {
    if (!activeSource) return false;
    let succeeded = false;
    await runAction(async () => {
      await deleteScriptVersion({
        sourceId: activeSource.id,
        versionId: version.id,
      }).unwrap();
      succeeded = true;
      return `剧本草稿 v${version.version_no} 已删除。`;
    });
    return succeeded;
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

  async function handleCreateShot(request: API.ShotCreateRequest): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      const created = await createShot({ episodeId, body: request }).unwrap();
      setSelectedShotId(created.id);
      succeeded = true;
      return `镜头“${created.title}”已加入清单。`;
    });
    return succeeded;
  }

  async function handleCreateShotFromCandidate(
    candidate: API.ExtractionCandidateResponse,
  ): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      const created = await createShotFromCandidate({
        candidateId: candidate.id,
        episodeId,
      }).unwrap();
      setSelectedShotId(created.id);
      succeeded = true;
      return `已确认候选“${created.title}”已加入镜头清单。`;
    });
    return succeeded;
  }

  async function handleUpdateShot(
    shot: API.ShotResponse,
    title: string,
  ): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      const updated = await updateShot({
        episodeId,
        shotId: shot.id,
        body: { expected_revision: shot.revision, title },
      }).unwrap();
      succeeded = true;
      return `镜头标题已更新为“${updated.title}”。`;
    });
    return succeeded;
  }

  async function handleSaveShotSpec(
    shotId: string,
    request: API.ShotSpecCreateRequest,
  ): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      const result = await appendShotSpec({
        episodeId,
        shotId,
        body: request,
      }).unwrap();
      succeeded = true;
      return `镜头规格 v${result.version.version_no} 已保存，准备度将按最新事实刷新。`;
    });
    return succeeded;
  }

  async function handleReorderShots(shotIds: string[]) {
    const order = shotOrderQuery.data;
    if (!order) return;
    if (selectedShot) setSelectedShotId(selectedShot.id);
    await runAction(async () => {
      await reorderShots({
        episodeId,
        body: { shot_ids: shotIds, expected_order_hash: order.order_hash },
      }).unwrap();
      return "镜头顺序已更新。";
    });
  }

  async function handleCopyShot(shot: API.ShotResponse) {
    const order = shotOrderQuery.data;
    const sourceSpecVersionId = shot.current_spec_version_id;
    if (!order || !sourceSpecVersionId) return;
    await runAction(async () => {
      const result = await copyShot({
        episodeId,
        shotId: shot.id,
        body: {
          title: `${shot.title} · 副本`,
          expected_source_spec_version_id: sourceSpecVersionId,
          expected_order_hash: order.order_hash,
          idempotency_key: `studio-copy:${shot.id}:${crypto.randomUUID()}`,
        },
      }).unwrap();
      const copiedId = result.transform.result_shot_ids[0];
      if (copiedId) setSelectedShotId(copiedId);
      return `镜头“${shot.title}”已复制，历史生产证据不会被继承。`;
    });
  }

  async function handleSetCurrentShotSpec(
    shot: API.ShotResponse,
    version: API.ShotSpecVersionResponse,
  ) {
    await runAction(async () => {
      await setCurrentShotSpec({
        episodeId,
        shotId: shot.id,
        body: {
          version_id: version.id,
          expected_current_spec_version_id: shot.current_spec_version_id,
          expected_revision: shot.revision,
        },
      }).unwrap();
      return `镜头“${shot.title}”已切换到规格 v${version.version_no}。`;
    });
  }

  async function handleSplitPreflight(
    shotId: string,
    request: API.SplitPreflightRequest,
  ): Promise<API.ShotTransformPreflightResponse | undefined> {
    let result: API.ShotTransformPreflightResponse | undefined;
    await runAction(async () => {
      result = await splitShotPreflight({ shotId, body: request }).unwrap();
      return "拆分影响已固定，请确认两个目标镜头。";
    });
    return result;
  }

  async function handleSplitShot(
    shotId: string,
    request: API.SplitShotRequest,
  ): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      const result = await splitShot({ episodeId, shotId, body: request }).unwrap();
      const firstResultId = result.transform.result_shot_ids[0];
      if (firstResultId) setSelectedShotId(firstResultId);
      succeeded = true;
      return "镜头已拆分为两个目标，来源镜头及其证据已归档保留。";
    });
    return succeeded;
  }

  async function handleMergePrepare(
    source: API.ShotResponse,
    partner: API.ShotResponse,
  ): Promise<MergePreparation | undefined> {
    const order = shotOrderQuery.data;
    const sourceSpecId = source.current_spec_version_id;
    const partnerSpecId = partner.current_spec_version_id;
    if (!order || !sourceSpecId || !partnerSpecId) return undefined;
    const orderedShots = [source, partner].sort((left, right) =>
      left.position - right.position
    );
    const orderedSpecIds = orderedShots.map((shot) => {
      const specId = shot.current_spec_version_id;
      if (!specId) throw new Error("相邻镜头缺少当前规格");
      return specId;
    });
    let result: MergePreparation | undefined;
    await runAction(async () => {
      const [firstVersion, secondVersion] = await Promise.all([
        loadShotSpecVersion(orderedSpecIds[0], true).unwrap(),
        loadShotSpecVersion(orderedSpecIds[1], true).unwrap(),
      ]);
      const preflight = await mergeShotsPreflight({
        shot_ids: orderedShots.map((shot) => shot.id),
        expected_spec_version_ids: orderedSpecIds,
        expected_order_hash: order.order_hash,
      }).unwrap();
      result = {
        preflight,
        sources: [
          { shot: orderedShots[0], version: firstVersion },
          { shot: orderedShots[1], version: secondVersion },
        ],
      };
      return "合并影响已固定，请确认目标镜头规格。";
    });
    return result;
  }

  async function handleMergeShots(
    request: API.MergeShotRequest,
  ): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      const result = await mergeShots({ episodeId, body: request }).unwrap();
      const resultId = result.transform.result_shot_ids[0];
      if (resultId) setSelectedShotId(resultId);
      succeeded = true;
      return "相邻镜头已合并，两个来源及其证据已归档保留。";
    });
    return succeeded;
  }

  async function handleShotDeletePreflight(
    shotId: string,
  ): Promise<API.ShotDeletePreflightResponse | undefined> {
    let result: API.ShotDeletePreflightResponse | undefined;
    await runAction(async () => {
      result = await shotDeletePreflight(shotId).unwrap();
      return result.allowed
        ? "删除条件已确认。"
        : "镜头已有稳定证据，不能永久删除。";
    });
    return result;
  }

  async function handleDeleteShot(shot: API.ShotResponse): Promise<boolean> {
    const order = shotOrderQuery.data;
    if (!order) return false;
    let succeeded = false;
    await runAction(async () => {
      const result = await deleteShot({
        episodeId,
        shotId: shot.id,
        expectedRevision: shot.revision,
        expectedOrderHash: order.order_hash,
      }).unwrap();
      setSelectedShotId(result.order.items[0]?.id ?? null);
      succeeded = true;
      return `空镜头“${shot.title}”已永久删除。`;
    });
    return succeeded;
  }

  async function handleToggleShotArchived(shot: API.ShotResponse) {
    const order = shotOrderQuery.data;
    if (!order) return;
    const archived = shot.status === "active";
    await runAction(async () => {
      const result = await setShotArchived({
        episodeId,
        shotId: shot.id,
        archived,
        body: {
          expected_revision: shot.revision,
          expected_order_hash: order.order_hash,
        },
      }).unwrap();
      if (archived && selectedShotId === shot.id) {
        setSelectedShotId(result.order.items[0]?.id ?? null);
      }
      if (!archived) setSelectedShotId(result.shot.id);
      return archived ? `镜头“${shot.title}”已归档。` : `镜头“${shot.title}”已恢复到清单末尾。`;
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
  const storyboardError = storyboardActive
    ? shotOrderQuery.error ??
      archivedShotsQuery.error ??
      shotReadinessQuery.error ??
      structureQuery.error ??
      shotSpecVersionsQuery.error
    : undefined;

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
        <div className="pointer-events-none fixed top-24 right-6 z-50 flex max-w-md items-center gap-2 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg" role="status">
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
        ) : pageError || storyboardError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>生产事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError ?? storyboardError)}</AlertDescription>
          </Alert>
        ) : !episode || !project || !snapshot || storyboardLoading ? (
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

            <nav className="mt-7 grid gap-2 rounded-2xl border border-slate-200 bg-white p-2 sm:grid-cols-2 xl:grid-cols-5" aria-label="单集制作模块">
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
                  onCompareVersions={handleCompareVersions}
                  onDecide={handleDecision}
                  onDeleteDraft={handleDeleteScriptDraft}
                  onImport={handleImport}
                  onPublish={handlePublish}
                  onSetCurrent={handleSetCurrent}
                  onSetSourceArchived={handleSetScriptSourceArchived}
                  onStartExtraction={handleStartExtraction}
                />
              ) : initialPanel === "assets" ? (
                <EpisodeAssetOverview assets={assetsQuery.data?.items ?? []} summary={snapshot.asset_summary} />
              ) : initialPanel === "storyboard" ? (
                <StoryboardWorkspace
                  archivedShots={archivedShotsQuery.data ?? []}
                  assets={assetsQuery.data?.items ?? []}
                  busy={busy}
                  confirmedShotCandidates={confirmedShotCandidates}
                  order={shotOrderQuery.data ?? { items: [], order_hash: "" }}
                  readiness={shotReadinessQuery.data}
                  selectedShotId={selectedShot?.id ?? null}
                  structure={structureQuery.data}
                  versions={shotSpecVersionsQuery.currentData ?? []}
                  onCopy={handleCopyShot}
                  onCreate={handleCreateShot}
                  onCreateFromCandidate={handleCreateShotFromCandidate}
                  onDelete={handleDeleteShot}
                  onDeletePreflight={handleShotDeletePreflight}
                  onMerge={handleMergeShots}
                  onMergePrepare={handleMergePrepare}
                  onReorder={handleReorderShots}
                  onSaveSpec={handleSaveShotSpec}
                  onSelectShot={setSelectedShotId}
                  onSetCurrentSpec={handleSetCurrentShotSpec}
                  onSplit={handleSplitShot}
                  onSplitPreflight={handleSplitPreflight}
                  onToggleArchived={handleToggleShotArchived}
                  onUpdate={handleUpdateShot}
                />
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
