// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 查询风格库 GET /api/style-library */
export async function listStyleLibrary(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listStyleLibraryParams,
  options?: RequestOptions,
) {
  return request<API.StyleLibraryListResponse>("/api/style-library", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 创建个人风格 POST /api/style-library */
export async function createStyleLibraryItem(
  body: API.StyleLibrarySaveRequest,
  options?: RequestOptions,
) {
  return request<API.StyleLibraryChangeResult>("/api/style-library", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取风格详情 GET /api/style-library/${param0} */
export async function getStyleLibraryItem(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getStyleLibraryItemParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.StyleLibraryDetail>(`/api/style-library/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 删除个人风格 DELETE /api/style-library/${param0} */
export async function deleteStyleLibraryItem(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteStyleLibraryItemParams,
  body: API.StyleLibraryDeleteRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.StyleLibraryChangeResult>(`/api/style-library/${param0}`, {
    method: "DELETE",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 编辑个人风格 PATCH /api/style-library/${param0} */
export async function updateStyleLibraryItem(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateStyleLibraryItemParams,
  body: API.StyleLibraryUpdateRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.StyleLibraryChangeResult>(`/api/style-library/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
