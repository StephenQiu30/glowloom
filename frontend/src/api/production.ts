// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/api-request";

/** Get Costs GET /api/v1/costs */
export async function getCostsApiV1CostsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCostsApiV1CostsGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponseCostQueryResponse_>("/api/v1/costs", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** List Model Capabilities GET /api/v1/model-capabilities */
export async function listModelCapabilitiesApiV1ModelCapabilitiesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listModelCapabilitiesApiV1ModelCapabilitiesGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponseListModelCapabilityResponse_>(
    "/api/v1/model-capabilities",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Preflight Generation POST /api/v1/shots/${param0}/generation-preflight */
export async function preflightGenerationApiV1ShotsShotIdGenerationPreflightPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.preflightGenerationApiV1ShotsShotIdGenerationPreflightPostParams,
  body: API.GenerationPreflightRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseGenerationPreflightResponse_>(
    `/api/v1/shots/${param0}/generation-preflight`,
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

/** Submit Generation POST /api/v1/shots/${param0}/generation-requests */
export async function submitGenerationApiV1ShotsShotIdGenerationRequestsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.submitGenerationApiV1ShotsShotIdGenerationRequestsPostParams,
  body: API.GenerationSubmissionRequest,
  options?: RequestOptions
) {
  const { shot_id: param0, ...queryParams } = params;
  return request<API.ApiResponseGenerationSubmissionResponse_>(
    `/api/v1/shots/${param0}/generation-requests`,
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
