// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Retry Task POST /v1/tasks/${param0}:retry */
export async function retryTask(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryTaskParams,
  options?: RequestOptions
) {
  const { task_id: param0, ...queryParams } = params;
  return request<any>(`/v1/tasks/${param0}:retry`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
