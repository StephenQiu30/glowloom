import request, { type RequestOptions } from "@/lib/request";

/** Get Costs GET /api/v1/costs */
export async function getCostsApiV1CostsGet(
  params: API.getCostsApiV1CostsGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponseCostQueryResponse_>(`/api/v1/costs`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** List Model Capabilities GET /api/v1/model-capabilities */
export async function listModelCapabilitiesApiV1ModelCapabilitiesGet(
  params: API.listModelCapabilitiesApiV1ModelCapabilitiesGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponseListModelCapabilityResponse_>(`/api/v1/model-capabilities`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** Preflight Generation POST /api/v1/shots/{shot_id}/generation-preflight */
export async function preflightGenerationApiV1ShotsShotIdGenerationPreflightPost(
  params: API.preflightGenerationApiV1ShotsShotIdGenerationPreflightPostParams,
  body: API.GenerationPreflightRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseGenerationPreflightResponse_>(`/api/v1/shots/${path0}/generation-preflight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Submit Generation POST /api/v1/shots/{shot_id}/generation-requests */
export async function submitGenerationApiV1ShotsShotIdGenerationRequestsPost(
  params: API.submitGenerationApiV1ShotsShotIdGenerationRequestsPostParams,
  body: API.GenerationSubmissionRequest,
  options?: RequestOptions,
) {
  const { shot_id: path0 } = params;
  return request<API.ApiResponseGenerationSubmissionResponse_>(`/api/v1/shots/${path0}/generation-requests`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
