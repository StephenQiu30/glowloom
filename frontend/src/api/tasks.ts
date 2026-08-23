// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** List Tasks GET /api/v1/tasks */
export async function listTasksApiV1TasksGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listTasksApiV1TasksGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponsePaginatedTasks_>("/api/v1/tasks", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Get Task GET /api/v1/tasks/${param0} */
export async function getTaskApiV1TasksTaskIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getTaskApiV1TasksTaskIdGetParams,
  options?: RequestOptions
) {
  const { task_id: param0, ...queryParams } = params;
  return request<API.ApiResponseTaskResponse_>(`/api/v1/tasks/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Cancel Generation Task POST /api/v1/tasks/${param0}/cancel */
export async function cancelGenerationTaskApiV1TasksTaskIdCancelPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelGenerationTaskApiV1TasksTaskIdCancelPostParams,
  body: API.GenerationTaskCancellationRequest,
  options?: RequestOptions
) {
  const { task_id: param0, ...queryParams } = params;
  return request<API.ApiResponseGenerationTaskCancellationResponse_>(
    `/api/v1/tasks/${param0}/cancel`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
