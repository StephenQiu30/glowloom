import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function getCostBudgetApiProjectsProjectIdCostBudgetGet(
  params: { project_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.CostBudgetResponse>>(
    `/api/projects/${params.project_id}/cost-budget`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function setCostBudgetApiProjectsProjectIdCostBudgetPost(
  params: { project_id: string },
  body: API.CostBudgetSetRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.CostBudgetResponse>>(
    `/api/projects/${params.project_id}/cost-budget`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getCurrentCostPriceQuoteApiProjectsProjectIdMediaModelProfilesProfileVersionIdCostPriceGet(
  params: { project_id: string; profile_version_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.CostPriceQuoteResponse>>(
    `/api/projects/${params.project_id}/media-model-profiles/${params.profile_version_id}/cost-price`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function setCostPriceQuoteApiProjectsProjectIdMediaModelProfilesProfileVersionIdCostPricePost(
  params: { project_id: string; profile_version_id: string },
  body: API.CostPriceQuoteSetRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.CostPriceQuoteResponse>>(
    `/api/projects/${params.project_id}/media-model-profiles/${params.profile_version_id}/cost-price`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
