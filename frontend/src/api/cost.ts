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

export function getCurrentCostPriceQuoteApiProjectsProjectIdCostPricesMetricGet(
  params: { project_id: string; metric: "generation.image" },
  options?: RequestOptions,
) {
  return request<Envelope<API.CostPriceQuoteResponse>>(
    `/api/projects/${params.project_id}/cost-prices/${params.metric}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function setCostPriceQuoteApiProjectsProjectIdCostPricesMetricPost(
  params: { project_id: string; metric: "generation.image" },
  body: API.CostPriceQuoteSetRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.CostPriceQuoteResponse>>(
    `/api/projects/${params.project_id}/cost-prices/${params.metric}`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
