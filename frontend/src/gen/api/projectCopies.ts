// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 读取项目复制状态 GET /api/project-copies/${param0} */
export async function getProjectCopy(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectCopyParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectCopyResponse>(
    `/api/project-copies/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 请求取消项目复制 POST /api/project-copies/${param0}/cancel */
export async function cancelProjectCopy(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelProjectCopyParams,
  body: API.internalWorkspaceAdapterHttpProjectTransitionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/project-copies/${param0}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 核验原项目复制的未知结果 POST /api/project-copies/${param0}/reconcile */
export async function reconcileProjectCopy(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reconcileProjectCopyParams,
  body: API.internalWorkspaceAdapterHttpProjectTransitionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/project-copies/${param0}/reconcile`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 重试已知失败的项目复制 POST /api/project-copies/${param0}/retry */
export async function retryProjectCopy(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryProjectCopyParams,
  body: API.internalWorkspaceAdapterHttpProjectTransitionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/project-copies/${param0}/retry`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 分页恢复项目复制任务 GET /api/projects/${param0}/copies */
export async function listProjectCopies(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectCopiesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalWorkspaceAdapterHttpProjectCopyListResponse>(
    `/api/projects/${param0}/copies`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 创建完整项目内容副本 POST /api/projects/${param0}/copies */
export async function createProjectCopy(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createProjectCopyParams,
  body: API.internalWorkspaceAdapterHttpProjectCopyRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/copies`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
