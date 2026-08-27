import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function getWorkflowRunApiV1WorkflowRunsWorkflowRunIdGet(
  params: { workflow_run_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkflowRunViewResponse>>(
    `/api/v1/workflow-runs/${params.workflow_run_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}
