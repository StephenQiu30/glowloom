import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function createBibleApiV1DocumentRevisionsRevisionIdProductionBiblesPost(
  params: { revision_id: string },
  body: API.ProductionBibleCreateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/v1/document-revisions/${params.revision_id}/production-bibles`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getBibleApiV1ProductionBiblesBibleIdGet(
  params: { bible_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/v1/production-bibles/${params.bible_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function resumeBibleApiV1ProductionBiblesBibleIdResumePost(
  params: { bible_id: string },
  body: API.ProductionBibleResumeRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/v1/production-bibles/${params.bible_id}/resume`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getCurrentBibleApiV1ProjectsProjectIdProductionBibleGet(
  params: { project_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/v1/projects/${params.project_id}/production-bible`,
    { method: "GET", ...(options ?? {}) },
  );
}
