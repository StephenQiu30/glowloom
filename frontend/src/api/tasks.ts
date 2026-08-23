import request, { type RequestOptions } from "@/lib/request";

/** List Tasks GET /api/v1/tasks */
export async function listTasksApiV1TasksGet(
  params: API.listTasksApiV1TasksGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponsePaginatedTasks_>(`/api/v1/tasks`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** Get Task GET /api/v1/tasks/{task_id} */
export async function getTaskApiV1TasksTaskIdGet(
  params: API.getTaskApiV1TasksTaskIdGetParams,
  options?: RequestOptions,
) {
  const { task_id: path0 } = params;
  return request<API.ApiResponseTaskResponse_>(`/api/v1/tasks/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Cancel Generation Task POST /api/v1/tasks/{task_id}/cancel */
export async function cancelGenerationTaskApiV1TasksTaskIdCancelPost(
  params: API.cancelGenerationTaskApiV1TasksTaskIdCancelPostParams,
  body: API.GenerationTaskCancellationRequest,
  options?: RequestOptions,
) {
  const { task_id: path0 } = params;
  return request<API.ApiResponseGenerationTaskCancellationResponse_>(`/api/v1/tasks/${path0}/cancel`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}
