/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 将独立画布归入个人项目 POST /project/canvas/attach */
export async function attachProjectCanvas(
  body: CanvasContract.ProjectCanvasAttachRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectCanvasOperationStatusVO>(
    "/project/canvas/attach",
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

/** 在个人项目中新建画布 POST /project/canvas/create */
export async function createProjectCanvas(
  body: CanvasContract.ProjectCanvasCreateRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectCanvasOperationStatusVO>(
    "/project/canvas/create",
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

/** 查询原项目画布操作 GET /project/canvas/operation/get */
export async function getProjectCanvasOperation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getProjectCanvasOperationParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectCanvasOperationStatusVO>(
    "/project/canvas/operation/get",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 创建空项目或子项目 POST /project/create */
export async function createPersonalProject(
  body: CanvasContract.ProjectCreateRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectCreationStatusVO>(
    "/project/create",
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

/** 查询原项目创建命令 GET /project/creation/get */
export async function getPersonalProjectCreation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getPersonalProjectCreationParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectCreationStatusVO>(
    "/project/creation/get",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 查询原项目元数据命令 GET /project/workspace/command/get */
export async function getPersonalProjectCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getPersonalProjectCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectWorkspaceVO>(
    "/project/workspace/command/get",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 删除无内容及未完成操作的项目 POST /project/workspace/delete */
export async function deletePersonalProject(
  body: CanvasContract.WorkspaceDeleteRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectWorkspaceVO>(
    "/project/workspace/delete",
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

/** 修改个人项目名称 POST /project/workspace/edit */
export async function editPersonalProject(
  body: CanvasContract.WorkspaceEditRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectWorkspaceVO>(
    "/project/workspace/edit",
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

/** 读取个人项目元数据 GET /project/workspace/get */
export async function getPersonalProjectWorkspace(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getPersonalProjectWorkspaceParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectWorkspaceVO>(
    "/project/workspace/get",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 分页读取本人项目 POST /project/workspace/list/page */
export async function listPersonalProjects(
  body: CanvasContract.ProjectQueryRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseProjectWorkspacePageVO>(
    "/project/workspace/list/page",
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
