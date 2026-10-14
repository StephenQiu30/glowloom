// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 项目目录列表 GET /api/project-folders */
export async function listProjectFolders(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectFoldersParams,
  options?: RequestOptions
) {
  return request<API.ProjectFolderListResponse>("/api/project-folders", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 新建项目目录 POST /api/project-folders */
export async function createProjectFolder(
  body: API.ProjectFolderCreateRequest,
  options?: RequestOptions
) {
  return request<API.ProjectFolderChangeResponse>("/api/project-folders", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 回收目录及其中全部项目 DELETE /api/project-folders/${param0} */
export async function recycleProjectFolder(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.recycleProjectFolderParams,
  body: API.ProjectTransitionRequest,
  options?: RequestOptions
) {
  const { folder_id: param0, ...queryParams } = params;
  return request<API.ProjectFolderChangeResponse>(
    `/api/project-folders/${param0}`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** 修改项目目录 PATCH /api/project-folders/${param0} */
export async function updateProjectFolder(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateProjectFolderParams,
  body: API.ProjectFolderUpdateRequest,
  options?: RequestOptions
) {
  const { folder_id: param0, ...queryParams } = params;
  return request<API.ProjectFolderChangeResponse>(
    `/api/project-folders/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** 有权项目列表 GET /api/projects */
export async function listProjects(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectsParams,
  options?: RequestOptions
) {
  return request<API.WorkspaceListResponse>("/api/projects", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 创建项目 POST /api/projects */
export async function createProject(
  body: API.CreateProjectRequest,
  options?: RequestOptions
) {
  return request<API.CreatedProject>("/api/projects", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 项目详情 GET /api/projects/${param0} */
export async function getProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectParams,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectDetailResponse>(`/api/projects/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 项目移入回收站 DELETE /api/projects/${param0} */
export async function deleteProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteProjectParams,
  body: API.ProjectTransitionRequest,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectDetailResponse>(`/api/projects/${param0}`, {
    method: "DELETE",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 修改项目名称与设置 PATCH /api/projects/${param0} */
export async function updateProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateProjectParams,
  body: API.ProjectUpdateRequest,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectDetailResponse>(`/api/projects/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 归档项目 POST /api/projects/${param0}/archive */
export async function archiveProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveProjectParams,
  body: API.ProjectTransitionRequest,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectDetailResponse>(`/api/projects/${param0}/archive`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 移动项目目录位置 PUT /api/projects/${param0}/folder */
export async function moveProjectToFolder(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.moveProjectToFolderParams,
  body: API.ProjectFolderMoveRequest,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectFolderChangeResponse>(
    `/api/projects/${param0}/folder`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** 恢复回收项目 POST /api/projects/${param0}/restore */
export async function restoreProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreProjectParams,
  body: API.ProjectTransitionRequest,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectDetailResponse>(`/api/projects/${param0}/restore`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 取消项目归档 POST /api/projects/${param0}/unarchive */
export async function unarchiveProject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.unarchiveProjectParams,
  body: API.ProjectTransitionRequest,
  options?: RequestOptions
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectDetailResponse>(
    `/api/projects/${param0}/unarchive`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** 查询可用于创建项目的风格预设 GET /api/style-presets */
export async function listStylePresets(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listStylePresetsParams,
  options?: RequestOptions
) {
  return request<API.StylePresetListResponse>("/api/style-presets", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
