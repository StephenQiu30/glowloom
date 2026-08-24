import request, { type RequestOptions } from "@/lib/request";

/** Create Bible POST /api/v1/document-revisions/{revision_id}/production-bibles */
export async function createBibleApiV1DocumentRevisionsRevisionIdProductionBiblesPost(
  params: API.createBibleApiV1DocumentRevisionsRevisionIdProductionBiblesPostParams,
  body: API.ProductionBibleCreateRequest,
  options?: RequestOptions,
) {
  const { revision_id: path0 } = params;
  return request<API.ApiResponseProductionBibleResponse_>(`/api/v1/document-revisions/${path0}/production-bibles`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Bible GET /api/v1/production-bibles/{bible_id} */
export async function getBibleApiV1ProductionBiblesBibleIdGet(
  params: API.getBibleApiV1ProductionBiblesBibleIdGetParams,
  options?: RequestOptions,
) {
  const { bible_id: path0 } = params;
  return request<API.ApiResponseProductionBibleResponse_>(`/api/v1/production-bibles/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Confirm Bible POST /api/v1/production-bibles/{bible_id}/confirm */
export async function confirmBibleApiV1ProductionBiblesBibleIdConfirmPost(
  params: API.confirmBibleApiV1ProductionBiblesBibleIdConfirmPostParams,
  body: API.ProductionBibleConfirmRequest,
  options?: RequestOptions,
) {
  const { bible_id: path0 } = params;
  return request<API.ApiResponseProductionBibleResponse_>(`/api/v1/production-bibles/${path0}/confirm`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Resume Bible POST /api/v1/production-bibles/{bible_id}/resume */
export async function resumeBibleApiV1ProductionBiblesBibleIdResumePost(
  params: API.resumeBibleApiV1ProductionBiblesBibleIdResumePostParams,
  body: API.ProductionBibleResumeRequest,
  options?: RequestOptions,
) {
  const { bible_id: path0 } = params;
  return request<API.ApiResponseProductionBibleResponse_>(`/api/v1/production-bibles/${path0}/resume`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Resolve Bible Review Issue POST /api/v1/production-bibles/{bible_id}/review-issue-resolutions */
export async function resolveBibleReviewIssueApiV1ProductionBiblesBibleIdReviewIssueResolutionsPost(
  params: API.resolveBibleReviewIssueApiV1ProductionBiblesBibleIdReviewIssueResolutionsPostParams,
  body: API.ProductionBibleReviewIssueResolutionRequest,
  options?: RequestOptions,
) {
  const { bible_id: path0 } = params;
  return request<API.ApiResponseProductionBibleResponse_>(`/api/v1/production-bibles/${path0}/review-issue-resolutions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Current Bible GET /api/v1/projects/{project_id}/production-bible */
export async function getCurrentBibleApiV1ProjectsProjectIdProductionBibleGet(
  params: API.getCurrentBibleApiV1ProjectsProjectIdProductionBibleGetParams,
  options?: RequestOptions,
) {
  const { project_id: path0 } = params;
  return request<API.ApiResponseProductionBibleResponse_>(`/api/v1/projects/${path0}/production-bible`, {
    method: "GET",
    ...(options ?? {}),
  });
}
