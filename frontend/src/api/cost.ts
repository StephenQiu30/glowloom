import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function getCostBudgetApiV1ProjectsProjectIdCostBudgetGet(
  params: { project_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.CostBudgetResponse>>(
    `/api/v1/projects/${params.project_id}/cost-budget`,
    { method: "GET", ...(options ?? {}) },
  );
}

export function setCostBudgetApiV1ProjectsProjectIdCostBudgetPost(
  params: { project_id: string },
  body: API.CostBudgetSetRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.CostBudgetResponse>>(
    `/api/v1/projects/${params.project_id}/cost-budget`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}
