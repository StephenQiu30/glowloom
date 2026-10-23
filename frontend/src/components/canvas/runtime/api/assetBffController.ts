/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 恢复素材命令结果 GET /assets/command */
export async function getAssetCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetVO>("/assets/command", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 移除本人素材 POST /assets/delete */
export async function deleteMyAsset(
  body: CanvasContract.AssetDeleteRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetVO>("/assets/delete", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 原子移除选中的本人素材，任一失败全批回滚 POST /assets/delete/batch */
export async function deleteMyAssets(
  body: CanvasContract.AssetBatchDeleteRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetBatchDeleteVO>(
    "/assets/delete/batch",
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

/** 恢复批量删除素材的原命令结果 GET /assets/delete/batch/command */
export async function getAssetBatchDeleteCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetBatchDeleteCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetBatchDeleteVO>(
    "/assets/delete/batch/command",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 读取本人素材 GET /assets/detail */
export async function getMyAsset(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getMyAssetParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetVO>("/assets/detail", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 下载本人素材的对应单文件 POST /assets/download */
export async function downloadMyAsset(
  body: CanvasContract.AssetDownloadRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetDownloadVO>(
    "/assets/download",
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

/** 恢复创建素材库命令 GET /assets/library/command */
export async function getAssetLibraryCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetLibraryCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetLibraryVO>(
    "/assets/library/command",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 创建项目素材库 POST /assets/library/create */
export async function createAssetLibrary(
  body: CanvasContract.AssetLibraryCreateRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetLibraryVO>(
    "/assets/library/create",
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

/** 读取项目素材库状态 GET /assets/library/detail */
export async function getAssetLibrary(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetLibraryParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetLibraryVO>(
    "/assets/library/detail",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 分页查询本人项目素材库及候选项目 GET /assets/library/list */
export async function listAssetLibraries(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.listAssetLibrariesParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetLibraryPageVO>(
    "/assets/library/list",
    {
      method: "GET",
      params: {
        // current has a default value: 1
        current: "1",
        // pageSize has a default value: 50
        pageSize: "50",

        // enabledOnly has a default value: true
        enabledOnly: "true",
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 分页查询本人素材 GET /assets/list */
export async function listMyAssets(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.listMyAssetsParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetPageVO>("/assets/list", {
    method: "GET",
    params: {
      // current has a default value: 1
      current: "1",
      // pageSize has a default value: 12
      pageSize: "12",

      // kind has a default value: all
      kind: "all",
      // libraryScope has a default value: ALL
      libraryScope: "ALL",

      ...params,
    },
    ...(options || {}),
  });
}

/** 原子移动所选素材 POST /assets/move */
export async function moveMyAssets(
  body: CanvasContract.AssetMoveRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetMoveVO>("/assets/move", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 恢复移动素材命令 GET /assets/move/command */
export async function getAssetMoveCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetMoveCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetMoveVO>(
    "/assets/move/command",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 保存本人素材 POST /assets/save */
export async function saveMyAsset(
  body: CanvasContract.AssetSaveRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetVO>("/assets/save", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取素材检索同步状态 GET /assets/search/status */
export async function getAssetSearchStatus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetSearchStatusParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseSearchVisibilityVO>(
    "/assets/search/status",
    {
      method: "GET",
      params: {
        // libraryScope has a default value: ALL
        libraryScope: "ALL",
        ...params,
      },
      ...(options || {}),
    },
  );
}
