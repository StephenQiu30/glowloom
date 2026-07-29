import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

import {
  loginApiV1AuthLoginPost,
  logoutApiV1AuthLogoutPost,
  meApiV1MeGet,
  registerApiV1AuthRegisterPost,
} from "@/api/identity";
import {
  createEpisodeApiV1ProjectsProjectIdEpisodesPost,
  createProjectApiV1ProjectsPost,
  getProjectApiV1ProjectsProjectIdGet,
  listEpisodesApiV1ProjectsProjectIdEpisodesGet,
  listProjectsApiV1ProjectsGet,
  projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet,
} from "@/api/projects";
import { ApiClientError } from "@/lib/request";

export type AppApiError = {
  message: string;
  code: string;
  nextAction?: string;
};

export function appApiErrorMessage(error: unknown): string {
  return (error as Partial<AppApiError> | undefined)?.message ?? "服务暂时不可用，请稍后重试。";
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
  tagTypes: ["Me", "Projects", "Project", "Episodes", "Snapshot"],
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
  useCreateEpisodeMutation,
  useCreateProjectMutation,
  useEpisodesQuery,
  useLoginMutation,
  useLogoutMutation,
  useMeQuery,
  useProjectQuery,
  useProjectSnapshotQuery,
  useProjectsQuery,
  useRegisterMutation,
} = appApi;
