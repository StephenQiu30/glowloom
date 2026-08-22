// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/workspaces/${param0}/projects */
export async function createProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createProjectParams,
  body: API.CreateProjectRequest,
  options?: RequestOptions
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<API.ProjectResponse>(`/api/workspaces/${param0}/projects`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
