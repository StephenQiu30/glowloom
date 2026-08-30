import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function getWorkflowRunApiWorkflowRunsWorkflowRunIdGet(
  params: { workflow_run_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkflowRunViewResponse>>(
    `/api/workflow-runs/${params.workflow_run_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}
