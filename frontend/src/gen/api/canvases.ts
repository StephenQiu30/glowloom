// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 获取画布文档 GET /api/canvases/${param0} */
export async function getCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCanvasParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.domainDocument>(`/api/canvases/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 提交画布命令批次 POST /api/canvases/${param0}/commands */
export async function applyCanvasCommands(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.applyCanvasCommandsParams,
  body: API.applicationCommandsInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.applicationResult>(`/api/canvases/${param0}/commands`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 项目画布列表 GET /api/projects/${param0}/canvases */
export async function listCanvases(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCanvasesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.internalCanvasAdapterHttpListResponse>(
    `/api/projects/${param0}/canvases`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 创建备注画布 POST /api/projects/${param0}/canvases */
export async function createCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createCanvasParams,
  body: API.applicationCreateInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.domainDocument>(`/api/projects/${param0}/canvases`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
