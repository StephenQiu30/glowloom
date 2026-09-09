import request, { type RequestOptions } from "@/lib/request";
import type { operations } from "./schema";

type Envelope<T> = { data: T };

type ReviewDecisionOperation = operations["decideProductionBibleReviewIssue"];
type CurrentStructureIdentityOperation = operations["getCurrentStructureIdentity"];

export function getCurrentStructureIdentity(
  params: CurrentStructureIdentityOperation["parameters"]["path"],
  options?: RequestOptions,
) {
  return request<CurrentStructureIdentityOperation["responses"][200]["content"]["application/json"]>(
    `/api/projects/${params.project_id}/structure-identity`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function decideProductionBibleReviewIssue(
  params: ReviewDecisionOperation["parameters"]["path"],
  body: ReviewDecisionOperation["requestBody"]["content"]["application/json"],
  options?: RequestOptions,
) {
  return request<ReviewDecisionOperation["responses"][200]["content"]["application/json"]>(
    `/api/production-bibles/${params.bible_id}/review-decisions`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function createBibleApiDocumentRevisionsRevisionIdProductionBiblesPost(
  params: { revision_id: string },
  body: API.ProductionBibleCreateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/document-revisions/${params.revision_id}/production-bibles`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getBibleApiProductionBiblesBibleIdGet(
  params: { bible_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/production-bibles/${params.bible_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function resumeBibleApiProductionBiblesBibleIdResumePost(
  params: { bible_id: string },
  body: API.ProductionBibleResumeRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/production-bibles/${params.bible_id}/resume`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getCurrentBibleApiProjectsProjectIdProductionBibleGet(
  params: { project_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.ProductionBibleResponse>>(
    `/api/projects/${params.project_id}/production-bible`,
    { method: "GET", ...(options ?? {}) },
  );
}
