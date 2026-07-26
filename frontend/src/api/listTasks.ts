// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List Tasks GET /v1/tasks */
export async function listTasks(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listTasksParams,
  options?: RequestOptions
) {
  return request<API.TaskListResponse>("/v1/tasks", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
