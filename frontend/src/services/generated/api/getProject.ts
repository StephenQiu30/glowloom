// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get Project GET /v1/projects/${param0} */
export async function getProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ProjectDetailResponse>(`/v1/projects/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
