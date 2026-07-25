// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Task GET /v1/tasks/${param0} */
export async function getTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getTaskParams,
  options?: RequestOptions
) {
  const { task_id: param0, ...queryParams } = params;
  return request<API.TaskResponse>(`/v1/tasks/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
