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

/** 创建项目 POST /api/projects */
export async function createProject(
  body: API.internalWorkspaceAdapterHttpCreateProjectRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.githubComStephenQiu30LanverseBackendInternalWorkspaceApplicationCreatedProject>(
    "/api/projects",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 查询可用于创建项目的风格预设 GET /api/style-presets */
export async function listStylePresets(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listStylePresetsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.internalWorkspaceAdapterHttpStylePresetListResponse>(
    "/api/style-presets",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}
