// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** Get Run GET /api/v1/adaptation-runs/${param0} */
export async function getRunApiV1AdaptationRunsRunIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getRunApiV1AdaptationRunsRunIdGetParams,
  options?: RequestOptions
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(
    `/api/v1/adaptation-runs/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Cancel Run POST /api/v1/adaptation-runs/${param0}/cancel */
export async function cancelRunApiV1AdaptationRunsRunIdCancelPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelRunApiV1AdaptationRunsRunIdCancelPostParams,
  body: API.AdaptationCancelRequest,
  options?: RequestOptions
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(
    `/api/v1/adaptation-runs/${param0}/cancel`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Diff Run GET /api/v1/adaptation-runs/${param0}/diff */
export async function diffRunApiV1AdaptationRunsRunIdDiffGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.diffRunApiV1AdaptationRunsRunIdDiffGetParams,
  options?: RequestOptions
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAdaptationDiffResponse_>(
    `/api/v1/adaptation-runs/${param0}/diff`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Update Draft PATCH /api/v1/adaptation-runs/${param0}/draft */
export async function updateDraftApiV1AdaptationRunsRunIdDraftPatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateDraftApiV1AdaptationRunsRunIdDraftPatchParams,
  body: API.AdaptationDraftUpdateRequest,
  options?: RequestOptions
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(
    `/api/v1/adaptation-runs/${param0}/draft`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Publish Run POST /api/v1/adaptation-runs/${param0}/publish */
export async function publishRunApiV1AdaptationRunsRunIdPublishPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishRunApiV1AdaptationRunsRunIdPublishPostParams,
  body: API.AdaptationPublishRequest,
  options?: RequestOptions
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAdaptationPublishResponse_>(
    `/api/v1/adaptation-runs/${param0}/publish`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Create Run POST /api/v1/episodes/${param0}/adaptation-runs */
export async function createRunApiV1EpisodesEpisodeIdAdaptationRunsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createRunApiV1EpisodesEpisodeIdAdaptationRunsPostParams,
  body: API.AdaptationRunCreateRequest,
  options?: RequestOptions
) {
  const { episode_id: param0, ...queryParams } = params;
  return request<API.ApiResponseAdaptationRunResponse_>(
    `/api/v1/episodes/${param0}/adaptation-runs`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
