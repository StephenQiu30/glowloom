// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 当前项目约定 GET /api/projects/${param0}/memory */
export async function listProjectMemory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectMemoryParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.ProjectMemoryView>(`/api/projects/${param0}/memory`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 项目约定的修订记录 GET /api/projects/${param0}/memory/${param1} */
export async function listProjectMemoryHistory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectMemoryHistoryParams,
  options?: RequestOptions,
) {
  const { pid: param0, key: param1, ...queryParams } = params;
  return request<API.ProjectMemory[]>(
    `/api/projects/${param0}/memory/${param1}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 确认发布、更正或撤回项目约定 PUT /api/projects/${param0}/memory/${param1} */
export async function publishProjectMemory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishProjectMemoryParams,
  body: API.ProjectMemoryChange,
  options?: RequestOptions,
) {
  const { pid: param0, key: param1, ...queryParams } = params;
  return request<API.ProjectMemory>(
    `/api/projects/${param0}/memory/${param1}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}
