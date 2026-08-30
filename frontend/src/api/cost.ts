import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function getCostBudgetApiV1ProjectsProjectIdCostBudgetGet(
  params: { project_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.CostBudgetResponse>>(
    `/api/projects/${params.project_id}/cost-budget`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function setCostBudgetApiV1ProjectsProjectIdCostBudgetPost(
  params: { project_id: string },
  body: API.CostBudgetSetRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.CostBudgetResponse>>(
    `/api/projects/${params.project_id}/cost-budget`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function getCurrentCostPriceQuoteApiV1ProjectsProjectIdCostPricesMetricGet(
  params: { project_id: string; metric: "generation.image" },
  options?: RequestOptions,
) {
  return request<Envelope<API.CostPriceQuoteResponse>>(
    `/api/projects/${params.project_id}/cost-prices/${params.metric}`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function setCostPriceQuoteApiV1ProjectsProjectIdCostPricesMetricPost(
  params: { project_id: string; metric: "generation.image" },
  body: API.CostPriceQuoteSetRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.CostPriceQuoteResponse>>(
    `/api/projects/${params.project_id}/cost-prices/${params.metric}`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
