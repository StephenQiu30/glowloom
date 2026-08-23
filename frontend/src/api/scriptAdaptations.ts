import request, { type RequestOptions } from "@/lib/request";

/** Get Run GET /api/v1/adaptation-runs/{run_id} */
export async function getRunApiV1AdaptationRunsRunIdGet(
  params: API.getRunApiV1AdaptationRunsRunIdGetParams,
  options?: RequestOptions,
) {
  const { run_id: path0 } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(`/api/v1/adaptation-runs/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Cancel Run POST /api/v1/adaptation-runs/{run_id}/cancel */
export async function cancelRunApiV1AdaptationRunsRunIdCancelPost(
  params: API.cancelRunApiV1AdaptationRunsRunIdCancelPostParams,
  body: API.AdaptationCancelRequest,
  options?: RequestOptions,
) {
  const { run_id: path0 } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(`/api/v1/adaptation-runs/${path0}/cancel`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Diff Run GET /api/v1/adaptation-runs/{run_id}/diff */
export async function diffRunApiV1AdaptationRunsRunIdDiffGet(
  params: API.diffRunApiV1AdaptationRunsRunIdDiffGetParams,
  options?: RequestOptions,
) {
  const { run_id: path0 } = params;
  return request<API.ApiResponseAdaptationDiffResponse_>(`/api/v1/adaptation-runs/${path0}/diff`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Update Draft PATCH /api/v1/adaptation-runs/{run_id}/draft */
export async function updateDraftApiV1AdaptationRunsRunIdDraftPatch(
  params: API.updateDraftApiV1AdaptationRunsRunIdDraftPatchParams,
  body: API.AdaptationDraftUpdateRequest,
  options?: RequestOptions,
) {
  const { run_id: path0 } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(`/api/v1/adaptation-runs/${path0}/draft`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Publish Run POST /api/v1/adaptation-runs/{run_id}/publish */
export async function publishRunApiV1AdaptationRunsRunIdPublishPost(
  params: API.publishRunApiV1AdaptationRunsRunIdPublishPostParams,
  body: API.AdaptationPublishRequest,
  options?: RequestOptions,
) {
  const { run_id: path0 } = params;
  return request<API.ApiResponseAdaptationPublishResponse_>(`/api/v1/adaptation-runs/${path0}/publish`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Create Run POST /api/v1/episodes/{episode_id}/adaptation-runs */
export async function createRunApiV1EpisodesEpisodeIdAdaptationRunsPost(
  params: API.createRunApiV1EpisodesEpisodeIdAdaptationRunsPostParams,
  body: API.AdaptationRunCreateRequest,
  options?: RequestOptions,
) {
  const { episode_id: path0 } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(`/api/v1/episodes/${path0}/adaptation-runs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
