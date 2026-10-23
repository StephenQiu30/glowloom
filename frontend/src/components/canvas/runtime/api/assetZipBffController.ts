/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 按原名生成单项或批量ZIP POST /assets/export */
export async function exportAssetZip(
  body: CanvasContract.AssetZipExportRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetZipExportVO>(
    "/assets/export",
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

/** 取消ZIP打包 POST /assets/export/cancel */
export async function cancelAssetZipExport(
  body: CanvasContract.AssetZipJobCommand,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetZipExportVO>(
    "/assets/export/cancel",
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

/** 恢复本人最近ZIP下载任务 GET /assets/export/list */
export async function listAssetZipExports(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseListAssetZipExportVO>(
    "/assets/export/list",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 查询ZIP打包并重新授权续传 GET /assets/export/status */
export async function getAssetZipExport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetZipExportParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetZipExportVO>(
    "/assets/export/status",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 取消未完成的ZIP条目 POST /assets/import/cancel */
export async function cancelAssetZipImport(
  body: CanvasContract.AssetZipJobCommand,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetZipImportVO>(
    "/assets/import/cancel",
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

/** 读取本人ZIP导入任务 GET /assets/import/list */
export async function listAssetZipImports(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseListAssetZipImportVO>(
    "/assets/import/list",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 预检ZIP并保存原名计划 POST /assets/import/prepare */
export async function prepareAssetZipImport(
  body: CanvasContract.AssetZipImportRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetZipImportVO>(
    "/assets/import/prepare",
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

/** 开始或继续原ZIP导入任务 POST /assets/import/start */
export async function startAssetZipImport(
  body: CanvasContract.AssetZipJobCommand,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetZipImportVO>(
    "/assets/import/start",
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

/** 按任务或原命令恢复ZIP导入 GET /assets/import/status */
export async function getAssetZipImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getAssetZipImportParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetZipImportVO>(
    "/assets/import/status",
    {
      method: "GET",
      params: {
        // current has a default value: 1
        current: "1",
        ...params,
      },
      ...(options || {}),
    },
  );
}
