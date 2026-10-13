// @ts-ignore
/* eslint-disable */
import { request } from "../../auth/request";

/** 获取画布文档 GET /api/canvases/${param0} */
export async function getCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: ProjectAPI.getCanvasParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<ProjectAPI.Document>(`/api/canvases/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 提交画布命令批次 POST /api/canvases/${param0}/commands */
export async function applyCanvasCommands(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: ProjectAPI.applyCanvasCommandsParams,
  body: ProjectAPI.CommandsInput,
  options?: import("../../auth/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<ProjectAPI.Result>(`/api/canvases/${param0}/commands`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 有权项目列表 GET /api/projects */
export async function listProjects(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: ProjectAPI.listProjectsParams,
  options?: import("../../auth/request").RequestOptions,
) {
  return request<ProjectAPI.ListResponse>("/api/projects", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 创建项目 POST /api/projects */
export async function createProject(
  body: ProjectAPI.CreateProjectRequest,
  options?: import("../../auth/request").RequestOptions,
) {
  return request<ProjectAPI.CreatedProject>("/api/projects", {
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
  params: ProjectAPI.getProjectParams,
  options?: import("../../auth/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<ProjectAPI.ProjectDetailResponse>(`/api/projects/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
