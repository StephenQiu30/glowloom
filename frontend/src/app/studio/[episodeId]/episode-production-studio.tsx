"use client";

import {
  AlertCircle,
  CheckCircle2,
  LoaderCircle,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { MetricGroup } from "@/components/studio/metric-group";
import { PageHeader } from "@/components/studio/page-header";
import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useAdaptationRunQuery,
  useApplyStoryboardDraftMutation,
  useApproveStoryboardDraftMutation,
  useAssetBibleQuery,
  useAssetsQuery,
  useAppendShotSpecMutation,
  useArchivedShotsQuery,
  useCompleteMediaUploadMutation,
  useCreateMediaAccessMutation,
  useCancelAdaptationRunMutation,
  useCancelGenerationTaskMutation,
  useConfigureScheduleMutation,
  useCostsQuery,
  useCoverageQuery,
  useConfirmStructureMutation,
  useConfirmedStructureQuery,
  useCopyShotMutation,
  useCreateShotMutation,
  useCreateStoryboardDraftMutation,
  useCreateAdaptationRunMutation,
  useCreateShotFromCandidateMutation,
  useDeleteScriptVersionMutation,
  useDeleteShotMutation,
  useDecideExtractionCandidateMutation,
  useDecideCoverageMutation,
  useDecideStoryboardDraftMutation,
  useEpisodeQuery,
  useEpisodeSnapshotQuery,
  useEpisodesQuery,
  useExtractionBatchQuery,
  useExtractionCandidatesQuery,
  useImportScriptMutation,
  useInitializeMediaUploadMutation,
  useInitializeMediaVersionUploadMutation,
  useLazyShotSpecVersionQuery,
  useLazyAdaptationDiffQuery,
  useLazyScriptVersionDiffQuery,
  useMeQuery,
  useMergeShotsMutation,
  useMergeShotsPreflightMutation,
  useModelCapabilitiesQuery,
  useMediaVersionsQuery,
  useMediaLocationsQuery,
  useNarrativeStructureQuery,
  useProjectQuery,
  usePreflightStoryboardDraftMutation,
  usePreflightStoryboardExportMutation,
  usePublishScriptVersionMutation,
  usePublishAdaptationRunMutation,
  usePauseScheduleMutation,
  useRetryMediaProbeMutation,
  useRequestMediaLocationMigrationMutation,
  useRequestMediaLocationRollbackMutation,
  useRequestStoryboardExportMutation,
  useResumeScheduleMutation,
  useReviseNarrativeStructureMutation,
  useReorderShotsMutation,
  useReplaceNarrativeReferencesMutation,
  useScriptSourcesQuery,
  useScriptVersionQuery,
  useScriptVersionsQuery,
  useSetCurrentScriptVersionMutation,
  useSetCurrentMediaVersionMutation,
  useSetScriptSourceArchivedMutation,
  useSetCurrentShotSpecMutation,
  useSetShotArchivedMutation,
  useSetMediaArchivedMutation,
  useShotDeletePreflightMutation,
  useShotOrderQuery,
  useShotReadinessQuery,
  useShotSpecVersionsQuery,
  useSplitShotMutation,
  useSplitShotPreflightMutation,
  useStartExtractionMutation,
  useStoryboardDraftQuery,
  useStoryboardExportsQuery,
  useSchedulesQuery,
  useTasksQuery,
  useTriggerScheduleMutation,
  useUpdateShotMutation,
  useUpdateAdaptationDraftMutation,
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
import { StoryboardDrafts } from "./storyboard-drafts";
import { StoryboardExports } from "./StoryboardExports";
import { StoryboardCoverage } from "./storyboard-coverage";
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
  const scriptActive = initialPanel === "script";
  const narrativeStructureQuery = useNarrativeStructureQuery(
    episode?.current_script_version_id ?? "",
    {
      skip: !episode?.current_script_version_id || !scriptActive,
    },
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
  const taskCenterActive = initialPanel === "tasks";
  const modelCapabilitiesQuery = useModelCapabilitiesQuery(workspaceId ?? "", {
    skip: !workspaceId || !taskCenterActive,
  });
  const costsQuery = useCostsQuery(
    {
      workspaceId: workspaceId ?? "",
      projectId: episode?.project_id ?? "",
    },
    {
      skip: !workspaceId || !episode?.project_id || !taskCenterActive,
    },
  );
  const schedulesQuery = useSchedulesQuery(workspaceId ?? "", {
    pollingInterval: 10_000,
    skip: !workspaceId || !taskCenterActive,
  });
  const workspaceTasks = tasksQuery.data?.items;
  const episodeTasks = useMemo(
    () =>
      (workspaceTasks ?? []).filter(
        (task) => task.scope.episode_id === episodeId,
      ),
    [episodeId, workspaceTasks],
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
  const [startedAdaptationRunId, setStartedAdaptationRunId] = useState<
    string | null | undefined
  >(undefined);
  const taskAdaptationRunId = episodeTasks.find(
    (task) => task.task_type === "script_adaptation",
  )?.request_id;
  const adaptationRunId =
    startedAdaptationRunId === undefined
      ? taskAdaptationRunId
      : startedAdaptationRunId;
  const adaptationRunQuery = useAdaptationRunQuery(adaptationRunId ?? "", {
    pollingInterval: adaptationRunId ? 3_000 : 0,
    skip: !adaptationRunId,
  });
  const adaptationRun = adaptationRunId ? adaptationRunQuery.data : undefined;
  const assetsQuery = useAssetsQuery(episode?.project_id ?? "", { skip: !episode });
  const assetBibleQuery = useAssetBibleQuery(episode?.project_id ?? "", {
    skip: !episode,
  });
  const mediaQuery = useMediaVersionsQuery(workspaceId ?? "", { skip: !workspaceId });
  const [locationVersionId, setLocationVersionId] = useState<string | null>(null);
  const mediaLocationsQuery = useMediaLocationsQuery(locationVersionId ?? "", {
    pollingInterval: locationVersionId ? 4_000 : 0,
    skip: !locationVersionId,
  });
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
  const coverageQuery = useCoverageQuery(episodeId, {
    pollingInterval: 5_000,
    skip: !authenticated || !storyboardActive,
  });
  const storyboardExportsQuery = useStoryboardExportsQuery(episodeId, {
    pollingInterval: 3_000,
    skip: !authenticated || !storyboardActive,
  });
  const confirmedVersionId =
    snapshot?.script_summary.status === "confirmed"
      ? episode?.current_script_version_id
      : null;
  const structureQuery = useConfirmedStructureQuery(confirmedVersionId ?? "", {
    skip: !confirmedVersionId || !storyboardActive,
  });
  const taskDraftBatchId = episodeTasks.find(
    (task) => task.task_type === "storyboard_draft",
  )?.request_id;
  const [startedDraftBatchId, setStartedDraftBatchId] = useState<
    string | null | undefined
  >(undefined);
  const draftBatchId =
    startedDraftBatchId === undefined ? taskDraftBatchId : startedDraftBatchId;
  const draftBatchQuery = useStoryboardDraftQuery(draftBatchId ?? "", {
    pollingInterval: draftBatchId ? 3_000 : 0,
    skip: !draftBatchId || !storyboardActive,
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
      coverageQuery.isLoading ||
      storyboardExportsQuery.isLoading ||
      structureQuery.isLoading);
  const scriptLoading =
    scriptActive &&
    Boolean(episode?.current_script_version_id) &&
    narrativeStructureQuery.isLoading;

  const [importScript, importState] = useImportScriptMutation();
  const [publishVersion, publishState] = usePublishScriptVersionMutation();
  const [startExtraction, extractionState] = useStartExtractionMutation();
  const [decideCandidate, decisionState] = useDecideExtractionCandidateMutation();
  const [confirmStructure, confirmationState] = useConfirmStructureMutation();
  const [setCurrentVersion, currentState] = useSetCurrentScriptVersionMutation();
  const [reviseNarrativeStructure, narrativeRevisionState] =
    useReviseNarrativeStructureMutation();
  const [loadScriptVersionDiff, scriptDiffState] =
    useLazyScriptVersionDiffQuery();
  const [setScriptSourceArchived, scriptSourceState] =
    useSetScriptSourceArchivedMutation();
  const [deleteScriptVersion, scriptDeleteState] =
    useDeleteScriptVersionMutation();
  const [createAdaptationRun, adaptationCreateState] =
    useCreateAdaptationRunMutation();
  const [updateAdaptationDraft, adaptationDraftState] =
    useUpdateAdaptationDraftMutation();
  const [loadAdaptationDiff, adaptationDiffState] =
    useLazyAdaptationDiffQuery();
  const [publishAdaptationRun, adaptationPublishState] =
    usePublishAdaptationRunMutation();
  const [cancelAdaptationRun, adaptationCancelState] =
    useCancelAdaptationRunMutation();
  const [initializeUpload, initializationState] = useInitializeMediaUploadMutation();
  const [initializeVersionUpload, versionInitializationState] =
    useInitializeMediaVersionUploadMutation();
  const [completeUpload, completionState] = useCompleteMediaUploadMutation();
  const [retryProbe, retryState] = useRetryMediaProbeMutation();
  const [requestLocationMigration, locationMigrationState] =
    useRequestMediaLocationMigrationMutation();
  const [requestLocationRollback, locationRollbackState] =
    useRequestMediaLocationRollbackMutation();
  const [configureSchedule, configureScheduleState] =
    useConfigureScheduleMutation();
  const [pauseSchedule, pauseScheduleState] = usePauseScheduleMutation();
  const [resumeSchedule, resumeScheduleState] = useResumeScheduleMutation();
  const [triggerSchedule, triggerScheduleState] = useTriggerScheduleMutation();
  const [cancelGenerationTask, cancelGenerationTaskState] =
    useCancelGenerationTaskMutation();
  const [setCurrentMediaVersion, mediaCurrentState] =
    useSetCurrentMediaVersionMutation();
  const [setMediaArchived, mediaArchiveState] = useSetMediaArchivedMutation();
  const [createShot, createShotState] = useCreateShotMutation();
  const [createStoryboardDraft, storyboardDraftCreateState] =
    useCreateStoryboardDraftMutation();
  const [decideStoryboardDraft, storyboardDraftDecisionState] =
    useDecideStoryboardDraftMutation();
  const [approveStoryboardDraft, storyboardDraftApproveState] =
    useApproveStoryboardDraftMutation();
  const [preflightStoryboardDraft, storyboardDraftPreflightState] =
    usePreflightStoryboardDraftMutation();
  const [applyStoryboardDraft, storyboardDraftApplyState] =
    useApplyStoryboardDraftMutation();
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
  const [replaceNarrativeReferences, narrativeReferenceState] =
    useReplaceNarrativeReferencesMutation();
  const [decideCoverage, coverageDecisionState] = useDecideCoverageMutation();
  const [preflightStoryboardExport, storyboardExportPreflightState] =
    usePreflightStoryboardExportMutation();
  const [requestStoryboardExport, storyboardExportRequestState] =
    useRequestStoryboardExportMutation();
  const [createMediaAccess, mediaAccessState] = useCreateMediaAccessMutation();
  const [loadShotSpecVersion, shotSpecLookupState] =
    useLazyShotSpecVersionQuery();
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [scriptVersionImpact, setScriptVersionImpact] =
    useState<API.ScriptVersionImpactResponse | null>(null);
  const [adaptationDifference, setAdaptationDifference] =
    useState<API.AdaptationDiffResponse | null>(null);

  const busy = [
    importState,
    publishState,
    extractionState,
    decisionState,
    confirmationState,
    currentState,
    narrativeRevisionState,
    scriptDiffState,
    scriptSourceState,
    scriptDeleteState,
    adaptationCreateState,
    adaptationDraftState,
    adaptationDiffState,
    adaptationPublishState,
    adaptationCancelState,
    initializationState,
    versionInitializationState,
    completionState,
    retryState,
    locationMigrationState,
    locationRollbackState,
    configureScheduleState,
    pauseScheduleState,
    resumeScheduleState,
    triggerScheduleState,
    cancelGenerationTaskState,
    mediaCurrentState,
    mediaArchiveState,
    createShotState,
    storyboardDraftCreateState,
    storyboardDraftDecisionState,
    storyboardDraftApproveState,
    storyboardDraftPreflightState,
    storyboardDraftApplyState,
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
    narrativeReferenceState,
    coverageDecisionState,
    storyboardExportPreflightState,
    storyboardExportRequestState,
    mediaAccessState,
    shotSpecLookupState,
  ].some((state) => state.isLoading);

  async function runAction(action: () => Promise<string>): Promise<boolean> {
    setActionError(null);
    setNotice(null);
    try {
      setNotice(await action());
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function handleImport(request: API.ScriptImportRequest) {
    await runAction(async () => {
      const result = await importScript({ episodeId, body: request }).unwrap();
      return `剧本《${result.source.title}》已导入为 v${result.version.version_no} 草稿。`;
    });
  }

  async function handlePauseSchedule(schedule: API.ScheduleResponse) {
    if (!workspaceId) return;
    await runAction(async () => {
      await pauseSchedule({
        scheduleId: schedule.id,
        workspaceId,
        body: { expected_revision: schedule.revision },
      }).unwrap();
      return "上传过期清理计划已暂停；已创建的清理任务不会被取消。";
    });
  }

  async function handleConfigureSchedule(
    schedule: API.ScheduleResponse,
    configuration: Omit<
      API.ScheduleConfigurationRequest,
      "expected_revision" | "effective_from"
    >,
  ): Promise<boolean> {
    if (!workspaceId) return false;
    return runAction(async () => {
      await configureSchedule({
        scheduleId: schedule.id,
        workspaceId,
        body: {
          ...configuration,
          expected_revision: schedule.revision,
          effective_from: new Date().toISOString(),
        },
      }).unwrap();
      return "补偿清理计划配置已保存；下一触发时刻由服务端时区规则计算。";
    });
  }

  async function handleResumeSchedule(
    schedule: API.ScheduleResponse,
    misfirePolicy: API.ScheduleResumeRequest["misfire_policy"],
    maxCatchUp: number,
  ): Promise<boolean> {
    if (!workspaceId) return false;
    return runAction(async () => {
      await resumeSchedule({
        scheduleId: schedule.id,
        workspaceId,
        body: {
          expected_revision: schedule.revision,
          resume_from: new Date().toISOString(),
          misfire_policy: misfirePolicy,
          max_catch_up: maxCatchUp,
        },
      }).unwrap();
      return `清理计划已恢复，并将按 ${misfirePolicy} 策略处理到期工作。`;
    });
  }

  async function handleTriggerSchedule(schedule: API.ScheduleResponse) {
    if (!workspaceId) return;
    await runAction(async () => {
      const fire = await triggerSchedule({
        scheduleId: schedule.id,
        workspaceId,
        body: {
          expected_revision: schedule.revision,
          idempotency_key: `studio-schedule-trigger:${crypto.randomUUID()}`,
        },
      }).unwrap();
      return `清理任务已创建，任务 ID：${fire.task.id}`;
    });
  }

  async function handleCancelGenerationTask(
    task: API.TaskResponse,
  ): Promise<boolean> {
    if (!workspaceId || !episode) return false;
    return runAction(async () => {
      const result = await cancelGenerationTask({
        taskId: task.id,
        projectId: episode.project_id,
        body: {
          workspace_id: workspaceId,
          expected_revision: task.revision,
          idempotency_key: `studio-generation-cancel:${crypto.randomUUID()}`,
          reason: "user_requested",
        },
      }).unwrap();
      return `生成任务已取消，已释放 ${result.release_cost_entry.currency} ${result.release_cost_entry.amount} 预占。`;
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
      setScriptVersionImpact(result.impact);
      setStartedBatchId(null);
      const affected = result.impact.affected_shot_ids.length;
      return affected
        ? `剧本版本已切换；${affected} 个镜头仍引用其他版本。`
        : "剧本版本已切换；现有镜头均引用该版本。";
    });
    return result;
  }

  async function handleReviseNarrative(
    request: API.NarrativeStructureRevisionRequest,
  ) {
    const structure = narrativeStructureQuery.data;
    if (!structure) return;
    await runAction(async () => {
      const result = await reviseNarrativeStructure({
        episodeId,
        versionId: structure.script_version_id,
        structureId: structure.id,
        body: request,
      }).unwrap();
      return `叙事结构已追加 revision ${result.structure.revision}；分镜准备度、覆盖和导出依赖已失效重算。`;
    });
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

  async function handleCreateAdaptation(
    request: API.AdaptationRunCreateRequest,
  ) {
    await runAction(async () => {
      const result = await createAdaptationRun({
        episodeId,
        body: request,
      }).unwrap();
      setStartedAdaptationRunId(result.id);
      setAdaptationDifference(null);
      return "剧本改写任务已创建；AI 只会生成候选，不会覆盖原稿。";
    });
  }

  async function handleSaveAdaptationDraft(body: string) {
    const run = adaptationRun;
    if (!run) return;
    await runAction(async () => {
      await updateAdaptationDraft({
        runId: run.id,
        body: { body, expected_revision: run.revision },
      }).unwrap();
      setAdaptationDifference(null);
      return "改写工作稿已保存。";
    });
  }

  async function handleCompareAdaptation() {
    const run = adaptationRun;
    if (!run) return;
    await runAction(async () => {
      setAdaptationDifference(
        await loadAdaptationDiff(run.id, true).unwrap(),
      );
      return "已加载原稿与改写工作稿的服务端差异。";
    });
  }

  async function handlePublishAdaptation() {
    const run = adaptationRun;
    const currentVersionId = episode?.current_script_version_id;
    if (!run || !currentVersionId) return;
    await runAction(async () => {
      const result = await publishAdaptationRun({
        episodeId,
        sourceId: run.source_id,
        runId: run.id,
        body: {
          expected_run_revision: run.revision,
          expected_current_version_id: currentVersionId,
          idempotency_key: `studio-adaptation-publish:${run.id}:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setScriptVersionImpact(result.current.impact);
      setStartedBatchId(null);
      return `改写稿已发布为 v${result.version.version_no} 并设为当前版本。`;
    });
  }

  async function handleCancelAdaptation() {
    const run = adaptationRun;
    if (!run) return;
    await runAction(async () => {
      await cancelAdaptationRun({
        runId: run.id,
        body: {
          expected_revision: run.revision,
          idempotency_key: `studio-adaptation-cancel:${run.id}:${crypto.randomUUID()}`,
        },
      }).unwrap();
      return "剧本改写任务已取消；原稿与当前版本未改变。";
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
      const result = await uploadAndComplete(initialized, file, workspaceId);
      succeeded = true;
      return `${result.version.filename} 已上传，媒体探测任务已创建。`;
    });
    return succeeded;
  }

  async function uploadAndComplete(
    initialized: API.UploadInitializationResponse,
    file: File,
    activeWorkspaceId: string,
  ) {
    if (!initialized.upload.url || !initialized.upload.method) {
      throw new Error("对象存储未返回有效的上传地址");
    }
    const uploaded = await fetch(initialized.upload.url, {
      method: initialized.upload.method,
      headers: initialized.upload.headers as HeadersInit,
      body: file,
    });
    if (!uploaded.ok) throw new Error(`对象存储返回 ${uploaded.status}`);
    return completeUpload({
      uploadSessionId: initialized.upload_session.id,
      workspaceId: activeWorkspaceId,
    }).unwrap();
  }

  async function handleAppendMediaVersion(
    current: API.MediaVersionResponse,
    file: File,
  ): Promise<boolean> {
    if (!workspaceId || !current.media_object_current_version_id) return false;
    const expectedCurrentVersionId = current.media_object_current_version_id;
    let succeeded = false;
    await runAction(async () => {
      const sha256 = await sha256File(file);
      const initialized = await initializeVersionUpload({
        mediaObjectId: current.media_object_id,
        body: {
          workspace_id: workspaceId,
          kind: current.media_object_kind,
          filename: file.name,
          size_bytes: file.size,
          mime_type: file.type || "application/octet-stream",
          sha256,
          idempotency_key: `studio-media-version:${current.media_object_id}:${sha256}:${file.name}`,
          expected_current_version_id: expectedCurrentVersionId,
        },
      }).unwrap();
      const result = await uploadAndComplete(initialized, file, workspaceId);
      succeeded = true;
      return `${result.version.filename} 已追加为 v${result.version.version_no} 并设为当前版本。`;
    });
    return succeeded;
  }

  async function handleSetCurrentMediaVersion(
    version: API.MediaVersionResponse,
  ) {
    if (!workspaceId || !version.media_object_current_version_id) return;
    const expectedCurrentVersionId = version.media_object_current_version_id;
    await runAction(async () => {
      await setCurrentMediaVersion({
        mediaObjectId: version.media_object_id,
        workspaceId,
        body: {
          version_id: version.id,
          expected_current_version_id: expectedCurrentVersionId,
          expected_revision: version.media_object_revision,
        },
      }).unwrap();
      return `${version.filename} 已设为当前媒体版本。`;
    });
  }

  async function handleToggleMediaArchived(
    current: API.MediaVersionResponse,
  ) {
    if (!workspaceId) return;
    const archived = current.media_object_status === "active";
    await runAction(async () => {
      await setMediaArchived({
        mediaObjectId: current.media_object_id,
        workspaceId,
        archived,
        body: { expected_revision: current.media_object_revision },
      }).unwrap();
      return archived
        ? `${current.filename} 已归档，历史固定引用仍可读取。`
        : `${current.filename} 已恢复，可继续追加版本和建立新引用。`;
    });
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

  async function handleLocationMigration(
    version: API.MediaVersionResponse,
    activeLocationId: string,
  ) {
    if (!workspaceId) return;
    const locationEpoch =
      mediaLocationsQuery.data?.items.find(
        (location) => location.id !== activeLocationId,
      )?.id ?? "initial";
    await runAction(async () => {
      await requestLocationMigration({
        versionId: version.id,
        workspaceId,
        body: {
          idempotency_key: `studio-location-migrate:${version.id}:${activeLocationId}:${locationEpoch}`,
        },
      }).unwrap();
      return `${version.filename} 的位置迁移任务已创建；校验完成前仍从原位置读取。`;
    });
  }

  async function handleLocationRollback(
    version: API.MediaVersionResponse,
    targetLocationId: string,
    activeLocationId: string,
  ) {
    if (!workspaceId) return;
    await runAction(async () => {
      await requestLocationRollback({
        versionId: version.id,
        workspaceId,
        body: {
          target_location_id: targetLocationId,
          idempotency_key: `studio-location-rollback:${version.id}:${targetLocationId}:${activeLocationId}`,
        },
      }).unwrap();
      return `${version.filename} 的位置回滚任务已创建；旧 active 会重新进入完整保护窗口。`;
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

  async function handleCreateStoryboardDraft(assetStateIds: string[]) {
    if (!confirmedVersionId) return;
    await runAction(async () => {
      const created = await createStoryboardDraft({
        episodeId,
        body: {
          input_script_version_id: confirmedVersionId,
          asset_state_ids: assetStateIds,
          idempotency_key: `studio-storyboard-draft:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setStartedDraftBatchId(created.id);
      return `分镜草案批次已创建，固定了 ${created.input.narrative_unit_version_ids.length} 个叙事单元和 ${created.input.asset_version_ids.length} 个资产版本。`;
    });
  }

  async function handleDecideStoryboardDraft(
    draft: API.DraftShotResponse,
    action: "accepted" | "modified" | "ignored",
    target?: API.DraftTarget,
  ) {
    const batch = draftBatchQuery.data;
    if (!batch) return;
    await runAction(async () => {
      const result = await decideStoryboardDraft({
        batchId: batch.id,
        draftId: draft.id,
        body: {
          action,
          expected_batch_revision: batch.revision,
          idempotency_key: `studio-storyboard-decision:${crypto.randomUUID()}`,
          target: target ?? null,
        },
      }).unwrap();
      await draftBatchQuery.refetch();
      return `镜头 ${String(draft.position).padStart(2, "0")} 已记录为 ${result.draft.decision_history.at(-1)?.action ?? action}。`;
    });
  }

  async function handleApproveStoryboardDraft() {
    const batch = draftBatchQuery.data;
    if (!batch) return;
    await runAction(async () => {
      await approveStoryboardDraft({
        batchId: batch.id,
        body: {
          expected_revision: batch.revision,
          idempotency_key: `studio-storyboard-approve:${crypto.randomUUID()}`,
        },
      }).unwrap();
      await draftBatchQuery.refetch();
      return "分镜草案已整批批准，正式镜头仍未写入。";
    });
  }

  async function handleStoryboardDraftPreflight(): Promise<
    API.DraftApplyPreflightResponse | undefined
  > {
    const batch = draftBatchQuery.data;
    if (!batch) return undefined;
    let preflight: API.DraftApplyPreflightResponse | undefined;
    await runAction(async () => {
      preflight = await preflightStoryboardDraft({
        batchId: batch.id,
        body: { expected_revision: batch.revision },
      }).unwrap();
      return `写入预检通过：保留 ${preflight.diff.kept} 个现有镜头，新建 ${preflight.diff.created} 个镜头。`;
    });
    return preflight;
  }

  async function handleApplyStoryboardDraft(
    preflight: API.DraftApplyPreflightResponse,
  ) {
    const batch = draftBatchQuery.data;
    if (!batch) return;
    await runAction(async () => {
      const result = await applyStoryboardDraft({
        episodeId,
        batchId: batch.id,
        body: {
          expected_revision: preflight.batch_revision,
          expected_order_hash: preflight.order_hash,
          impact_hash: preflight.impact_hash,
          idempotency_key: `studio-storyboard-apply:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setSelectedShotId(result.created_shot_ids[0] ?? selectedShotId);
      await Promise.all([draftBatchQuery.refetch(), shotOrderQuery.refetch()]);
      return `已原子写入 ${result.created_shot_ids.length} 个正式镜头。`;
    });
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
      await Promise.all([
        shotOrderQuery.refetch(),
        shotSpecVersionsQuery.refetch(),
      ]);
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
      await Promise.all([
        shotOrderQuery.refetch(),
        shotSpecVersionsQuery.refetch(),
      ]);
      return `镜头“${shot.title}”已切换到规格 v${version.version_no}。`;
    });
  }

  async function handleReplaceNarrativeReferences(
    shot: API.ShotResponse,
    references: API.NarrativeReferenceInput[],
  ): Promise<boolean> {
    const report = coverageQuery.data;
    const currentSpecVersionId = shot.current_spec_version_id;
    if (!report || !currentSpecVersionId) return false;
    let succeeded = false;
    await runAction(async () => {
      await replaceNarrativeReferences({
        episodeId,
        shotId: shot.id,
        body: {
          expected_shot_revision: shot.revision,
          expected_current_spec_version_id: currentSpecVersionId,
          expected_evaluation_hash: report.evaluation_hash,
          references,
        },
      }).unwrap();
      succeeded = true;
      return `镜头“${shot.title}”的叙事来源已保存为新规格版本。`;
    });
    return succeeded;
  }

  async function handleCoverageDecision(
    request: API.CoverageDecisionRequest,
  ): Promise<boolean> {
    let succeeded = false;
    await runAction(async () => {
      await decideCoverage({ episodeId, body: request }).unwrap();
      succeeded = true;
      return "覆盖决议已追加保存，准备度将按固定版本重新计算。";
    });
    return succeeded;
  }

  async function handleStoryboardExportPreflight() {
    await runAction(async () => {
      const result = await preflightStoryboardExport(episodeId).unwrap();
      if (result.status === "ready") {
        return `导出预检通过，已固定 ${result.shot_spec_version_ids.length} 个镜头规格和 ${result.asset_version_ids.length} 个资产版本。`;
      }
      return `导出预检发现 ${result.blockers.length} 个阻断，请按清单修正后重试。`;
    });
  }

  async function handleStoryboardExport(inputHash: string) {
    await runAction(async () => {
      const result = await requestStoryboardExport({
        episodeId,
        body: {
          expected_input_hash: inputHash,
          idempotency_key: `studio-storyboard-export:${crypto.randomUUID()}`,
        },
      }).unwrap();
      return `可信分镜包任务已创建：${result.id.slice(0, 8)}。`;
    });
  }

  async function handleStoryboardExportDownload(mediaVersionId: string) {
    await runAction(async () => {
      const access = await createMediaAccess({
        mediaVersionId,
        purpose: "download",
      }).unwrap();
      window.open(access.url, "_blank", "noopener,noreferrer");
      return "分镜包临时下载地址已打开。";
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
    if (!order || !sourceSpecId || !partnerSpecId || !coverageQuery.data) {
      return undefined;
    }
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
          {
            shot: orderedShots[0],
            version: firstVersion,
            narrativeReferences:
              coverageQuery.data?.references.filter(
                (reference) =>
                  reference.shot_spec_version_id === firstVersion.id,
              ) ?? [],
            narrativeUnits: coverageQuery.data?.units ?? [],
          },
          {
            shot: orderedShots[1],
            version: secondVersion,
            narrativeReferences:
              coverageQuery.data?.references.filter(
                (reference) =>
                  reference.shot_spec_version_id === secondVersion.id,
              ) ?? [],
            narrativeUnits: coverageQuery.data?.units ?? [],
          },
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
    (scriptActive ? narrativeStructureQuery.error : undefined) ??
    versionsQuery.error ??
    tasksQuery.error ??
    schedulesQuery.error ??
    batchQuery.error ??
    candidatesQuery.error ??
    assetsQuery.error ??
    assetBibleQuery.error ??
    mediaQuery.error;
  const adaptationError = adaptationRunId
    ? adaptationRunQuery.error
    : undefined;
  const storyboardError = storyboardActive
    ? shotOrderQuery.error ??
      archivedShotsQuery.error ??
      shotReadinessQuery.error ??
      coverageQuery.error ??
      storyboardExportsQuery.error ??
      structureQuery.error ??
      draftBatchQuery.error ??
      shotSpecVersionsQuery.error
    : undefined;

  if (sessionState === "checking") {
    return <div className="grid min-h-screen place-items-center"><LoaderCircle className="animate-spin text-foreground" aria-label="正在读取登录状态" /></div>;
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
        ) : pageError || storyboardError || adaptationError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>生产事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError ?? storyboardError ?? adaptationError)}</AlertDescription>
          </Alert>
        ) : !episode || !project || !snapshot || storyboardLoading || scriptLoading ? (
          <div className="grid min-h-96 place-items-center"><LoaderCircle className="animate-spin text-foreground" aria-label="正在加载生产工作台" /></div>
        ) : (
          <>
            <PageHeader
              accessibleTitle={episode.name}
              actions={(
                <div className="flex flex-wrap items-center gap-2">
                  <Button asChild variant="outline"><Link href={`/projects/${episode.project_id}`}>项目概览</Link></Button>
                  <Select value={episode.id} onValueChange={(value) => { window.location.href = `/studio/${value}/${initialPanel}`; }}>
                    <SelectTrigger aria-label="切换单集" className="h-10 min-w-56"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {(episodesQuery.data ?? []).map((item) => (
                        <SelectItem key={item.id} value={item.id}>第 {item.position} 集 · {item.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}
              badges={[
                { label: stageLabels[snapshot.current_stage] },
                { label: project.aspect_ratio },
                { label: project.visual_style ?? "未设视觉风格" },
              ]}
              breadcrumbs={[
                { label: project.name, href: `/projects/${project.id}` },
                { label: `第 ${episode.position} 集 · ${episode.name}` },
              ]}
              description={`${episode.name} · 服务端计算 ${snapshot.completion}% · revision ${episode.revision}`}
              note="AI 候选经人工确认后才进入下游事实。"
              title="今天，把这一集往前推进。"
            />

            <MetricGroup
              className="mt-8"
              items={[
                { label: "当前阶段", value: stageLabels[snapshot.current_stage] },
                { label: "剧本状态", value: `v${editableVersion?.version_no ?? "-"}` },
                { label: "Ready 资产", value: `${snapshot.asset_summary.ready} / ${snapshot.asset_summary.total}` },
                { label: "Ready 分镜", value: `${snapshot.storyboard_summary.ready ?? 0} / ${snapshot.storyboard_summary.total ?? 0}` },
                { label: "进行中任务", value: snapshot.task_summary.running },
              ]}
              label="生产摘要"
            />

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

            <nav className="mt-7 grid border-y sm:grid-cols-2 xl:grid-cols-5 xl:divide-x" aria-label="单集制作模块">
              {episodePanels.map((panel) => (
                <Link
                  aria-current={panel.id === initialPanel ? "page" : undefined}
                  className={`flex items-center gap-3 px-4 py-4 transition ${panel.id === initialPanel ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"}`}
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
                  adaptationDifference={adaptationDifference}
                  adaptationRun={adaptationRun}
                  batch={batchQuery.data}
                  busy={busy}
                  candidates={candidatesQuery.data?.items ?? []}
                  editableVersion={editableVersion}
                  episode={episode}
                  key={editableVersion?.id ?? activeSource?.id ?? "script-import"}
                  narrativeStructure={narrativeStructureQuery.data}
                  snapshot={snapshot}
                  source={activeSource}
                  versionImpact={scriptVersionImpact}
                  versions={versions}
                  onConfirm={handleConfirm}
                  onCancelAdaptation={handleCancelAdaptation}
                  onCompareAdaptation={handleCompareAdaptation}
                  onCompareVersions={handleCompareVersions}
                  onDecide={handleDecision}
                  onDeleteDraft={handleDeleteScriptDraft}
                  onCreateAdaptation={handleCreateAdaptation}
                  onImport={handleImport}
                  onPublish={handlePublish}
                  onPublishAdaptation={handlePublishAdaptation}
                  onResetAdaptation={() => {
                    setStartedAdaptationRunId(null);
                    setAdaptationDifference(null);
                  }}
                  onSaveAdaptationDraft={handleSaveAdaptationDraft}
                  onReviseNarrative={handleReviseNarrative}
                  onDismissVersionImpact={() => setScriptVersionImpact(null)}
                  onSetCurrent={handleSetCurrent}
                  onSetSourceArchived={handleSetScriptSourceArchived}
                  onStartExtraction={handleStartExtraction}
                />
              ) : initialPanel === "assets" ? (
                <EpisodeAssetOverview
                  assetBible={assetBibleQuery.data}
                  assets={assetsQuery.data?.items ?? []}
                  summary={snapshot.asset_summary}
                />
              ) : initialPanel === "storyboard" ? (
                <div className="grid gap-5">
                  <StoryboardExports
                    busy={busy}
                    history={storyboardExportsQuery.data}
                    preflight={storyboardExportPreflightState.data}
                    onDownload={handleStoryboardExportDownload}
                    onExport={handleStoryboardExport}
                    onPreflight={handleStoryboardExportPreflight}
                  />
                  <StoryboardDrafts
                    assetBible={assetBibleQuery.data}
                    batch={draftBatchQuery.data}
                    busy={busy}
                    canCreate={Boolean(confirmedVersionId && structureQuery.data?.scenes.length)}
                    episodeId={episodeId}
                    onApply={handleApplyStoryboardDraft}
                    onApprove={handleApproveStoryboardDraft}
                    onCreate={handleCreateStoryboardDraft}
                    onDecide={handleDecideStoryboardDraft}
                    onPreflight={handleStoryboardDraftPreflight}
                  />
                  {coverageQuery.data ? (
                    <StoryboardCoverage
                      busy={busy}
                      report={coverageQuery.data}
                      selectedShotId={selectedShot?.id ?? null}
                      shots={shotOrderQuery.data?.items ?? []}
                      onDecide={handleCoverageDecision}
                      onReplace={handleReplaceNarrativeReferences}
                      onSelectShot={setSelectedShotId}
                    />
                  ) : null}
                  <StoryboardWorkspace
                    archivedShots={archivedShotsQuery.data ?? []}
                    assetBible={assetBibleQuery.data}
                    busy={busy}
                    confirmedShotCandidates={confirmedShotCandidates}
                    order={shotOrderQuery.data ?? { items: [], order_hash: "" }}
                    coverage={coverageQuery.data}
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
                </div>
              ) : initialPanel === "media" ? (
                <MediaWorkspace
                  busy={busy}
                  locationBusy={
                    mediaLocationsQuery.isFetching ||
                    locationMigrationState.isLoading ||
                    locationRollbackState.isLoading
                  }
                  locationVersionId={locationVersionId}
                  locations={mediaLocationsQuery.data?.items ?? []}
                  media={mediaQuery.data?.items ?? []}
                  onCloseLocations={() => setLocationVersionId(null)}
                  onLocationMigration={handleLocationMigration}
                  onLocationRollback={handleLocationRollback}
                  onOpenLocations={(version) => setLocationVersionId(version.id)}
                  onAppendVersion={handleAppendMediaVersion}
                  onRetry={handleRetry}
                  onSetCurrent={handleSetCurrentMediaVersion}
                  onToggleArchived={handleToggleMediaArchived}
                  onUpload={handleUpload}
                />
              ) : (
                <TaskWorkspace
                  busy={busy}
                  capabilities={modelCapabilitiesQuery.data ?? []}
                  costs={costsQuery.data ?? null}
                  productionFactsLoading={
                    modelCapabilitiesQuery.isLoading || costsQuery.isLoading
                  }
                  productionFactsUnavailable={
                    modelCapabilitiesQuery.isError || costsQuery.isError
                  }
                  schedules={schedulesQuery.data?.items ?? []}
                  tasks={workspaceTasks ?? []}
                  onCancelGenerationTask={handleCancelGenerationTask}
                  onConfigureSchedule={handleConfigureSchedule}
                  onPauseSchedule={handlePauseSchedule}
                  onResumeSchedule={handleResumeSchedule}
                  onTriggerSchedule={handleTriggerSchedule}
                />
              )}
            </section>
          </>
        )}
      </div>
    </StudioShell>
  );
}
