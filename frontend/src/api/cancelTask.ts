// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Cancel Task POST /v1/tasks/${param0}:cancel */
export async function cancelTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelTaskParams,
  options?: RequestOptions
) {
  const { task_id: param0, ...queryParams } = params;
  return request<API.TaskResponse>(`/v1/tasks/${param0}:cancel`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
