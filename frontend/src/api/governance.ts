// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/api-request";

/** List Audit Events GET /api/v1/audit-events */
export async function listAuditEventsApiV1AuditEventsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAuditEventsApiV1AuditEventsGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponsePaginatedAuditEvents_>("/api/v1/audit-events", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** List Consents GET /api/v1/consents */
export async function listConsentsApiV1ConsentsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listConsentsApiV1ConsentsGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponsePaginatedConsents_>("/api/v1/consents", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Create Consent POST /api/v1/consents */
export async function createConsentApiV1ConsentsPost(
  body: API.ConsentCreateRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseConsentDetailResponse_>("/api/v1/consents", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Get Consent GET /api/v1/consents/${param0} */
export async function getConsentApiV1ConsentsConsentIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getConsentApiV1ConsentsConsentIdGetParams,
  options?: RequestOptions
) {
  const { consent_id: param0, ...queryParams } = params;
  return request<API.ApiResponseConsentDetailResponse_>(
    `/api/v1/consents/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Revise Consent POST /api/v1/consents/${param0}/revisions */
export async function reviseConsentApiV1ConsentsConsentIdRevisionsPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reviseConsentApiV1ConsentsConsentIdRevisionsPostParams,
  body: API.ConsentRevisionRequest,
  options?: RequestOptions
) {
  const { consent_id: param0, ...queryParams } = params;
  return request<API.ApiResponseConsentDetailResponse_>(
    `/api/v1/consents/${param0}/revisions`,
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

/** Revoke Consent POST /api/v1/consents/${param0}/revoke */
export async function revokeConsentApiV1ConsentsConsentIdRevokePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.revokeConsentApiV1ConsentsConsentIdRevokePostParams,
  body: API.ConsentRevokeRequest,
  options?: RequestOptions
) {
  const { consent_id: param0, ...queryParams } = params;
  return request<API.ApiResponseConsentDetailResponse_>(
    `/api/v1/consents/${param0}/revoke`,
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
