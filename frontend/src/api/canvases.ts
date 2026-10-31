// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 直接新建画布或短剧 POST /api/canvas-assets */
export async function createAssetCanvas(
  body: API.AssetCreationInput,
  options?: RequestOptions,
) {
  return request<API.CreatedAssetCanvas>("/api/canvas-assets", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 画布资产收藏与生命周期操作 POST /api/canvas-assets/commands */
export async function applyCanvasAssetCommand(
  body: API.AssetCommandInput,
  options?: RequestOptions,
) {
  return request<API.AssetCommandReceipt>("/api/canvas-assets/commands", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 画布与短剧系统模板 GET /api/canvas-assets/templates */
export async function listCanvasAssetTemplates(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCanvasAssetTemplatesParams,
  options?: RequestOptions,
) {
  return request<API.AssetTemplateCatalog>("/api/canvas-assets/templates", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 定位有权访问的画布空间 GET /api/canvas-locations/${param0} */
export async function locateCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.locateCanvasParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Location>(`/api/canvas-locations/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 资产画布列表 GET /api/canvases */
export async function listAssetCanvases(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAssetCanvasesParams,
  options?: RequestOptions,
) {
  return request<API.AssetListResponse>("/api/canvases", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 获取画布文档 GET /api/canvases/${param0} */
export async function getCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCanvasParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.DocumentDTO>(`/api/canvases/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 删除画布 DELETE /api/canvases/${param0} */
export async function deleteCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteCanvasParams,
  body: API.DeleteDTO,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.DeleteResultDTO>(`/api/canvases/${param0}`, {
    method: "DELETE",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 修改画布名称 PATCH /api/canvases/${param0} */
export async function renameCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.renameCanvasParams,
  body: API.RenameDTO,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.DocumentDTO>(`/api/canvases/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 读取导演台私有恢复稿 GET /api/canvases/${param0}/director-recovery/${param1} */
export async function readDirectorRecovery(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.readDirectorRecoveryParams,
  options?: RequestOptions,
) {
  const { id: param0, nid: param1, ...queryParams } = params;
  return request<API.DirectorRecoveryResult>(
    `/api/canvases/${param0}/director-recovery/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 保存导演台私有恢复稿 PUT /api/canvases/${param0}/director-recovery/${param1} */
export async function saveDirectorRecovery(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveDirectorRecoveryParams,
  body: API.SaveDirectorRecoveryInput,
  options?: RequestOptions,
) {
  const { id: param0, nid: param1, ...queryParams } = params;
  return request<API.DirectorRecovery>(
    `/api/canvases/${param0}/director-recovery/${param1}`,
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

/** 删除指定版本导演台恢复稿 DELETE /api/canvases/${param0}/director-recovery/${param1} */
export async function deleteDirectorRecovery(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteDirectorRecoveryParams,
  body: API.DeleteDirectorRecoveryInput,
  options?: RequestOptions,
) {
  const { id: param0, nid: param1, ...queryParams } = params;
  return request<any>(`/api/canvases/${param0}/director-recovery/${param1}`, {
    method: "DELETE",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 画布项目信息与费用明细 GET /api/canvases/${param0}/information */
export async function getCanvasInformation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCanvasInformationParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.CanvasInformation>(`/api/canvases/${param0}/information`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 项目画布列表 GET /api/projects/${param0}/canvases */
export async function listCanvases(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCanvasesParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.CanvasListResponse>(`/api/projects/${param0}/canvases`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 创建画布 POST /api/projects/${param0}/canvases */
export async function createCanvas(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createCanvasParams,
  body: API.CanvasCreateInput,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.DocumentDTO>(`/api/projects/${param0}/canvases`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
