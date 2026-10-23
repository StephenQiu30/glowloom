/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 取消上传 POST /files/cancel */
export async function cancelFileTransfer(
  body: CanvasContract.FileTransferCommand,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileTransferVO>("/files/cancel", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 开始文件发布 POST /files/complete */
export async function completeFileTransfer(
  body: CanvasContract.FileTransferCommand,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileTransferVO>("/files/complete", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 初始化上传 POST /files/init */
export async function initFileTransfer(
  body: CanvasContract.FileTransferInitRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileTransferVO>("/files/init", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 列出本人文件 GET /files/list */
export async function listFileTransfers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.listFileTransfersParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseListFileTransferVO>("/files/list", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 授权下载 POST /files/read */
export async function readFileTransfer(
  body: CanvasContract.FileTransferCommand,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileSignedUrlVO>("/files/read", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 解析文件上传用途 POST /files/resolve */
export async function resolveFileUpload(
  body: CanvasContract.FileUploadChoiceRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileUploadChoiceVO>(
    "/files/resolve",
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

/** 查询上传进度 GET /files/session */
export async function getFileTransfer(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getFileTransferParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileTransferVO>("/files/session", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 授权上传分片 POST /files/sign */
export async function signFilePart(
  body: CanvasContract.FileTransferCommand,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileSignedUrlVO>("/files/sign", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
