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
  return request<API.githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument>(
    `/api/canvases/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 删除画布 DELETE /api/canvases/${param0} */
export async function deleteCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteCanvasParams,
  body: API.githubComStephenQiu30LanverseBackendInternalCanvasApplicationDeleteInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCanvasApplicationDeleteResult>(
    `/api/canvases/${param0}`,
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

/** 修改画布名称 PATCH /api/canvases/${param0} */
export async function renameCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.renameCanvasParams,
  body: API.githubComStephenQiu30LanverseBackendInternalCanvasApplicationRenameInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument>(
    `/api/canvases/${param0}`,
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

/** 提交画布命令批次 POST /api/canvases/${param0}/commands */
export async function applyCanvasCommands(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.applyCanvasCommandsParams,
  body: API.githubComStephenQiu30LanverseBackendInternalCanvasApplicationCommandsInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCanvasApplicationResult>(
    `/api/canvases/${param0}/commands`,
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

/** 创建画布 POST /api/projects/${param0}/canvases */
export async function createCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createCanvasParams,
  body: API.githubComStephenQiu30LanverseBackendInternalCanvasApplicationCreateInput,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalCanvasDomainDocument>(
    `/api/projects/${param0}/canvases`,
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
