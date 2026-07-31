import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

import {
  appendAssetVersionApiV1AssetsAssetIdVersionsPost,
  archiveAssetApiV1AssetsAssetIdArchivePost,
  createAssetApiV1ProjectsProjectIdAssetsPost,
  getAssetReadinessApiV1AssetVersionsVersionIdReadinessGet,
  listAssetsApiV1ProjectsProjectIdAssetsGet,
  listAssetVersionsApiV1AssetsAssetIdVersionsGet,
  restoreAssetApiV1AssetsAssetIdRestorePost,
} from "@/api/assets";
import {
  createConsentApiV1ConsentsPost,
  getConsentApiV1ConsentsConsentIdGet,
  listConsentsApiV1ConsentsGet,
  reviseConsentApiV1ConsentsConsentIdRevisionsPost,
  revokeConsentApiV1ConsentsConsentIdRevokePost,
} from "@/api/governance";
import {
  archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePost,
  changePasswordApiV1AuthChangePasswordPost,
  createWorkspaceApiV1WorkspacesPost,
  deactivateMeApiV1MeDeactivatePost,
  listWorkspacesApiV1WorkspacesGet,
  loginApiV1AuthLoginPost,
  logoutApiV1AuthLogoutPost,
  meApiV1MeGet,
  registerApiV1AuthRegisterPost,
  restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePost,
  updateMeApiV1MePatch,
  updateWorkspaceApiV1WorkspacesWorkspaceIdPatch,
} from "@/api/identity";
import {
  completeUploadApiV1MediaUploadsUploadSessionIdCompletePost,
  initializeUploadApiV1MediaUploadsPost,
  listMediaApiV1MediaGet,
  retryProbeApiV1MediaVersionIdProbeRetryPost,
} from "@/api/media";
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
  confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePost,
  decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPost,
  getExtractionBatchApiV1ExtractionBatchesBatchIdGet,
  getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGet,
  getVersionApiV1ScriptVersionsVersionIdGet,
  importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPost,
  listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGet,
  listSourcesApiV1EpisodesEpisodeIdScriptSourcesGet,
  listVersionsApiV1ScriptSourcesSourceIdVersionsGet,
  publishVersionApiV1ScriptSourcesSourceIdVersionsPost,
  setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPost,
  startExtractionApiV1ScriptVersionsVersionIdExtractionsPost,
} from "@/api/scripts";
import {
  applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePost,
  appendSpecVersionApiV1ShotsShotIdSpecVersionsPost,
  archiveShotApiV1ShotsShotIdArchivePost,
  copyShotApiV1ShotsShotIdCopyPost,
  createManualShotApiV1EpisodesEpisodeIdShotsPost,
  deleteShotApiV1ShotsShotIdDelete,
  getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGet,
  getSpecVersionApiV1ShotSpecVersionsVersionIdGet,
  listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGet,
  listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGet,
  listShotsApiV1EpisodesEpisodeIdShotsGet,
  listSpecVersionsApiV1ShotsShotIdSpecVersionsGet,
  mergePreflightApiV1ShotsMergePreflightPost,
  mergeShotsApiV1ShotsMergePost,
  preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPost,
  reorderShotsApiV1EpisodesEpisodeIdShotsReorderPost,
  restoreShotApiV1ShotsShotIdRestorePost,
  setCurrentSpecVersionApiV1ShotsShotIdCurrentSpecVersionPost,
  shotDeletePreflightApiV1ShotsShotIdDeletePreflightGet,
  splitPreflightApiV1ShotsShotIdSplitPreflightPost,
  splitShotApiV1ShotsShotIdSplitPost,
} from "@/api/storyboards";
import { listTasksApiV1TasksGet } from "@/api/tasks";
import { ApiClientError } from "@/lib/api-request";

export type AppApiError = {
  message: string;
  code: string;
  nextAction?: string;
};

const errorMessages: Record<string, string> = {
  unauthenticated: "邮箱或密码不正确，请重新输入。",
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
    "Media",
    "Assets",
    "Asset",
    "AssetVersions",
    "AssetReadiness",
    "AssetShotUsages",
    "ScriptSources",
    "ScriptVersions",
    "ScriptVersion",
    "Tasks",
    "ExtractionBatch",
    "ExtractionCandidates",
    "ConfirmedStructure",
    "Shots",
    "ArchivedShots",
    "ShotSpecs",
    "ShotReadiness",
  ],
  endpoints: (builder) => ({
    login: builder.mutation<API.AuthResponse, API.LoginRequest>({
      queryFn: (body) => runRequest(() => loginApiV1AuthLoginPost(body)),
    }),
    register: builder.mutation<API.AuthResponse, API.RegisterRequest>({
      queryFn: (body) => runRequest(() => registerApiV1AuthRegisterPost(body)),
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
    scriptVersion: builder.query<API.ScriptVersionResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getVersionApiV1ScriptVersionsVersionIdGet({ version_id: versionId }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "ScriptVersion", id: versionId },
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
    mediaVersions: builder.query<API.PaginatedMedia, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listMediaApiV1MediaGet({
            workspace_id: workspaceId,
            kind: null,
            source_type: null,
            include_archived: false,
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
    assetVersions: builder.query<API.PaginatedAssetVersions, string>({
      queryFn: (assetId) =>
        runRequest(() =>
          listAssetVersionsApiV1AssetsAssetIdVersionsGet({
            asset_id: assetId,
            limit: 100,
            offset: 0,
          }),
        ),
      providesTags: (result, _error, assetId) => [
        { type: "AssetVersions", id: assetId },
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
      ],
    }),
    appendAssetVersion: builder.mutation<
      API.AssetVersionCreateResponse,
      { assetId: string; body: API.AssetVersionCreateRequest }
    >({
      queryFn: ({ assetId, body }) =>
        runRequest(() =>
          appendAssetVersionApiV1AssetsAssetIdVersionsPost(
            { asset_id: assetId },
            body,
          ),
        ),
      invalidatesTags: (result, _error, { assetId }) => [
        { type: "Asset", id: assetId },
        { type: "AssetVersions", id: assetId },
        ...(result
          ? [
              { type: "Assets" as const, id: result.asset.project_id },
              { type: "AssetReadiness" as const, id: result.version.id },
            ]
          : []),
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
              ...(result.current_version_id
                ? [
                    {
                      type: "AssetReadiness" as const,
                      id: result.current_version_id,
                    },
                  ]
                : []),
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
    createConsent: builder.mutation<
      API.ConsentDetailResponse,
      API.ConsentCreateRequest
    >({
      queryFn: (body) =>
        runRequest(() => createConsentApiV1ConsentsPost(body)),
      invalidatesTags: (_result, _error, body) => [
        { type: "Consents", id: body.workspace_id },
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
          ? [{ type: "Consents" as const, id: result.workspace_id }]
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
          ? [{ type: "Consents" as const, id: result.workspace_id }]
          : []),
      ],
    }),
  }),
});

export const {
  useApplyAssetUpgradeMutation,
  useAppendAssetVersionMutation,
  useAppendShotSpecMutation,
  useArchivedShotsQuery,
  useAssetReadinessQuery,
  useAssetShotUsagesQuery,
  useAssetUpgradePreflightMutation,
  useAssetsQuery,
  useAssetVersionsQuery,
  useChangePasswordMutation,
  useCompleteMediaUploadMutation,
  useConfirmStructureMutation,
  useConfirmedStructureQuery,
  useConsentQuery,
  useConsentsQuery,
  useCreateConsentMutation,
  useCreateAssetMutation,
  useCreateShotMutation,
  useCreateEpisodeMutation,
  useCreateProjectMutation,
  useCreateWorkspaceMutation,
  useDeleteShotMutation,
  useDeleteEpisodeMutation,
  useDeleteProjectMutation,
  useDeactivateAccountMutation,
  useDecideExtractionCandidateMutation,
  useEpisodeQuery,
  useEpisodeSnapshotQuery,
  useEpisodesQuery,
  useExtractionBatchQuery,
  useExtractionCandidatesQuery,
  useImportScriptMutation,
  useInitializeMediaUploadMutation,
  useLoginMutation,
  useLogoutMutation,
  useLazyShotSpecVersionQuery,
  useMeQuery,
  useMergeShotsMutation,
  useMergeShotsPreflightMutation,
  useMediaVersionsQuery,
  useProjectQuery,
  useProjectDeletePreflightMutation,
  useProjectSnapshotQuery,
  useProjectsQuery,
  usePublishScriptVersionMutation,
  useRegisterMutation,
  useRetryMediaProbeMutation,
  useReviseConsentMutation,
  useRevokeConsentMutation,
  useReorderEpisodesMutation,
  useEpisodeDeletePreflightMutation,
  useSetEpisodeArchivedMutation,
  useSetAssetArchivedMutation,
  useSetCurrentScriptVersionMutation,
  useSetCurrentShotSpecMutation,
  useSetShotArchivedMutation,
  useSetProjectArchivedMutation,
  useSetWorkspaceArchivedMutation,
  useUpdateProfileMutation,
  useUpdateEpisodeMutation,
  useUpdateProjectBudgetMutation,
  useUpdateProjectMutation,
  useUpdateWorkspaceMutation,
  useWorkspacesQuery,
  useScriptSourcesQuery,
  useScriptVersionQuery,
  useScriptVersionsQuery,
  useStartExtractionMutation,
  useCopyShotMutation,
  useReorderShotsMutation,
  useShotOrderQuery,
  useShotReadinessQuery,
  useShotDeletePreflightMutation,
  useShotSpecVersionsQuery,
  useSplitShotMutation,
  useSplitShotPreflightMutation,
  useTasksQuery,
} = appApi;
