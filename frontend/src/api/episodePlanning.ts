import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPost(
  params: { revision_id: string },
  body: API.EpisodePlanCreateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.EpisodePlanDetailResponse>>(
    `/api/v1/document-revisions/${params.revision_id}/episode-plans`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getEpisodePlanApiV1EpisodePlansPlanIdGet(
  params: { plan_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.EpisodePlanDetailResponse>>(
    `/api/v1/episode-plans/${params.plan_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPost(
  params: { plan_id: string },
  body: API.ConfirmEpisodePlanRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.EpisodePlanDetailResponse>>(
    `/api/v1/episode-plans/${params.plan_id}/confirm`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPost(
  params: { plan_id: string },
  body: API.MaterializeEpisodePlanRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ImportCommitDetailResponse>>(
    `/api/v1/episode-plans/${params.plan_id}/materializations`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function publishImportCommitApiV1ImportCommitsCommitIdPublishPost(
  params: { commit_id: string },
  body: API.PublishImportCommitRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ImportCommitDetailResponse>>(
    `/api/v1/import-commits/${params.commit_id}/publish`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
