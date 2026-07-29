import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

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
  createEpisodeApiV1ProjectsProjectIdEpisodesPost,
  createProjectApiV1ProjectsPost,
  getProjectApiV1ProjectsProjectIdGet,
  listEpisodesApiV1ProjectsProjectIdEpisodesGet,
  listProjectsApiV1ProjectsGet,
  projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet,
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
  tagTypes: ["Me", "Workspaces", "Projects", "Project", "Episodes", "Snapshot"],
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
            include_archived: false,
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
            include_archived: false,
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
    projectSnapshot: builder.query<API.ProjectProductionSnapshot, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet({
            project_id: projectId,
          }),
        ),
      providesTags: (_result, _error, projectId) => [{ type: "Snapshot", id: projectId }],
    }),
  }),
});

export const {
  useChangePasswordMutation,
  useCreateEpisodeMutation,
  useCreateProjectMutation,
  useCreateWorkspaceMutation,
  useDeactivateAccountMutation,
  useEpisodesQuery,
  useLoginMutation,
  useLogoutMutation,
  useMeQuery,
  useProjectQuery,
  useProjectSnapshotQuery,
  useProjectsQuery,
  useRegisterMutation,
  useSetWorkspaceArchivedMutation,
  useUpdateProfileMutation,
  useUpdateWorkspaceMutation,
  useWorkspacesQuery,
} = appApi;
