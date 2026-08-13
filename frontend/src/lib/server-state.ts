import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

import {
  appendAssetVersionApiV1AssetStatesStateIdVersionsPost,
  archiveAssetApiV1AssetsAssetIdArchivePost,
  assetDisablePreflightApiV1AssetsAssetIdDisablePreflightPost,
  assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGet,
  assetRenamePreflightApiV1AssetsAssetIdRenamePreflightPost,
  assetStateDisablePreflightApiV1AssetStatesStateIdDisablePreflightPost,
  createAssetStateApiV1AssetsAssetIdStatesPost,
  createAssetApiV1ProjectsProjectIdAssetsPost,
  currentAssetVersionPreflightApiV1AssetStatesStateIdCurrentVersionPreflightPost,
  deleteAssetApiV1AssetsAssetIdDelete,
  disableAssetApiV1AssetsAssetIdDisablePost,
  disableAssetStateApiV1AssetStatesStateIdDisablePost,
  enableAssetApiV1AssetsAssetIdEnablePost,
  enableAssetStateApiV1AssetStatesStateIdEnablePost,
  getAssetBibleApiV1ProjectsProjectIdAssetBibleGet,
  getAssetReadinessApiV1AssetVersionsVersionIdReadinessGet,
  listAssetsApiV1ProjectsProjectIdAssetsGet,
  listAssetVersionsApiV1AssetStatesStateIdVersionsGet,
  renameAssetApiV1AssetsAssetIdRenamePost,
  restoreAssetApiV1AssetsAssetIdRestorePost,
  setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPost,
  updateAssetApiV1AssetsAssetIdPatch,
  updateAssetStateApiV1AssetStatesStateIdPatch,
} from "@/api/assets";
import {
  confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPost,
  createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPost,
  getEpisodePlanApiV1EpisodePlansPlanIdGet,
  materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPost,
  mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePost,
  moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPost,
  publishImportCommitApiV1ImportCommitsCommitIdPublishPost,
  renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePost,
  splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPost,
} from "@/api/episodePlanning";
import {
  createConsentApiV1ConsentsPost,
  getConsentApiV1ConsentsConsentIdGet,
  listAuditEventsApiV1AuditEventsGet,
  listConsentsApiV1ConsentsGet,
  reviseConsentApiV1ConsentsConsentIdRevisionsPost,
  revokeConsentApiV1ConsentsConsentIdRevokePost,
} from "@/api/governance";
import {
    archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePost,
    changePasswordApiV1AuthChangePasswordPost,
    confirmRegistrationVerificationApiV1AuthRegistrationVerificationsConfirmPost,
    createWorkspaceApiV1WorkspacesPost,
  deactivateMeApiV1MeDeactivatePost,
  listWorkspacesApiV1WorkspacesGet,
  loginApiV1AuthLoginPost,
  logoutApiV1AuthLogoutPost,
  meApiV1MeGet,
    registerApiV1AuthRegisterPost,
    requestRegistrationVerificationApiV1AuthRegistrationVerificationsPost,
  restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePost,
  updateMeApiV1MePatch,
  updateWorkspaceApiV1WorkspacesWorkspaceIdPatch,
} from "@/api/identity";
import {
  archiveMediaApiV1MediaObjectsMediaObjectIdArchivePost,
  completeUploadApiV1MediaUploadsUploadSessionIdCompletePost,
  getMediaApiV1MediaVersionIdGet,
  initializeUploadApiV1MediaUploadsPost,
  initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPost,
  listMediaLocationsApiV1MediaVersionIdLocationsGet,
  listMediaApiV1MediaGet,
  requestMediaLocationMigrationApiV1MediaVersionIdLocationMigrationsPost,
  requestMediaLocationRollbackApiV1MediaVersionIdLocationRollbacksPost,
  restoreMediaApiV1MediaObjectsMediaObjectIdRestorePost,
  retryProbeApiV1MediaVersionIdProbeRetryPost,
  setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPost,
} from "@/api/media";
import {
  importDocumentApiV1ProjectsProjectIdScriptImportsPost,
  listDocumentsApiV1ProjectsProjectIdScriptDocumentsGet,
} from "@/api/scriptDocuments";
import {
  archiveEpisodeApiV1EpisodesEpisodeIdArchivePost,
  archiveProjectApiV1ProjectsProjectIdArchivePost,
  createEpisodeApiV1ProjectsProjectIdEpisodesPost,
  createProjectApiV1ProjectsPost,
  deleteEpisodeApiV1EpisodesEpisodeIdDelete,
  deletePreflightApiV1ProjectsProjectIdDeletePreflightPost,
  deleteProjectApiV1ProjectsProjectIdDelete,
  episodeDeletePreflightApiV1EpisodesEpisodeIdDeletePreflightPost,
  episodeProductionSnapshotApiV1EpisodesEpisodeIdProductionSnapshotGet,
  getEpisodeApiV1EpisodesEpisodeIdGet,
  getProjectApiV1ProjectsProjectIdGet,
  listEpisodesApiV1ProjectsProjectIdEpisodesGet,
  listProjectsApiV1ProjectsGet,
  projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet,
  reorderEpisodesApiV1ProjectsProjectIdEpisodesReorderPost,
  restoreEpisodeApiV1EpisodesEpisodeIdRestorePost,
  restoreProjectApiV1ProjectsProjectIdRestorePost,
  updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPost,
  updateEpisodeApiV1EpisodesEpisodeIdPatch,
  updateProjectApiV1ProjectsProjectIdPatch,
} from "@/api/projects";
import {
  getCostsApiV1CostsGet,
  listModelCapabilitiesApiV1ModelCapabilitiesGet,
} from "@/api/production";
import {
  archiveSourceApiV1ScriptSourcesSourceIdArchivePost,
  confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePost,
  decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPost,
  deleteDraftVersionApiV1ScriptVersionsVersionIdDelete,
  diffVersionsApiV1ScriptVersionsVersionIdDiffGet,
  getExtractionBatchApiV1ExtractionBatchesBatchIdGet,
  getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGet,
  getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGet,
  getVersionApiV1ScriptVersionsVersionIdGet,
  importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPost,
  listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGet,
  listSourcesApiV1EpisodesEpisodeIdScriptSourcesGet,
  listVersionsApiV1ScriptSourcesSourceIdVersionsGet,
  publishVersionApiV1ScriptSourcesSourceIdVersionsPost,
  reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPost,
  restoreSourceApiV1ScriptSourcesSourceIdRestorePost,
  setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPost,
  startExtractionApiV1ScriptVersionsVersionIdExtractionsPost,
} from "@/api/scripts";
import {
  configureScheduleApiV1SchedulesScheduleIdConfigurationPut,
  listSchedulesApiV1SchedulesGet,
  pauseScheduleApiV1SchedulesScheduleIdPausePost,
  resumeScheduleApiV1SchedulesScheduleIdResumePost,
  triggerScheduleApiV1SchedulesScheduleIdTriggerPost,
} from "@/api/schedules";
import {
  cancelRunApiV1AdaptationRunsRunIdCancelPost,
  createRunApiV1EpisodesEpisodeIdAdaptationRunsPost,
  diffRunApiV1AdaptationRunsRunIdDiffGet,
  getRunApiV1AdaptationRunsRunIdGet,
  publishRunApiV1AdaptationRunsRunIdPublishPost,
  updateDraftApiV1AdaptationRunsRunIdDraftPatch,
} from "@/api/scriptAdaptations";
import {
  applyBatchApiV1StoryboardDraftBatchesBatchIdApplyPost,
  applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePost,
  appendSpecVersionApiV1ShotsShotIdSpecVersionsPost,
  approveBatchApiV1StoryboardDraftBatchesBatchIdApprovePost,
  archiveShotApiV1ShotsShotIdArchivePost,
  copyShotApiV1ShotsShotIdCopyPost,
  createBatchApiV1EpisodesEpisodeIdStoryboardDraftBatchesPost,
  createManualShotApiV1EpisodesEpisodeIdShotsPost,
  createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPost,
  deleteShotApiV1ShotsShotIdDelete,
  decideDraftApiV1StoryboardDraftsDraftIdDecisionsPost,
  getBatchApiV1StoryboardDraftBatchesBatchIdGet,
  getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGet,
  getSpecVersionApiV1ShotSpecVersionsVersionIdGet,
  listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGet,
  listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGet,
  listShotsApiV1EpisodesEpisodeIdShotsGet,
  listSpecVersionsApiV1ShotsShotIdSpecVersionsGet,
  mergePreflightApiV1ShotsMergePreflightPost,
  mergeShotsApiV1ShotsMergePost,
  preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPost,
  preflightApplyApiV1StoryboardDraftBatchesBatchIdApplyPreflightPost,
  reorderShotsApiV1EpisodesEpisodeIdShotsReorderPost,
  restoreShotApiV1ShotsShotIdRestorePost,
  setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPost,
  shotDeletePreflightApiV1ShotsShotIdDeletePreflightGet,
  splitPreflightApiV1ShotsShotIdSplitPreflightPost,
  splitShotApiV1ShotsShotIdSplitPost,
  updateShotApiV1ShotsShotIdPatch,
} from "@/api/storyboards";
import {
  cancelGenerationTaskApiV1TasksTaskIdCancelPost,
  listTasksApiV1TasksGet,
} from "@/api/tasks";
import { ApiClientError } from "@/lib/request";

export type AppApiError = {
  message: string;
  code: string;
  nextAction?: string;
};

const errorMessages: Record<string, string> = {
  dependency_unavailable: "注册服务暂时不可用，请稍后重试。",
  invalid_verification_code: "验证码不正确，请检查后重新输入。",
  rate_limited: "验证码发送过于频繁，请等待倒计时结束后重试。",
  resource_conflict: "该邮箱已经注册，请直接登录。",
  unauthenticated: "邮箱或密码不正确，请重新输入。",
  verification_expired: "验证码或注册凭证已失效，请重新发送验证码。",
};

export function appApiErrorMessage(error: unknown): string {
  const apiError = error as Partial<AppApiError> | undefined;
  return apiError?.code
    ? (errorMessages[apiError.code] ?? apiError.message ?? "服务暂时不可用，请稍后重试。")
    : (apiError?.message ?? "服务暂时不可用，请稍后重试。");
}

async function runRequest<T>(
  request: () => Promise<{ data: T }>,
): Promise<{ data: T } | { error: AppApiError }> {
  try {
    const response = await request();
    return { data: response.data };
  } catch (error: unknown) {
    if (error instanceof ApiClientError) {
      return {
        error: {
          message: error.message,
          code: error.code,
          nextAction: error.nextAction,
        },
      };
    }
    return {
      error: { message: "服务暂时不可用，请稍后重试。", code: "request_failed" },
    };
  }
}

export const appApi = createApi({
  reducerPath: "appApi",
  baseQuery: fakeBaseQuery<AppApiError>(),
  tagTypes: [
    "Me",
    "Workspaces",
    "Projects",
    "Project",
    "Episodes",
    "Snapshot",
    "Consents",
    "Consent",
    "AuditEvents",
    "Media",
    "MediaLocations",
    "Assets",
    "AssetBible",
    "Asset",
    "AssetStates",
    "AssetVersions",
    "AssetReadiness",
    "AssetShotUsages",
    "ScriptSources",
    "ScriptDocuments",
    "EpisodePlans",
    "ScriptVersions",
    "ScriptVersion",
    "AdaptationRun",
    "Tasks",
    "ModelCapabilities",
    "Costs",
    "Schedules",
    "ExtractionBatch",
    "ExtractionCandidates",
    "ConfirmedStructure",
    "NarrativeStructure",
    "Shots",
    "ArchivedShots",
    "ShotSpecs",
    "ShotReadiness",
    "StoryboardDraft",
  ],
  endpoints: (builder) => ({
    login: builder.mutation<API.AuthResponse, API.LoginRequest>({
      queryFn: (body) => runRequest(() => loginApiV1AuthLoginPost(body)),
    }),
    register: builder.mutation<API.AuthResponse, API.RegisterRequest>({
      queryFn: (body) => runRequest(() => registerApiV1AuthRegisterPost(body)),
    }),
    requestRegistrationVerification: builder.mutation<
      API.RegistrationVerificationAccepted,
      API.RegistrationVerificationRequest
    >({
      queryFn: (body) =>
        runRequest(() =>
          requestRegistrationVerificationApiV1AuthRegistrationVerificationsPost(body),
        ),
    }),
    confirmRegistrationVerification: builder.mutation<
      API.RegistrationVerificationConfirmed,
      API.RegistrationVerificationConfirmRequest
    >({
      queryFn: (body) =>
        runRequest(() =>
          confirmRegistrationVerificationApiV1AuthRegistrationVerificationsConfirmPost(
            body,
          ),
        ),
    }),
    me: builder.query<API.MeResponse, void>({
      queryFn: () => runRequest(() => meApiV1MeGet()),
      providesTags: ["Me"],
    }),
    logout: builder.mutation<API.RevocationResponse, void>({
      queryFn: () => runRequest(() => logoutApiV1AuthLogoutPost()),
    }),
    updateProfile: builder.mutation<API.MeResponse, API.ProfileUpdateRequest>({
      queryFn: (body) => runRequest(() => updateMeApiV1MePatch(body)),
      invalidatesTags: ["Me"],
    }),
    changePassword: builder.mutation<API.RevocationResponse, API.ChangePasswordRequest>({
      queryFn: (body) => runRequest(() => changePasswordApiV1AuthChangePasswordPost(body)),
    }),
    deactivateAccount: builder.mutation<
      API.RevocationResponse,
      API.DeactivateAccountRequest
    >({
      queryFn: (body) => runRequest(() => deactivateMeApiV1MeDeactivatePost(body)),
    }),
    workspaces: builder.query<API.WorkspaceResponse[], void>({
      queryFn: () =>
        runRequest(() => listWorkspacesApiV1WorkspacesGet({ include_archived: true })),
      providesTags: ["Workspaces"],
    }),
    createWorkspace: builder.mutation<API.WorkspaceResponse, API.WorkspaceCreateRequest>({
      queryFn: (body) => runRequest(() => createWorkspaceApiV1WorkspacesPost(body)),
      invalidatesTags: ["Workspaces"],
    }),
    updateWorkspace: builder.mutation<
      API.WorkspaceResponse,
      { workspaceId: string; body: API.WorkspaceUpdateRequest }
    >({
      queryFn: ({ workspaceId, body }) =>
        runRequest(() =>
          updateWorkspaceApiV1WorkspacesWorkspaceIdPatch(
            { workspace_id: workspaceId },
            body,
          ),
        ),
      invalidatesTags: ["Me", "Workspaces"],
    }),
    setWorkspaceArchived: builder.mutation<
      API.WorkspaceResponse,
      { workspaceId: string; expectedRevision: number; archived: boolean }
    >({
      queryFn: ({ workspaceId, expectedRevision, archived }) =>
        runRequest(() => {
          const params = { workspace_id: workspaceId };
          const body = { expected_revision: expectedRevision };
          return archived
            ? archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePost(params, body)
            : restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePost(params, body);
        }),
      invalidatesTags: ["Me", "Workspaces", "Projects"],
    }),
    projects: builder.query<API.PaginatedProjects, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listProjectsApiV1ProjectsGet({
            workspace_id: workspaceId,
            include_archived: true,
            search: null,
            sort: "updated_at",
            order: "desc",
            limit: 50,
            offset: 0,
          }),
        ),
      providesTags: ["Projects"],
    }),
    createProject: builder.mutation<API.ProjectResponse, API.ProjectCreateRequest>({
      queryFn: (body) => runRequest(() => createProjectApiV1ProjectsPost(body)),
      invalidatesTags: ["Projects"],
    }),
    updateProject: builder.mutation<
      API.ProjectResponse,
      { projectId: string; body: API.ProjectUpdateRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          updateProjectApiV1ProjectsProjectIdPatch({ project_id: projectId }, body),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        "Projects",
        { type: "Project", id: projectId },
      ],
    }),
    updateProjectBudget: builder.mutation<
      API.ProjectResponse,
      { projectId: string; body: API.BudgetLimitRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPost(
            { project_id: projectId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        "Projects",
        { type: "Project", id: projectId },
      ],
    }),
    setProjectArchived: builder.mutation<
      API.ProjectResponse,
      { projectId: string; expectedRevision: number; archived: boolean }
    >({
      queryFn: ({ projectId, expectedRevision, archived }) =>
        runRequest(() => {
          const params = { project_id: projectId };
          const body = { expected_revision: expectedRevision };
          return archived
            ? archiveProjectApiV1ProjectsProjectIdArchivePost(params, body)
            : restoreProjectApiV1ProjectsProjectIdRestorePost(params, body);
        }),
      invalidatesTags: (_result, _error, { projectId }) => [
        "Projects",
        { type: "Project", id: projectId },
        { type: "Episodes", id: projectId },
        { type: "Snapshot", id: projectId },
      ],
    }),
    projectDeletePreflight: builder.mutation<API.DeletePreflightResponse, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          deletePreflightApiV1ProjectsProjectIdDeletePreflightPost({
            project_id: projectId,
          }),
        ),
    }),
    deleteProject: builder.mutation<
      API.DeleteResponse,
      { projectId: string; expectedRevision: number }
    >({
      queryFn: ({ projectId, expectedRevision }) =>
        runRequest(() =>
          deleteProjectApiV1ProjectsProjectIdDelete({
            project_id: projectId,
            expected_revision: expectedRevision,
          }),
        ),
      invalidatesTags: ["Projects"],
    }),
    project: builder.query<API.ProjectResponse, string>({
      queryFn: (projectId) =>
        runRequest(() => getProjectApiV1ProjectsProjectIdGet({ project_id: projectId })),
      providesTags: (_result, _error, projectId) => [{ type: "Project", id: projectId }],
    }),
    episodes: builder.query<API.EpisodeResponse[], string>({
      queryFn: (projectId) =>
        runRequest(() =>
          listEpisodesApiV1ProjectsProjectIdEpisodesGet({
            project_id: projectId,
            include_archived: true,
          }),
        ),
      providesTags: (_result, _error, projectId) => [{ type: "Episodes", id: projectId }],
    }),
    createEpisode: builder.mutation<
      API.EpisodeResponse,
      { projectId: string; body: API.EpisodeCreateRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          createEpisodeApiV1ProjectsProjectIdEpisodesPost(
            { project_id: projectId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "Project", id: projectId },
        { type: "Episodes", id: projectId },
        { type: "Snapshot", id: projectId },
      ],
    }),
    updateEpisode: builder.mutation<
      API.EpisodeResponse,
      { projectId: string; episodeId: string; body: API.EpisodeUpdateRequest }
    >({
      queryFn: ({ episodeId, body }) =>
        runRequest(() =>
          updateEpisodeApiV1EpisodesEpisodeIdPatch({ episode_id: episodeId }, body),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "Episodes", id: projectId },
        { type: "Snapshot", id: projectId },
      ],
    }),
    setEpisodeArchived: builder.mutation<
      API.EpisodeResponse,
      { projectId: string; episodeId: string; expectedRevision: number; archived: boolean }
    >({
      queryFn: ({ episodeId, expectedRevision, archived }) =>
        runRequest(() => {
          const params = { episode_id: episodeId };
          const body = { expected_revision: expectedRevision };
          return archived
            ? archiveEpisodeApiV1EpisodesEpisodeIdArchivePost(params, body)
            : restoreEpisodeApiV1EpisodesEpisodeIdRestorePost(params, body);
        }),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "Project", id: projectId },
        { type: "Episodes", id: projectId },
        { type: "Snapshot", id: projectId },
      ],
    }),
    reorderEpisodes: builder.mutation<
      API.EpisodeOrderResponse,
      { projectId: string; body: API.EpisodeReorderRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          reorderEpisodesApiV1ProjectsProjectIdEpisodesReorderPost(
            { project_id: projectId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "Project", id: projectId },
        { type: "Episodes", id: projectId },
        { type: "Snapshot", id: projectId },
      ],
    }),
    episodeDeletePreflight: builder.mutation<API.DeletePreflightResponse, string>({
      queryFn: (episodeId) =>
        runRequest(() =>
          episodeDeletePreflightApiV1EpisodesEpisodeIdDeletePreflightPost({
            episode_id: episodeId,
          }),
        ),
    }),
    deleteEpisode: builder.mutation<
      API.DeleteResponse,
      { projectId: string; episodeId: string; expectedRevision: number }
    >({
      queryFn: ({ episodeId, expectedRevision }) =>
        runRequest(() =>
          deleteEpisodeApiV1EpisodesEpisodeIdDelete({
            episode_id: episodeId,
            expected_revision: expectedRevision,
          }),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "Project", id: projectId },
        { type: "Episodes", id: projectId },
        { type: "Snapshot", id: projectId },
      ],
    }),
    projectSnapshot: builder.query<API.ProjectProductionSnapshot, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet({
            project_id: projectId,
          }),
        ),
      providesTags: (_result, _error, projectId) => [{ type: "Snapshot", id: projectId }],
    }),
    episode: builder.query<API.EpisodeResponse, string>({
      queryFn: (episodeId) =>
        runRequest(() =>
          getEpisodeApiV1EpisodesEpisodeIdGet({ episode_id: episodeId }),
        ),
      providesTags: (_result, _error, episodeId) => [
        { type: "Episodes", id: episodeId },
      ],
    }),
    episodeSnapshot: builder.query<API.EpisodeProductionSnapshot, string>({
      queryFn: (episodeId) =>
        runRequest(() =>
          episodeProductionSnapshotApiV1EpisodesEpisodeIdProductionSnapshotGet({
            episode_id: episodeId,
          }),
        ),
      providesTags: (_result, _error, episodeId) => [
        { type: "Snapshot", id: episodeId },
      ],
    }),
    scriptSources: builder.query<API.PaginatedScriptSources, string>({
      queryFn: (episodeId) =>
        runRequest(() =>
          listSourcesApiV1EpisodesEpisodeIdScriptSourcesGet({
            episode_id: episodeId,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, episodeId) => [
        { type: "ScriptSources", id: episodeId },
      ],
    }),
    scriptDocuments: builder.query<API.PaginatedScriptDocuments, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          listDocumentsApiV1ProjectsProjectIdScriptDocumentsGet({
            project_id: projectId,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "ScriptDocuments", id: projectId },
      ],
    }),
    importScriptDocument: builder.mutation<
      API.ScriptDocumentAnalysisResponse,
      { projectId: string; body: API.ScriptDocumentImportRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          importDocumentApiV1ProjectsProjectIdScriptImportsPost(
            { project_id: projectId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "ScriptDocuments", id: projectId },
        "AuditEvents",
      ],
    }),
    episodePlan: builder.query<API.EpisodePlanDetailResponse, string>({
      queryFn: (planId) =>
        runRequest(() =>
          getEpisodePlanApiV1EpisodePlansPlanIdGet({ plan_id: planId }),
        ),
      providesTags: (_result, _error, planId) => [
        { type: "EpisodePlans", id: planId },
      ],
    }),
    createEpisodePlan: builder.mutation<
      API.EpisodePlanDetailResponse,
      { revisionId: string; body: API.EpisodePlanCreateRequest }
    >({
      queryFn: ({ revisionId, body }) =>
        runRequest(() =>
          createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPost(
            { revision_id: revisionId },
            body,
          ),
        ),
      invalidatesTags: ["EpisodePlans", "Tasks", "AuditEvents"],
    }),
    renameEpisodeProposal: builder.mutation<
      API.EpisodePlanDetailResponse,
      { planId: string; body: API.RenameEpisodeProposalRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { planId }) => [
        { type: "EpisodePlans", id: planId },
      ],
    }),
    moveEpisodeBoundary: builder.mutation<
      API.EpisodePlanDetailResponse,
      { planId: string; body: API.MoveEpisodeBoundaryRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { planId }) => [
        { type: "EpisodePlans", id: planId },
      ],
    }),
    splitEpisodeProposal: builder.mutation<
      API.EpisodePlanDetailResponse,
      { planId: string; body: API.SplitEpisodeProposalRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { planId }) => [
        { type: "EpisodePlans", id: planId },
      ],
    }),
    mergeEpisodeProposals: builder.mutation<
      API.EpisodePlanDetailResponse,
      { planId: string; body: API.MergeEpisodeProposalRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { planId }) => [
        { type: "EpisodePlans", id: planId },
      ],
    }),
    confirmEpisodePlan: builder.mutation<
      API.EpisodePlanDetailResponse,
      { planId: string; body: API.ConfirmEpisodePlanRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { planId }) => [
        { type: "EpisodePlans", id: planId },
        "AuditEvents",
      ],
    }),
    materializeEpisodePlan: builder.mutation<
      API.ImportCommitDetailResponse,
      { planId: string; body: API.MaterializeEpisodePlanRequest }
    >({
      queryFn: ({ planId, body }) =>
        runRequest(() =>
          materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPost(
            { plan_id: planId },
            body,
          ),
        ),
      invalidatesTags: ["EpisodePlans", "Episodes", "Project", "Snapshot", "AuditEvents"],
    }),
    publishImportCommit: builder.mutation<
      API.ImportCommitDetailResponse,
      { commitId: string; body: API.PublishImportCommitRequest }
    >({
      queryFn: ({ commitId, body }) =>
        runRequest(() =>
          publishImportCommitApiV1ImportCommitsCommitIdPublishPost(
            { commit_id: commitId },
            body,
          ),
        ),
      invalidatesTags: [
        "EpisodePlans",
        "Episodes",
        "Project",
        "Snapshot",
        "ScriptSources",
        "ScriptVersions",
        "AuditEvents",
      ],
    }),
    mediaVersion: builder.query<API.MediaVersionResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getMediaApiV1MediaVersionIdGet({ version_id: versionId }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "Media", id: versionId },
      ],
    }),
    scriptVersion: builder.query<API.ScriptVersionResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getVersionApiV1ScriptVersionsVersionIdGet({ version_id: versionId }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "ScriptVersion", id: versionId },
      ],
    }),
    adaptationRun: builder.query<API.AdaptationRunResponse, string>({
      queryFn: (runId) =>
        runRequest(() =>
          getRunApiV1AdaptationRunsRunIdGet({ run_id: runId }),
        ),
      providesTags: (_result, _error, runId) => [
        { type: "AdaptationRun", id: runId },
      ],
    }),
    confirmedStructure: builder.query<API.ConfirmedStructureResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGet({
            version_id: versionId,
          }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "ConfirmedStructure", id: versionId },
      ],
    }),
    narrativeStructure: builder.query<API.NarrativeStructureResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGet({
            version_id: versionId,
            revision: null,
          }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "NarrativeStructure", id: versionId },
      ],
    }),
    scriptVersions: builder.query<API.PaginatedScriptVersions, string>({
      queryFn: (sourceId) =>
        runRequest(() =>
          listVersionsApiV1ScriptSourcesSourceIdVersionsGet({
            source_id: sourceId,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, sourceId) => [
        { type: "ScriptVersions", id: sourceId },
      ],
    }),
    tasks: builder.query<API.PaginatedTasks, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listTasksApiV1TasksGet({
            workspace_id: workspaceId,
            task_type: null,
            status: null,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, workspaceId) => [
        { type: "Tasks", id: workspaceId },
      ],
    }),
    cancelGenerationTask: builder.mutation<
      API.GenerationTaskCancellationResponse,
      {
        taskId: string;
        projectId: string;
        body: API.GenerationTaskCancellationRequest;
      }
    >({
      queryFn: ({ taskId, body }) =>
        runRequest(() =>
          cancelGenerationTaskApiV1TasksTaskIdCancelPost(
            { task_id: taskId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, body }) => [
        { type: "Tasks", id: body.workspace_id },
        { type: "Costs", id: projectId },
        { type: "AuditEvents", id: body.workspace_id },
      ],
    }),
    modelCapabilities: builder.query<API.ModelCapabilityResponse[], string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listModelCapabilitiesApiV1ModelCapabilitiesGet({
            workspace_id: workspaceId,
            kind: null,
            model: null,
          }),
        ),
      providesTags: (_result, _error, workspaceId) => [
        { type: "ModelCapabilities", id: workspaceId },
      ],
    }),
    costs: builder.query<
      API.CostQueryResponse,
      { workspaceId: string; projectId: string }
    >({
      queryFn: ({ workspaceId, projectId }) =>
        runRequest(() =>
          getCostsApiV1CostsGet({
            workspace_id: workspaceId,
            project_id: projectId,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, { projectId }) => [
        { type: "Costs", id: projectId },
      ],
    }),
    schedules: builder.query<API.PaginatedSchedules, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listSchedulesApiV1SchedulesGet({
            workspace_id: workspaceId,
            status: null,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, workspaceId) => [
        { type: "Schedules", id: workspaceId },
      ],
    }),
    configureSchedule: builder.mutation<
      API.ScheduleResponse,
      {
        scheduleId: string;
        workspaceId: string;
        body: API.ScheduleConfigurationRequest;
      }
    >({
      queryFn: ({ scheduleId, body }) =>
        runRequest(() =>
          configureScheduleApiV1SchedulesScheduleIdConfigurationPut(
            { schedule_id: scheduleId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Schedules", id: workspaceId },
        { type: "AuditEvents", id: workspaceId },
      ],
    }),
    pauseSchedule: builder.mutation<
      API.ScheduleResponse,
      { scheduleId: string; workspaceId: string; body: API.ScheduleStateRequest }
    >({
      queryFn: ({ scheduleId, body }) =>
        runRequest(() =>
          pauseScheduleApiV1SchedulesScheduleIdPausePost(
            { schedule_id: scheduleId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Schedules", id: workspaceId },
      ],
    }),
    resumeSchedule: builder.mutation<
      API.ScheduleResponse,
      { scheduleId: string; workspaceId: string; body: API.ScheduleResumeRequest }
    >({
      queryFn: ({ scheduleId, body }) =>
        runRequest(() =>
          resumeScheduleApiV1SchedulesScheduleIdResumePost(
            { schedule_id: scheduleId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Schedules", id: workspaceId },
        { type: "Tasks", id: workspaceId },
      ],
    }),
    triggerSchedule: builder.mutation<
      API.ScheduleFireResponse,
      { scheduleId: string; workspaceId: string; body: API.ScheduleTriggerRequest }
    >({
      queryFn: ({ scheduleId, body }) =>
        runRequest(() =>
          triggerScheduleApiV1SchedulesScheduleIdTriggerPost(
            { schedule_id: scheduleId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Schedules", id: workspaceId },
        { type: "Tasks", id: workspaceId },
      ],
    }),
    extractionBatch: builder.query<API.ExtractionBatchResponse, string>({
      queryFn: (batchId) =>
        runRequest(() =>
          getExtractionBatchApiV1ExtractionBatchesBatchIdGet({ batch_id: batchId }),
        ),
      providesTags: (_result, _error, batchId) => [
        { type: "ExtractionBatch", id: batchId },
      ],
    }),
    extractionCandidates: builder.query<API.PaginatedExtractionCandidates, string>({
      queryFn: (batchId) =>
        runRequest(() =>
          listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGet({
            batch_id: batchId,
            kind: null,
            status: null,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, batchId) => [
        { type: "ExtractionCandidates", id: batchId },
      ],
    }),
    shotOrder: builder.query<API.ShotOrderResponse, string>({
      queryFn: (episodeId) =>
        runRequest(() =>
          listShotsApiV1EpisodesEpisodeIdShotsGet({ episode_id: episodeId }),
        ),
      providesTags: (_result, _error, episodeId) => [
        { type: "Shots", id: episodeId },
      ],
    }),
    storyboardDraft: builder.query<API.DraftBatchResponse, string>({
      queryFn: (batchId) =>
        runRequest(() =>
          getBatchApiV1StoryboardDraftBatchesBatchIdGet({ batch_id: batchId }),
        ),
      providesTags: (_result, _error, batchId) => [
        { type: "StoryboardDraft", id: batchId },
      ],
    }),
    createStoryboardDraft: builder.mutation<
      API.DraftBatchResponse,
      { episodeId: string; body: API.DraftBatchCreateRequest }
    >({
      queryFn: ({ episodeId, body }) =>
        runRequest(() =>
          createBatchApiV1EpisodesEpisodeIdStoryboardDraftBatchesPost(
            { episode_id: episodeId },
            body,
          ),
        ),
      invalidatesTags: ["Tasks"],
    }),
    decideStoryboardDraft: builder.mutation<
      API.DraftDecisionResult,
      { batchId: string; draftId: string; body: API.DraftDecisionRequest }
    >({
      queryFn: ({ draftId, body }) =>
        runRequest(() =>
          decideDraftApiV1StoryboardDraftsDraftIdDecisionsPost(
            { draft_id: draftId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { batchId }) => [
        { type: "StoryboardDraft", id: batchId },
      ],
    }),
    approveStoryboardDraft: builder.mutation<
      API.DraftBatchResponse,
      { batchId: string; body: API.DraftApproveRequest }
    >({
      queryFn: ({ batchId, body }) =>
        runRequest(() =>
          approveBatchApiV1StoryboardDraftBatchesBatchIdApprovePost(
            { batch_id: batchId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { batchId }) => [
        { type: "StoryboardDraft", id: batchId },
      ],
    }),
    preflightStoryboardDraft: builder.mutation<
      API.DraftApplyPreflightResponse,
      { batchId: string; body: API.DraftApplyPreflightRequest }
    >({
      queryFn: ({ batchId, body }) =>
        runRequest(() =>
          preflightApplyApiV1StoryboardDraftBatchesBatchIdApplyPreflightPost(
            { batch_id: batchId },
            body,
          ),
        ),
    }),
    applyStoryboardDraft: builder.mutation<
      API.DraftApplyResponse,
      { episodeId: string; batchId: string; body: API.DraftApplyRequest }
    >({
      queryFn: ({ batchId, body }) =>
        runRequest(() =>
          applyBatchApiV1StoryboardDraftBatchesBatchIdApplyPost(
            { batch_id: batchId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId, batchId }) => [
        { type: "StoryboardDraft", id: batchId },
        { type: "Shots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    archivedShots: builder.query<API.ShotResponse[], string>({
      queryFn: (episodeId) =>
        runRequest(() =>
          listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGet({
            episode_id: episodeId,
          }),
        ),
      providesTags: (_result, _error, episodeId) => [
        { type: "ArchivedShots", id: episodeId },
      ],
    }),
    assetShotUsages: builder.query<
      API.PaginatedAssetShotUsages,
      { assetVersionId: string; limit: number; offset: number }
    >({
      queryFn: ({ assetVersionId, limit, offset }) =>
        runRequest(() =>
          listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGet({
            asset_version_id: assetVersionId,
            limit,
            offset,
          }),
        ),
      providesTags: (_result, _error, { assetVersionId }) => [
        { type: "AssetShotUsages", id: assetVersionId },
      ],
    }),
    assetUpgradePreflight: builder.mutation<
      API.AssetUpgradePreflightResponse,
      { assetVersionId: string; body: API.AssetUpgradePreflightRequest }
    >({
      queryFn: ({ assetVersionId, body }) =>
        runRequest(() =>
          preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPost(
            { asset_version_id: assetVersionId },
            body,
          ),
        ),
    }),
    applyAssetUpgrade: builder.mutation<
      API.AssetUpgradeApplyResponse,
      { assetVersionId: string; body: API.AssetUpgradeApplyRequest }
    >({
      queryFn: ({ assetVersionId, body }) =>
        runRequest(() =>
          applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePost(
            { asset_version_id: assetVersionId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { assetVersionId, body }) => [
        { type: "AssetShotUsages", id: assetVersionId },
        { type: "AssetShotUsages", id: body.new_asset_version_id },
        ...(result?.shots.flatMap((shot) => [
          { type: "Shots" as const, id: shot.episode_id },
          { type: "ShotSpecs" as const, id: shot.id },
          { type: "ShotReadiness" as const, id: shot.episode_id },
          { type: "Snapshot" as const, id: shot.episode_id },
        ]) ?? []),
      ],
    }),
    shotReadiness: builder.query<API.ShotReadinessBatchResponse, string>({
      queryFn: (episodeId) =>
        runRequest(() =>
          getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGet({
            episode_id: episodeId,
          }),
        ),
      providesTags: (_result, _error, episodeId) => [
        { type: "ShotReadiness", id: episodeId },
      ],
    }),
    shotSpecVersions: builder.query<API.ShotSpecVersionResponse[], string>({
      queryFn: (shotId) =>
        runRequest(() =>
          listSpecVersionsApiV1ShotsShotIdSpecVersionsGet({ shot_id: shotId }),
        ),
      providesTags: (_result, _error, shotId) => [
        { type: "ShotSpecs", id: shotId },
      ],
    }),
    shotSpecVersion: builder.query<API.ShotSpecVersionResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getSpecVersionApiV1ShotSpecVersionsVersionIdGet({
            version_id: versionId,
          }),
        ),
      providesTags: (result, _error, versionId) => [
        { type: "ShotSpecs", id: result?.shot_id ?? versionId },
      ],
    }),
    createShot: builder.mutation<
      API.ShotResponse,
      { episodeId: string; body: API.ShotCreateRequest }
    >({
      queryFn: ({ episodeId, body }) =>
        runRequest(() =>
          createManualShotApiV1EpisodesEpisodeIdShotsPost(
            { episode_id: episodeId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    createShotFromCandidate: builder.mutation<
      API.ShotResponse,
      { candidateId: string; episodeId: string }
    >({
      queryFn: ({ candidateId }) =>
        runRequest(() =>
          createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPost({
            candidate_id: candidateId,
          }),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    updateShot: builder.mutation<
      API.ShotResponse,
      { episodeId: string; shotId: string; body: API.ShotUpdateRequest }
    >({
      queryFn: ({ shotId, body }) =>
        runRequest(() =>
          updateShotApiV1ShotsShotIdPatch({ shot_id: shotId }, body),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
      ],
    }),
    appendShotSpec: builder.mutation<
      API.ShotSpecCreateResponse,
      { episodeId: string; shotId: string; body: API.ShotSpecCreateRequest }
    >({
      queryFn: ({ shotId, body }) =>
        runRequest(() =>
          appendSpecVersionApiV1ShotsShotIdSpecVersionsPost(
            { shot_id: shotId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId, shotId }) => [
        { type: "Shots", id: episodeId },
        { type: "ShotSpecs", id: shotId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    setCurrentShotSpec: builder.mutation<
      API.ShotResponse,
      {
        episodeId: string;
        shotId: string;
        body: API.ShotCurrentSpecRequest;
      }
    >({
      queryFn: ({ shotId, body }) =>
        runRequest(() =>
          setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPost(
            { shot_id: shotId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId, shotId }) => [
        { type: "Shots", id: episodeId },
        { type: "ShotSpecs", id: shotId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    reorderShots: builder.mutation<
      API.ShotOrderResponse,
      { episodeId: string; body: API.ShotReorderRequest }
    >({
      queryFn: ({ episodeId, body }) =>
        runRequest(() =>
          reorderShotsApiV1EpisodesEpisodeIdShotsReorderPost(
            { episode_id: episodeId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
      ],
    }),
    copyShot: builder.mutation<
      API.ShotTransformResponse,
      { episodeId: string; shotId: string; body: API.CopyShotRequest }
    >({
      queryFn: ({ shotId, body }) =>
        runRequest(() =>
          copyShotApiV1ShotsShotIdCopyPost({ shot_id: shotId }, body),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    splitShotPreflight: builder.mutation<
      API.ShotTransformPreflightResponse,
      { shotId: string; body: API.SplitPreflightRequest }
    >({
      queryFn: ({ shotId, body }) =>
        runRequest(() =>
          splitPreflightApiV1ShotsShotIdSplitPreflightPost(
            { shot_id: shotId },
            body,
          ),
        ),
    }),
    splitShot: builder.mutation<
      API.ShotTransformResponse,
      { episodeId: string; shotId: string; body: API.SplitShotRequest }
    >({
      queryFn: ({ shotId, body }) =>
        runRequest(() =>
          splitShotApiV1ShotsShotIdSplitPost({ shot_id: shotId }, body),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
        { type: "ArchivedShots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    mergeShotsPreflight: builder.mutation<
      API.ShotTransformPreflightResponse,
      API.MergePreflightRequest
    >({
      queryFn: (body) =>
        runRequest(() => mergePreflightApiV1ShotsMergePreflightPost(body)),
    }),
    mergeShots: builder.mutation<
      API.ShotTransformResponse,
      { episodeId: string; body: API.MergeShotRequest }
    >({
      queryFn: ({ body }) =>
        runRequest(() => mergeShotsApiV1ShotsMergePost(body)),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
        { type: "ArchivedShots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    shotDeletePreflight: builder.mutation<
      API.ShotDeletePreflightResponse,
      string
    >({
      queryFn: (shotId) =>
        runRequest(() =>
          shotDeletePreflightApiV1ShotsShotIdDeletePreflightGet({
            shot_id: shotId,
          }),
        ),
    }),
    deleteShot: builder.mutation<
      API.ShotDeleteResponse,
      {
        episodeId: string;
        shotId: string;
        expectedRevision: number;
        expectedOrderHash: string;
      }
    >({
      queryFn: ({ shotId, expectedRevision, expectedOrderHash }) =>
        runRequest(() =>
          deleteShotApiV1ShotsShotIdDelete({
            shot_id: shotId,
            expected_revision: expectedRevision,
            expected_order_hash: expectedOrderHash,
          }),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    setShotArchived: builder.mutation<
      API.ShotStateResponse,
      {
        episodeId: string;
        shotId: string;
        archived: boolean;
        body: API.ShotStateRequest;
      }
    >({
      queryFn: ({ shotId, archived, body }) =>
        runRequest(() =>
          archived
            ? archiveShotApiV1ShotsShotIdArchivePost({ shot_id: shotId }, body)
            : restoreShotApiV1ShotsShotIdRestorePost({ shot_id: shotId }, body),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Shots", id: episodeId },
        { type: "ArchivedShots", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    importScript: builder.mutation<
      API.ScriptImportResponse,
      { episodeId: string; body: API.ScriptImportRequest }
    >({
      queryFn: ({ episodeId, body }) =>
        runRequest(() =>
          importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPost(
            { episode_id: episodeId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "ScriptSources", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    publishScriptVersion: builder.mutation<
      API.ScriptVersionPublishResponse,
      { episodeId: string; sourceId: string; body: API.ScriptVersionPublishRequest }
    >({
      queryFn: ({ sourceId, body }) =>
        runRequest(() =>
          publishVersionApiV1ScriptSourcesSourceIdVersionsPost(
            { source_id: sourceId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { episodeId, sourceId }) => [
        { type: "ScriptVersions", id: sourceId },
        { type: "Episodes", id: episodeId },
        { type: "Snapshot", id: episodeId },
        ...(result
          ? [{ type: "ScriptVersion" as const, id: result.version.id }]
          : []),
      ],
    }),
    reviseNarrativeStructure: builder.mutation<
      API.NarrativeRevisionResponse,
      {
        episodeId: string;
        versionId: string;
        structureId: string;
        body: API.NarrativeStructureRevisionRequest;
      }
    >({
      queryFn: ({ structureId, body }) =>
        runRequest(() =>
          reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPost(
            { structure_id: structureId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId, versionId }) => [
        { type: "NarrativeStructure", id: versionId },
        { type: "Snapshot", id: episodeId },
        { type: "ShotReadiness", id: episodeId },
        "AuditEvents",
      ],
    }),
    createAdaptationRun: builder.mutation<
      API.AdaptationRunResponse,
      { episodeId: string; body: API.AdaptationRunCreateRequest }
    >({
      queryFn: ({ episodeId, body }) =>
        runRequest(() =>
          createRunApiV1EpisodesEpisodeIdAdaptationRunsPost(
            { episode_id: episodeId },
            body,
          ),
        ),
      invalidatesTags: ["Tasks", "AuditEvents"],
    }),
    updateAdaptationDraft: builder.mutation<
      API.AdaptationRunResponse,
      { runId: string; body: API.AdaptationDraftUpdateRequest }
    >({
      queryFn: ({ runId, body }) =>
        runRequest(() =>
          updateDraftApiV1AdaptationRunsRunIdDraftPatch(
            { run_id: runId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { runId }) => [
        { type: "AdaptationRun", id: runId },
      ],
    }),
    adaptationDiff: builder.query<API.AdaptationDiffResponse, string>({
      queryFn: (runId) =>
        runRequest(() =>
          diffRunApiV1AdaptationRunsRunIdDiffGet({ run_id: runId }),
        ),
    }),
    publishAdaptationRun: builder.mutation<
      API.AdaptationPublishResponse,
      {
        episodeId: string;
        sourceId: string;
        runId: string;
        body: API.AdaptationPublishRequest;
      }
    >({
      queryFn: ({ runId, body }) =>
        runRequest(() =>
          publishRunApiV1AdaptationRunsRunIdPublishPost(
            { run_id: runId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { episodeId, sourceId, runId }) => [
        { type: "AdaptationRun", id: runId },
        { type: "ScriptVersions", id: sourceId },
        { type: "Episodes", id: episodeId },
        { type: "Snapshot", id: episodeId },
        "AuditEvents",
        ...(result
          ? [{ type: "ScriptVersion" as const, id: result.version.id }]
          : []),
      ],
    }),
    cancelAdaptationRun: builder.mutation<
      API.AdaptationRunResponse,
      { runId: string; body: API.AdaptationCancelRequest }
    >({
      queryFn: ({ runId, body }) =>
        runRequest(() =>
          cancelRunApiV1AdaptationRunsRunIdCancelPost(
            { run_id: runId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { runId }) => [
        { type: "AdaptationRun", id: runId },
        "Tasks",
        "AuditEvents",
      ],
    }),
    setCurrentScriptVersion: builder.mutation<
      API.CurrentScriptVersionResponse,
      { episodeId: string; body: API.CurrentScriptVersionRequest }
    >({
      queryFn: ({ episodeId, body }) =>
        runRequest(() =>
          setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPost(
            { episode_id: episodeId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "Episodes", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    scriptVersionDiff: builder.query<
      API.ScriptVersionDiffResponse,
      { versionId: string; otherVersionId: string }
    >({
      queryFn: ({ versionId, otherVersionId }) =>
        runRequest(() =>
          diffVersionsApiV1ScriptVersionsVersionIdDiffGet({
            version_id: versionId,
            other_version_id: otherVersionId,
          }),
        ),
    }),
    setScriptSourceArchived: builder.mutation<
      API.ScriptSourceResponse,
      {
        episodeId: string;
        sourceId: string;
        expectedRevision: number;
        archived: boolean;
      }
    >({
      queryFn: ({ sourceId, expectedRevision, archived }) => {
        const params = { source_id: sourceId };
        const body = { expected_revision: expectedRevision };
        return runRequest(() =>
          archived
            ? archiveSourceApiV1ScriptSourcesSourceIdArchivePost(params, body)
            : restoreSourceApiV1ScriptSourcesSourceIdRestorePost(params, body),
        );
      },
      invalidatesTags: (_result, _error, { episodeId }) => [
        { type: "ScriptSources", id: episodeId },
        { type: "Snapshot", id: episodeId },
      ],
    }),
    deleteScriptVersion: builder.mutation<
      API.ScriptVersionDeleteResponse,
      { sourceId: string; versionId: string }
    >({
      queryFn: ({ versionId }) =>
        runRequest(() =>
          deleteDraftVersionApiV1ScriptVersionsVersionIdDelete({
            version_id: versionId,
            confirm: true,
          }),
        ),
      invalidatesTags: (_result, _error, { sourceId, versionId }) => [
        { type: "ScriptVersions", id: sourceId },
        { type: "ScriptVersion", id: versionId },
      ],
    }),
    startExtraction: builder.mutation<
      API.ExtractionBatchResponse,
      { episodeId: string; workspaceId: string; versionId: string; body: API.ScriptExtractionRequest }
    >({
      queryFn: ({ versionId, body }) =>
        runRequest(() =>
          startExtractionApiV1ScriptVersionsVersionIdExtractionsPost(
            { version_id: versionId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { episodeId, workspaceId }) => [
        { type: "Snapshot", id: episodeId },
        { type: "Tasks", id: workspaceId },
        ...(result
          ? [{ type: "ExtractionBatch" as const, id: result.id }]
          : []),
      ],
    }),
    decideExtractionCandidate: builder.mutation<
      API.CandidateDecisionResultResponse,
      { candidateId: string; batchId: string; episodeId: string; projectId: string; body: API.CandidateDecisionRequest }
    >({
      queryFn: ({ candidateId, body }) =>
        runRequest(() =>
          decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPost(
            { candidate_id: candidateId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { batchId, episodeId, projectId }) => [
        { type: "ExtractionCandidates", id: batchId },
        { type: "ExtractionBatch", id: batchId },
        { type: "Snapshot", id: episodeId },
        { type: "Assets", id: projectId },
      ],
    }),
    confirmStructure: builder.mutation<
      API.StructureConfirmationResponse,
      { batchId: string; episodeId: string }
    >({
      queryFn: ({ batchId }) =>
        runRequest(() =>
          confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePost({
            batch_id: batchId,
          }),
        ),
      invalidatesTags: (result, _error, { batchId, episodeId }) => [
        { type: "ExtractionBatch", id: batchId },
        { type: "Snapshot", id: episodeId },
        ...(result
          ? [
              {
                type: "ScriptVersion" as const,
                id: result.confirmed_version.id,
              },
              {
                type: "ScriptVersions" as const,
                id: result.confirmed_version.source_id,
              },
            ]
          : []),
      ],
    }),
    initializeMediaUpload: builder.mutation<
      API.UploadInitializationResponse,
      API.UploadDeclaration
    >({
      queryFn: (body) =>
        runRequest(() => initializeUploadApiV1MediaUploadsPost(body)),
    }),
    initializeMediaVersionUpload: builder.mutation<
      API.UploadInitializationResponse,
      { mediaObjectId: string; body: API.AppendVersionRequest }
    >({
      queryFn: ({ mediaObjectId, body }) =>
        runRequest(() =>
          initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPost(
            { media_object_id: mediaObjectId },
            body,
          ),
        ),
    }),
    completeMediaUpload: builder.mutation<
      API.UploadCompletionResponse,
      { uploadSessionId: string; workspaceId: string }
    >({
      queryFn: ({ uploadSessionId }) =>
        runRequest(() =>
          completeUploadApiV1MediaUploadsUploadSessionIdCompletePost({
            upload_session_id: uploadSessionId,
          }),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Media", id: workspaceId },
        { type: "Tasks", id: workspaceId },
        { type: "AuditEvents", id: workspaceId },
        "AssetReadiness",
      ],
    }),
    retryMediaProbe: builder.mutation<
      API.TaskResponse,
      { versionId: string; workspaceId: string; body: API.ProbeRetryRequest }
    >({
      queryFn: ({ versionId, body }) =>
        runRequest(() =>
          retryProbeApiV1MediaVersionIdProbeRetryPost(
            { version_id: versionId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Media", id: workspaceId },
        { type: "Tasks", id: workspaceId },
        "AssetReadiness",
      ],
    }),
    mediaLocations: builder.query<API.MediaLocationsResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          listMediaLocationsApiV1MediaVersionIdLocationsGet({
            version_id: versionId,
          }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "MediaLocations", id: versionId },
      ],
    }),
    requestMediaLocationMigration: builder.mutation<
      API.TaskResponse,
      {
        versionId: string;
        workspaceId: string;
        body: API.MediaLocationMigrationRequest;
      }
    >({
      queryFn: ({ versionId, body }) =>
        runRequest(() =>
          requestMediaLocationMigrationApiV1MediaVersionIdLocationMigrationsPost(
            { version_id: versionId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { versionId, workspaceId }) => [
        { type: "MediaLocations", id: versionId },
        { type: "Tasks", id: workspaceId },
        { type: "Schedules", id: workspaceId },
      ],
    }),
    requestMediaLocationRollback: builder.mutation<
      API.TaskResponse,
      {
        versionId: string;
        workspaceId: string;
        body: API.MediaLocationRollbackRequest;
      }
    >({
      queryFn: ({ versionId, body }) =>
        runRequest(() =>
          requestMediaLocationRollbackApiV1MediaVersionIdLocationRollbacksPost(
            { version_id: versionId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { versionId, workspaceId }) => [
        { type: "MediaLocations", id: versionId },
        { type: "Tasks", id: workspaceId },
        { type: "Schedules", id: workspaceId },
      ],
    }),
    setCurrentMediaVersion: builder.mutation<
      API.MediaObjectResponse,
      {
        mediaObjectId: string;
        workspaceId: string;
        body: API.CurrentMediaVersionRequest;
      }
    >({
      queryFn: ({ mediaObjectId, body }) =>
        runRequest(() =>
          setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPost(
            { media_object_id: mediaObjectId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Media", id: workspaceId },
        { type: "AuditEvents", id: workspaceId },
      ],
    }),
    setMediaArchived: builder.mutation<
      API.MediaObjectResponse,
      {
        mediaObjectId: string;
        workspaceId: string;
        archived: boolean;
        body: API.ArchiveMediaRequest;
      }
    >({
      queryFn: ({ mediaObjectId, archived, body }) =>
        runRequest(() => {
          const params = { media_object_id: mediaObjectId };
          return archived
            ? archiveMediaApiV1MediaObjectsMediaObjectIdArchivePost(params, body)
            : restoreMediaApiV1MediaObjectsMediaObjectIdRestorePost(params, body);
        }),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Media", id: workspaceId },
        { type: "AuditEvents", id: workspaceId },
      ],
    }),
    mediaVersions: builder.query<API.PaginatedMedia, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listMediaApiV1MediaGet({
            workspace_id: workspaceId,
            kind: null,
            source_type: null,
            include_archived: true,
            created_from: null,
            created_to: null,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, workspaceId) => [
        { type: "Media", id: workspaceId },
      ],
    }),
    assets: builder.query<API.PaginatedAssets, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          listAssetsApiV1ProjectsProjectIdAssetsGet({
            project_id: projectId,
            kind: null,
            include_archived: true,
            query: null,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (result, _error, projectId) => [
        { type: "Assets", id: projectId },
        ...(result?.items.map((asset) => ({
          type: "Asset" as const,
          id: asset.id,
        })) ?? []),
      ],
    }),
    assetBible: builder.query<API.AssetBibleResponse, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          getAssetBibleApiV1ProjectsProjectIdAssetBibleGet({
            project_id: projectId,
            purpose: "ai_short_drama_generation",
            channel: "lanverse_preview",
            region: "CN",
          }),
        ),
      providesTags: (result, _error, projectId) => [
        { type: "AssetBible", id: projectId },
        ...(result?.items.flatMap(({ asset, states }) => [
          { type: "Asset" as const, id: asset.id },
          ...states.map(({ state }) => ({
            type: "AssetStates" as const,
            id: state.id,
          })),
        ]) ?? []),
      ],
    }),
    assetVersions: builder.query<API.PaginatedAssetVersions, string>({
      queryFn: (stateId) =>
        runRequest(() =>
          listAssetVersionsApiV1AssetStatesStateIdVersionsGet({
            state_id: stateId,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (result, _error, stateId) => [
        { type: "AssetVersions", id: stateId },
        ...(result?.items.map((version) => ({
          type: "AssetReadiness" as const,
          id: version.id,
        })) ?? []),
      ],
    }),
    assetReadiness: builder.query<API.AssetReadinessResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getAssetReadinessApiV1AssetVersionsVersionIdReadinessGet({
            version_id: versionId,
            purpose: "ai_short_drama_generation",
            channel: "lanverse_preview",
            region: "CN",
          }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "AssetReadiness", id: versionId },
      ],
    }),
    createAsset: builder.mutation<
      API.AssetResponse,
      { projectId: string; body: API.AssetCreateRequest }
    >({
      queryFn: ({ projectId, body }) =>
        runRequest(() =>
          createAssetApiV1ProjectsProjectIdAssetsPost(
            { project_id: projectId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
      ],
    }),
    createAssetState: builder.mutation<
      API.AssetStateCreateResponse,
      { projectId: string; assetId: string; body: API.AssetStateCreateRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          createAssetStateApiV1AssetsAssetIdStatesPost(
            { asset_id: assetId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId }) => [
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
      ],
    }),
    appendAssetVersion: builder.mutation<
      API.AssetVersionCreateResponse,
      {
        projectId: string;
        assetId: string;
        stateId: string;
        body: API.AssetVersionCreateRequest;
      }
    >({
      queryFn: ({ stateId, body }) =>
        runRequest(() =>
          appendAssetVersionApiV1AssetStatesStateIdVersionsPost(
            { state_id: stateId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { projectId, assetId, stateId }) => [
        { type: "Asset", id: assetId },
        { type: "AssetBible", id: projectId },
        { type: "AssetStates", id: stateId },
        { type: "AssetVersions", id: stateId },
        ...(result
          ? [
              { type: "AssetReadiness" as const, id: result.version.id },
            ]
          : []),
      ],
    }),
    updateAsset: builder.mutation<
      API.AssetResponse,
      { projectId: string; assetId: string; body: API.AssetUpdateRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          updateAssetApiV1AssetsAssetIdPatch({ asset_id: assetId }, body),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
      ],
    }),
    assetRenamePreflight: builder.mutation<
      API.AssetImpactResponse,
      { assetId: string; body: API.AssetRenamePreflightRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          assetRenamePreflightApiV1AssetsAssetIdRenamePreflightPost(
            { asset_id: assetId },
            body,
          ),
        ),
    }),
    renameAsset: builder.mutation<
      API.AssetRenameResponse,
      { projectId: string; assetId: string; body: API.AssetRenameRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          renameAssetApiV1AssetsAssetIdRenamePost({ asset_id: assetId }, body),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
      ],
    }),
    assetDisablePreflight: builder.mutation<
      API.AssetImpactResponse,
      { assetId: string; body: API.AssetDisablePreflightRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          assetDisablePreflightApiV1AssetsAssetIdDisablePreflightPost(
            { asset_id: assetId },
            body,
          ),
        ),
    }),
    disableAsset: builder.mutation<
      API.AssetAvailabilityResponse,
      { projectId: string; assetId: string; body: API.AssetDisableRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          disableAssetApiV1AssetsAssetIdDisablePost({ asset_id: assetId }, body),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
      ],
    }),
    enableAsset: builder.mutation<
      API.AssetResponse,
      { projectId: string; assetId: string; body: API.AssetEnableRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          enableAssetApiV1AssetsAssetIdEnablePost({ asset_id: assetId }, body),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
      ],
    }),
    updateAssetState: builder.mutation<
      API.AssetStateResponse,
      { projectId: string; assetId: string; stateId: string; body: API.AssetStateUpdateRequest }
    >({
      queryFn: ({ stateId, body }) =>
        runRequest(() =>
          updateAssetStateApiV1AssetStatesStateIdPatch({ state_id: stateId }, body),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId, stateId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
        { type: "AssetStates", id: stateId },
      ],
    }),
    assetStateDisablePreflight: builder.mutation<
      API.AssetImpactResponse,
      { stateId: string; body: API.AssetDisablePreflightRequest }
    >({
      queryFn: ({ stateId, body }) =>
        runRequest(() =>
          assetStateDisablePreflightApiV1AssetStatesStateIdDisablePreflightPost(
            { state_id: stateId },
            body,
          ),
        ),
    }),
    disableAssetState: builder.mutation<
      API.AssetStateAvailabilityResponse,
      {
        projectId: string;
        assetId: string;
        stateId: string;
        body: API.AssetDisableRequest;
      }
    >({
      queryFn: ({ stateId, body }) =>
        runRequest(() =>
          disableAssetStateApiV1AssetStatesStateIdDisablePost(
            { state_id: stateId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId, stateId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
        { type: "AssetStates", id: stateId },
      ],
    }),
    enableAssetState: builder.mutation<
      API.AssetStateResponse,
      {
        projectId: string;
        assetId: string;
        stateId: string;
        body: API.AssetStateEnableRequest;
      }
    >({
      queryFn: ({ stateId, body }) =>
        runRequest(() =>
          enableAssetStateApiV1AssetStatesStateIdEnablePost(
            { state_id: stateId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId, stateId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
        { type: "AssetStates", id: stateId },
      ],
    }),
    currentAssetVersionPreflight: builder.mutation<
      API.AssetImpactResponse,
      { stateId: string; body: API.AssetStateCurrentPreflightRequest }
    >({
      queryFn: ({ stateId, body }) =>
        runRequest(() =>
          currentAssetVersionPreflightApiV1AssetStatesStateIdCurrentVersionPreflightPost(
            { state_id: stateId },
            body,
          ),
        ),
    }),
    setCurrentAssetVersion: builder.mutation<
      API.AssetStateCurrentResponse,
      {
        projectId: string;
        stateId: string;
        body: API.AssetStateCurrentRequest;
      }
    >({
      queryFn: ({ stateId, body }) =>
        runRequest(() =>
          setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPost(
            { state_id: stateId },
            body,
          ),
        ),
      invalidatesTags: (_result, _error, { projectId, stateId, body }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "AssetStates", id: stateId },
        { type: "AssetVersions", id: stateId },
        { type: "AssetReadiness", id: body.version_id },
        ...(body.expected_current_version_id &&
        body.expected_current_version_id !== body.version_id
          ? [
              {
                type: "AssetReadiness" as const,
                id: body.expected_current_version_id,
              },
            ]
          : []),
      ],
    }),
    assetDeletePreflight: builder.mutation<
      API.AssetDeletePreflightResponse,
      string
    >({
      queryFn: (assetId) =>
        runRequest(() =>
          assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGet({
            asset_id: assetId,
          }),
        ),
    }),
    deleteAsset: builder.mutation<
      API.AssetDeleteResponse,
      { projectId: string; assetId: string; expectedRevision: number }
    >({
      queryFn: ({ assetId, expectedRevision }) =>
        runRequest(() =>
          deleteAssetApiV1AssetsAssetIdDelete({
            asset_id: assetId,
            expected_revision: expectedRevision,
          }),
        ),
      invalidatesTags: (_result, _error, { projectId, assetId }) => [
        { type: "Assets", id: projectId },
        { type: "AssetBible", id: projectId },
        { type: "Asset", id: assetId },
      ],
    }),
    setAssetArchived: builder.mutation<
      API.AssetResponse,
      { assetId: string; expectedRevision: number; archived: boolean }
    >({
      queryFn: ({ assetId, expectedRevision, archived }) =>
        runRequest(() => {
          const params = { asset_id: assetId };
          const body = { expected_revision: expectedRevision };
          return archived
            ? archiveAssetApiV1AssetsAssetIdArchivePost(params, body)
            : restoreAssetApiV1AssetsAssetIdRestorePost(params, body);
        }),
      invalidatesTags: (result, _error, { assetId }) => [
        { type: "Asset", id: assetId },
        ...(result
          ? [
              { type: "Assets" as const, id: result.project_id },
              { type: "AssetBible" as const, id: result.project_id },
            ]
          : []),
      ],
    }),
    consents: builder.query<API.PaginatedConsents, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listConsentsApiV1ConsentsGet({
            workspace_id: workspaceId,
            limit: 50,
            offset: 0,
          }),
        ),
      providesTags: (result, _error, workspaceId) => [
        { type: "Consents", id: workspaceId },
        ...(result?.items.map((consent) => ({
          type: "Consent" as const,
          id: consent.id,
        })) ?? []),
      ],
    }),
    consent: builder.query<API.ConsentDetailResponse, string>({
      queryFn: (consentId) =>
        runRequest(() =>
          getConsentApiV1ConsentsConsentIdGet({ consent_id: consentId }),
        ),
      providesTags: (_result, _error, consentId) => [
        { type: "Consent", id: consentId },
      ],
    }),
    auditEvents: builder.query<
      API.PaginatedAuditEvents,
      {
        workspaceId: string;
        actorId?: string;
        targetId?: string;
        targetType?: string;
        action?: string;
        occurredFrom?: string;
        occurredTo?: string;
      }
    >({
      queryFn: ({
        workspaceId,
        actorId,
        targetId,
        targetType,
        action,
        occurredFrom,
        occurredTo,
      }) =>
        runRequest(() =>
          listAuditEventsApiV1AuditEventsGet({
            workspace_id: workspaceId,
            actor_id: actorId ?? null,
            target_id: targetId ?? null,
            target_type: targetType ?? null,
            action: action ?? null,
            occurred_from: occurredFrom ?? null,
            occurred_to: occurredTo ?? null,
            limit: 50,
            offset: 0,
          }),
        ),
      providesTags: (_result, _error, { workspaceId }) => [
        { type: "AuditEvents", id: workspaceId },
      ],
    }),
    createConsent: builder.mutation<
      API.ConsentDetailResponse,
      API.ConsentCreateRequest
    >({
      queryFn: (body) =>
        runRequest(() => createConsentApiV1ConsentsPost(body)),
      invalidatesTags: (_result, _error, body) => [
        { type: "Consents", id: body.workspace_id },
        { type: "AuditEvents", id: body.workspace_id },
        "AssetReadiness",
      ],
    }),
    reviseConsent: builder.mutation<
      API.ConsentDetailResponse,
      { consentId: string; body: API.ConsentRevisionRequest }
    >({
      queryFn: ({ consentId, body }) =>
        runRequest(() =>
          reviseConsentApiV1ConsentsConsentIdRevisionsPost(
            { consent_id: consentId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { consentId }) => [
        { type: "Consent", id: consentId },
        "AssetReadiness",
        ...(result
          ? [
              { type: "Consents" as const, id: result.workspace_id },
              { type: "AuditEvents" as const, id: result.workspace_id },
            ]
          : []),
      ],
    }),
    revokeConsent: builder.mutation<
      API.ConsentDetailResponse,
      { consentId: string; body: API.ConsentRevokeRequest }
    >({
      queryFn: ({ consentId, body }) =>
        runRequest(() =>
          revokeConsentApiV1ConsentsConsentIdRevokePost(
            { consent_id: consentId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { consentId }) => [
        { type: "Consent", id: consentId },
        "AssetReadiness",
        ...(result
          ? [
              { type: "Consents" as const, id: result.workspace_id },
              { type: "AuditEvents" as const, id: result.workspace_id },
            ]
          : []),
      ],
    }),
  }),
});

export const {
  useAdaptationRunQuery,
  useApplyAssetUpgradeMutation,
  useApplyStoryboardDraftMutation,
  useApproveStoryboardDraftMutation,
  useCancelAdaptationRunMutation,
  useCancelGenerationTaskMutation,
  useAppendAssetVersionMutation,
  useAppendShotSpecMutation,
  useAuditEventsQuery,
  useArchivedShotsQuery,
  useAssetReadinessQuery,
  useAssetDisablePreflightMutation,
  useAssetRenamePreflightMutation,
  useAssetStateDisablePreflightMutation,
  useAssetShotUsagesQuery,
  useAssetUpgradePreflightMutation,
  useAssetDeletePreflightMutation,
  useAssetBibleQuery,
  useAssetsQuery,
  useAssetVersionsQuery,
  useChangePasswordMutation,
  useCompleteMediaUploadMutation,
  useConfirmStructureMutation,
  useConfigureScheduleMutation,
  useConfirmRegistrationVerificationMutation,
  useConfirmedStructureQuery,
  useConsentQuery,
  useConsentsQuery,
  useCreateAdaptationRunMutation,
  useCreateConsentMutation,
  useCreateAssetMutation,
  useCreateAssetStateMutation,
  useCurrentAssetVersionPreflightMutation,
  useCreateShotMutation,
  useCreateStoryboardDraftMutation,
  useCreateShotFromCandidateMutation,
  useCreateEpisodeMutation,
  useCreateProjectMutation,
  useCreateWorkspaceMutation,
  useDeleteScriptVersionMutation,
  useDeleteAssetMutation,
  useDisableAssetMutation,
  useDisableAssetStateMutation,
  useDeleteShotMutation,
  useDeleteEpisodeMutation,
  useDeleteProjectMutation,
  useDeactivateAccountMutation,
  useDecideExtractionCandidateMutation,
  useDecideStoryboardDraftMutation,
  useEpisodeQuery,
  useEnableAssetMutation,
  useEnableAssetStateMutation,
  useEpisodeSnapshotQuery,
  useEpisodesQuery,
  useExtractionBatchQuery,
  useExtractionCandidatesQuery,
  useImportScriptMutation,
  useImportScriptDocumentMutation,
  useLazyEpisodePlanQuery,
  useCreateEpisodePlanMutation,
  useRenameEpisodeProposalMutation,
  useMoveEpisodeBoundaryMutation,
  useSplitEpisodeProposalMutation,
  useMergeEpisodeProposalsMutation,
  useConfirmEpisodePlanMutation,
  useMaterializeEpisodePlanMutation,
  usePublishImportCommitMutation,
  useInitializeMediaUploadMutation,
  useInitializeMediaVersionUploadMutation,
  useLoginMutation,
  useLogoutMutation,
  useLazyShotSpecVersionQuery,
  useLazyAdaptationDiffQuery,
  useLazyScriptVersionDiffQuery,
  useMeQuery,
  useMergeShotsMutation,
  useMergeShotsPreflightMutation,
  useMediaVersionsQuery,
  useLazyMediaVersionQuery,
  useMediaLocationsQuery,
  useNarrativeStructureQuery,
  useProjectQuery,
  useProjectDeletePreflightMutation,
  useProjectSnapshotQuery,
  usePreflightStoryboardDraftMutation,
  useProjectsQuery,
  usePublishAdaptationRunMutation,
  usePublishScriptVersionMutation,
  usePauseScheduleMutation,
  useRegisterMutation,
  useRetryMediaProbeMutation,
  useRequestMediaLocationMigrationMutation,
  useRequestMediaLocationRollbackMutation,
  useRequestRegistrationVerificationMutation,
  useResumeScheduleMutation,
  useReviseNarrativeStructureMutation,
  useReviseConsentMutation,
  useRevokeConsentMutation,
  useReorderEpisodesMutation,
  useRenameAssetMutation,
  useEpisodeDeletePreflightMutation,
  useSetEpisodeArchivedMutation,
  useSetAssetArchivedMutation,
  useSetCurrentAssetVersionMutation,
  useSetCurrentMediaVersionMutation,
  useSetCurrentScriptVersionMutation,
  useSetScriptSourceArchivedMutation,
  useSetCurrentShotSpecMutation,
  useSetShotArchivedMutation,
  useSetMediaArchivedMutation,
  useSetProjectArchivedMutation,
  useSetWorkspaceArchivedMutation,
  useUpdateProfileMutation,
  useUpdateAssetMutation,
  useUpdateAssetStateMutation,
  useUpdateShotMutation,
  useUpdateEpisodeMutation,
  useUpdateProjectBudgetMutation,
  useUpdateProjectMutation,
  useUpdateWorkspaceMutation,
  useWorkspacesQuery,
  useScriptSourcesQuery,
  useScriptDocumentsQuery,
  useScriptVersionQuery,
  useScriptVersionsQuery,
  useStartExtractionMutation,
  useCopyShotMutation,
  useReorderShotsMutation,
  useShotOrderQuery,
  useShotReadinessQuery,
  useShotDeletePreflightMutation,
  useShotSpecVersionsQuery,
  useStoryboardDraftQuery,
  useSplitShotMutation,
  useSplitShotPreflightMutation,
  useTasksQuery,
  useModelCapabilitiesQuery,
  useCostsQuery,
  useSchedulesQuery,
  useTriggerScheduleMutation,
  useUpdateAdaptationDraftMutation,
} = appApi;
