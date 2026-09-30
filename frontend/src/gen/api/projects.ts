// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 有权项目列表 GET /api/projects */
export async function listProjects(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalWorkspaceAdapterHttpListResponse>(
    "/api/projects",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}
