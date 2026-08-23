// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 获取项目分析 GET /api/projects/${param0}/analysis */
export async function projectAnalysisGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.projectAnalysisGetParams,
  options?: RequestOptions
) {
  const { projectID: param0, ...queryParams } = params;
  return request<API.AnalysisEnvelope>(`/api/projects/${param0}/analysis`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 查询项目列表 GET /api/workspaces/${param0}/projects */
export async function projectList(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.projectListParams,
  options?: RequestOptions
) {
  const { workspaceID: param0, ...queryParams } = params;
  return request<API.ProjectPageEnvelope>(
    `/api/workspaces/${param0}/projects`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** 创建项目 POST /api/workspaces/${param0}/projects */
export async function projectCreate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.projectCreateParams,
  body: API.createProjectRequest,
  options?: RequestOptions
) {
  const { workspaceID: param0, ...queryParams } = params;
  return request<API.ProjectEnvelope>(`/api/workspaces/${param0}/projects`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
