import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function listProjectsApiV1ProjectsGet(
  params: {
    workspace_id: string;
    include_archived: boolean;
    search: string | null;
    sort: "updated_at";
    order: "desc";
    limit: number;
    offset: number;
  },
  options?: RequestOptions,
) {
  return request<Envelope<API.PaginatedProjects>>("/api/projects", {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

export function createProjectApiV1ProjectsPost(
  body: API.ProjectCreateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ProjectResponse>>("/api/projects", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function getProjectApiV1ProjectsProjectIdGet(
  params: { project_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.ProjectResponse>>(
    `/api/projects/${params.project_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function listEpisodesApiV1ProjectsProjectIdEpisodesGet(
  params: { project_id: string; include_archived: boolean },
  options?: RequestOptions,
) {
  const { project_id: projectId, ...query } = params;
  return request<Envelope<API.EpisodeResponse[]>>(
    `/api/projects/${projectId}/episodes`,
    { method: "GET", params: query, ...(options ?? {}) },
  );
}
