import request, { type RequestOptions } from "@/lib/request";

/** List Audit Events GET /api/v1/audit-events */
export async function listAuditEventsApiV1AuditEventsGet(
  params: API.listAuditEventsApiV1AuditEventsGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponsePaginatedAuditEvents_>(`/api/v1/audit-events`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** List Consents GET /api/v1/consents */
export async function listConsentsApiV1ConsentsGet(
  params: API.listConsentsApiV1ConsentsGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponsePaginatedConsents_>(`/api/v1/consents`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** Create Consent POST /api/v1/consents */
export async function createConsentApiV1ConsentsPost(
  body: API.ConsentCreateRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseConsentDetailResponse_>(`/api/v1/consents`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Consent GET /api/v1/consents/{consent_id} */
export async function getConsentApiV1ConsentsConsentIdGet(
  params: API.getConsentApiV1ConsentsConsentIdGetParams,
  options?: RequestOptions,
) {
  const { consent_id: path0 } = params;
  return request<API.ApiResponseConsentDetailResponse_>(`/api/v1/consents/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Revise Consent POST /api/v1/consents/{consent_id}/revisions */
export async function reviseConsentApiV1ConsentsConsentIdRevisionsPost(
  params: API.reviseConsentApiV1ConsentsConsentIdRevisionsPostParams,
  body: API.ConsentRevisionRequest,
  options?: RequestOptions,
) {
  const { consent_id: path0 } = params;
  return request<API.ApiResponseConsentDetailResponse_>(`/api/v1/consents/${path0}/revisions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Revoke Consent POST /api/v1/consents/{consent_id}/revoke */
export async function revokeConsentApiV1ConsentsConsentIdRevokePost(
  params: API.revokeConsentApiV1ConsentsConsentIdRevokePostParams,
  body: API.ConsentRevokeRequest,
  options?: RequestOptions,
) {
  const { consent_id: path0 } = params;
  return request<API.ApiResponseConsentDetailResponse_>(`/api/v1/consents/${path0}/revoke`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
