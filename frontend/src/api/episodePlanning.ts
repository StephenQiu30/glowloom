import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function createEpisodePlanApiDocumentRevisionsRevisionIdEpisodePlansPost(
  params: { revision_id: string },
  body: API.EpisodePlanCreateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.EpisodePlanDetailResponse>>(
    `/api/document-revisions/${params.revision_id}/episode-plans`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getEpisodePlanApiEpisodePlansPlanIdGet(
  params: { plan_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.EpisodePlanDetailResponse>>(
    `/api/episode-plans/${params.plan_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function confirmEpisodePlanApiEpisodePlansPlanIdConfirmPost(
  params: { plan_id: string },
  body: API.ConfirmEpisodePlanRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.EpisodePlanDetailResponse>>(
    `/api/episode-plans/${params.plan_id}/confirm`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function materializeEpisodePlanApiEpisodePlansPlanIdMaterializationsPost(
  params: { plan_id: string },
  body: API.MaterializeEpisodePlanRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ImportCommitDetailResponse>>(
    `/api/episode-plans/${params.plan_id}/materializations`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function publishImportCommitApiImportCommitsCommitIdPublishPost(
  params: { commit_id: string },
  body: API.PublishImportCommitRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ImportCommitDetailResponse>>(
    `/api/import-commits/${params.commit_id}/publish`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
