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

/** 项目详情 GET /api/projects/${param0} */
export async function getProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectDetailResponse>(
    `/api/projects/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 项目移入回收站 DELETE /api/projects/${param0} */
export async function deleteProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteProjectParams,
  body: API.internalWorkspaceAdapterHttpProjectTransitionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectDetailResponse>(
    `/api/projects/${param0}`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 修改项目名称与设置 PATCH /api/projects/${param0} */
export async function updateProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateProjectParams,
  body: API.internalWorkspaceAdapterHttpProjectUpdateRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectDetailResponse>(
    `/api/projects/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 归档项目 POST /api/projects/${param0}/archive */
export async function archiveProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveProjectParams,
  body: API.internalWorkspaceAdapterHttpProjectTransitionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectDetailResponse>(
    `/api/projects/${param0}/archive`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 恢复回收项目 POST /api/projects/${param0}/restore */
export async function restoreProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreProjectParams,
  body: API.internalWorkspaceAdapterHttpProjectTransitionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectDetailResponse>(
    `/api/projects/${param0}/restore`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 取消项目归档 POST /api/projects/${param0}/unarchive */
export async function unarchiveProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.unarchiveProjectParams,
  body: API.internalWorkspaceAdapterHttpProjectTransitionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectDetailResponse>(
    `/api/projects/${param0}/unarchive`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
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
