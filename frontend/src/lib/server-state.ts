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
import { listMediaApiV1MediaGet } from "@/api/media";
import {
  archiveEpisodeApiV1EpisodesEpisodeIdArchivePost,
  archiveProjectApiV1ProjectsProjectIdArchivePost,
  createEpisodeApiV1ProjectsProjectIdEpisodesPost,
  createProjectApiV1ProjectsPost,
  deleteEpisodeApiV1EpisodesEpisodeIdDelete,
  deletePreflightApiV1ProjectsProjectIdDeletePreflightPost,
  deleteProjectApiV1ProjectsProjectIdDelete,
  episodeDeletePreflightApiV1EpisodesEpisodeIdDeletePreflightPost,
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
  useAppendAssetVersionMutation,
  useAssetReadinessQuery,
  useAssetsQuery,
  useAssetVersionsQuery,
  useChangePasswordMutation,
  useConsentQuery,
  useConsentsQuery,
  useCreateConsentMutation,
  useCreateAssetMutation,
  useCreateEpisodeMutation,
  useCreateProjectMutation,
  useCreateWorkspaceMutation,
  useDeleteEpisodeMutation,
  useDeleteProjectMutation,
  useDeactivateAccountMutation,
  useEpisodesQuery,
  useLoginMutation,
  useLogoutMutation,
  useMeQuery,
  useMediaVersionsQuery,
  useProjectQuery,
  useProjectDeletePreflightMutation,
  useProjectSnapshotQuery,
  useProjectsQuery,
  useRegisterMutation,
  useReviseConsentMutation,
  useRevokeConsentMutation,
  useReorderEpisodesMutation,
  useEpisodeDeletePreflightMutation,
  useSetEpisodeArchivedMutation,
  useSetAssetArchivedMutation,
  useSetProjectArchivedMutation,
  useSetWorkspaceArchivedMutation,
  useUpdateProfileMutation,
  useUpdateEpisodeMutation,
  useUpdateProjectBudgetMutation,
  useUpdateProjectMutation,
  useUpdateWorkspaceMutation,
  useWorkspacesQuery,
} = appApi;
