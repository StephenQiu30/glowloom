import request, { type RequestOptions } from "@/lib/request";

/** Delete Episode DELETE /api/v1/episodes/{episode_id} */
export async function deleteEpisodeApiV1EpisodesEpisodeIdDelete(
  params: API.deleteEpisodeApiV1EpisodesEpisodeIdDeleteParams,
  options?: RequestOptions,
) {
  const { episode_id: path0, ...queryParams } = params;
  return request<API.ApiResponseDeleteResponse_>(`/api/v1/episodes/${path0}`, {
    method: "DELETE",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Episode GET /api/v1/episodes/{episode_id} */
export async function getEpisodeApiV1EpisodesEpisodeIdGet(
  params: API.getEpisodeApiV1EpisodesEpisodeIdGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseEpisodeResponse_>(`/api/v1/episodes/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Update Episode PATCH /api/v1/episodes/{episode_id} */
export async function updateEpisodeApiV1EpisodesEpisodeIdPatch(
  params: API.updateEpisodeApiV1EpisodesEpisodeIdPatchParams,
  body: API.EpisodeUpdateRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseEpisodeResponse_>(`/api/v1/episodes/${path0}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Archive Episode POST /api/v1/episodes/{episode_id}/archive */
export async function archiveEpisodeApiV1EpisodesEpisodeIdArchivePost(
  params: API.archiveEpisodeApiV1EpisodesEpisodeIdArchivePostParams,
  body: API.EpisodeStateRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseEpisodeResponse_>(`/api/v1/episodes/${path0}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Episode Delete Preflight POST /api/v1/episodes/{episode_id}/delete-preflight */
export async function episodeDeletePreflightApiV1EpisodesEpisodeIdDeletePreflightPost(
  params: API.episodeDeletePreflightApiV1EpisodesEpisodeIdDeletePreflightPostParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseDeletePreflightResponse_>(`/api/v1/episodes/${path0}/delete-preflight`, {
    method: "POST",
    ...(options ?? {}),
  });
}

/** Episode Production Snapshot GET /api/v1/episodes/{episode_id}/production-snapshot */
export async function episodeProductionSnapshotApiV1EpisodesEpisodeIdProductionSnapshotGet(
  params: API.episodeProductionSnapshotApiV1EpisodesEpisodeIdProductionSnapshotGetParams,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseEpisodeProductionSnapshot_>(`/api/v1/episodes/${path0}/production-snapshot`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Restore Episode POST /api/v1/episodes/{episode_id}/restore */
export async function restoreEpisodeApiV1EpisodesEpisodeIdRestorePost(
  params: API.restoreEpisodeApiV1EpisodesEpisodeIdRestorePostParams,
  body: API.EpisodeStateRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseEpisodeResponse_>(`/api/v1/episodes/${path0}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Projects GET /api/v1/projects */
export async function listProjectsApiV1ProjectsGet(
  params: API.listProjectsApiV1ProjectsGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponsePaginatedProjects_>(`/api/v1/projects`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** Create Project POST /api/v1/projects */
export async function createProjectApiV1ProjectsPost(
  body: API.ProjectCreateRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseProjectResponse_>(`/api/v1/projects`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Delete Project DELETE /api/v1/projects/{project_id} */
export async function deleteProjectApiV1ProjectsProjectIdDelete(
  params: API.deleteProjectApiV1ProjectsProjectIdDeleteParams,
  options?: RequestOptions,
) {
  const { project_id: path0, ...queryParams } = params;
  return request<API.ApiResponseDeleteResponse_>(`/api/v1/projects/${path0}`, {
    method: "DELETE",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Get Project GET /api/v1/projects/{project_id} */
export async function getProjectApiV1ProjectsProjectIdGet(
  params: API.getProjectApiV1ProjectsProjectIdGetParams,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseProjectResponse_>(`/api/v1/projects/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Update Project PATCH /api/v1/projects/{project_id} */
export async function updateProjectApiV1ProjectsProjectIdPatch(
  params: API.updateProjectApiV1ProjectsProjectIdPatchParams,
  body: API.ProjectUpdateRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseProjectResponse_>(`/api/v1/projects/${path0}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Archive Project POST /api/v1/projects/{project_id}/archive */
export async function archiveProjectApiV1ProjectsProjectIdArchivePost(
  params: API.archiveProjectApiV1ProjectsProjectIdArchivePostParams,
  body: API.ProjectStateRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseProjectResponse_>(`/api/v1/projects/${path0}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Update Budget Limit POST /api/v1/projects/{project_id}/budget-limit */
export async function updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPost(
  params: API.updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPostParams,
  body: API.BudgetLimitRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseProjectResponse_>(`/api/v1/projects/${path0}/budget-limit`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Delete Preflight POST /api/v1/projects/{project_id}/delete-preflight */
export async function deletePreflightApiV1ProjectsProjectIdDeletePreflightPost(
  params: API.deletePreflightApiV1ProjectsProjectIdDeletePreflightPostParams,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseDeletePreflightResponse_>(`/api/v1/projects/${path0}/delete-preflight`, {
    method: "POST",
    ...(options ?? {}),
  });
}

/** List Episodes GET /api/v1/projects/{project_id}/episodes */
export async function listEpisodesApiV1ProjectsProjectIdEpisodesGet(
  params: API.listEpisodesApiV1ProjectsProjectIdEpisodesGetParams,
  options?: RequestOptions,
) {
  const { project_id: path0, ...queryParams } = params;
  return request<API.ApiResponseListEpisodeResponse_>(`/api/v1/projects/${path0}/episodes`, {
    method: "GET",
    params: queryParams,
    ...(options ?? {}),
  });
}

/** Create Episode POST /api/v1/projects/{project_id}/episodes */
export async function createEpisodeApiV1ProjectsProjectIdEpisodesPost(
  params: API.createEpisodeApiV1ProjectsProjectIdEpisodesPostParams,
  body: API.EpisodeCreateRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseEpisodeResponse_>(`/api/v1/projects/${path0}/episodes`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Reorder Episodes POST /api/v1/projects/{project_id}/episodes/reorder */
export async function reorderEpisodesApiV1ProjectsProjectIdEpisodesReorderPost(
  params: API.reorderEpisodesApiV1ProjectsProjectIdEpisodesReorderPostParams,
  body: API.EpisodeReorderRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseEpisodeOrderResponse_>(`/api/v1/projects/${path0}/episodes/reorder`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Project Production Snapshot GET /api/v1/projects/{project_id}/production-snapshot */
export async function projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet(
  params: API.projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGetParams,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseProjectProductionSnapshot_>(`/api/v1/projects/${path0}/production-snapshot`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Restore Project POST /api/v1/projects/{project_id}/restore */
export async function restoreProjectApiV1ProjectsProjectIdRestorePost(
  params: API.restoreProjectApiV1ProjectsProjectIdRestorePostParams,
  body: API.ProjectStateRequest,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseProjectResponse_>(`/api/v1/projects/${path0}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
