import {
  listProjectsApiProjectsGet,
  createProjectApiProjectsPost,
  getProjectApiProjectsProjectIdGet,
  listEpisodesApiProjectsProjectIdEpisodesGet,
} from "@/api/projects";
import { appApi, runRequest } from "@/lib/server-state";

export const projectApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    projects: builder.query<API.PaginatedProjects, string>({
      queryFn: (workspaceId) =>
        runRequest(() =>
          listProjectsApiProjectsGet({
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
      queryFn: (body) => runRequest(() => createProjectApiProjectsPost(body)),
      invalidatesTags: ["Projects"],
    }),
    project: builder.query<API.ProjectResponse, string>({
      queryFn: (projectId) =>
        runRequest(() =>
          getProjectApiProjectsProjectIdGet({ project_id: projectId }),
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "Project", id: projectId },
      ],
    }),
    episodes: builder.query<API.EpisodeResponse[], string>({
      queryFn: (projectId) =>
        runRequest(() =>
          listEpisodesApiProjectsProjectIdEpisodesGet({
            project_id: projectId,
            include_archived: true,
          }),
        ),
      providesTags: (_result, _error, projectId) => [
        { type: "Episodes", id: projectId },
      ],
    }),
  }),
});

export const {
  useCreateProjectMutation,
  useEpisodesQuery,
  useProjectQuery,
  useProjectsQuery,
} = projectApi;
